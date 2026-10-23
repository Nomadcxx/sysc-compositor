package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// fakeNiri serves the niri-ipc socket and records what it was asked.
func fakeNiri(t *testing.T) (string, func() []ipc.Request) {
	path := filepath.Join(t.TempDir(), "niri.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []ipc.Request
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				ipc.ServeConn(c, func(r ipc.Request) (ipc.Reply, func()) {
					mu.Lock()
					got = append(got, r)
					mu.Unlock()
					switch r.Name {
					case "Version":
						return ipc.Reply{Ok: ipc.VersionResponse("fake 1")}, nil
					case "ReturnError":
						return ipc.Reply{Err: "example compositor error"}, nil
					case "Outputs":
						return ipc.Reply{Ok: ipc.SuccessValue("Outputs", map[string]ipc.Output{})}, nil
					case "Workspaces":
						return ipc.Reply{Ok: ipc.SuccessValue("Workspaces", []ipc.Workspace{})}, nil
					case "Windows":
						return ipc.Reply{Ok: ipc.SuccessValue("Windows", []ipc.Window{})}, nil
					case "Layers":
						return ipc.Reply{Ok: ipc.SuccessValue("Layers", []ipc.LayerSurface{})}, nil
					case "KeyboardLayouts":
						return ipc.Reply{Ok: ipc.SuccessValue("KeyboardLayouts", ipc.KeyboardLayouts{Names: []string{"English (US)"}})}, nil
					case "FocusedOutput":
						return ipc.Reply{Ok: ipc.SuccessValue("FocusedOutput", (*ipc.Output)(nil))}, nil
					case "FocusedWindow":
						return ipc.Reply{Ok: ipc.SuccessValue("FocusedWindow", (*ipc.Window)(nil))}, nil
					case "PickWindow":
						return ipc.Reply{Ok: ipc.SuccessValue("PickedWindow", (*ipc.Window)(nil))}, nil
					case "PickColor":
						return ipc.Reply{Ok: ipc.SuccessValue("PickedColor", nil)}, nil
					case "OverviewState":
						return ipc.Reply{Ok: ipc.SuccessValue("OverviewState", ipc.OverviewState{})}, nil
					case "Casts":
						return ipc.Reply{Ok: ipc.SuccessValue("Casts", []any{})}, nil
					case "Output":
						result := "Applied"
						if r.Output != nil && r.Output.Name == "missing" {
							result = "OutputWasMissing"
						}
						return ipc.Reply{Ok: ipc.SuccessValue("OutputConfigChanged", result)}, nil
					}
					return ipc.Reply{Ok: ipc.Handled}, nil
				})
			}()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return path, func() []ipc.Request {
		mu.Lock()
		defer mu.Unlock()
		return append([]ipc.Request(nil), got...)
	}
}

func runMsg(socket string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	env := Env{Getenv: func(k string) string {
		if k == "NIRI_SOCKET" {
			return socket
		}
		return ""
	}, Stdout: &out, Stderr: &errOut}
	code := Main("niri", append([]string{"msg"}, args...), env)
	return code, out.String(), errOut.String()
}

func TestParseSessionStartupOptions(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		config  string
		session bool
	}{
		{args: []string{}},
		{args: []string{"--session"}, session: true},
		{args: []string{"-c", "/tmp/niri.kdl", "--session"}, config: "/tmp/niri.kdl", session: true},
		{args: []string{"--session", "--config", "/tmp/other.kdl"}, config: "/tmp/other.kdl", session: true},
	} {
		got, err := parseRunOptions(tc.args)
		if err != nil {
			t.Fatalf("parseRunOptions(%v): %v", tc.args, err)
		}
		if got.configPath != tc.config || got.session != tc.session {
			t.Errorf("parseRunOptions(%v) = %+v, want config %q, session %v", tc.args, got, tc.config, tc.session)
		}
	}
	if _, err := parseRunOptions([]string{"--session", "--unknown"}); err == nil {
		t.Fatal("parseRunOptions accepted an unknown startup option")
	}
}

