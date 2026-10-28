package session

import (
	"context"
	"encoding/xml"
	"io"
	"net"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

func testPortal(t *testing.T, hooks PortalHooks) (*Portal, *dbus.Conn) {
	t.Helper()
	address := startPrivateBus(t)
	p, err := connectPortal(address, hooks)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return p, client
}

func TestPortalServicesUseNiriSignatures(t *testing.T) {
	p, client := testPortal(t, PortalHooks{
		Screenshot: func(context.Context, bool) (string, error) { return "/tmp/shot.png", nil },
		PickColor:  func(context.Context) ([3]float64, error) { return [3]float64{.25, .5, .75}, nil },
	})
	index := uint32(0)
	p.Update(map[string]ipc.Output{"eDP-1": {Name: "eDP-1", Make: "Acme", Model: "Panel", Modes: []ipc.Mode{{Width: 1920, Height: 1080, RefreshRate: 60000, IsPreferred: true}}, CurrentMode: &index, Logical: &ipc.LogicalOutput{Scale: 1.5, Transform: ipc.TransformNormal}}}, []ipc.Window{{ID: 42, Title: ptr("Editor"), AppID: ptr("app.editor")}})
	for _, tc := range []struct {
		name    string
		path    dbus.ObjectPath
		methods map[string][2]string
	}{
		{displayConfigName, displayConfigPath, map[string][2]string{"GetCurrentState": {"", "ua((ssss)a(siiddada{sv})a{sv})a(iiduba(ssss)a{sv})a{sv}"}, "ApplyMonitorsConfig": {"uua(iiduba(ssa{sv}))a{sv}", ""}}},
		{introspectName, introspectPath, map[string][2]string{"GetWindows": {"", "a{ta{sv}}"}}},
		{screenshotName, screenshotPath, map[string][2]string{"Screenshot": {"bbs", "bs"}, "PickColor": {"", "a{sv}"}}},
		{serviceChannelName, serviceChannelPath, map[string][2]string{"OpenWaylandServiceConnection": {"u", "h"}}},
	} {
		var document string
		if err := client.Object(tc.name, tc.path).Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&document); err != nil {
			t.Fatal(err)
		}
		var node introspect.Node
		if err := xml.Unmarshal([]byte(document), &node); err != nil {
			t.Fatal(err)
		}
		got := map[string][2]string{}
		for _, iface := range node.Interfaces {
			if iface.Name != tc.name {
				continue
			}
			for _, method := range iface.Methods {
				var signatures [2]string
				for _, arg := range method.Args {
					i := 0
					if arg.Direction == "out" {
						i = 1
					}
					signatures[i] += arg.Type
				}
				got[method.Name] = signatures
			}
		}
		if !reflect.DeepEqual(got, tc.methods) {
			t.Fatalf("%s methods = %v, want %v", tc.name, got, tc.methods)
		}
	}
	var serial uint32
	var monitors []Monitor
	var logical []LogicalMonitor
	var properties map[string]dbus.Variant
	object := client.Object(displayConfigName, displayConfigPath)
	if err := object.Call(displayConfigName+".GetCurrentState", 0).Store(&serial, &monitors, &logical, &properties); err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 1 || monitors[0].Names.Serial != "eDP-1" || logical[0].Scale != 1.5 || properties["layout-mode"].Value() != uint32(1) {
		t.Fatalf("invalid display state: %v %v %v", monitors, logical, properties)
	}
	old := serial
	p.Update(map[string]ipc.Output{}, nil)
	if err := object.Call(displayConfigName+".GetCurrentState", 0).Store(&serial, &monitors, &logical, &properties); err != nil {
		t.Fatal(err)
	}
	if serial == old || len(monitors) != 0 {
		t.Fatalf("stale state after hot unplug: %d %v", serial, monitors)
	}
	// Snapshot-only until M15: never claim that a requested configuration applied.
	if err := object.Call(displayConfigName+".ApplyMonitorsConfig", 0, serial, uint32(1), []LogicalMonitorConfiguration{}, map[string]dbus.Variant{}).Err; err == nil {
		t.Fatal("unsupported output application succeeded")
	}
	p.Update(nil, []ipc.Window{{ID: 42, Title: ptr("Editor"), AppID: ptr("app.editor")}})
	var windows map[uint64]map[string]dbus.Variant
	if err := client.Object(introspectName, introspectPath).Call(introspectName+".GetWindows", 0).Store(&windows); err != nil {
		t.Fatal(err)
	}
	if windows[42]["title"].Value() != "Editor" || windows[42]["app-id"].Value() != "app.editor.desktop" {
		t.Fatalf("windows = %v", windows)
	}
	var success bool
	var path string
	if err := client.Object(screenshotName, screenshotPath).Call(screenshotName+".Screenshot", 0, true, false, "ignored.png").Store(&success, &path); err != nil {
		t.Fatal(err)
	}
	if !success || path != "/tmp/shot.png" {
		t.Fatalf("screenshot = %v %s", success, path)
	}
	colorCall := client.Object(screenshotName, screenshotPath).Call(screenshotName+".PickColor", 0)
	if colorCall.Err != nil {
		t.Fatal(colorCall.Err)
	}
	color := colorCall.Body[0].(map[string]dbus.Variant)
	if color["color"].Signature().String() != "(ddd)" {
		t.Fatalf("color signature = %s", color["color"].Signature())
	}
}

