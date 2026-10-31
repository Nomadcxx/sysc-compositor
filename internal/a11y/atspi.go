package a11y

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// AT-SPI values from at-spi2-core's atspi-constants.h (2.62).
const (
	roleAlert       = 2
	roleFrame       = 23
	roleLabel       = 29
	rolePanel       = 39
	rolePushButton  = 43
	roleApplication = 75

	stateActive    = 1
	stateEnabled   = 8
	stateFocusable = 11
	stateFocused   = 12
	stateModal     = 16
	stateSensitive = 24
	stateShowing   = 25
	stateVisible   = 30

	livePolite = 1
)

const (
	rootPath   dbus.ObjectPath = "/org/a11y/atspi/accessible/root"
	nullPath   dbus.ObjectPath = "/org/a11y/atspi/null"
	nodePrefix                 = "/org/a11y/atspi/accessible/sysc/"

	accessibleIface  = "org.a11y.atspi.Accessible"
	applicationIface = "org.a11y.atspi.Application"
	objectEvents     = "org.a11y.atspi.Event.Object"
	windowEvents     = "org.a11y.atspi.Event.Window"
)

// eventQueue bounds AT-SPI events waiting for the bus; the compositor never
// waits for a screen reader.
const eventQueue = 64

// objectAddress is AT-SPI's (so): a bus name and an object path.
type objectAddress struct {
	Name string
	Path dbus.ObjectPath
}

// Node is one of the compositor's accessible elements, Niri's AccessKit tree
// in src/a11y.rs: a window holding a polite live label and the four pieces of
// compositor UI that take keyboard focus.
type Node int

const (
	// NoFocus is the window itself: no compositor UI has keyboard focus.
	NoFocus Node = iota
	announcement
	ScreenshotUI
	ExitDialog
	Overview
	RecentWindows
	nodeCount
)

type element struct {
	role              uint32
	name, description string
}

func (n Node) path() dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s%d", nodePrefix, int(n)))
}

// Announcer is the compositor's AT-SPI application, standing in for Niri's
// AccessKit adapter. It joins the accessibility bus only while
// org.a11y.Status.IsEnabled is true, as accesskit_unix does. Its methods
// never block on D-Bus and are safe from any goroutine.
type Announcer struct {
	session *dbus.Conn
	signals chan *dbus.Signal
	events  chan pendingEvent
	done    chan struct{}
	closing sync.Once
	wg      sync.WaitGroup

	mu       sync.Mutex
	bus      *dbus.Conn
	parent   objectAddress
	id       int32
	elements [nodeCount]element
	focus    Node
	last     string // the last announcement, to repeat identical text
}

type pendingEvent struct {
	bus     *dbus.Conn
	path    dbus.ObjectPath
	name    string
	kind    string
	detail1 int32
	data    any
}

// StartAnnouncer watches the session bus's accessibility status.
func StartAnnouncer() (*Announcer, error) {
	session, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	a, err := startAnnouncer(session)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return a, nil
}

func startAnnouncer(session *dbus.Conn) (*Announcer, error) {
	a := &Announcer{session: session, signals: make(chan *dbus.Signal, 16),
		events: make(chan pendingEvent, eventQueue), done: make(chan struct{})}
	a.elements = [nodeCount]element{
		NoFocus:       {role: roleFrame},
		announcement:  {role: roleLabel},
		ScreenshotUI:  {role: rolePanel, name: "Screenshot UI"},
		ExitDialog:    {role: roleAlert, name: "Exit niri"},
		Overview:      {role: rolePanel, name: "Overview"},
		RecentWindows: {role: rolePanel, name: "Recent windows"},
	}
	if err := session.AddMatchSignal(dbus.WithMatchSender("org.a11y.Bus"), dbus.WithMatchObjectPath("/org/a11y/bus"),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged")); err != nil {
		return nil, err
	}
	session.Signal(a.signals)
	a.wg.Add(2)
	go a.watch()
	go a.send()
	return a, nil
}

