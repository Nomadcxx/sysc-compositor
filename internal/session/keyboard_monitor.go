package session

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	keyboardMonitorName                  = "org.freedesktop.a11y.Manager"
	keyboardMonitorPath  dbus.ObjectPath = "/org/freedesktop/a11y/Manager"
	keyboardMonitorIface                 = "org.freedesktop.a11y.KeyboardMonitor"
)

// keyEventQueue bounds KeyEvent signals waiting for the bus. Input never
// waits for D-Bus; a full queue drops events instead.
const keyEventQueue = 256

type keyEvent struct {
	to  string
	key a11y.Key
}

// KeyboardMonitor serves org.freedesktop.a11y.KeyboardMonitor for Orca, as
// Niri 26.04's src/dbus/freedesktop_a11y.rs does, around an a11y.Monitor.
// Callers are keyed by unique bus name and forgotten when they leave.
type KeyboardMonitor struct {
	conn    *dbus.Conn
	monitor *a11y.Monitor
	signals chan *dbus.Signal
	events  chan keyEvent
	done    chan struct{}
	stopped sync.Once
	dropped atomic.Bool
}

// StartKeyboardMonitor serves the interface on the session bus.
func StartKeyboardMonitor(monitor *a11y.Monitor) (*KeyboardMonitor, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	k, err := startKeyboardMonitor(conn, monitor)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return k, nil
}

