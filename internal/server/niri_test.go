package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

func TestNiriOutputSnapshotConversion(t *testing.T) {
	serial := "123"
	physical := [2]uint32{310, 170}
	current := uint32(1)
	logical := &wlr.LogicalOutputSnapshot{
		X: -1920, Y: 0, Width: 1920, Height: 1080, Scale: 1.25, Transform: 1,
	}
	got := toNiriOutput(wlr.OutputSnapshot{
		Name: "DP-1", Make: "Acme", Model: "Panel", Serial: &serial,
		PhysicalSize: &physical,
		Modes: []wlr.OutputModeSnapshot{
			{Width: 1280, Height: 720, RefreshRate: 60_000},
			{Width: 1920, Height: 1080, RefreshRate: 144_000, Preferred: true},
		},
		CurrentMode: &current, VRRSupported: true, VRREnabled: true, Logical: logical,
	})
	want := ipc.Output{
		Name: "DP-1", Make: "Acme", Model: "Panel", Serial: &serial, PhysicalSize: &physical,
		Modes: []ipc.Mode{
			{Width: 1280, Height: 720, RefreshRate: 60_000},
			{Width: 1920, Height: 1080, RefreshRate: 144_000, IsPreferred: true},
		},
		CurrentMode: &current, VrrSupported: true, VrrEnabled: true,
		Logical: &ipc.LogicalOutput{
			X: -1920, Y: 0, Width: 1920, Height: 1080, Scale: 1.25, Transform: ipc.Transform90,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toNiriOutput() = %+v, want %+v", got, want)
	}
}

func TestNiriLayerSnapshotConversion(t *testing.T) {
	got := toNiriLayer(wlr.LayerSnapshot{
		Namespace: "panel", Output: "DP-1", Layer: wlr.LayerTopKind,
		KeyboardInteractivity: wlr.LayerKeyboardExclusiveMode,
	})
	want := ipc.LayerSurface{
		Namespace: "panel", Output: "DP-1", Layer: ipc.LayerTop,
		KeyboardInteractivity: ipc.LayerKeyboardExclusive,
	}
	if got != want {
		t.Errorf("toNiriLayer() = %+v, want %+v", got, want)
	}
}

func TestNiriHandler(t *testing.T) {
	quits := 0
	h := niriHandler{quit: func() { quits++ }, spawn: newSpawner("w", "", nil)}
	for _, tc := range []struct {
		line, want string
		after      bool
	}{
		{`"ReturnError"`, `{"Err":"example compositor error"}`, false},
		{`"Outputs"`, `{"Ok":{"Outputs":{}}}`, false},
		{`"Workspaces"`, `{"Ok":{"Workspaces":[]}}`, false},
		{`"Windows"`, `{"Ok":{"Windows":[]}}`, false},
		{`"Layers"`, `{"Ok":{"Layers":[]}}`, false},
		{`"FocusedOutput"`, `{"Ok":{"FocusedOutput":null}}`, false},
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

func TestNiriWorkspaceIDsFollowOutputLifetimes(t *testing.T) {
	state := newNiriState()
	output := ipc.Output{Name: "DP-1"}
	focused := output
	state.replace(niriSnapshot{outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused})
	first := state.snapshot().workspaces[0]
	if first.ID == 0 || first.Idx != 1 || first.Output == nil || *first.Output != "DP-1" || !first.IsActive || !first.IsFocused {
		t.Fatalf("initial workspace = %+v", first)
	}
	state.replace(niriSnapshot{outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused})
	if got := state.snapshot().workspaces[0].ID; got != first.ID {
		t.Fatalf("workspace id changed while output remained connected: %d -> %d", first.ID, got)
	}
	state.replace(niriSnapshot{outputs: map[string]ipc.Output{}})
	state.replace(niriSnapshot{outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused})
	if got := state.snapshot().workspaces[0].ID; got == first.ID {
		t.Fatalf("workspace id reused after output disconnect: %d", got)
	}
}