// watch joins and leaves the accessibility bus as AT-SPI is enabled.
func (a *Announcer) watch() {
	defer a.wg.Done()
	var enabled bool
	if v, err := a.session.Object("org.a11y.Bus", "/org/a11y/bus").GetProperty("org.a11y.Status.IsEnabled"); err == nil {
		enabled, _ = v.Value().(bool)
	}
	a.setEnabled(enabled)
	for {
		select {
		case <-a.done:
			return
		case signal, ok := <-a.signals:
			if !ok {
				return
			}
			if signal.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" || len(signal.Body) < 2 ||
				signal.Body[0] != "org.a11y.Status" {
				continue
			}
			changed, _ := signal.Body[1].(map[string]dbus.Variant)
			if v, ok := changed["IsEnabled"]; ok {
				enabled, _ := v.Value().(bool)
				a.setEnabled(enabled)
			}
		}
	}
}

func (a *Announcer) setEnabled(enabled bool) {
	a.mu.Lock()
	joined := a.bus != nil
	a.mu.Unlock()
	if enabled == joined {
		return
	}
	if !enabled {
		a.leave()
		return
	}
	if err := a.join(); err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: AT-SPI: %v\n", err)
	}
}

func (a *Announcer) join() error {
	address := os.Getenv("AT_SPI_BUS_ADDRESS")
	if address == "" {
		if err := a.session.Object("org.a11y.Bus", "/org/a11y/bus").Call("org.a11y.Bus.GetAddress", 0).Store(&address); err != nil {
			return err
		}
	}
	bus, err := dbus.Connect(address)
	if err != nil {
		return err
	}
	exports := []struct {
		object any
		path   dbus.ObjectPath
		iface  string
	}{{rootAccessible{a}, rootPath, accessibleIface}, {properties{a, NoFocus, true}, rootPath, "org.freedesktop.DBus.Properties"}}
	for n := NoFocus; n < nodeCount; n++ {
		exports = append(exports, struct {
			object any
			path   dbus.ObjectPath
			iface  string
		}{nodeAccessible{a, n}, n.path(), accessibleIface}, struct {
			object any
			path   dbus.ObjectPath
			iface  string
		}{properties{a, n, false}, n.path(), "org.freedesktop.DBus.Properties"})
	}
	for _, e := range exports {
		if err := bus.Export(e.object, e.path, e.iface); err != nil {
			_ = bus.Close()
			return err
		}
	}
	a.mu.Lock()
	a.bus = bus
	a.mu.Unlock()
	var parent objectAddress
	if err := bus.Object("org.a11y.atspi.Registry", rootPath).Call("org.a11y.atspi.Socket.Embed", 0,
		objectAddress{bus.Names()[0], rootPath}).Store(&parent); err != nil {
		a.leave()
		return fmt.Errorf("embedding in the registry: %w", err)
	}
	a.mu.Lock()
	a.parent = parent
	a.mu.Unlock()
	return nil
}

func (a *Announcer) leave() {
	a.mu.Lock()
	bus := a.bus
	a.bus = nil
	a.mu.Unlock()
	if bus != nil {
		_ = bus.Close()
	}
}

// send emits queued events on the bus they were raised for.
func (a *Announcer) send() {
	defer a.wg.Done()
	for {
		select {
		case <-a.done:
			return
		case e := <-a.events:
			_ = e.bus.Emit(e.path, e.name, e.kind, e.detail1, int32(0), dbus.MakeVariant(e.data), map[string]dbus.Variant{})
		}
	}
}

// emit queues an event while joined. Callers hold mu.
func (a *Announcer) emit(path dbus.ObjectPath, name, kind string, detail1 int32, data any) {
	if a.bus == nil {
		return
	}
	select {
	case a.events <- pendingEvent{a.bus, path, name, kind, detail1, data}:
	default:
	}
}

// Announce has a screen reader speak text politely: Niri's a11y_announce.
func (a *Announcer) Announce(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// A screen reader speaks a live label only when its text changes.
	if text == a.last {
		text += " "
	}
	a.last = text
	a.elements[announcement].name = text
	path := announcement.path()
	a.emit(path, objectEvents+".PropertyChange", "accessible-name", 0, text)
	a.emit(path, objectEvents+".Announcement", "", livePolite, text)
}

