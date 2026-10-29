package session

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"syscall"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	displayConfigName                  = "org.gnome.Mutter.DisplayConfig"
	displayConfigPath  dbus.ObjectPath = "/org/gnome/Mutter/DisplayConfig"
	serviceChannelName                 = "org.gnome.Mutter.ServiceChannel"
	serviceChannelPath dbus.ObjectPath = "/org/gnome/Mutter/ServiceChannel"
	introspectName                     = "org.gnome.Shell.Introspect"
	introspectPath     dbus.ObjectPath = "/org/gnome/Shell/Introspect"
	screenshotName                     = "org.gnome.Shell.Screenshot"
	screenshotPath     dbus.ObjectPath = "/org/gnome/Shell/Screenshot"
)

// PortalHooks cross the session/compositor boundary. Each operation owns its
// result and must return when ctx is cancelled. OpenWayland borrows fd for the
// duration of its call; the compositor duplicates it before native ownership.
type PortalHooks struct {
	Screenshot  func(ctx context.Context, includePointer bool) (string, error)
	PickColor   func(ctx context.Context) ([3]float64, error)
	OpenWayland func(ctx context.Context, fd int) error
}

type serviceReplyID struct {
	sender string
	serial uint32
}

func serviceReplyKey(message dbus.Message) serviceReplyID {
	sender, _ := message.Headers[dbus.FieldSender].Value().(string)
	return serviceReplyID{sender, message.Serial()}
}

type portalRequest struct {
	sender dbus.Sender
	cancel context.CancelFunc
}

// Portal owns the session-only discovery and screenshot interfaces. Construction
// is explicit: nested servers never call StartPortal and never acquire names.
type Portal struct {
	conn                            *dbus.Conn
	hooks                           PortalHooks
	ctx                             context.Context
	cancel                          context.CancelFunc
	close                           sync.Once
	wg                              sync.WaitGroup
	signals                         chan *dbus.Signal
	manualReplies                   *sync.Map
	mu                              sync.Mutex
	requests                        map[*portalRequest]struct{}
	names                           []string
	state                           displayState
	windows                         map[uint64]map[string]dbus.Variant
	screenCast                      *screenCastService
	nextCastSession, nextCastStream uint64
}

func StartPortal(hooks PortalHooks) (*Portal, error) { return connectPortal("", hooks) }

func connectPortal(address string, hooks PortalHooks) (*Portal, error) {
	// godbus leaves returned UnixFDs open and has no post-send hook. Suppress its
	// automatic success reply for this one method: send synchronously below and
	// close the local endpoint only after SCM_RIGHTS has been written.
	replies := &sync.Map{}
	option := dbus.WithIncomingInterceptor(func(message *dbus.Message) {
		// None of these interfaces accepts descriptors. godbus gives received
		// SCM_RIGHTS descriptors to the caller, even when argument decoding rejects
		// a message, and its numeric conversion can otherwise turn h into an int
		// or a filename. Close before dispatch and make acceptance impossible.
		if closeIncomingDescriptors(message.Body) {
			switch message.Type {
			case dbus.TypeMethodCall:
				message.Headers[dbus.FieldInterface] = dbus.MakeVariant("org.sysc.RejectedIncomingDescriptors")
			case dbus.TypeSignal:
				message.Type = 0 // drop the rejected signal before it reaches subscribers
			case dbus.TypeMethodReply, dbus.TypeError:
				message.Type = dbus.TypeError
				message.Headers[dbus.FieldErrorName] = dbus.MakeVariant("org.freedesktop.DBus.Error.InvalidArgs")
				message.Body = []any{"Portal services do not accept file descriptors"}
				message.Headers[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(message.Body...))
			}
			return
		}
		if message.Type != dbus.TypeMethodCall || message.Flags&dbus.FlagNoReplyExpected != 0 ||
			message.Headers[dbus.FieldPath].Value() != serviceChannelPath ||
			(message.Headers[dbus.FieldInterface].Value() != nil && message.Headers[dbus.FieldInterface].Value() != serviceChannelName) ||
			message.Headers[dbus.FieldMember].Value() != "OpenWaylandServiceConnection" || len(message.Body) != 1 {
			return
		}
		if _, ok := message.Body[0].(uint32); !ok {
			return
		}
		replies.Store(serviceReplyKey(*message), true)
		message.Flags |= dbus.FlagNoReplyExpected
	})
	var conn *dbus.Conn
	var err error
	if address == "" {
		conn, err = dbus.ConnectSessionBus(option)
	} else {
		conn, err = dbus.Connect(address, option)
	}
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(conn.Context())
	p := &Portal{conn: conn, hooks: hooks, ctx: ctx, cancel: cancel, signals: make(chan *dbus.Signal, 64), manualReplies: replies, requests: map[*portalRequest]struct{}{}, state: displayState{Serial: 1}, windows: map[uint64]map[string]dbus.Variant{}}
	fail := func(err error) (*Portal, error) { p.Close(); return nil, err }
	if err := conn.AddMatchSignal(dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged")); err != nil {
		return fail(err)
	}
	conn.Signal(p.signals)
	for _, item := range []struct {
		name    string
		path    dbus.ObjectPath
		object  any
		replace bool
		signals []introspect.Signal
	}{
		{displayConfigName, displayConfigPath, portalDisplayConfig{p}, true, []introspect.Signal{{Name: "MonitorsChanged"}}},
		{serviceChannelName, serviceChannelPath, portalServiceChannel{p}, false, nil},
		{introspectName, introspectPath, portalIntrospect{p}, true, []introspect.Signal{{Name: "WindowsChanged"}}},
		{screenshotName, screenshotPath, portalScreenshot{p}, true, nil},
	} {
		if err := conn.Export(item.object, item.path, item.name); err != nil {
			return fail(err)
		}
		iface := introspect.Interface{Name: item.name, Methods: introspect.Methods(item.object), Signals: item.signals}
		if item.name == displayConfigName {
			props := prop.Map{displayConfigName: {"PowerSaveMode": {Value: int32(-1), Writable: true, Emit: prop.EmitFalse, Callback: func(*prop.Change) *dbus.Error {
				return unsupported("Output power changes through DisplayConfig are unsupported")
			}}, "PanelOrientationManaged": {Value: false, Emit: prop.EmitFalse}, "ApplyMonitorsConfigAllowed": {Value: false, Emit: prop.EmitFalse}, "NightLightSupported": {Value: false, Emit: prop.EmitFalse}}}
			properties, err := prop.Export(conn, item.path, props)
			if err != nil {
				return fail(err)
			}
			iface.Properties = properties.Introspection(displayConfigName)
		}
		node := &introspect.Node{Name: string(item.path), Interfaces: []introspect.Interface{iface}}
		if item.name == displayConfigName {
			node.Interfaces = append(node.Interfaces, prop.IntrospectData)
		}
		if err := conn.Export(introspect.NewIntrospectable(node), item.path, "org.freedesktop.DBus.Introspectable"); err != nil {
			return fail(err)
		}
		flags := dbus.NameFlagDoNotQueue
		if item.replace {
			flags |= dbus.NameFlagAllowReplacement | dbus.NameFlagReplaceExisting
		}
		reply, err := conn.RequestName(item.name, flags)
		if err != nil {
			return fail(err)
		}
		if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
			return fail(fmt.Errorf("%s is owned by another service", item.name))
		}
		p.names = append(p.names, item.name)
	}
	go p.watch()
	return p, nil
}