func ptr[T any](v T) *T { return &v }

func TestServiceChannelOwnsItsDescriptors(t *testing.T) {
	accepted := make(chan int, 1)
	p, client := testPortal(t, PortalHooks{OpenWayland: func(_ context.Context, fd int) error {
		copy, err := syscall.Dup(fd)
		if err == nil {
			accepted <- copy
		}
		return err
	}})
	object := client.Object(serviceChannelName, serviceChannelPath)
	if err := object.Call(serviceChannelName+".OpenWaylandServiceConnection", 0, uint32(2)).Err; err == nil {
		t.Fatal("invalid type accepted")
	}
	var fd dbus.UnixFD
	if err := object.Call(serviceChannelName+".OpenWaylandServiceConnection", 0, uint32(1)).Store(&fd); err != nil {
		t.Fatal(err)
	}
	received := os.NewFile(uintptr(fd), "received")
	defer received.Close()
	peer := os.NewFile(uintptr(receive(t, accepted)), "peer")
	connection, err := net.FileConn(peer)
	peer.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := received.Write([]byte("proof")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(connection, buf); err != nil || string(buf) != "proof" {
		t.Fatalf("descriptor transfer = %q, %v", buf, err)
	}
	// Closing the recipient must close the peer: no server copy of its endpoint remains.
	received.Close()
	// A reply may arrive while the sender is still returning from Send. Wait
	// for EOF with a deadline, rather than assuming its deferred close ran first.
	if n, err := connection.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("returned endpoint retained: read=%d, %v", n, err)
	}
	p.Close()
}

