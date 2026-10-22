package session

import (
	"errors"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	screenSaverName  = "org.freedesktop.ScreenSaver"
	screenSaverIface = "org.freedesktop.ScreenSaver"
)

// screenSaverPaths are the two object paths Niri serves the interface at.
var screenSaverPaths = []dbus.ObjectPath{"/org/freedesktop/ScreenSaver", "/ScreenSaver"}

// ScreenSaver is org.freedesktop.ScreenSaver as Niri serves it in a session:
// Inhibit and UnInhibit, with each cookie kept against the caller's unique
// bus name and dropped when that name leaves the bus. It reports whether any
// inhibitor is left through onChange; it owns no idle policy itself.
type ScreenSaver struct {
	conn     *dbus.Conn
	signals  chan *dbus.Signal
	done     chan struct{}
	onChange func(inhibited bool)

	mu        sync.Mutex
	next      uint32
	cookies   map[uint32]string
	inhibited bool
}

// StartScreenSaver serves the interface on the session bus.
func StartScreenSaver(onChange func(inhibited bool)) (*ScreenSaver, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	s, err := startScreenSaver(conn, onChange)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

func startScreenSaver(conn *dbus.Conn, onChange func(bool)) (*ScreenSaver, error) {
	s := &ScreenSaver{
		conn: conn, signals: make(chan *dbus.Signal, 64), done: make(chan struct{}),
		onChange: onChange, cookies: map[uint32]string{},
	}
	// Watch for callers leaving before taking any cookie, so none is missed.
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged")); err != nil {
		return nil, err
	}
	conn.Signal(s.signals)
	go s.watch()
	exported := 0
	var exportErr error
	for _, path := range screenSaverPaths {
		if err := conn.Export(s, path, screenSaverIface); err != nil {
			exportErr = errors.Join(exportErr, err)
			continue
		}
		exported++
	}
	if exported == 0 {
		s.stop()
		return nil, exportErr
	}
	reply, err := conn.RequestName(screenSaverName,
		dbus.NameFlagAllowReplacement|dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err != nil {
		s.stop()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		s.stop()
		return nil, fmt.Errorf("%s is owned by another service", screenSaverName)
	}
	return s, nil
}

// Inhibit takes a cookie for the caller. The application name and reason are
// accepted and not used, as in Niri.
func (s *ScreenSaver) Inhibit(sender dbus.Sender, application, reason string) (uint32, *dbus.Error) {
	if sender == "" {
		return 0, dbus.MakeFailedError(errors.New("no sender"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for range 3 {
		s.next++
		if s.next == 0 {
			s.next++
		}
		if _, taken := s.cookies[s.next]; !taken {
			s.cookies[s.next] = string(sender)
			s.update()
			return s.next, nil
		}
	}
	return 0, dbus.MakeFailedError(errors.New("no available cookie"))
}

// UnInhibit drops one cookie, whoever took it, as Niri does.
func (s *ScreenSaver) UnInhibit(cookie uint32) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cookies[cookie]; !ok {
		return dbus.MakeFailedError(errors.New("invalid cookie"))
	}
	delete(s.cookies, cookie)
	s.update()
	return nil
}

// update reports a change in whether any inhibitor is left. It runs with mu
// held, so reports arrive in the order the changes happened.
func (s *ScreenSaver) update() {
	inhibited := len(s.cookies) > 0
	if inhibited == s.inhibited {
		return
	}
	s.inhibited = inhibited
	if s.onChange != nil {
		s.onChange(inhibited)
	}
}

// watch drops every cookie of a caller whose unique name leaves the bus.
func (s *ScreenSaver) watch() {
	for {
		select {
		case <-s.done:
			return
		case signal, ok := <-s.signals:
			if !ok {
				return
			}
			if signal.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(signal.Body) != 3 {
				continue
			}
			name, _ := signal.Body[0].(string)
			oldOwner, _ := signal.Body[1].(string)
			newOwner, _ := signal.Body[2].(string)
			if oldOwner == "" || newOwner != "" {
				continue
			}
			s.dropSender(name)
		}
	}
}

func (s *ScreenSaver) dropSender(sender string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for cookie, owner := range s.cookies {
		if owner == sender {
			delete(s.cookies, cookie)
		}
	}
	s.update()
}

func (s *ScreenSaver) stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.conn.RemoveSignal(s.signals)
	for _, path := range screenSaverPaths {
		_ = s.conn.Export(nil, path, screenSaverIface)
	}
}

// Close gives up the name and the connection. Inhibition ends with it.
func (s *ScreenSaver) Close() {
	s.stop()
	_, _ = s.conn.ReleaseName(screenSaverName)
	_ = s.conn.Close()
	s.mu.Lock()
	clear(s.cookies)
	s.update()
	s.mu.Unlock()
}