// The first two CLI tests design §6 names: sysc-greet's quit and
// sysc-launch's spawn.
func TestMsgSendsWhatNiriMsgSends(t *testing.T) {
	socket, requests := fakeNiri(t)
	for _, args := range [][]string{
		{"action", "quit", "--skip-confirmation"},
		{"action", "spawn", "--", "kitty", "--hold"},
		{"action", "spawn-sh", "--", "echo a; echo b"},
		{"action", "quit"},
	} {
		if code, _, stderr := runMsg(socket, args...); code != 0 {
			t.Errorf("%v: exit %d: %s", args, code, stderr)
		}
	}
	want := []ipc.Request{
		{Name: "Action", Action: &ipc.Action{Name: "Quit", SkipConfirmation: true}},
		{Name: "Action", Action: &ipc.Action{Name: "Spawn", Command: []string{"kitty", "--hold"}}},
		{Name: "Action", Action: &ipc.Action{Name: "SpawnSh", ShellCommand: "echo a; echo b"}},
		{Name: "Action", Action: &ipc.Action{Name: "Quit"}},
	}
	if got := requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("server saw %d requests:\n%+v\nwant %+v", len(got), got, want)
	}
}

func TestMsgSendsM4WindowActions(t *testing.T) {
	socket, requests := fakeNiri(t)
	for _, args := range [][]string{
		{"action", "focus-window", "--id", "42"},
		{"action", "close-window", "--id", "42"},
		{"action", "close-window"},
		{"action", "focus-workspace", "2"},
		{"action", "focus-workspace", "main"},
	} {
		if code, _, stderr := runMsg(socket, args...); code != 0 {
			t.Errorf("%v: exit %d: %s", args, code, stderr)
		}
	}
	focusID, closeID, workspaceIndex, workspaceName := uint64(42), uint64(42), uint8(2), "main"
	want := []ipc.Request{
		{Name: "Action", Action: &ipc.Action{Name: "FocusWindow", WindowID: &focusID}},
		{Name: "Action", Action: &ipc.Action{Name: "CloseWindow", WindowID: &closeID}},
		{Name: "Action", Action: &ipc.Action{Name: "CloseWindow"}},
		{Name: "Action", Action: &ipc.Action{Name: "FocusWorkspace", Workspace: &ipc.WorkspaceReference{Index: &workspaceIndex}}},
		{Name: "Action", Action: &ipc.Action{Name: "FocusWorkspace", Workspace: &ipc.WorkspaceReference{Name: &workspaceName}}},
	}
	if got := requests(); !reflect.DeepEqual(got, want) {
		t.Fatalf("server saw %+v, want %+v", got, want)
	}
}

func TestMsgVersionAndErrors(t *testing.T) {
	socket, _ := fakeNiri(t)
	if code, out, _ := runMsg(socket, "--json", "version"); code != 0 || !strings.Contains(out, `"compositor":"fake 1"`) {
		t.Errorf("version --json: exit %d, %q", code, out)
	}
	if code, _, stderr := runMsg(socket, "request-error"); code != 1 || !strings.Contains(stderr, "example compositor error") {
		t.Errorf("request-error: exit %d, %q", code, stderr)
	}
	if code, _, stderr := runMsg("", "version"); code != 1 || !strings.Contains(stderr, "NIRI_SOCKET is not set") {
		t.Errorf("no socket: exit %d, %q", code, stderr)
	}
	for _, args := range [][]string{
		{"outputs", "unexpected"}, {"action", "focus-workspace"}, {"action", "focus-window"}, {"action", "spawn", "kitty"},
		{"action", "focus-window", "--id", "bad"}, {"action", "focus-window", "--id", "42", "extra"},
		{"action", "close-window", "--id", "bad"}, {"action", "focus-workspace", "256"},
		{"action", "spawn-sh", "--", "a", "b"}, {"action", "quit", "--now"}, {},
	} {
		if code, _, _ := runMsg(socket, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2 (usage)", args, code)
		}
	}
}