// Focus moves the screen reader's focus to a piece of compositor UI, or with
// NoFocus back to applications.
func (a *Announcer) Focus(node Node) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if node == a.focus || node < NoFocus || node >= nodeCount || node == announcement {
		return
	}
	previous := a.focus
	a.focus = node
	if previous == NoFocus {
		a.emit(NoFocus.path(), windowEvents+".Activate", "", 0, "")
	} else {
		a.emit(previous.path(), objectEvents+".StateChanged", "focused", 0, int32(0))
	}
	if node == NoFocus {
		a.emit(NoFocus.path(), windowEvents+".Deactivate", "", 0, "")
	} else {
		a.emit(node.path(), objectEvents+".StateChanged", "focused", 1, int32(0))
	}
}

// Close leaves the accessibility bus and stops watching the session bus.
func (a *Announcer) Close() {
	a.closing.Do(func() {
		close(a.done)
		a.session.RemoveSignal(a.signals)
		_ = a.session.Close()
		a.wg.Wait()
		a.leave()
	})
}

func (a *Announcer) address(path dbus.ObjectPath) objectAddress {
	if a.bus == nil {
		return objectAddress{"", nullPath}
	}
	return objectAddress{a.bus.Names()[0], path}
}

func (a *Announcer) children(n Node) []objectAddress {
	if n != NoFocus {
		return []objectAddress{}
	}
	children := make([]objectAddress, 0, nodeCount-1)
	for child := announcement; child < nodeCount; child++ {
		children = append(children, a.address(child.path()))
	}
	return children
}

func (a *Announcer) states(n Node) []uint32 {
	set := uint64(1)<<stateEnabled | 1<<stateSensitive | 1<<stateShowing | 1<<stateVisible
	switch {
	case n == NoFocus && a.focus != NoFocus:
		set |= 1 << stateActive
	case n != NoFocus && n != announcement:
		set |= 1 << stateFocusable
		if n == a.focus {
			set |= 1 << stateFocused
		}
	}
	if n == ExitDialog {
		set |= 1 << stateModal
	}
	return []uint32{uint32(set), uint32(set >> 32)}
}

var roleNames = map[uint32]string{roleAlert: "alert", roleFrame: "frame", roleLabel: "label",
	rolePanel: "panel", rolePushButton: "push button", roleApplication: "application"}

// relation is AT-SPI's (ua(so)).
type relation struct {
	Type    uint32
	Targets []objectAddress
}

// rootAccessible is the application object the registry embeds.
type rootAccessible struct{ a *Announcer }

func (r rootAccessible) GetChildAtIndex(index int32) (objectAddress, *dbus.Error) {
	r.a.mu.Lock()
	defer r.a.mu.Unlock()
	if index != 0 {
		return objectAddress{}, dbus.MakeFailedError(errors.New("no such child"))
	}
	return r.a.address(NoFocus.path()), nil
}

func (r rootAccessible) GetChildren() ([]objectAddress, *dbus.Error) {
	r.a.mu.Lock()
	defer r.a.mu.Unlock()
	return []objectAddress{r.a.address(NoFocus.path())}, nil
}

func (r rootAccessible) GetIndexInParent() (int32, *dbus.Error)      { return -1, nil }
func (r rootAccessible) GetRelationSet() ([]relation, *dbus.Error)   { return []relation{}, nil }
func (r rootAccessible) GetRole() (uint32, *dbus.Error)              { return roleApplication, nil }
func (r rootAccessible) GetRoleName() (string, *dbus.Error)          { return "application", nil }
func (r rootAccessible) GetLocalizedRoleName() (string, *dbus.Error) { return "application", nil }
func (r rootAccessible) GetAttributes() (map[string]string, *dbus.Error) {
	return map[string]string{}, nil
}
func (r rootAccessible) GetState() ([]uint32, *dbus.Error) { return []uint32{0, 0}, nil }

func (r rootAccessible) GetApplication() (objectAddress, *dbus.Error) {
	r.a.mu.Lock()
	defer r.a.mu.Unlock()
	return r.a.address(rootPath), nil
}

func (r rootAccessible) GetInterfaces() ([]string, *dbus.Error) {
	return []string{accessibleIface, applicationIface}, nil
}

// nodeAccessible is one element of the compositor's window.
type nodeAccessible struct {
	a *Announcer
	n Node
}

func (o nodeAccessible) GetChildAtIndex(index int32) (objectAddress, *dbus.Error) {
	o.a.mu.Lock()
	defer o.a.mu.Unlock()
	children := o.a.children(o.n)
	if index < 0 || int(index) >= len(children) {
		return objectAddress{}, dbus.MakeFailedError(errors.New("no such child"))
	}
	return children[index], nil
}

