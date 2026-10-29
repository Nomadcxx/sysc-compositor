package session

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func screenSaverCall(t *testing.T, conn *dbus.Conn, path dbus.ObjectPath, method string, args ...any) *dbus.Call {
	t.Helper()
	return conn.Object(screenSaverName, path).Call(screenSaverIface+"."+method, 0, args...)
}

func inhibit(t *testing.T, conn *dbus.Conn, path dbus.ObjectPath) uint32 {
	t.Helper()
	var cookie uint32
	if err := screenSaverCall(t, conn, path, "Inhibit", "test", "testing").Store(&cookie); err != nil {
		t.Fatalf("Inhibit at %s: %v", path, err)
	}
	if cookie == 0 {
		t.Fatalf("Inhibit at %s returned cookie 0", path)
	}
	return cookie
}

func expectNoChange(t *testing.T, changes <-chan bool) {
	t.Helper()
	select {
	case got := <-changes:
		t.Fatalf("inhibited changed to %v, want no change", got)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestScreenSaverInhibitsPerSenderAndDropsDepartedCallers: both of Niri's
// object paths serve Inhibit; UnInhibit drops one cookie; a caller that
// leaves the bus loses its own cookies and no one else's.
func TestScreenSaverInhibitsPerSenderAndDropsDepartedCallers(t *testing.T) {
	address := startPrivateBus(t)
	serviceConn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	changes := make(chan bool, 8)
	service, err := startScreenSaver(serviceConn, func(inhibited bool) { changes <- inhibited })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	connect := func() *dbus.Conn {
		conn, err := dbus.Connect(address)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	first, second := connect(), connect()
	t.Cleanup(func() { _ = second.Close() })

	a := inhibit(t, first, "/org/freedesktop/ScreenSaver")
	if !receive(t, changes) {
		t.Fatal("first inhibitor did not inhibit")
	}
	if err := second.Emit("/org/freedesktop/DBus", "org.freedesktop.DBus.NameOwnerChanged", first.Names()[0], first.Names()[0], ""); err != nil {
		t.Fatal(err)
	}
	expectNoChange(t, changes)
	b := inhibit(t, second, "/ScreenSaver")
	if a == b {
		t.Fatalf("two inhibitors share cookie %d", a)
	}
	expectNoChange(t, changes)

	if err := screenSaverCall(t, first, "/ScreenSaver", "UnInhibit", uint32(0xdead)).Err; err == nil {
		t.Error("UnInhibit of an unknown cookie succeeded")
	}
	if err := screenSaverCall(t, first, "/ScreenSaver", "UnInhibit", a).Err; err != nil {
		t.Fatalf("UnInhibit: %v", err)
	}
	expectNoChange(t, changes)

	// The first caller takes another cookie and leaves; the second's stays.
	inhibit(t, first, "/ScreenSaver")
	expectNoChange(t, changes)
	_ = first.Close()
	expectNoChange(t, changes)

	// The second caller leaving drops the last cookie.
	_ = second.Close()
	if receive(t, changes) {
		t.Fatal("inhibition stayed after every caller left the bus")
	}
}

// TestScreenSaverRefusesAnOwnedName: a name another service holds without
// allowing replacement is not taken over, and the start fails cleanly.
func TestScreenSaverRefusesAnOwnedName(t *testing.T) {
	address := startPrivateBus(t)
	owner, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if reply, err := owner.RequestName(screenSaverName, dbus.NameFlagDoNotQueue); err != nil ||
		reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("test owner RequestName = %v, %v", reply, err)
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if service, err := startScreenSaver(conn, nil); err == nil {
		service.Close()
		t.Fatal("started while another service owns the name without allowing replacement")
	}
}
