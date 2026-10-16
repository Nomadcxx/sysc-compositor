package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

func TestNiriHandler(t *testing.T) {
	quits := 0
	h := niriHandler{quit: func() { quits++ }, spawn: newSpawner("w", "", nil)}
	for _, tc := range []struct {
		line, want string
		after      bool
	}{
		{`"ReturnError"`, `{"Err":"example compositor error"}`, false},
		{`"Outputs"`, `{"Err":"unimplemented: Outputs"}`, false},
		{`{"Action":{"ToggleDebugTint":{}}}`, `{"Err":"unimplemented: ToggleDebugTint"}`, false},
		{`{"Action":{"Spawn":{"command":["true"]}}}`, `{"Ok":"Handled"}`, false},
		{`{"Action":{"SpawnSh":{"command":"true"}}}`, `{"Ok":"Handled"}`, false},
		{`{"Action":{"Quit":{"skip_confirmation":true}}}`, `{"Ok":"Handled"}`, true},
	} {
		r, err := ipc.DecodeRequest([]byte(tc.line))
		if err != nil {
			t.Fatalf("%s: %v", tc.line, err)
		}
		reply, after := h.handle(r)
		out, _ := json.Marshal(reply)
		if string(out) != tc.want || (after != nil) != tc.after {
			t.Errorf("%s: reply %s (after hook %v), want %s (%v)", tc.line, out, after != nil, tc.want, tc.after)
		}
		if after != nil {
			after()
		}
	}
	if quits != 1 {
		t.Errorf("quit ran %d times, want once, after its reply", quits)
	}
	reply, _ := h.handle(ipc.Request{Name: "Version"})
	if !strings.Contains(string(reply.Ok), "(niri-ipc 26.4.0)") {
		t.Errorf("Version reply %s does not name the wire", reply.Ok)
	}
}

func TestNiriSocketPath(t *testing.T) {
	got, err := niriSocketPath("/run/user/1000", "wayland-1", 42)
	if err != nil || got != "/run/user/1000/niri.wayland-1.42.sock" {
		t.Fatalf("niriSocketPath = %q, %v", got, err)
	}
	if _, err := niriSocketPath("relative", "wayland-1", 42); err == nil {
		t.Fatal("a relative runtime dir was accepted")
	}
}
