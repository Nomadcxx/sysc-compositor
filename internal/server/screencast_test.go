package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/session"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
	"github.com/godbus/dbus/v5"
)

func TestScreenCastProducesFramesAndSurvivesPipeWireLoss(t *testing.T) {
	if os.Getenv("SYSC_NESTED_TESTS") != "1" {
		t.Skip("requires consent for this nested run")
	}
	bus := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	out, err := bus.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { bus.Process.Kill(); bus.Wait() }()
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() {
		t.Fatal("private bus has no address")
	}
	address := scanner.Text()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	dir := t.TempDir()
	t.Setenv("PIPEWIRE_RUNTIME_DIR", dir)
	t.Setenv("PIPEWIRE_REMOTE", "pipewire-0")
	pwConfig := filepath.Join(dir, "pipewire.conf")
	if err := os.WriteFile(pwConfig, []byte(`
context.properties = { core.daemon = true core.name = pipewire-0 }
context.spa-libs = { support.* = support/libspa-support }
context.modules = [
 { name = libpipewire-module-protocol-native }
 { name = libpipewire-module-client-node }
 { name = libpipewire-module-spa-node-factory }
 { name = libpipewire-module-adapter }
 { name = libpipewire-module-link-factory }
 { name = libpipewire-module-access args = { access.force = unrestricted } }
]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	daemon := exec.Command("pipewire", "-c", pwConfig)
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { daemon.Process.Kill(); daemon.Wait() }()
	for deadline := time.Now().Add(3 * time.Second); ; {
		if _, err := os.Stat(filepath.Join(dir, "pipewire-0")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("private PipeWire socket missing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	type startup struct {
		s   *Server
		err error
	}
	ready := make(chan startup, 1)
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		cfg := config.Default()
		cfg.Animations.Off = true
		s, err := New(config.LoadResult{Config: cfg}, config.Path{}, false)
		if err != nil {
			ready <- startup{err: err}
			return
		}
		portal, err := session.StartPortal(session.PortalHooks{})
		if err != nil {
			s.Destroy()
			ready <- startup{err: err}
			return
		}
		s.portal = portal
		s.refreshNiriState()
		s.startPortalScreenCast(portal)
		ready <- startup{s: s}
		err = s.Run()
		s.Destroy()
		done <- err
	}()
	result := <-ready
	if result.err != nil {
		t.Fatal(result.err)
	}
	s := result.s
	defer func() {
		s.Terminate()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	}()
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	const name = "org.gnome.Mutter.ScreenCast"
	const rootPath dbus.ObjectPath = "/org/gnome/Mutter/ScreenCast"
	wait := func(check func() bool, message string) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); !check(); {
			if time.Now().After(deadline) {
				t.Fatal(message)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	ownsName := func(name string) bool {
		var owned bool
		_ = client.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned)
		return owned
	}
	wait(func() bool { return ownsName(name) }, "ScreenCast not advertised after PipeWire initialization")
	window, err := testclient.StartToplevelClient(s.c.SocketName(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer window.Close()
	if err := window.Paint(0xff3498db, 64, 32); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return len(s.state.snapshot().windows) == 1 }, "native window not published")
	id := s.state.snapshot().windows[0].ID
	var windows map[uint64]map[string]dbus.Variant
	windows = nil // godbus Store merges maps; request a fresh snapshot.
	if err := client.Object("org.gnome.Shell.Introspect", "/org/gnome/Shell/Introspect").Call("org.gnome.Shell.Introspect.GetWindows", 0).Store(&windows); err != nil {
		t.Fatal(err)
	}
	var dynamic uint64
	for id, window := range windows {
		if window["title"].Value() == "niri Dynamic Cast Target" {
			dynamic = id
		}
	}
	if dynamic == 0 || dynamic == id {
		t.Fatal("synthetic cast target missing or aliases real mapping")
	}
	var path, stream dbus.ObjectPath
	if err := client.Object(name, rootPath).Call(name+".CreateSession", 0, map[string]dbus.Variant{}).Store(&path); err != nil {
		t.Fatal(err)
	}
	object := client.Object(name, path)
	if err := object.Call(name+".Session.RecordWindow", 0, map[string]dbus.Variant{"window-id": dbus.MakeVariant(dynamic)}).Store(&stream); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 16)
	client.Signal(signals)
	defer client.RemoveSignal(signals)
	if err := client.AddMatchSignal(dbus.WithMatchInterface(name + ".Stream")); err != nil {
		t.Fatal(err)
	}
	if err := object.Call(name+".Session.Start", 0).Err; err != nil {
		t.Fatal(err)
	}
	casts := s.state.snapshot().casts
	if len(casts) != 1 || casts[0].PWNodeID != nil || !casts[0].IsDynamicTarget {
		t.Fatalf("pending cast=%+v", casts)
	}
	conn, err := net.DialTimeout("unix", s.NiriSocket(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = fmt.Fprintf(conn, "%s\n", ipc.EncodeRequest(ipc.Request{Name: "Action", Action: &ipc.Action{Name: "SetDynamicCastWindow", WindowID: &id}}))
	if err == nil {
		_, err = bufio.NewReader(conn).ReadString('\n')
	}
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	var node uint32
	select {
	case signal := <-signals:
		if signal.Name != name+".Stream.PipeWireStreamAdded" || signal.Path != stream {
			t.Fatalf("unexpected stream signal: %+v", signal)
		}
		node = signal.Body[0].(uint32)
	case <-time.After(3 * time.Second):
		t.Fatal("no PipeWire node signal")
	}
	consumer, err := testclient.NewPipeWireReceiver(node, 64, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	frame, err := consumer.NextFrame(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width != 64 || frame.Height != 32 || [4]byte(frame.Pixels[:4]) != [4]byte{0x34, 0x98, 0xdb, 255} {
		t.Fatalf("wrong frame: %dx%d", frame.Width, frame.Height)
	}
	legacy, err := testclient.StartLegacyCapture(s.c.SocketName(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if legacy != nil {
			legacy.Close()
		}
	}()
	if err := legacy.ArmLegacy(true, false); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Sync(); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return len(s.state.snapshot().casts) == 2 }, "legacy cast not published alongside PipeWire")
	mixed := s.state.snapshot().casts
	if mixed[0].StreamID == mixed[1].StreamID || mixed[0].SessionID == mixed[1].SessionID {
		t.Fatal("cast producers share identity")
	}
	var pwID, legacyID uint64
	for _, cast := range mixed {
		if cast.Kind == ipc.CastKindPipeWire {
			pwID = cast.StreamID
		}
		if cast.Kind == ipc.CastKindWlrScreencopy && cast.PID != nil && *cast.PID == int32(os.Getpid()) {
			legacyID = cast.StreamID
		}
	}
	if pwID == 0 || legacyID == 0 {
		t.Fatalf("mixed cast state=%+v", mixed)
	}
	events, err := net.DialTimeout("unix", s.NiriSocket(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	_ = events.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := fmt.Fprintln(events, `"EventStream"`); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(events)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, `"CastsChanged"`) {
			var baseline struct {
				CastsChanged struct {
					Casts []ipc.Cast `json:"casts"`
				}
			}
			if err := json.Unmarshal([]byte(line), &baseline); err != nil {
				t.Fatal(err)
			}
			if len(baseline.CastsChanged.Casts) != 2 {
				t.Fatalf("mixed baseline=%s", line)
			}
			break
		}
	}
	_ = daemon.Process.Kill()
	wait(func() bool { return !ownsName(name) && len(s.state.snapshot().casts) == 1 }, "PipeWire loss did not isolate legacy activity")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, `"CastStopped"`) {
			var event struct {
				CastStopped struct {
					StreamID uint64 `json:"stream_id"`
				}
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatal(err)
			}
			if event.CastStopped.StreamID != pwID {
				t.Fatalf("daemon loss stopped wrong stream: %s", line)
			}
			break
		}
	}
	if remaining := s.state.snapshot().casts; remaining[0].StreamID != legacyID {
		t.Fatal("PipeWire loss removed legacy cast")
	}
	legacy.Close()
	legacy = nil
	wait(func() bool { return len(s.state.snapshot().casts) == 0 }, "legacy departure retained cast")
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, fmt.Sprintf(`"CastStopped":{"stream_id":%d}`, legacyID)) {
		t.Fatalf("legacy stop event=%s: %v", line, err)
	}
	if !ownsName("org.gnome.Shell.Screenshot") {
		t.Fatal("PipeWire loss removed other session services")
	}
	windows = nil // Store merges maps instead of removing retired keys.
	if err := client.Object("org.gnome.Shell.Introspect", "/org/gnome/Shell/Introspect").Call("org.gnome.Shell.Introspect.GetWindows", 0).Store(&windows); err != nil {
		t.Fatal(err)
	}
	if _, exists := windows[dynamic]; exists {
		t.Fatal("PipeWire loss retained synthetic portal window")
	}
}