func TestMsgReportsVersionMismatch(t *testing.T) {
	socket, _ := fakeNiri(t)
	if code, _, stderr := runMsg(socket, "version"); code != 0 || !strings.Contains(stderr, "different version") {
		t.Errorf("version mismatch: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runMsg(socket, "request-error"); code != 1 ||
		!strings.Contains(stderr, "different version") || !strings.Contains(stderr, "example compositor error") {
		t.Errorf("request error mismatch: exit %d, stderr %q", code, stderr)
	}
}

func TestMsgSupportsStateQueries(t *testing.T) {
	socket, requests := fakeNiri(t)
	queries := []struct{ command, request string }{
		{"outputs", "Outputs"},
		{"workspaces", "Workspaces"},
		{"windows", "Windows"},
		{"layers", "Layers"},
		{"focused-output", "FocusedOutput"},
	}
	for _, query := range queries {
		code, out, stderr := runMsg(socket, "--json", query.command)
		if code != 0 || !json.Valid([]byte(strings.TrimSpace(out))) {
			t.Errorf("msg --json %s: exit %d, stdout %q, stderr %q", query.command, code, out, stderr)
		}
	}
	got := requests()
	if len(got) != len(queries) {
		t.Fatalf("sent %d requests, want %d: %+v", len(got), len(queries), got)
	}
	for i, query := range queries {
		if got[i].Name != query.request {
			t.Errorf("request %d = %s, want %s", i, got[i].Name, query.request)
		}
	}
}

func TestParseMsgSupportsAllNiriRequests(t *testing.T) {
	tests := []struct {
		args []string
		name string
	}{
		{[]string{"version"}, "Version"},
		{[]string{"outputs"}, "Outputs"},
		{[]string{"workspaces"}, "Workspaces"},
		{[]string{"windows"}, "Windows"},
		{[]string{"layers"}, "Layers"},
		{[]string{"keyboard-layouts"}, "KeyboardLayouts"},
		{[]string{"focused-output"}, "FocusedOutput"},
		{[]string{"focused-window"}, "FocusedWindow"},
		{[]string{"pick-window"}, "PickWindow"},
		{[]string{"pick-color"}, "PickColor"},
		{[]string{"action", "quit"}, "Action"},
		{[]string{"output", "eDP-1", "off"}, "Output"},
		{[]string{"event-stream"}, "EventStream"},
		{[]string{"request-error"}, "ReturnError"},
		{[]string{"overview-state"}, "OverviewState"},
		{[]string{"casts"}, "Casts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := parseMsg(test.args)
			if err != nil {
				t.Fatalf("parseMsg(%v): %v", test.args, err)
			}
			if request.Name != test.name {
				t.Fatalf("parseMsg(%v).Name = %q, want %q", test.args, request.Name, test.name)
			}
		})
	}
}

func TestParseMsgSupportsM9Actions(t *testing.T) {
	for _, args := range [][]string{
		{"action", "switch-layout", "next"},
		{"action", "switch-layout", "prev"},
		{"action", "switch-layout", "2"},
		{"action", "load-config-file"},
		{"action", "load-config-file", "--path", "/tmp/niri.kdl"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			request, err := parseMsg(args)
			if err != nil {
				t.Fatalf("parseMsg(%v): %v", args, err)
			}
			if request.Name != "Action" || request.Action == nil {
				t.Fatalf("parseMsg(%v) = %+v, want Action", args, request)
			}
		})
	}
}