func unsupported(message string) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.NotSupported", []any{message})
}

func (p *Portal) begin(sender dbus.Sender) (context.Context, func(), *dbus.Error) {
	p.mu.Lock()
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		return nil, nil, dbus.MakeFailedError(errors.New("session services stopped"))
	}
	ctx, cancel := context.WithCancel(p.ctx)
	request := &portalRequest{sender: sender, cancel: cancel}
	p.requests[request] = struct{}{}
	p.wg.Add(1)
	p.mu.Unlock()
	finish := func() { cancel(); p.mu.Lock(); delete(p.requests, request); p.mu.Unlock(); p.wg.Done() }
	// A dispatch goroutine may start after NameOwnerChanged already reported
	// departure. Register first, then ask the bus, so departure on either side
	// of this check cancels the request without retaining a departed-name ledger.
	var present bool
	err := p.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, string(sender)).Store(&present)
	if err != nil || !present {
		finish()
		return nil, nil, dbus.MakeFailedError(errors.New("request caller is no longer on the bus"))
	}
	return ctx, finish, nil
}

func (p *Portal) watch() {
	for {
		select {
		case <-p.ctx.Done():
			p.DisableScreenCast()
			return
		case signal, ok := <-p.signals:
			if !ok {
				p.cancel()
				p.DisableScreenCast()
				return
			}
			if signal == nil || signal.Sender != "org.freedesktop.DBus" || signal.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(signal.Body) != 3 {
				continue
			}
			name, _ := signal.Body[0].(string)
			old, _ := signal.Body[1].(string)
			next, _ := signal.Body[2].(string)
			if old == "" || next != "" {
				continue
			}
			p.mu.Lock()
			for request := range p.requests {
				if string(request.sender) == name {
					request.cancel()
				}
			}
			p.mu.Unlock()
			p.closeCallerCasts(name)
		}
	}
}

func (p *Portal) Close() {
	if p == nil {
		return
	}
	p.close.Do(func() {
		p.mu.Lock()
		p.cancel()
		p.mu.Unlock()
		p.DisableScreenCast()
		p.wg.Wait()
		for _, name := range p.names {
			_, _ = p.conn.ReleaseName(name)
		}
		p.conn.RemoveSignal(p.signals)
		_ = p.conn.Close()
		p.manualReplies.Clear()
	})
}

type portalIntrospect struct{ p *Portal }

