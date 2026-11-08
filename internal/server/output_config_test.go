package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/session"
	"github.com/godbus/dbus/v5"
)

func TestDisplayConfigAndIPCShareTheOutputEngine(t *testing.T) {
	if os.Getenv("SYSC_NESTED_TESTS") != "1" {
		t.Skip("requires consent for this nested run")
	}
	bus := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := bus.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = bus.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { bus.Process.Kill(); bus.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("private bus has no address")
	}
	address := scanner.Text()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	type started struct {
		server *Server
		err    error
	}
	ready := make(chan started, 1)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		s, err := New(config.LoadResult{}, config.Path{}, false)
		if err != nil {
			ready <- started{err: err}
			return
		}
		defer s.Destroy()
		s.StartSessionDBus()
		ready <- started{server: s}
		if err = s.Run(); err != nil {
			t.Error(err)
		}
	}()
	start := <-ready
	if start.err != nil {
		t.Fatal(start.err)
	}
	s := start.server
	watchdog := time.AfterFunc(15*time.Second, s.Terminate)
	t.Cleanup(func() {
		watchdog.Stop()
		s.Terminate()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	object := client.Object("org.gnome.Mutter.DisplayConfig", "/org/gnome/Mutter/DisplayConfig")
	var serial uint32
	var monitors []session.Monitor
	var logical []session.LogicalMonitor
	var properties map[string]dbus.Variant
	read := func() {
		t.Helper()
		if err := object.Call("org.gnome.Mutter.DisplayConfig.GetCurrentState", 0).Store(&serial, &monitors, &logical, &properties); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if len(monitors) != 1 || len(logical) != 1 {
		t.Fatalf("display state = %+v %+v", monitors, logical)
	}
	mode := ""
	for _, candidate := range monitors[0].Modes {
		if property, ok := candidate.Properties["is-current"]; ok && property.Value() == true {
			mode = candidate.ID
			break
		}
	}
	if mode == "" {
		t.Fatal("current mode missing")
	}
	originalSerial, originalScale := serial, logical[0].Scale
	requested := []session.LogicalMonitorConfiguration{{Scale: 1.25, Monitors: []session.MonitorConfiguration{{Connector: monitors[0].Names.Connector, Mode: mode}}}}
	if err := object.Call("org.gnome.Mutter.DisplayConfig.ApplyMonitorsConfig", 0, serial, uint32(0), requested, map[string]dbus.Variant{}).Err; err != nil {
		t.Fatal(err)
	}
	read()
	if serial != originalSerial || logical[0].Scale != originalScale {
		t.Fatal("verify changed the live output")
	}
	if err := object.Call("org.gnome.Mutter.DisplayConfig.ApplyMonitorsConfig", 0, serial, uint32(1), requested, map[string]dbus.Variant{}).Err; err != nil {
		t.Fatal(err)
	}
	read()
	if logical[0].Scale != 1.25 || serial == originalSerial {
		t.Fatalf("D-Bus apply scale=%g serial=%d", logical[0].Scale, serial)
	}
	connection, err := net.Dial("unix", s.NiriSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	request := ipc.Request{Name: "Output", Output: &ipc.OutputRequest{Name: monitors[0].Names.Connector, Action: json.RawMessage(`{"Scale":{"scale":{"Specific":1.5}}}`)}}
	if _, err = fmt.Fprintf(connection, "%s\n", ipc.EncodeRequest(request)); err != nil {
		t.Fatal(err)
	}
	var reply ipc.Reply
	if err = json.NewDecoder(connection).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.Err != "" || string(reply.Ok) != string(ipc.SuccessValue("OutputConfigChanged", "Applied")) {
		t.Fatalf("output reply = %+v", reply)
	}
	read()
	if logical[0].Scale != 1.5 {
		t.Fatalf("IPC scale was not published over D-Bus: %g", logical[0].Scale)
	}
}
