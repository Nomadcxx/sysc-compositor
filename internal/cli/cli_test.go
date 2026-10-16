package cli

import (
	"bytes"
	"net"
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
		{"windows"}, {"action", "focus-workspace", "2"}, {"action", "spawn", "kitty"},
		{"action", "spawn-sh", "--", "a", "b"}, {"action", "quit", "--now"}, {},
	} {
		if code, _, _ := runMsg(socket, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2 (usage)", args, code)
		}
	}
}