func TestPortalRequestsFailCleanlyOnShutdown(t *testing.T) {
	started := make(chan struct{}, 2)
	p, client := testPortal(t, PortalHooks{Screenshot: func(ctx context.Context, _ bool) (string, error) {
		started <- struct{}{}
		<-ctx.Done()
		return "", ctx.Err()
	}})
	object := client.Object(screenshotName, screenshotPath)
	first := object.Go(screenshotName+".Screenshot", 0, make(chan *dbus.Call, 1), false, false, "")
	second := object.Go(screenshotName+".Screenshot", 0, make(chan *dbus.Call, 1), true, false, "")
	receive(t, started)
	receive(t, started)
	p.Close()
	for _, call := range []*dbus.Call{first, second} {
		select {
		case result := <-call.Done:
			if result.Err == nil {
				t.Fatal("shutdown request succeeded")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("request hung after shutdown")
		}
	}
	p.Close()
}

func TestPortalConcurrentRepliesStayWithTheirRequests(t *testing.T) {
	first, release := make(chan struct{}), make(chan struct{})
	_, client := testPortal(t, PortalHooks{Screenshot: func(ctx context.Context, pointer bool) (string, error) {
		if pointer {
			close(first)
			select {
			case <-release:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			return "with-pointer.png", nil
		}
		return "plain.png", nil
	}})
	object := client.Object(screenshotName, screenshotPath)
	call := object.Go(screenshotName+".Screenshot", 0, make(chan *dbus.Call, 1), true, false, "")
	receive(t, first)
	var success bool
	var path string
	if err := object.Call(screenshotName+".Screenshot", 0, false, false, "").Store(&success, &path); err != nil || path != "plain.png" {
		t.Fatalf("second reply = %s, %v", path, err)
	}
	close(release)
	if err := (<-call.Done).Store(&success, &path); err != nil || path != "with-pointer.png" {
		t.Fatalf("first reply = %s, %v", path, err)
	}
}

func TestServiceChannelSupportsOmittedInterfaceAndIndependentSenders(t *testing.T) {
	accepted := make(chan int, 2)
	address := startPrivateBus(t)
	p, err := connectPortal(address, PortalHooks{OpenWayland: func(_ context.Context, fd int) error {
		copy, err := syscall.Dup(fd)
		if err == nil {
			accepted <- copy
		}
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	// A fresh connection's first application call uses the same serial as any
	// other fresh connection. Request serials are scoped to the sender's name.
	second, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	calls := []*dbus.Call{
		first.Object(serviceChannelName, serviceChannelPath).Go("OpenWaylandServiceConnection", 0, make(chan *dbus.Call, 1), uint32(1)),
		second.Object(serviceChannelName, serviceChannelPath).Go(serviceChannelName+".OpenWaylandServiceConnection", 0, make(chan *dbus.Call, 1), uint32(1)),
	}
	for _, call := range calls {
		var fd dbus.UnixFD
		select {
		case result := <-call.Done:
			if err := result.Store(&fd); err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("service request did not reply")
		}
		file := os.NewFile(uintptr(fd), "portal endpoint")
		if _, err := file.Write([]byte("x")); err != nil {
			file.Close()
			t.Fatal(err)
		}
		file.Close()
	}
	for range calls {
		peer := os.NewFile(uintptr(receive(t, accepted)), "native endpoint")
		data := make([]byte, 1)
		if _, err := peer.Read(data); err != nil || string(data) != "x" {
			peer.Close()
			t.Fatalf("independent endpoint failed: %q, %v", data, err)
		}
		if n, err := peer.Read(data); n != 0 || err != io.EOF {
			peer.Close()
			t.Fatalf("sender retained transferred endpoint: %d, %v", n, err)
		}
		peer.Close()
	}
}

func TestPortalRequestsCancelOnBusLossAndCallerDeparture(t *testing.T) {
	started, cancelled := make(chan struct{}, 2), make(chan struct{}, 2)
	p, client := testPortal(t, PortalHooks{PickColor: func(ctx context.Context) ([3]float64, error) {
		started <- struct{}{}
		<-ctx.Done()
		cancelled <- struct{}{}
		return [3]float64{}, ctx.Err()
	}})
	client.Object(screenshotName, screenshotPath).Go(screenshotName+".PickColor", 0, make(chan *dbus.Call, 1))
	receive(t, started)
	client.Close()
	receive(t, cancelled)
	// A queued call can reach the handler after its departure signal. Unique
	// names are never reused, so an already-absent sender must not start work.
	ctx, finish, err := p.begin(dbus.Sender(":1.999999999"))
	if err == nil {
		finish()
		t.Fatalf("already-departed caller started a request: %v", ctx)
	}
}

func TestPortalBusLossCancelsPendingRequests(t *testing.T) {
	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	p, client := testPortal(t, PortalHooks{PickColor: func(ctx context.Context) ([3]float64, error) {
		started <- struct{}{}
		<-ctx.Done()
		cancelled <- struct{}{}
		return [3]float64{}, ctx.Err()
	}})
	client.Object(screenshotName, screenshotPath).Go(screenshotName+".PickColor", 0, make(chan *dbus.Call, 1))
	receive(t, started)
	p.conn.Close()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("pending picker survived session-bus loss")
	}
	select {
	case <-p.ctx.Done():
	default:
		t.Fatal("portal remained active after bus loss")
	}
}

func TestServiceChannelRejectsMalformedWireTypes(t *testing.T) {
	_, client := testPortal(t, PortalHooks{OpenWayland: func(context.Context, int) error { t.Error("malformed type created a native client"); return nil }})
	object := client.Object(serviceChannelName, serviceChannelPath)
	for _, kind := range []any{int32(1), uint64(1), dbus.MakeVariant(uint32(1))} {
		call := object.Call(serviceChannelName+".OpenWaylandServiceConnection", 0, kind)
		if call.Err == nil {
			// Close only the received SCM_RIGHTS duplicate if the regression returns
			// stdin; never leave the malformed reply descriptor owned by the test.
			if len(call.Body) == 1 {
				if fd, ok := call.Body[0].(dbus.UnixFD); ok {
					syscall.Close(int(fd))
				}
			}
			t.Errorf("accepted service-client type with signature %s", dbus.SignatureOf(kind))
		}
	}
}

func TestPortalRejectsAndClosesIncomingDescriptors(t *testing.T) {
	_, client := testPortal(t, PortalHooks{
		OpenWayland: func(context.Context, int) error { t.Error("incoming descriptor reached service handler"); return nil },
		Screenshot: func(context.Context, bool) (string, error) {
			t.Error("incoming descriptor reached screenshot handler")
			return "wrong.png", nil
		},
	})
	cases := []struct {
		name    string
		service string
		path    dbus.ObjectPath
		method  string
		args    func(dbus.UnixFD) []any
	}{
		{"direct", serviceChannelName, serviceChannelPath, "OpenWaylandServiceConnection", func(fd dbus.UnixFD) []any { return []any{fd} }},
		{"variant", serviceChannelName, serviceChannelPath, "OpenWaylandServiceConnection", func(fd dbus.UnixFD) []any { return []any{dbus.MakeVariant(fd)} }},
		{"array", serviceChannelName, serviceChannelPath, "OpenWaylandServiceConnection", func(fd dbus.UnixFD) []any { return []any{[]dbus.UnixFD{fd, fd}} }},
		{"nested-variant-array", serviceChannelName, serviceChannelPath, "OpenWaylandServiceConnection", func(fd dbus.UnixFD) []any {
			return []any{dbus.MakeVariant([]dbus.Variant{dbus.MakeVariant(fd), dbus.MakeVariant(fd)})}
		}},
		{"filename", screenshotName, screenshotPath, "Screenshot", func(fd dbus.UnixFD) []any { return []any{false, false, fd} }},
		{"properties", displayConfigName, displayConfigPath, "ApplyMonitorsConfig", func(fd dbus.UnixFD) []any {
			return []any{uint32(1), uint32(1), []LogicalMonitorConfiguration{}, map[string]dbus.Variant{"unexpected": dbus.MakeVariant(fd)}}
		}},
	}
	for _, tc := range cases {
		pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		call := client.Object(tc.service, tc.path).Call(tc.service+"."+tc.method, 0, tc.args(dbus.UnixFD(pair[0]))...)
		syscall.Close(pair[0])
		if call.Err == nil {
			t.Errorf("%s: incoming descriptor request succeeded", tc.name)
		}
		if err := syscall.SetNonblock(pair[1], true); err != nil {
			syscall.Close(pair[1])
			t.Fatal(err)
		}
		var buffer [1]byte
		n, err := syscall.Read(pair[1], buffer[:])
		syscall.Close(pair[1])
		if n != 0 || err != nil {
			t.Errorf("%s: received SCM_RIGHTS endpoint leaked: read=%d, %v", tc.name, n, err)
		}
	}
}