func (o nodeAccessible) GetChildren() ([]objectAddress, *dbus.Error) {
	o.a.mu.Lock()
	defer o.a.mu.Unlock()
	return o.a.children(o.n), nil
}

func (o nodeAccessible) GetIndexInParent() (int32, *dbus.Error) {
	if o.n == NoFocus {
		return 0, nil
	}
	return int32(o.n - announcement), nil
}

func (o nodeAccessible) GetRelationSet() ([]relation, *dbus.Error) { return []relation{}, nil }

func (o nodeAccessible) GetRole() (uint32, *dbus.Error) {
	o.a.mu.Lock()
	defer o.a.mu.Unlock()
	return o.a.elements[o.n].role, nil
}

func (o nodeAccessible) GetRoleName() (string, *dbus.Error) {
	role, _ := o.GetRole()
	return roleNames[role], nil
}

func (o nodeAccessible) GetLocalizedRoleName() (string, *dbus.Error) { return o.GetRoleName() }

func (o nodeAccessible) GetState() ([]uint32, *dbus.Error) {
	o.a.mu.Lock()
	defer o.a.mu.Unlock()
	return o.a.states(o.n), nil
}

func (o nodeAccessible) GetAttributes() (map[string]string, *dbus.Error) {
	if o.n == announcement {
		return map[string]string{"live": "polite"}, nil
	}
	return map[string]string{}, nil
}

func (o nodeAccessible) GetApplication() (objectAddress, *dbus.Error) {
	o.a.mu.Lock()
	defer o.a.mu.Unlock()
	return o.a.address(rootPath), nil
}

func (o nodeAccessible) GetInterfaces() ([]string, *dbus.Error) {
	return []string{accessibleIface}, nil
}

// properties serves org.freedesktop.DBus.Properties for one object. Values
// change as the compositor announces, so they are read live.
type properties struct {
	a    *Announcer
	n    Node
	root bool
}

func (p properties) values(iface string) (map[string]dbus.Variant, *dbus.Error) {
	p.a.mu.Lock()
	defer p.a.mu.Unlock()
	switch {
	case iface == applicationIface && p.root:
		return map[string]dbus.Variant{"ToolkitName": dbus.MakeVariant("niri"), "Version": dbus.MakeVariant(""),
			"AtspiVersion": dbus.MakeVariant("2.1"), "Id": dbus.MakeVariant(p.a.id)}, nil
	case iface != accessibleIface:
		return nil, dbus.NewError("org.freedesktop.DBus.Error.UnknownInterface", []any{iface})
	case p.root:
		return map[string]dbus.Variant{"Name": dbus.MakeVariant("niri"), "Description": dbus.MakeVariant(""),
			"Parent": dbus.MakeVariant(p.a.parent), "ChildCount": dbus.MakeVariant(int32(1)),
			"Locale": dbus.MakeVariant(""), "AccessibleId": dbus.MakeVariant(""), "HelpText": dbus.MakeVariant("")}, nil
	}
	parent := p.a.address(NoFocus.path())
	if p.n == NoFocus {
		parent = p.a.address(rootPath)
	}
	e := p.a.elements[p.n]
	return map[string]dbus.Variant{"Name": dbus.MakeVariant(e.name), "Description": dbus.MakeVariant(e.description),
		"Parent": dbus.MakeVariant(parent), "ChildCount": dbus.MakeVariant(int32(len(p.a.children(p.n)))),
		"Locale": dbus.MakeVariant(""), "AccessibleId": dbus.MakeVariant(""), "HelpText": dbus.MakeVariant("")}, nil
}

func (p properties) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	values, err := p.values(iface)
	if err != nil {
		return dbus.Variant{}, err
	}
	v, ok := values[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{name})
	}
	return v, nil
}

func (p properties) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	return p.values(iface)
}

// Set takes only the Id the registry assigns the application.
func (p properties) Set(iface, name string, value dbus.Variant) *dbus.Error {
	id, ok := value.Value().(int32)
	if !p.root || iface != applicationIface || name != "Id" || !ok {
		return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly", []any{name})
	}
	p.a.mu.Lock()
	p.a.id = id
	p.a.mu.Unlock()
	return nil
}
