package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/unixsock"
)

func TestEventStreamInitialStateOrderAndUpdates(t *testing.T) {
	state := newNiriState(false)
	dp := ipc.Output{Name: "DP-1"}
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": dp}, focusedOutput: &dp,
		keyboardLayouts: &ipc.KeyboardLayouts{Names: []string{"English (US)", "German"}, CurrentIdx: 1},
	})
	hub := newEventHub()
	h := niriHandler{
		snapshot: state.snapshot,
		stream:   func(conn net.Conn) { state.serveEventStream(hub, conn) },
	}
	client := serveUnixNiri(t, h)
	defer client.Close()
	if _, err := client.Write([]byte(`"EventStream"` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(client)
	if got, err := reader.ReadString('\n'); err != nil || got != `{"Ok":"Handled"}`+"\n" {
		t.Fatalf("handshake = %q, %v", got, err)
	}
	want := []string{
		`{"WorkspacesChanged":{"workspaces":[{"id":1,"idx":1,"name":null,"output":"DP-1","is_urgent":false,"is_active":true,"is_focused":true,"active_window_id":null}]}}`,
		`{"WindowsChanged":{"windows":[]}}`,
		`{"KeyboardLayoutsChanged":{"keyboard_layouts":{"names":["English (US)","German"],"current_idx":1}}}`,
		`{"OverviewOpenedOrClosed":{"is_open":false}}`,
		`{"ConfigLoaded":{"failed":false}}`,
		`{"CastsChanged":{"casts":[]}}`,
	}
	for _, line := range want {
		if got, err := reader.ReadString('\n'); err != nil || got != line+"\n" {
			t.Fatalf("event = %q, %v; want %q", got, err, line+"\n")
		}
	}

	before := state.snapshot()
	hdmi := ipc.Output{Name: "HDMI-A-1"}
	focused := hdmi
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": dp, "HDMI-A-1": hdmi}, focusedOutput: &focused,
		keyboardLayouts: &ipc.KeyboardLayouts{Names: []string{"English (US)", "German"}, CurrentIdx: 1},
	})
	hub.publish(eventStreamChanges(before, state.snapshot()))
	wantUpdate := `{"WorkspacesChanged":{"workspaces":[` +
		`{"id":1,"idx":1,"name":null,"output":"DP-1","is_urgent":false,"is_active":true,"is_focused":false,"active_window_id":null},` +
		`{"id":2,"idx":1,"name":null,"output":"HDMI-A-1","is_urgent":false,"is_active":true,"is_focused":true,"active_window_id":null}]}}` + "\n"
	if got, err := reader.ReadString('\n'); err != nil || got != wantUpdate {
		t.Fatalf("update = %q, %v; want %q", got, err, wantUpdate)
	}
	before = state.snapshot()
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": dp, "HDMI-A-1": hdmi}, focusedOutput: &focused,
		keyboardLayouts: &ipc.KeyboardLayouts{Names: []string{"English (US)", "German"}, CurrentIdx: 0},
	})
	hub.publish(eventStreamChanges(before, state.snapshot()))
	if got, err := reader.ReadString('\n'); err != nil || got != `{"KeyboardLayoutSwitched":{"idx":0}}`+"\n" {
		t.Fatalf("keyboard update = %q, %v", got, err)
	}
}

func TestEventStreamReportsRejectedStartupConfig(t *testing.T) {
	state := newNiriState(true)
	state.replace(niriSnapshot{})
	lines := initialEventLines(state.snapshot())
	var event struct {
		ConfigLoaded struct {
			Failed bool `json:"failed"`
		} `json:"ConfigLoaded"`
	}
	for _, line := range lines {
		if strings.Contains(string(line), `"ConfigLoaded"`) {
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatal(err)
			}
			if !event.ConfigLoaded.Failed {
				t.Fatal("ConfigLoaded.failed = false after startup rejected the config")
			}
			return
		}
	}
	t.Fatal("initial EventStream state omitted ConfigLoaded")
}

