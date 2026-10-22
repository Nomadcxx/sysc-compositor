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
					case "FocusedOutput":
						return ipc.Reply{Ok: ipc.SuccessValue("FocusedOutput", (*ipc.Output)(nil))}, nil
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
	// screenshot is a niri action sysc does not apply yet (M11). close-window,
	// which this used, has been applied since M4.
	source := fmt.Sprintf("spawn-sh-at-startup %q\nbinds {\n    Mod+Q { close-window; }\n    Print { screenshot; }\n}\n", "touch "+marker)
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
	if !strings.Contains(stderr.String(), "binds") {
		t.Errorf("validate warnings %q do not mention the accepted, unapplied binds section", stderr.String())
	}
	if !strings.Contains(stderr.String(), "bind actions are accepted but not applied yet: screenshot\n") {
		t.Errorf("validate warnings %q do not identify the unapplied bind action", stderr.String())
	}
}
