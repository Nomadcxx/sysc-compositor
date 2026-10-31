package a11y

import (
	"bufio"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

func privateBus(t *testing.T) string {
	t.Helper()
	command := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Skipf("dbus-daemon unavailable: %v", err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatalf("dbus-daemon did not print an address: %v", scanner.Err())
	}
	return scanner.Text()
}

func connectTo(t *testing.T, address string) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// a11yBusLauncher stands in for at-spi-bus-launcher on the session bus.
type a11yBusLauncher struct{ address string }

func (l a11yBusLauncher) GetAddress() (string, *dbus.Error) { return l.address, nil }

// registry stands in for at-spi2-registryd on the accessibility bus.
type registry struct {
	self     string
	embedded chan objectAddress
}

func (r registry) Embed(child objectAddress) (objectAddress, *dbus.Error) {
	r.embedded <- child
	return objectAddress{r.self, rootPath}, nil
}

type event struct {
	path          dbus.ObjectPath
	name, kind    string
	detail1       int32
	anyData       any
	fromAnnouncer bool
}

func nextEvent(t *testing.T, events chan *dbus.Signal, announcer string) event {
	t.Helper()
	select {
	case s := <-events:
		if len(s.Body) != 5 {
			t.Fatalf("event %s body %#v, want (siiva{sv})", s.Name, s.Body)
		}
		kind, _ := s.Body[0].(string)
		detail1, _ := s.Body[1].(int32)
		anyData, _ := s.Body[3].(dbus.Variant)
		if _, ok := s.Body[4].(map[string]dbus.Variant); !ok {
			t.Fatalf("event %s properties %#v, want a{sv}", s.Name, s.Body[4])
		}
		return event{s.Path, s.Name, kind, detail1, anyData.Value(), s.Sender == announcer}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an AT-SPI event")
		return event{}
	}
}

// harness is a session bus with a fake at-spi-bus-launcher, an
// accessibility bus with a fake registry and a listener, and an announcer
// started with AT-SPI disabled.
type harness struct {
	announcer *Announcer
	status    *prop.Properties
	embedded  chan objectAddress
	events    chan *dbus.Signal
	listener  *dbus.Conn
}

func startHarness(t *testing.T) harness {
	t.Helper()
	session, accessibility := privateBus(t), privateBus(t)

	launcher := connectTo(t, session)
	if err := launcher.Export(a11yBusLauncher{accessibility}, "/org/a11y/bus", "org.a11y.Bus"); err != nil {
		t.Fatal(err)
	}
	status, err := prop.Export(launcher, "/org/a11y/bus", prop.Map{"org.a11y.Status": {
		"IsEnabled":           {Value: false, Writable: true, Emit: prop.EmitTrue},
		"ScreenReaderEnabled": {Value: false, Writable: true, Emit: prop.EmitTrue},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.RequestName("org.a11y.Bus", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}

	registryConn := connectTo(t, accessibility)
	embedded := make(chan objectAddress, 4)
	if err := registryConn.Export(registry{registryConn.Names()[0], embedded}, rootPath, "org.a11y.atspi.Socket"); err != nil {
		t.Fatal(err)
	}
	if _, err := registryConn.RequestName("org.a11y.atspi.Registry", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	listener := connectTo(t, accessibility)
	for _, iface := range []string{"org.a11y.atspi.Event.Object", "org.a11y.atspi.Event.Window"} {
		if err := listener.AddMatchSignal(dbus.WithMatchInterface(iface)); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan *dbus.Signal, 32)
	listener.Signal(events)

	announcer, err := startAnnouncer(connectTo(t, session))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(announcer.Close)

	return harness{announcer, status, embedded, events, listener}
}

// enable turns AT-SPI on and returns where the compositor embedded.
func (h harness) enable(t *testing.T) objectAddress {
	t.Helper()
	if err := h.status.Set("org.a11y.Status", "IsEnabled", dbus.MakeVariant(true)); err != nil {
		t.Fatal(err)
	}
	select {
	case app := <-h.embedded:
		return app
	case <-time.After(3 * time.Second):
		t.Fatal("the compositor never embedded in the registry")
		return objectAddress{}
	}
}

// TestAnnouncementsReachAnATSPIListener: while AT-SPI is enabled the
// compositor embeds an application in the registry whose tree a screen
// reader can walk (Niri's AccessKit tree), announces through a polite live
// label with AccessKit's events, repeats identical text with a trailing
// space, and moves focus with window and focused-state events. Disabled, it
// registers nothing.
func TestAnnouncementsReachAnATSPIListener(t *testing.T) {
	h := startHarness(t)
	announcer, status, embedded, events, listener := h.announcer, h.status, h.embedded, h.events, h.listener
	announcer.Announce("Workspace 1")
	select {
	case child := <-embedded:
		t.Fatalf("embedded %v while AT-SPI is disabled", child)
	case s := <-events:
		t.Fatalf("emitted %s while AT-SPI is disabled", s.Name)
	case <-time.After(300 * time.Millisecond):
	}

	if err := status.Set("org.a11y.Status", "IsEnabled", dbus.MakeVariant(true)); err != nil {
		t.Fatal(err)
	}
	var app objectAddress
	select {
	case app = <-embedded:
	case <-time.After(3 * time.Second):
		t.Fatal("the compositor never embedded in the registry")
	}
	if app.Path != rootPath {
		t.Fatalf("embedded %v, want the application at %s", app, rootPath)
	}

	object := func(path dbus.ObjectPath) dbus.BusObject { return listener.Object(app.Name, path) }
	call := func(path dbus.ObjectPath, method string, out any, args ...any) {
		t.Helper()
		if err := object(path).Call("org.a11y.atspi.Accessible."+method, 0, args...).Store(out); err != nil {
			t.Fatalf("%s %s: %v", path, method, err)
		}
	}
	property := func(path dbus.ObjectPath, iface, name string) any {
		t.Helper()
		v, err := object(path).GetProperty(iface + "." + name)
		if err != nil {
			t.Fatalf("%s %s.%s: %v", path, iface, name, err)
		}
		return v.Value()
	}
	if got := property(rootPath, "org.a11y.atspi.Application", "ToolkitName"); got != "niri" {
		t.Errorf("ToolkitName = %v, want niri", got)
	}
	if err := object(rootPath).SetProperty("org.a11y.atspi.Application.Id", dbus.MakeVariant(int32(7))); err != nil {
		t.Errorf("the registry could not set the application Id: %v", err)
	}
	if got := property(rootPath, "org.a11y.atspi.Application", "Id"); got != int32(7) {
		t.Errorf("Id = %v, want 7", got)
	}
	var role uint32
	var children []objectAddress
	call(rootPath, "GetRole", &role)
	call(rootPath, "GetChildren", &children)
	if role != roleApplication || len(children) != 1 {
		t.Fatalf("root role %d children %v, want an application with one window", role, children)
	}
	window := children[0].Path
	call(window, "GetRole", &role)
	call(window, "GetChildren", &children)
	var names []string
	var roles []uint32
	for _, child := range children {
		var r uint32
		call(child.Path, "GetRole", &r)
		roles = append(roles, r)
		names = append(names, property(child.Path, "org.a11y.atspi.Accessible", "Name").(string))
		var parent objectAddress
		if v := property(child.Path, "org.a11y.atspi.Accessible", "Parent"); v != nil {
			_ = dbus.Store([]any{v}, &parent)
		}
		if parent.Path != window {
			t.Errorf("%s parent %v, want the window", child.Path, parent)
		}
	}
	wantNames := []string{"Workspace 1", "Screenshot UI", "Exit sysc-compositor", "Overview", "Recent windows"}
	wantRoles := []uint32{roleLabel, rolePanel, roleAlert, rolePanel, rolePanel}
	if role != roleFrame || !reflect.DeepEqual(names, wantNames) || !reflect.DeepEqual(roles, wantRoles) {
		t.Fatalf("window role %d, children %q roles %v; want frame with %q %v", role, names, roles, wantNames, wantRoles)
	}
	label, overview := children[0].Path, children[3].Path
	var attributes map[string]string
	call(label, "GetAttributes", &attributes)
	if attributes["live"] != "polite" {
		t.Errorf("announcement label attributes %v, want live polite", attributes)
	}

	expect := func(want event) {
		t.Helper()
		got := nextEvent(t, events, app.Name)
		want.fromAnnouncer = true
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("event %+v, want %+v", got, want)
		}
	}
	announcer.Announce("Workspace 2")
	expect(event{label, "org.a11y.atspi.Event.Object.PropertyChange", "accessible-name", 0, "Workspace 2", false})
	expect(event{label, "org.a11y.atspi.Event.Object.Announcement", "", 1, "Workspace 2", false})
	announcer.Announce("Workspace 2")
	expect(event{label, "org.a11y.atspi.Event.Object.PropertyChange", "accessible-name", 0, "Workspace 2 ", false})
	expect(event{label, "org.a11y.atspi.Event.Object.Announcement", "", 1, "Workspace 2 ", false})

	announcer.Focus(Overview)
	expect(event{window, "org.a11y.atspi.Event.Window.Activate", "", 0, "", false})
	expect(event{overview, "org.a11y.atspi.Event.Object.StateChanged", "focused", 1, int32(0), false})
	var state []uint32
	call(overview, "GetState", &state)
	if len(state) != 2 || state[0]&(1<<stateFocused) == 0 {
		t.Errorf("overview state %v, want focused", state)
	}
	announcer.Focus(NoFocus)
	expect(event{overview, "org.a11y.atspi.Event.Object.StateChanged", "focused", 0, int32(0), false})
	expect(event{window, "org.a11y.atspi.Event.Window.Deactivate", "", 0, "", false})

	if err := status.Set("org.a11y.Status", "IsEnabled", dbus.MakeVariant(false)); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		var owned bool
		if err := listener.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, app.Name).Store(&owned); err != nil {
			t.Fatal(err)
		}
		if !owned {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the compositor stayed on the accessibility bus after AT-SPI was disabled")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestRecentWindowsAndDescriptionsReachTheTree: the recent-windows node
// flips between a panel and a button named for the selected window, as
// Niri's MRU node does, repeats an identical title with a trailing space, and
// a node's description can be set from the compositor's own text.
func TestRecentWindowsAndDescriptionsReachTheTree(t *testing.T) {
	h := startHarness(t)
	app := h.enable(t)
	expect := func(want event) {
		t.Helper()
		got := nextEvent(t, h.events, app.Name)
		want.fromAnnouncer = true
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("event %+v, want %+v", got, want)
		}
	}
	mru := RecentWindows.path()
	role := func() uint32 {
		t.Helper()
		var role uint32
		if err := h.listener.Object(app.Name, mru).Call(accessibleIface+".GetRole", 0).Store(&role); err != nil {
			t.Fatal(err)
		}
		return role
	}
	const propertyChange = objectEvents + ".PropertyChange"
	h.announcer.SetRecentWindows("Firefox", true)
	expect(event{mru, propertyChange, "accessible-role", 0, uint32(rolePushButton), false})
	expect(event{mru, propertyChange, "accessible-name", 0, "Firefox", false})
	if got := role(); got != rolePushButton {
		t.Errorf("selected window role %d, want push button", got)
	}
	h.announcer.SetRecentWindows("Firefox", true)
	expect(event{mru, propertyChange, "accessible-name", 0, "Firefox ", false})
	h.announcer.SetRecentWindows("Recent windows empty, Scope all", false)
	expect(event{mru, propertyChange, "accessible-role", 0, uint32(rolePanel), false})
	expect(event{mru, propertyChange, "accessible-name", 0, "Recent windows empty, Scope all", false})

	dialog := ExitDialog.path()
	h.announcer.SetDescription(ExitDialog, "Are you sure you want to exit?")
	expect(event{dialog, propertyChange, "accessible-description", 0, "Are you sure you want to exit?", false})
	v, err := h.listener.Object(app.Name, dialog).GetProperty(accessibleIface + ".Description")
	if err != nil || v.Value() != "Are you sure you want to exit?" {
		t.Errorf("exit dialog description %v, %v", v, err)
	}
}