func startKeyboardMonitor(conn *dbus.Conn, monitor *a11y.Monitor) (*KeyboardMonitor, error) {
	k := &KeyboardMonitor{conn: conn, monitor: monitor, signals: make(chan *dbus.Signal, 64),
		events: make(chan keyEvent, keyEventQueue), done: make(chan struct{})}
	// Watch departures before any caller can grab, so none is missed.
	if err := conn.AddMatchSignal(dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged")); err != nil {
		return nil, err
	}
	conn.Signal(k.signals)
	go k.watch()
	go k.send()
	object := keyboardMonitorObject{k}
	node := &introspect.Node{Name: string(keyboardMonitorPath), Interfaces: []introspect.Interface{{
		Name: keyboardMonitorIface, Methods: introspect.Methods(object),
		Signals: []introspect.Signal{{Name: "KeyEvent", Args: []introspect.Arg{
			{Name: "released", Type: "b"}, {Name: "state", Type: "u"}, {Name: "keysym", Type: "u"},
			{Name: "unichar", Type: "u"}, {Name: "keycode", Type: "q"}}}},
	}}}
	if err := conn.Export(object, keyboardMonitorPath, keyboardMonitorIface); err != nil {
		k.stop()
		return nil, err
	}
	if err := conn.Export(introspect.NewIntrospectable(node), keyboardMonitorPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		k.stop()
		return nil, err
	}
	reply, err := conn.RequestName(keyboardMonitorName,
		dbus.NameFlagAllowReplacement|dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err != nil {
		k.stop()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		k.stop()
		return nil, fmt.Errorf("%s is owned by another service", keyboardMonitorName)
	}
	return k, nil
}

// ProcessKey asks the grab model what happens to key and queues its
// KeyEvent signals. It is called on the compositor thread and never blocks
// on the bus.
func (k *KeyboardMonitor) ProcessKey(key a11y.Key, repeatDelay time.Duration) a11y.Decision {
	decision, to := k.monitor.ProcessKey(key, repeatDelay)
	for _, name := range to {
		select {
		case k.events <- keyEvent{name, key}:
		default:
			if !k.dropped.Swap(true) {
				fmt.Fprintln(os.Stderr, "sysc-compositor: KeyboardMonitor: the bus is not keeping up; dropping key events")
			}
		}
	}
	return decision
}

// send emits each queued KeyEvent to its one caller.
func (k *KeyboardMonitor) send() {
	for {
		select {
		case <-k.done:
			return
		case event := <-k.events:
			key := event.key
			message := &dbus.Message{Type: dbus.TypeSignal, Headers: map[dbus.HeaderField]dbus.Variant{
				dbus.FieldPath:        dbus.MakeVariant(keyboardMonitorPath),
				dbus.FieldInterface:   dbus.MakeVariant(keyboardMonitorIface),
				dbus.FieldMember:      dbus.MakeVariant("KeyEvent"),
				dbus.FieldDestination: dbus.MakeVariant(event.to),
				dbus.FieldSignature:   dbus.MakeVariant(dbus.ParseSignatureMust("buuuq")),
			}, Body: []any{key.Released, key.Mods, key.Keysym, key.Unichar, key.Keycode}}
			_ = k.conn.Send(message, nil)
		}
	}
}

// watch forgets callers that leave the bus, and everyone when the name is
// taken over: no grab outlives the interface that took it.
func (k *KeyboardMonitor) watch() {
	for {
		select {
		case <-k.done:
			return
		case signal, ok := <-k.signals:
			if !ok {
				k.monitor.Reset()
				return
			}
			if signal == nil || signal.Sender != "org.freedesktop.DBus" {
				continue
			}
			switch signal.Name {
			case "org.freedesktop.DBus.NameLost":
				if len(signal.Body) == 1 && signal.Body[0] == keyboardMonitorName {
					k.monitor.Reset()
				}
			case "org.freedesktop.DBus.NameOwnerChanged":
				if len(signal.Body) != 3 {
					continue
				}
				name, _ := signal.Body[0].(string)
				oldOwner, _ := signal.Body[1].(string)
				newOwner, _ := signal.Body[2].(string)
				if oldOwner != "" && newOwner == "" {
					k.monitor.Forget(name)
				}
			}
		}
	}
}

func (k *KeyboardMonitor) stop() {
	k.stopped.Do(func() { close(k.done) })
	k.conn.RemoveSignal(k.signals)
	_ = k.conn.Export(nil, keyboardMonitorPath, keyboardMonitorIface)
	_ = k.conn.Export(nil, keyboardMonitorPath, "org.freedesktop.DBus.Introspectable")
}

// Close gives up the name and the connection, and every grab with them.
func (k *KeyboardMonitor) Close() {
	k.stop()
	_, _ = k.conn.ReleaseName(keyboardMonitorName)
	_ = k.conn.Close()
	k.monitor.Reset()
}

// keyboardMonitorObject carries only the interface's D-Bus methods, so
// KeyboardMonitor's Go methods are not exported on the bus.
type keyboardMonitorObject struct{ k *KeyboardMonitor }

func (o keyboardMonitorObject) GrabKeyboard(sender dbus.Sender) *dbus.Error {
	o.k.monitor.Grab(string(sender))
	return nil
}

func (o keyboardMonitorObject) UngrabKeyboard(sender dbus.Sender) *dbus.Error {
	o.k.monitor.Ungrab(string(sender))
	return nil
}

func (o keyboardMonitorObject) WatchKeyboard(sender dbus.Sender) *dbus.Error {
	o.k.monitor.Watch(string(sender))
	return nil
}

func (o keyboardMonitorObject) UnwatchKeyboard(sender dbus.Sender) *dbus.Error {
	o.k.monitor.Unwatch(string(sender))
	return nil
}

// SetKeyGrabs takes XKB modifier keysyms and (keysym, modifier mask)
// keystrokes, replacing the caller's previous grabs.
func (o keyboardMonitorObject) SetKeyGrabs(sender dbus.Sender, modifiers []uint32, keystrokes []struct{ Keysym, Mods uint32 }) *dbus.Error {
	strokes := make([][2]uint32, len(keystrokes))
	for i, stroke := range keystrokes {
		strokes[i] = [2]uint32{stroke.Keysym, stroke.Mods}
	}
	o.k.monitor.SetKeyGrabs(string(sender), modifiers, strokes)
	return nil
}