func TestEventStreamUsesWorkspaceActivatedForFocusChanges(t *testing.T) {
	state := newNiriState(false)
	dp := ipc.Output{Name: "DP-1"}
	hdmi := ipc.Output{Name: "HDMI-A-1"}
	focused := dp
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": dp, "HDMI-A-1": hdmi}, focusedOutput: &focused,
	})
	before := state.snapshot()
	focused = hdmi
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": dp, "HDMI-A-1": hdmi}, focusedOutput: &focused,
	})

	got := eventStreamChanges(before, state.snapshot())
	want := [][]byte{eventLine("WorkspaceActivated", struct {
		ID      uint64 `json:"id"`
		Focused bool   `json:"focused"`
	}{ID: 2, Focused: true})}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("focus change events = %q, want %q", got, want)
	}
}

func TestDestroyStopsActiveEventStream(t *testing.T) {
	state := newNiriState(false)
	hub := newEventHub()
	listener, err := unixsock.Listen(filepath.Join(t.TempDir(), "niri.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{niri: listener, events: hub}
	h := niriHandler{stream: func(conn net.Conn) { state.serveEventStream(hub, conn) }}
	go listener.Serve(h.serve)
	client, err := net.Dial("unix", listener.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte(`"EventStream"` + "\n")); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(client)
	if got, err := reader.ReadString('\n'); err != nil || got != `{"Ok":"Handled"}`+"\n" {
		t.Fatalf("handshake = %q, %v", got, err)
	}
	for i, want := range initialEventNames {
		line, err := reader.ReadString('\n')
		if err != nil || !strings.Contains(line, `"`+want+`"`) {
			t.Fatalf("initial event %d = %q, %v; want %s", i, line, err, want)
		}
	}
	waitFor(t, func() bool { return hub.clientCount() == 1 })

	done := make(chan struct{})
	go func() {
		server.Destroy()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		_ = client.Close()
		hub.publish([][]byte{eventLine("WorkspacesChanged", struct {
			Workspaces []ipc.Workspace `json:"workspaces"`
		}{Workspaces: []ipc.Workspace{}})})
		<-done
		t.Fatal("Destroy did not stop the active event stream")
	}
}

func TestEventStreamDisconnectsSlowClientAndReconnectsWithFullState(t *testing.T) {
	state := newNiriState(false)
	dp := ipc.Output{Name: "DP-1"}
	state.replace(niriSnapshot{outputs: map[string]ipc.Output{"DP-1": dp}, focusedOutput: &dp})
	hub := newEventHub()

	slowServer, slowClient := net.Pipe()
	defer slowClient.Close()
	go state.serveEventStream(hub, slowServer)
	waitFor(t, func() bool { return hub.clientCount() == 1 })
	line := []byte(`{"OverviewOpenedOrClosed":{"is_open":false}}` + "\n")
	burst := make([][]byte, eventStreamBufferSize+1)
	for i := range burst {
		burst[i] = line
	}
	hub.publish(burst)
	waitFor(t, func() bool { return hub.clientCount() == 0 })
	_ = slowClient.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.Copy(io.Discard, slowClient); err != nil {
		t.Fatalf("reading the disconnected slow client: %v", err)
	}

	server, client := net.Pipe()
	defer client.Close()
	go state.serveEventStream(hub, server)
	reader := bufio.NewReader(client)
	for i, want := range initialEventNames {
		line, err := reader.ReadString('\n')
		if err != nil || !strings.Contains(line, `"`+want+`"`) {
			t.Fatalf("reconnect event %d = %q, %v; want %s", i, line, err, want)
		}
	}
}

var initialEventNames = []string{
	"WorkspacesChanged", "WindowsChanged", "OverviewOpenedOrClosed", "ConfigLoaded", "CastsChanged",
}

func serveUnixNiri(t *testing.T, h niriHandler) *net.UnixConn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "niri.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.AcceptUnix()
		if err == nil {
			h.serve(conn)
		}
	}()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = listener.Close()
		}
	})
	return client
}

func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met")
}
