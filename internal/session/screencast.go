package session

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	screenCastName                        = "org.gnome.Mutter.ScreenCast"
	screenCastPath        dbus.ObjectPath = "/org/gnome/Mutter/ScreenCast"
	screenCastSessionName                 = screenCastName + ".Session"
	screenCastStreamName                  = screenCastName + ".Stream"
)

type CastRequest struct {
	SessionID, StreamID uint64
	Target              ipc.CastTarget
	CursorMode          uint32
}

// Start returns after the compositor accepts the request, and must observe ctx.
// The node and closed notifications may arrive later. Stop only schedules owner
// cleanup: it must not wait for the compositor, including during shutdown.
type ScreenCastHooks struct {
	Start func(ctx context.Context, request CastRequest, node func(uint32), closed func()) error
	Stop  func(sessionID uint64)
}

type screenCastService struct {
	p            *Portal
	hooks        ScreenCastHooks
	sessions     map[uint64]*castSession
	dynamicID    uint64
	stopLifetime func() bool
}

type castSession struct {
	service *screenCastService
	id      uint64
	path    dbus.ObjectPath
	owner   dbus.Sender
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
	streams []*castStream
}

type castStream struct {
	path           dbus.ObjectPath
	request        CastRequest
	started, added bool
}

func (p *Portal) EnableScreenCast(lifetime context.Context, hooks ScreenCastHooks, dynamicID uint64) error {
	if hooks.Start == nil || hooks.Stop == nil || dynamicID == 0 {
		return errors.New("invalid ScreenCast hooks")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		return p.ctx.Err()
	}
	if err := lifetime.Err(); err != nil {
		return err
	}
	if p.screenCast != nil {
		return errors.New("ScreenCast already enabled")
	}
	s := &screenCastService{p: p, hooks: hooks, sessions: make(map[uint64]*castSession), dynamicID: dynamicID}
	if err := p.exportCastObject(screenCastPath, screenCastName, s, prop.Map{screenCastName: {"Version": {Value: int32(4), Emit: prop.EmitFalse}}}, nil); err != nil {
		return err
	}
	reply, err := p.conn.RequestName(screenCastName, dbus.NameFlagAllowReplacement|dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err == nil && reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		err = errors.New("ScreenCast name is owned")
	}
	if err != nil {
		p.unexportCastObject(screenCastPath, screenCastName)
		return err
	}
	p.screenCast = s
	s.stopLifetime = context.AfterFunc(lifetime, func() { p.disableScreenCast(s) })
	p.windows = maps.Clone(p.windows)
	p.windows[dynamicID] = dynamicCastWindow()
	_ = p.conn.Emit(introspectPath, introspectName+".WindowsChanged")
	return nil
}

func dynamicCastWindow() map[string]dbus.Variant {
	return map[string]dbus.Variant{"title": dbus.MakeVariant("niri Dynamic Cast Target"), "app-id": dbus.MakeVariant("rs.bxt.niri.desktop")}
}

func (p *Portal) exportCastObject(path dbus.ObjectPath, name string, object any, properties prop.Map, signals []introspect.Signal) (err error) {
	defer func() {
		if err != nil {
			p.unexportCastObject(path, name)
		}
	}()
	if err = p.conn.Export(object, path, name); err != nil {
		return
	}
	iface := introspect.Interface{Name: name, Methods: introspect.Methods(object), Signals: signals}
	node := &introspect.Node{Name: string(path), Interfaces: []introspect.Interface{iface}}
	if properties != nil {
		var props *prop.Properties
		props, err = prop.Export(p.conn, path, properties)
		if err != nil {
			return
		}
		node.Interfaces[0].Properties = props.Introspection(name)
		node.Interfaces = append(node.Interfaces, prop.IntrospectData)
	}
	err = p.conn.Export(introspect.NewIntrospectable(node), path, "org.freedesktop.DBus.Introspectable")
	return
}

func (p *Portal) unexportCastObject(path dbus.ObjectPath, name string) {
	_ = p.conn.Export(nil, path, name)
	_ = p.conn.Export(nil, path, "org.freedesktop.DBus.Properties")
	_ = p.conn.Export(nil, path, "org.freedesktop.DBus.Introspectable")
}

func (p *Portal) DisableScreenCast() { p.disableScreenCast(nil) }

func (p *Portal) disableScreenCast(expected *screenCastService) {
	p.mu.Lock()
	s := p.screenCast
	if s == nil || (expected != nil && expected != s) {
		p.mu.Unlock()
		return
	}
	p.screenCast = nil
	s.stopLifetime()
	p.windows = maps.Clone(p.windows)
	delete(p.windows, s.dynamicID)
	p.unexportCastObject(screenCastPath, screenCastName)
	_, _ = p.conn.ReleaseName(screenCastName)
	sessions := make([]*castSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	p.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
	_ = p.conn.Emit(introspectPath, introspectName+".WindowsChanged")
}

func (s *screenCastService) CreateSession(sender dbus.Sender, properties map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	ctx, finish, err := s.p.begin(sender)
	if err != nil {
		return "", err
	}
	defer finish()
	if _, remote := properties["remote-desktop-session-id"]; remote {
		return "", dbus.MakeFailedError(errors.New("there are no remote desktop sessions"))
	}
	s.p.mu.Lock()
	defer s.p.mu.Unlock()
	if ctx.Err() != nil || s.p.screenCast != s {
		return "", dbus.MakeFailedError(errors.New("ScreenCast unavailable"))
	}
	id := ipc.NextCastSessionID()
	path := dbus.ObjectPath(fmt.Sprintf("%s/Session/u%d", screenCastPath, id))
	sessionCtx, cancel := context.WithCancel(s.p.ctx)
	session := &castSession{service: s, id: id, path: path, owner: sender, ctx: sessionCtx, cancel: cancel}
	if err := s.p.exportCastObject(path, screenCastSessionName, session, nil, []introspect.Signal{{Name: "Closed"}}); err != nil {
		cancel()
		return "", dbus.MakeFailedError(err)
	}
	s.sessions[id] = session
	return path, nil
}

// Caller authorization is checked under the same lock as object lifetime.
func (s *castSession) authorize(sender dbus.Sender) *dbus.Error {
	if sender != s.owner {
		return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"ScreenCast session belongs to another caller"})
	}
	if s.stopped || s.ctx.Err() != nil || s.service.p.screenCast != s.service {
		return dbus.MakeFailedError(errors.New("ScreenCast session stopped"))
	}
	return nil
}

