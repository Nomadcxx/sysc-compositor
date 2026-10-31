package session

import (
	"reflect"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/godbus/dbus/v5"
)

const repeatDelay = 600 * time.Millisecond

func monitorCall(t *testing.T, conn *dbus.Conn, method string, args ...any) {
	t.Helper()
	call := conn.Object(keyboardMonitorName, keyboardMonitorPath).Call(keyboardMonitorIface+"."+method, 0, args...)
	if call.Err != nil {
		t.Fatalf("%s: %v", method, call.Err)
	}
}

func keyEvents(t *testing.T, conn *dbus.Conn) chan *dbus.Signal {
	t.Helper()
	if err := conn.AddMatchSignal(dbus.WithMatchInterface(keyboardMonitorIface), dbus.WithMatchMember("KeyEvent")); err != nil {
		t.Fatal(err)
	}
	events := make(chan *dbus.Signal, 16)
	conn.Signal(events)
	return events
}

func expectKeyEvent(t *testing.T, events chan *dbus.Signal, want []any) {
	t.Helper()
	got := receive(t, events)
	if got.Path != keyboardMonitorPath || got.Name != keyboardMonitorIface+".KeyEvent" || !reflect.DeepEqual(got.Body, want) {
		t.Fatalf("KeyEvent = %s %s %#v, want %s %#v", got.Path, got.Name, got.Body, keyboardMonitorPath, want)
	}
}

func expectNoKeyEvent(t *testing.T, events chan *dbus.Signal) {
	t.Helper()
	select {
	case got := <-events:
		t.Fatalf("unexpected KeyEvent %#v", got.Body)
	case <-time.After(200 * time.Millisecond):
	}
}

// eventually retries check for up to three seconds: bus departures and name
// loss reach the service asynchronously.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); !check(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestKeyboardMonitorServesCallersOnPrivateBus: Orca's calls reach the grab
// model, KeyEvent goes only to the callers that watch or grab the key with
// Niri's argument types, and grabs end when their caller leaves the bus or the
// service loses its name, never on a NameOwnerChanged another client forges.
func TestKeyboardMonitorServesCallersOnPrivateBus(t *testing.T) {
	address := startPrivateBus(t)
	connect := func() *dbus.Conn {
		conn, err := dbus.Connect(address)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	service, err := startKeyboardMonitor(connect(), a11y.NewMonitor())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	orca, watcher, bystander := connect(), connect(), connect()
	orcaEvents, watcherEvents, bystanderEvents := keyEvents(t, orca), keyEvents(t, watcher), keyEvents(t, bystander)

	monitorCall(t, orca, "SetKeyGrabs", []uint32{0xffe5}, []struct{ Keysym, Mods uint32 }{{0x68, 4}})
	monitorCall(t, watcher, "WatchKeyboard")
	ctrlH := a11y.Key{Time: time.Second, Mods: 4, Keysym: 0x68, Unichar: 8, Keycode: 51}
	tap := func(at time.Duration) (a11y.Decision, a11y.Decision) {
		press, release := ctrlH, ctrlH
		press.Time, release.Time, release.Released = at, at+10*time.Millisecond, true
		return service.ProcessKey(press, repeatDelay), service.ProcessKey(release, repeatDelay)
	}

	if pressed, released := tap(time.Second); pressed != a11y.Block || released != a11y.Block {
		t.Fatalf("Orca's Ctrl+H = %v then %v, want Block twice", pressed, released)
	}
	for _, events := range []chan *dbus.Signal{orcaEvents, watcherEvents} {
		expectKeyEvent(t, events, []any{false, uint32(4), uint32(0x68), uint32(8), uint16(51)})
		expectKeyEvent(t, events, []any{true, uint32(4), uint32(0x68), uint32(8), uint16(51)})
	}
	expectNoKeyEvent(t, bystanderEvents)

	orcaName := orca.Names()[0]
	if err := watcher.Emit("/org/freedesktop/DBus", "org.freedesktop.DBus.NameOwnerChanged", orcaName, orcaName, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if pressed, _ := tap(2 * time.Second); pressed != a11y.Block {
		t.Fatalf("a forged departure dropped Orca's grab: Ctrl+H = %v", pressed)
	}

	_ = orca.Close()
	eventually(t, "Orca's grabs end after it leaves the bus", func() bool {
		pressed, _ := tap(3 * time.Second)
		return pressed == a11y.Pass
	})

	monitorCall(t, watcher, "GrabKeyboard")
	if pressed, _ := tap(4 * time.Second); pressed != a11y.Block {
		t.Fatalf("GrabKeyboard: Ctrl+H = %v, want Block", pressed)
	}
	monitorCall(t, watcher, "UngrabKeyboard")
	monitorCall(t, watcher, "UnwatchKeyboard")
	if pressed, _ := tap(5 * time.Second); pressed != a11y.Pass {
		t.Fatalf("after UngrabKeyboard: Ctrl+H = %v, want Pass", pressed)
	}

	monitorCall(t, watcher, "GrabKeyboard")
	if reply, err := bystander.RequestName(keyboardMonitorName, dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue); err != nil ||
		reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("replacement RequestName = %v, %v", reply, err)
	}
	eventually(t, "grabs end when the service loses its name", func() bool {
		pressed, _ := tap(6 * time.Second)
		return pressed == a11y.Pass
	})
}

// TestKeyboardMonitorRefusesAnOwnedName: a name held without allowing
// replacement is not taken over, and the start fails cleanly.
func TestKeyboardMonitorRefusesAnOwnedName(t *testing.T) {
	address := startPrivateBus(t)
	owner, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if reply, err := owner.RequestName(keyboardMonitorName, dbus.NameFlagDoNotQueue); err != nil ||
		reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("test owner RequestName = %v, %v", reply, err)
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if service, err := startKeyboardMonitor(conn, a11y.NewMonitor()); err == nil {
		service.Close()
		t.Fatal("started while another service owns the name without allowing replacement")
	}
}
