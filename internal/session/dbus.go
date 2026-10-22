package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/godbus/dbus/v5"
)

const (
	localeBus       = "org.freedesktop.locale1"
	localePath      = dbus.ObjectPath("/org/freedesktop/locale1")
	localeIface     = "org.freedesktop.locale1"
	loginBus        = "org.freedesktop.login1"
	loginPath       = dbus.ObjectPath("/org/freedesktop/login1")
	loginIface      = "org.freedesktop.login1.Manager"
	propertiesIface = "org.freedesktop.DBus.Properties"
)

// DBus watches locale1 XKB defaults and login1 lid state. Callbacks run on its
// signal goroutine; callers must post compositor changes to the owner thread.
type DBus struct {
	conn      *dbus.Conn
	signals   chan *dbus.Signal
	done      chan struct{}
	wg        sync.WaitGroup
	close     sync.Once
	onXKB     func(config.Keyboard)
	onLid     func(bool)
	locale    config.Keyboard
	lidClosed bool
	hasLid    bool
}

func StartDBus(onXKB func(config.Keyboard), onLid func(bool)) (*DBus, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect system bus: %w", err)
	}
	watcher, err := startDBus(conn, onXKB, onLid)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return watcher, nil
}

func startDBus(conn *dbus.Conn, onXKB func(config.Keyboard), onLid func(bool)) (*DBus, error) {
	w := &DBus{
		conn: conn, signals: make(chan *dbus.Signal, 64), done: make(chan struct{}),
		onXKB: onXKB, onLid: onLid,
	}
	for _, match := range [][]dbus.MatchOption{
		{dbus.WithMatchSender(localeBus), dbus.WithMatchInterface(propertiesIface),
			dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchObjectPath(localePath)},
		{dbus.WithMatchSender(loginBus), dbus.WithMatchInterface(propertiesIface),
			dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchObjectPath(loginPath)},
	} {
		if err := conn.AddMatchSignal(match...); err != nil {
			return nil, fmt.Errorf("subscribe to PropertiesChanged: %w", err)
		}
	}
	conn.Signal(w.signals)
	w.readInitialProperties()
	w.wg.Add(1)
	go w.readSignals()
	return w, nil
}

func (w *DBus) readInitialProperties() {
	if props, err := w.getAll(localeBus, localePath, localeIface); err != nil {
		warn("locale1 properties", err)
	} else if xkb, ok := localeXKB(props); ok {
		w.locale = xkb
		if w.onXKB != nil {
			w.onXKB(xkb)
		}
	} else {
		warn("locale1 properties", fmt.Errorf("X11 keyboard properties are absent"))
	}
	if props, err := w.getAll(loginBus, loginPath, loginIface); err != nil {
		warn("login1 properties", err)
	} else if closed, ok := lidClosedProperty(props); ok {
		w.lidClosed, w.hasLid = closed, true
		if w.onLid != nil {
			w.onLid(closed)
		}
	} else {
		warn("login1 properties", fmt.Errorf("LidClosed property is absent or invalid"))
	}
}

func (w *DBus) getAll(bus string, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var properties map[string]dbus.Variant
	call := w.conn.Object(bus, path).CallWithContext(ctx,
		propertiesIface+".GetAll", 0, iface)
	if err := call.Store(&properties); err != nil {
		return nil, err
	}
	return properties, nil
}

func (w *DBus) readSignals() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case signal, ok := <-w.signals:
			if !ok {
				return
			}
			w.handleSignal(signal)
		}
	}
}

func (w *DBus) handleSignal(signal *dbus.Signal) {
	if signal == nil || len(signal.Body) < 2 {
		return
	}
	iface, ok := signal.Body[0].(string)
	if !ok {
		return
	}
	changed, ok := signal.Body[1].(map[string]dbus.Variant)
	if !ok {
		return
	}
	switch {
	case signal.Path == localePath && iface == localeIface:
		if updated, changed := applyLocaleChanges(w.locale, changed); changed {
			w.locale = updated
			if w.onXKB != nil {
				w.onXKB(updated)
			}
		}
	case signal.Path == loginPath && iface == loginIface:
		if closed, exists := changed["LidClosed"]; exists {
			value, valid := closed.Value().(bool)
			if !valid {
				warn("login1 LidClosed", fmt.Errorf("invalid property type %T", closed.Value()))
				return
			}
			if !w.hasLid || w.lidClosed != value {
				w.lidClosed, w.hasLid = value, true
				if w.onLid != nil {
					w.onLid(value)
				}
			}
		}
	}
}

func localeXKB(properties map[string]dbus.Variant) (config.Keyboard, bool) {
	var keyboard config.Keyboard
	found := false
	for name, target := range map[string]*string{
		"X11Model": &keyboard.Model, "X11Layout": &keyboard.Layout,
		"X11Variant": &keyboard.Variant, "X11Options": &keyboard.Options,
	} {
		if value, ok := properties[name]; ok {
			if text, valid := value.Value().(string); valid {
				*target, found = text, true
			}
		}
	}
	return keyboard, found
}

func applyLocaleChanges(current config.Keyboard, properties map[string]dbus.Variant) (config.Keyboard, bool) {
	changed := false
	for name, target := range map[string]*string{
		"X11Model": &current.Model, "X11Layout": &current.Layout,
		"X11Variant": &current.Variant, "X11Options": &current.Options,
	} {
		if value, ok := properties[name]; ok {
			text, valid := value.Value().(string)
			if valid && *target != text {
				*target, changed = text, true
			}
		}
	}
	return current, changed
}

func lidClosedProperty(properties map[string]dbus.Variant) (bool, bool) {
	value, ok := properties["LidClosed"]
	if !ok {
		return false, false
	}
	closed, ok := value.Value().(bool)
	return closed, ok
}

func (w *DBus) Close() {
	if w == nil {
		return
	}
	w.close.Do(func() {
		close(w.done)
		w.conn.RemoveSignal(w.signals)
		_ = w.conn.Close()
		w.wg.Wait()
	})
}