func castProperties(properties map[string]dbus.Variant) (uint32, *dbus.Error) {
	mode := uint32(0)
	if value, exists := properties["cursor-mode"]; exists {
		var ok bool
		mode, ok = value.Value().(uint32)
		if !ok || mode > 2 {
			return 0, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"cursor-mode must be uint32 0, 1 or 2"})
		}
	}
	if value, exists := properties["is-recording"]; exists {
		if _, ok := value.Value().(bool); !ok {
			return 0, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"is-recording must be boolean"})
		}
	}
	return mode, nil
}

func (s *castSession) RecordMonitor(sender dbus.Sender, connector string, properties map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	s.service.p.mu.Lock()
	defer s.service.p.mu.Unlock()
	if err := s.authorize(sender); err != nil {
		return "", err
	}
	geometry, exists := s.service.p.state.Geometry[connector]
	if !exists {
		return "", dbus.MakeFailedError(errors.New("monitor is missing or disabled"))
	}
	return s.record(ipc.CastTarget{Output: &connector}, properties, geometry.X, geometry.Y, int32(geometry.Width), int32(geometry.Height))
}

func (s *castSession) RecordWindow(sender dbus.Sender, properties map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	s.service.p.mu.Lock()
	defer s.service.p.mu.Unlock()
	if err := s.authorize(sender); err != nil {
		return "", err
	}
	id, ok := properties["window-id"].Value().(uint64)
	if !ok {
		return "", dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"window-id must be uint64"})
	}
	return s.record(ipc.CastTarget{WindowID: &id}, properties, 0, 0, 1, 1)
}

// record runs with p.mu, and exports immutable stream parameters like Niri.
func (s *castSession) record(target ipc.CastTarget, properties map[string]dbus.Variant, x, y, width, height int32) (dbus.ObjectPath, *dbus.Error) {
	mode, err := castProperties(properties)
	if err != nil {
		return "", err
	}
	id := ipc.NextCastStreamID()
	path := dbus.ObjectPath(fmt.Sprintf("%s/Stream/u%d", screenCastPath, id))
	stream := &castStream{path: path, request: CastRequest{SessionID: s.id, StreamID: id, Target: target, CursorMode: mode}}
	params := map[string]dbus.Variant{"position": dbus.MakeVariant(struct{ X, Y int32 }{x, y}), "size": dbus.MakeVariant(struct{ W, H int32 }{width, height})}
	if err := s.service.p.exportCastObject(path, screenCastStreamName, stream, prop.Map{screenCastStreamName: {"Parameters": {Value: params, Emit: prop.EmitFalse}}}, []introspect.Signal{{Name: "PipeWireStreamAdded", Args: []introspect.Arg{{Name: "node_id", Type: "u"}}}}); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	s.streams = append(s.streams, stream)
	return path, nil
}

func (s *castSession) Start(sender dbus.Sender) *dbus.Error {
	p := s.service.p
	p.mu.Lock()
	if err := s.authorize(sender); err != nil {
		p.mu.Unlock()
		return err
	}
	var streams []*castStream
	for _, stream := range s.streams {
		if !stream.started {
			stream.started = true
			streams = append(streams, stream)
		}
	}
	p.mu.Unlock()
	for _, stream := range streams {
		err := s.service.hooks.Start(s.ctx, stream.request, func(node uint32) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if s.stopped || stream.added {
				return
			}
			stream.added = true
			_ = p.conn.Emit(stream.path, screenCastStreamName+".PipeWireStreamAdded", node)
		}, s.close)
		if err != nil {
			s.close()
			return dbus.MakeFailedError(err)
		}
	}
	return nil
}

func (s *castSession) Stop(sender dbus.Sender) *dbus.Error {
	p := s.service.p
	p.mu.Lock()
	err := s.authorize(sender)
	p.mu.Unlock()
	if err != nil {
		return err
	}
	s.close()
	return nil
}

func (s *castSession) close() {
	p := s.service.p
	p.mu.Lock()
	if s.stopped {
		p.mu.Unlock()
		return
	}
	s.stopped = true
	s.cancel()
	delete(s.service.sessions, s.id)
	_ = p.conn.Emit(s.path, screenCastSessionName+".Closed")
	for _, stream := range s.streams {
		p.unexportCastObject(stream.path, screenCastStreamName)
	}
	p.unexportCastObject(s.path, screenCastSessionName)
	p.mu.Unlock()
	s.service.hooks.Stop(s.id)
}

func (p *Portal) closeCallerCasts(name string) {
	p.mu.Lock()
	var sessions []*castSession
	if p.screenCast != nil {
		for _, session := range p.screenCast.sessions {
			if string(session.owner) == name {
				sessions = append(sessions, session)
			}
		}
	}
	p.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}