func TestParseOutputMatchesNiriWire(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"output", "DP-1", "off"}, `"Off"`},
		{[]string{"output", "DP-1", "on"}, `"On"`},
		{[]string{"output", "DP-1", "mode", "auto"}, `{"Mode":{"mode":"Automatic"}}`},
		{[]string{"output", "DP-1", "mode", "1920x1080@60"}, `{"Mode":{"mode":{"Specific":{"width":1920,"height":1080,"refresh":60}}}}`},
		{[]string{"output", "DP-1", "custom-mode", "1280x720"}, `{"CustomMode":{"mode":{"width":1280,"height":720,"refresh":null}}}`},
		{[]string{"output", "DP-1", "modeline", "148.5", "1920", "2008", "2052", "2200", "1080", "1084", "1089", "1125", "+hsync", "+vsync"}, `{"Modeline":{"clock":148.5,"hdisplay":1920,"hsync_start":2008,"hsync_end":2052,"htotal":2200,"vdisplay":1080,"vsync_start":1084,"vsync_end":1089,"vtotal":1125,"hsync_polarity":"PHSync","vsync_polarity":"PVSync"}}`},
		{[]string{"output", "DP-1", "scale", "1.5"}, `{"Scale":{"scale":{"Specific":1.5}}}`},
		{[]string{"output", "DP-1", "transform", "flipped-90"}, `{"Transform":{"transform":"Flipped90"}}`},
		{[]string{"output", "DP-1", "position", "auto"}, `{"Position":{"position":"Automatic"}}`},
		{[]string{"output", "DP-1", "position", "set", "10", "-20"}, `{"Position":{"position":{"Specific":{"x":10,"y":-20}}}}`},
		{[]string{"output", "DP-1", "vrr", "on", "--on-demand"}, `{"Vrr":{"vrr":true,"on_demand":true}}`},
		{[]string{"output", "DP-1", "vrr", "y"}, `{"Vrr":{"vrr":true,"on_demand":false}}`},
		{[]string{"output", "DP-1", "vrr", "N"}, `{"Vrr":{"vrr":false,"on_demand":false}}`},
		{[]string{"output", "DP-1", "vrr", "T"}, `{"Vrr":{"vrr":true,"on_demand":false}}`},
		{[]string{"output", "DP-1", "vrr", "f"}, `{"Vrr":{"vrr":false,"on_demand":false}}`},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args[2:], "_"), func(t *testing.T) {
			request, err := parseMsg(test.args)
			if err != nil {
				t.Fatalf("parseMsg(%v): %v", test.args, err)
			}
			if request.Output == nil {
				t.Fatalf("parseMsg(%v) returned no output request", test.args)
			}
			if _, err := ipc.DecodeRequest(ipc.EncodeRequest(request)); err != nil {
				t.Fatalf("output request is not valid niri-ipc: %v", err)
			}
			var gotValue, wantValue any
			if err := json.Unmarshal(request.Output.Action, &gotValue); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.want), &wantValue); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotValue, wantValue) {
				t.Fatalf("output action = %s, want %s", request.Output.Action, test.want)
			}
		})
	}
}

func TestMsgSendsEveryNonStreamingRequest(t *testing.T) {
	socket, requests := fakeNiri(t)
	queries := []struct {
		args []string
		name string
	}{
		{[]string{"outputs"}, "Outputs"},
		{[]string{"workspaces"}, "Workspaces"},
		{[]string{"windows"}, "Windows"},
		{[]string{"layers"}, "Layers"},
		{[]string{"keyboard-layouts"}, "KeyboardLayouts"},
		{[]string{"focused-output"}, "FocusedOutput"},
		{[]string{"focused-window"}, "FocusedWindow"},
		{[]string{"pick-window"}, "PickWindow"},
		{[]string{"pick-color"}, "PickColor"},
		{[]string{"action", "quit"}, "Action"},
		{[]string{"output", "DP-1", "off"}, "Output"},
		{[]string{"overview-state"}, "OverviewState"},
		{[]string{"casts"}, "Casts"},
	}
	for _, query := range queries {
		args := append([]string{"--json"}, query.args...)
		code, stdout, stderr := runMsg(socket, args...)
		if code != 0 || stderr != "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", query.args, code, stdout, stderr)
		}
		if query.name != "Action" && !json.Valid([]byte(strings.TrimSpace(stdout))) {
			t.Errorf("%v: stdout is not JSON: %q", query.args, stdout)
		}
	}
	got := requests()
	if len(got) != len(queries) {
		t.Fatalf("sent %d requests, want %d: %+v", len(got), len(queries), got)
	}
	for i, query := range queries {
		if got[i].Name != query.name {
			t.Errorf("request %d = %s, want %s", i, got[i].Name, query.name)
		}
	}
}

