package session

import (
	"context"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/godbus/dbus/v5"
)

func TestScreenCastSessionAndStreamLifecycle(t *testing.T) {
	address := startPrivateBus(t)
	p, err := connectPortal(address, PortalHooks{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	started := make(chan CastRequest, 8)
	stopped := make(chan uint64, 8)
	err = p.EnableScreenCast(context.Background(), ScreenCastHooks{
		Start: func(ctx context.Context, request CastRequest, node func(uint32), closed func()) error {
			started <- request
			node(47)
			return ctx.Err()
		},
		Stop: func(id uint64) { stopped <- id },
	}, 900)
	if err != nil {
		t.Fatal(err)
	}
	p.Update(map[string]ipc.Output{"DP-1": {Name: "DP-1", Logical: &ipc.LogicalOutput{X: 13, Y: 17, Width: 1920, Height: 1080, Scale: 1}}}, nil)
	root := client.Object(screenCastName, screenCastPath)
	version, err := root.GetProperty(screenCastName + ".Version")
	if err != nil || version.Value() != int32(4) {
		t.Fatalf("version=%v: %v", version, err)
	}
	if err := root.Call(screenCastName+".CreateSession", 0, map[string]dbus.Variant{"remote-desktop-session-id": dbus.MakeVariant("remote")}).Err; err == nil {
		t.Fatal("accepted remote desktop session")
	}
	var path dbus.ObjectPath
	if err := root.Call(screenCastName+".CreateSession", 0, map[string]dbus.Variant{}).Store(&path); err != nil {
		t.Fatal(err)
	}
	object := client.Object(screenCastName, path)
	var streamPath dbus.ObjectPath
	if err := object.Call(screenCastSessionName+".RecordMonitor", 0, "missing", map[string]dbus.Variant{}).Err; err == nil {
		t.Fatal("accepted missing monitor")
	}
	if err := object.Call(screenCastSessionName+".RecordWindow", 0, map[string]dbus.Variant{"window-id": dbus.MakeVariant(uint64(81)), "cursor-mode": dbus.MakeVariant(uint32(3))}).Err; err == nil {
		t.Fatal("accepted invalid pointer mode")
	}
	if err := object.Call(screenCastSessionName+".RecordWindow", 0, map[string]dbus.Variant{"window-id": dbus.MakeVariant(int64(81))}).Err; err == nil {
		t.Fatal("accepted signed window ID")
	}
	if err := object.Call(screenCastSessionName+".RecordMonitor", 0, "DP-1", map[string]dbus.Variant{"cursor-mode": dbus.MakeVariant(uint32(2))}).Store(&streamPath); err != nil {
		t.Fatal(err)
	}
	params, err := client.Object(screenCastName, streamPath).GetProperty(screenCastStreamName + ".Parameters")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]dbus.Variant
	if err := dbus.Store([]any{params.Value()}, &values); err != nil {
		t.Fatal(err)
	}
	var size struct{ W, H int32 }
	if err := dbus.Store([]any{values["size"].Value()}, &size); err != nil || size.W != 1920 || size.H != 1080 {
		t.Fatalf("parameters=%v: %v", values, err)
	}
	other, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Object(screenCastName, path).Call(screenCastSessionName+".Stop", 0).Err; err == nil {
		t.Fatal("another caller stopped owner's session")
	}
	if err := other.Emit("/org/freedesktop/DBus", "org.freedesktop.DBus.NameOwnerChanged", client.Names()[0], client.Names()[0], ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
		t.Fatal("forged departure stopped another caller's session")
	case <-time.After(50 * time.Millisecond):
	}
	signals := make(chan *dbus.Signal, 8)
	client.Signal(signals)
	defer client.RemoveSignal(signals)
	if err := client.AddMatchSignal(dbus.WithMatchInterface(screenCastStreamName)); err != nil {
		t.Fatal(err)
	}
	if err := object.Call(screenCastSessionName+".Start", 0).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-started:
		if request.CursorMode != 2 || request.Target.Output == nil || *request.Target.Output != "DP-1" {
			t.Fatalf("start=%+v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("start did not reach owner")
	}
	select {
	case signal := <-signals:
		if signal.Name != screenCastStreamName+".PipeWireStreamAdded" || len(signal.Body) != 1 || signal.Body[0] != uint32(47) {
			t.Fatalf("node signal=%+v", signal)
		}
	case <-time.After(time.Second):
		t.Fatal("node signal missing")
	}
	if err := object.Call(screenCastSessionName+".Start", 0).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
		t.Fatal("stream started twice")
	default:
	}
	if err := object.Call(screenCastSessionName+".Stop", 0).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not reach owner")
	}
	if err := object.Call(screenCastSessionName+".Start", 0).Err; err == nil {
		t.Fatal("retired session restarted")
	}
	if _, err := client.Object(screenCastName, streamPath).GetProperty(screenCastStreamName + ".Parameters"); err == nil {
		t.Fatal("retired stream remains exported")
	}
	// Caller departure must retire even sessions which never started.
	if err := root.Call(screenCastName+".CreateSession", 0, map[string]dbus.Variant{}).Store(&path); err != nil {
		t.Fatal(err)
	}
	client.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("departed caller retained session")
	}
	p.DisableScreenCast()
}