func (s portalIntrospect) GetWindows() (map[uint64]map[string]dbus.Variant, *dbus.Error) {
	s.p.mu.Lock()
	defer s.p.mu.Unlock()
	return s.p.windows, nil
}

type portalScreenshot struct{ p *Portal }

func (s portalScreenshot) Screenshot(sender dbus.Sender, includePointer, flash bool, filename string) (bool, string, *dbus.Error) {
	ctx, finish, err := s.p.begin(sender)
	if err != nil {
		return false, "", err
	}
	defer finish()
	if s.p.hooks.Screenshot == nil {
		return false, "", unsupported("Screenshot is unavailable")
	}
	path, e := s.p.hooks.Screenshot(ctx, includePointer)
	if e == nil {
		e = ctx.Err()
	}
	if e == nil && path == "" {
		e = errors.New("screenshot was not written")
	}
	if e != nil {
		return false, "", dbus.MakeFailedError(e)
	}
	return true, path, nil
}
func (s portalScreenshot) PickColor(sender dbus.Sender) (map[string]dbus.Variant, *dbus.Error) {
	ctx, finish, err := s.p.begin(sender)
	if err != nil {
		return nil, err
	}
	defer finish()
	if s.p.hooks.PickColor == nil {
		return nil, unsupported("Color picking is unavailable")
	}
	rgb, e := s.p.hooks.PickColor(ctx)
	if e == nil {
		e = ctx.Err()
	}
	if e != nil {
		return nil, dbus.MakeFailedError(e)
	}
	return map[string]dbus.Variant{"color": dbus.MakeVariant(struct{ R, G, B float64 }{rgb[0], rgb[1], rgb[2]})}, nil
}

type portalServiceChannel struct{ p *Portal }

func (s portalServiceChannel) OpenWaylandServiceConnection(message dbus.Message, kind uint32) (dbus.UnixFD, *dbus.Error) {
	if _, manual := s.p.manualReplies.LoadAndDelete(serviceReplyKey(message)); !manual {
		// godbus may convert signed/oversized integers or unwrap variants before
		// invoking this method. Only the intercepted exact wire uint32 is valid;
		// never let its automatic reply transmit descriptor zero.
		return 0, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"Service client type must have signature u and expect a reply"})
	}
	reply := &dbus.Message{Type: dbus.TypeMethodReply, Headers: map[dbus.HeaderField]dbus.Variant{dbus.FieldDestination: message.Headers[dbus.FieldSender], dbus.FieldReplySerial: dbus.MakeVariant(message.Serial())}}
	result := func() ([]any, *dbus.Error) {
		if kind != 1 {
			return nil, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"Invalid service client type"})
		}
		sender, _ := message.Headers[dbus.FieldSender].Value().(string)
		ctx, finish, err := s.p.begin(dbus.Sender(sender))
		if err != nil {
			return nil, err
		}
		defer finish()
		if s.p.hooks.OpenWayland == nil {
			return nil, unsupported("Service connections are unavailable")
		}
		if !s.p.conn.SupportsUnixFDs() {
			return nil, unsupported("D-Bus transport cannot pass descriptors")
		}
		pair, e := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
		if e != nil {
			return nil, dbus.MakeFailedError(e)
		}
		defer syscall.Close(pair[1])
		if e = s.p.hooks.OpenWayland(ctx, pair[1]); e == nil {
			e = ctx.Err()
		}
		if e != nil {
			syscall.Close(pair[0])
			return nil, dbus.MakeFailedError(e)
		}
		return []any{dbus.UnixFD(pair[0])}, nil
	}
	body, err := result()
	if err != nil {
		reply.Type = dbus.TypeError
		reply.Headers[dbus.FieldErrorName] = dbus.MakeVariant(err.Name)
		reply.Body = err.Body
	} else {
		reply.Body = body
		defer syscall.Close(int(body[0].(dbus.UnixFD)))
	}
	reply.Headers[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(reply.Body...))
	s.p.conn.Send(reply, nil)
	return 0, nil
}

// closeIncomingDescriptors follows the containers produced by godbus's decoder.
// A UnixFD may occur repeatedly in one decoded message; close each received
// descriptor once, including descriptors in variants and dictionary keys.
func closeIncomingDescriptors(body []any) bool {
	descriptors := map[dbus.UnixFD]struct{}{}
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case dbus.UnixFD:
			descriptors[value] = struct{}{}
			return
		case dbus.Variant:
			visit(value.Value())
			return
		}
		item := reflect.ValueOf(value)
		switch item.Kind() {
		case reflect.Array, reflect.Slice:
			for i := 0; i < item.Len(); i++ {
				visit(item.Index(i).Interface())
			}
		case reflect.Map:
			entries := item.MapRange()
			for entries.Next() {
				visit(entries.Key().Interface())
				visit(entries.Value().Interface())
			}
		}
	}
	visit(body)
	for fd := range descriptors {
		_ = syscall.Close(int(fd))
	}
	return len(descriptors) != 0
}