func TestMsgOutputUsesNiriResponseFormatting(t *testing.T) {
	socket, _ := fakeNiri(t)

	if code, stdout, stderr := runMsg(socket, "output", "DP-1", "off"); code != 0 || stdout != "" || stderr != "" {
		t.Errorf("applied output: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	const missing = "Output \"missing\" is not connected.\nThe change will apply when it is connected.\n"
	if code, stdout, stderr := runMsg(socket, "output", "missing", "off"); code != 0 || stdout != missing || stderr != "" {
		t.Errorf("missing output: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	if code, stdout, stderr := runMsg(socket, "--json", "output", "missing", "off"); code != 0 || stdout != "\"OutputWasMissing\"\n" || stderr != "" {
		t.Errorf("JSON missing output: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestMsgEventStreamKeepsTheConnectionOpenForEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "niri.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	requests := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		request, _ := bufio.NewReader(conn).ReadString('\n')
		requests <- request
		_, _ = conn.Write([]byte(`{"Ok":"Handled"}` + "\n" + `{"WorkspacesChanged":{"workspaces":[]}}` + "\n"))
	}()
	var stdout, stderr bytes.Buffer
	env := Env{
		Getenv: func(k string) string {
			if k == "NIRI_SOCKET" {
				return path
			}
			return ""
		},
		Stdout: &stdout, Stderr: &stderr,
	}
	code := Main("niri", []string{"msg", "--json", "event-stream"}, env)
	if code != 1 || stdout.String() != `{"WorkspacesChanged":{"workspaces":[]}}`+"\n" || !strings.Contains(stderr.String(), "reading event") {
		t.Fatalf("event-stream exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if got := <-requests; got != `"EventStream"`+"\n" {
		t.Errorf("request = %q", got)
	}
	<-done
}

func TestValidateParsesConfigWithoutRunningStartup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.kdl")
	marker := filepath.Join(dir, "startup-ran")
	// toggle-debug-tint is a niri action sysc does not apply yet (M15).
	// close-window and screenshot, which this used, are applied since M4 and
	// M11.
	source := fmt.Sprintf("spawn-sh-at-startup %q\nbinds {\n    Mod+Q { close-window; }\n    Mod+T { toggle-debug-tint; }\n}\n", "touch "+marker)
	if err := os.WriteFile(configPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	env := Env{
		Getenv: func(string) string { return "" }, Stdout: &stdout, Stderr: &stderr,
	}
	if code := Main("niri", []string{"validate", "-c", configPath}, env); code != 0 {
		t.Fatalf("validate exit %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("startup command ran during validation (stat error %v)", err)
	}
	if !strings.Contains(stdout.String(), "valid") {
		t.Errorf("validate output %q does not confirm the config", stdout.String())
	}
	if strings.Contains(stderr.String(), "section `binds`") {
		t.Errorf("validate warnings %q call the applied binds section unapplied", stderr.String())
	}
	if !strings.Contains(stderr.String(), "bind actions are accepted but not applied yet: toggle-debug-tint\n") {
		t.Errorf("validate warnings %q do not identify the unapplied bind action", stderr.String())
	}
}

func TestUsageLeadsWithSyscCompositorAndNamesTheNiriAlias(t *testing.T) {
	for _, name := range []string{"sysc-compositor", "niri"} {
		var stderr bytes.Buffer
		env := Env{Getenv: func(string) string { return "" }, Stdout: &bytes.Buffer{}, Stderr: &stderr}
		if code := Main(name, []string{"--no-such-flag"}, env); code != 2 {
			t.Fatalf("%s: exit %d, want 2 (usage)", name, code)
		}
		out := stderr.String()
		if !strings.Contains(out, "usage: sysc-compositor [--session] [-c CONFIG] | sysc-compositor validate [-c CONFIG] | sysc-compositor msg [--json] REQUEST\n") {
			t.Errorf("%s usage %q does not lead with sysc-compositor", name, out)
		}
		alias := strings.Contains(out, "niri is a compatibility alias of sysc-compositor")
		if alias != (name == "niri") {
			t.Errorf("%s usage %q: alias note present = %v", name, out, alias)
		}
	}
}