func TestScreenCastDepartureCancelsPendingStart(t *testing.T) {
	address := startPrivateBus(t)
	p, err := connectPortal(address, PortalHooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	stopped := make(chan uint64, 4)
	if err := p.EnableScreenCast(context.Background(), ScreenCastHooks{
		Start: func(ctx context.Context, _ CastRequest, _ func(uint32), _ func()) error {
			started <- struct{}{}
			<-ctx.Done()
			cancelled <- struct{}{}
			return ctx.Err()
		}, Stop: func(id uint64) { stopped <- id },
	}, 900); err != nil {
		t.Fatal(err)
	}
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	other, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	create := func(conn *dbus.Conn) dbus.ObjectPath {
		t.Helper()
		var path dbus.ObjectPath
		if err := conn.Object(screenCastName, screenCastPath).Call(screenCastName+".CreateSession", 0, map[string]dbus.Variant{}).Store(&path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path, otherPath := create(client), create(other)
	var windows map[uint64]map[string]dbus.Variant
	if err := other.Object(introspectName, introspectPath).Call(introspectName+".GetWindows", 0).Store(&windows); err != nil {
		t.Fatal(err)
	}
	if windows[900]["title"].Value() != "niri Dynamic Cast Target" || windows[900]["app-id"].Value() != "rs.bxt.niri.desktop" {
		t.Fatal("missing dynamic target")
	}
	var stream dbus.ObjectPath
	object := client.Object(screenCastName, path)
	if err := object.Call(screenCastSessionName+".RecordWindow", 0, map[string]dbus.Variant{"window-id": dbus.MakeVariant(uint64(900))}).Store(&stream); err != nil {
		t.Fatal(err)
	}
	call := object.Go(screenCastSessionName+".Start", 0, make(chan *dbus.Call, 1))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pending start missing")
	}
	client.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("caller departure did not cancel start")
	}
	select {
	case id := <-stopped:
		if id != 1 {
			t.Fatalf("stopped wrong caller: %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not stop")
	}
	<-call.Done
	if err := other.Object(screenCastName, otherPath).Call(screenCastSessionName+".Start", 0).Err; err != nil {
		t.Fatalf("other caller's session was cancelled: %v", err)
	}
	p.DisableScreenCast()
	select {
	case id := <-stopped:
		if id != 2 {
			t.Fatalf("disabled wrong session: %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("disable retained other session")
	}
	var present bool
	if err := other.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, screenCastName).Store(&present); err != nil || present {
		t.Fatalf("ScreenCast remains advertised: %v %v", present, err)
	}
	if err := other.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, screenshotName).Store(&present); err != nil || !present {
		t.Fatalf("ScreenCast loss removed screenshot service: %v %v", present, err)
	}
}

func TestScreenCastCannotAdvertiseAfterOwnerLoss(t *testing.T) {
	p, client := testPortal(t, PortalHooks{})
	hooks := ScreenCastHooks{Start: func(context.Context, CastRequest, func(uint32), func()) error { return nil }, Stop: func(uint64) {}}
	lifetime, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.EnableScreenCast(lifetime, hooks, 900); err == nil {
		t.Fatal("late initialization advertised a lost PipeWire connection")
	}
	var present bool
	if err := client.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, screenCastName).Store(&present); err != nil || present {
		t.Fatalf("lost service advertised: %v %v", present, err)
	}
	lifetime, cancel = context.WithCancel(context.Background())
	defer cancel()
	if err := p.EnableScreenCast(lifetime, hooks, 901); err != nil {
		t.Fatal(err)
	}
	cancel()
	for deadline := time.Now().Add(time.Second); ; {
		if err := client.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, screenCastName).Store(&present); err != nil {
			t.Fatal(err)
		}
		if !present {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lifetime cancellation retained service")
		}
		time.Sleep(time.Millisecond)
	}
	var windows map[uint64]map[string]dbus.Variant
	if err := client.Object(introspectName, introspectPath).Call(introspectName+".GetWindows", 0).Store(&windows); err != nil {
		t.Fatal(err)
	}
	if len(windows) != 0 {
		t.Fatalf("lost service retained synthetic targets: %v", windows)
	}
}
