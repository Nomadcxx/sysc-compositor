package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestNiriWindowSnapshotConversion(t *testing.T) {
	title, appID := "Terminal", "org.example.Terminal"
	logical := &ipc.LogicalOutput{X: -1920, Y: 10}
	got := toNiriWindow(wlr.WindowInfo{
		ID: 12, Title: &title, AppID: &appID, PID: 123,
		X: -1900, Y: 50, Width: 800, Height: 600, Output: "DP-1", IsFocused: true,
		FocusTimestamp: 8*time.Second + 23*time.Nanosecond, HasFocusTimestamp: true,
	}, 7, ipc.Output{Name: "DP-1", Logical: logical})
	workspaceID := uint64(7)
	pid := int32(123)
	focusTimestamp := &ipc.Timestamp{Secs: 8, Nanos: 23}
	position := [2]float64{20, 40}
	want := ipc.Window{
		ID: 12, Title: &title, AppID: &appID, PID: &pid, WorkspaceID: &workspaceID,
		IsFocused: true, IsFloating: true,
		Layout: ipc.WindowLayout{
			TileSize: [2]float64{800, 600}, WindowSize: [2]int32{800, 600},
			TilePosInWorkspaceView: &position,
		},
		FocusTimestamp: focusTimestamp,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toNiriWindow() = %+v, want %+v", got, want)
	}

	got = toNiriWindow(wlr.WindowInfo{ID: 13, Output: "DP-1"}, 7, ipc.Output{Name: "DP-1"})
	if got.Title != nil || got.AppID != nil || got.PID != nil || got.FocusTimestamp != nil {
		t.Fatalf("missing window metadata must stay null: %+v", got)
	}
}

func TestWindowOutputFallsBackToFocusedOutputWhenCenterIsOffscreen(t *testing.T) {
	outputs := map[string]ipc.Output{
		"DP-1":     {Name: "DP-1"},
		"HDMI-A-1": {Name: "HDMI-A-1"},
	}
	for _, tc := range []struct {
		name, windowOutput, focusedOutput, want string
	}{
		{"on geometry output", "HDMI-A-1", "DP-1", "HDMI-A-1"},
		{"offscreen center", "", "DP-1", "DP-1"},
		{"disconnected geometry output", "gone", "HDMI-A-1", "HDMI-A-1"},
		{"no connected fallback", "", "gone", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := niriWindowOutputName(tc.windowOutput, tc.focusedOutput, outputs); got != tc.want {
				t.Fatalf("niriWindowOutputName(%q, %q) = %q, want %q", tc.windowOutput, tc.focusedOutput, got, tc.want)
			}
		})
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

func TestNiriFocusedWindowQuery(t *testing.T) {
	title := "Terminal"
	focused := uint64(9)
	window := ipc.Window{ID: focused, Title: &title, IsFocused: true}
	h := niriHandler{snapshot: func() niriSnapshot {
		return niriSnapshot{windows: []ipc.Window{window}}
	}}
	reply, _ := h.handle(ipc.Request{Name: "FocusedWindow"})
	got, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Ok":{"FocusedWindow":{"id":9,"title":"Terminal","app_id":null,"pid":null,"workspace_id":null,"is_focused":true,"is_floating":false,"is_urgent":false,"layout":{"pos_in_scrolling_layout":null,"tile_size":[0,0],"window_size":[0,0],"tile_pos_in_workspace_view":null,"window_offset_in_tile":[0,0]},"focus_timestamp":null}}}`
	if string(got) != want {
		t.Fatalf("FocusedWindow = %s, want %s", got, want)
	}

	h.snapshot = func() niriSnapshot { return niriSnapshot{windows: []ipc.Window{}} }
	reply, _ = h.handle(ipc.Request{Name: "FocusedWindow"})
	got, err = json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"Ok":{"FocusedWindow":null}}` {
		t.Fatalf("FocusedWindow without a window = %s, want null", got)
	}
}

func TestNiriWindowActionsArePosted(t *testing.T) {
	output1, output2 := "DP-1", "HDMI-A-1"
	focusedOutput := ipc.Output{Name: output1}
	id1, id2 := uint64(7), uint64(8)
	var posted []struct {
		action ipc.Action
		output string
	}
	h := niriHandler{
		snapshot: func() niriSnapshot {
			return niriSnapshot{
				focusedOutput: &focusedOutput,
				workspaces: []ipc.Workspace{
					{ID: id1, Idx: 1, Output: &output1},
					{ID: id2, Idx: 1, Output: &output2},
				},
			}
		},
		postAction: func(action ipc.Action, output string) error {
			posted = append(posted, struct {
				action ipc.Action
				output string
			}{action, output})
			return nil
		},
	}
	for _, tc := range []struct {
		line, name, output string
	}{
		{`{"Action":{"FocusWindow":{"id":42}}}`, "FocusWindow", ""},
		{`{"Action":{"CloseWindow":{"id":42}}}`, "CloseWindow", ""},
		{`{"Action":{"CloseWindow":{"id":null}}}`, "CloseWindow", ""},
		{`{"Action":{"FocusWorkspace":{"reference":{"Id":8}}}}`, "FocusWorkspace", output2},
		{`{"Action":{"FocusWorkspace":{"reference":{"Index":1}}}}`, "FocusWorkspace", output1},
	} {
		before := len(posted)
		request, err := ipc.DecodeRequest([]byte(tc.line))
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", tc.line, err)
		}
		reply, after := h.handle(request)
		if string(reply.Ok) != string(ipc.Handled) || after == nil {
			t.Fatalf("%s reply = %+v, after = %v; want Handled and posted operation", tc.name, reply, after != nil)
		}
		if len(posted) != before {
			t.Fatalf("%s posted before reply callback", tc.name)
		}
		after()
		got := posted[len(posted)-1]
		if got.action.Name != tc.name || got.output != tc.output {
			t.Errorf("posted = %+v, want action %s output %q", got, tc.name, tc.output)
		}
	}
	request, err := ipc.DecodeRequest([]byte(`{"Action":{"FocusWorkspace":{"reference":{"Name":"missing"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reply, after := h.handle(request)
	if string(reply.Ok) != string(ipc.Handled) || after != nil || len(posted) != 5 {
		t.Fatalf("unmatched workspace = %+v, after %v, posted %d; want accepted no-op", reply, after != nil, len(posted))
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
	state := newNiriState(false)
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

func TestNiriWorkspacesReportTheirMostRecentWindow(t *testing.T) {
	state := newNiriState(false)
	output := ipc.Output{Name: "DP-1"}
	focused := output
	workspaceID := uint64(1)
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused,
		windows: []ipc.Window{
			{ID: 12, WorkspaceID: &workspaceID},
			{ID: 11, WorkspaceID: &workspaceID},
		},
	})
	workspace := state.snapshot().workspaces[0]
	if workspace.ActiveWindowID == nil || *workspace.ActiveWindowID != 12 {
		t.Fatalf("active_window_id = %v, want most recent mapped window 12", workspace.ActiveWindowID)
	}
}

func TestNiriSnapshotsDoNotSharePointerFields(t *testing.T) {
	state := newNiriState(false)
	output := ipc.Output{Name: "DP-1", Logical: &ipc.LogicalOutput{X: 10}}
	title := "before"
	workspaceID := uint64(1)
	outputName := "DP-1"
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": output},
		windows: []ipc.Window{{ID: 42, Title: &title, WorkspaceID: &workspaceID}},
	})
	// The state owns its committed snapshot, and each reader owns its copy.
	*output.Logical = ipc.LogicalOutput{X: 99}
	title = "after replace"
	first := state.snapshot()
	if got := first.windows[0].Title; got == nil || *got != "before" {
		t.Fatalf("stored title = %v, want before", got)
	}
	if got := first.outputs["DP-1"].Logical.X; got != 10 {
		t.Fatalf("stored output x = %d, want 10", got)
	}
	*first.workspaces[0].Output = outputName + " changed"
	*first.windows[0].Title = "reader mutation"
	*first.outputs["DP-1"].Logical = ipc.LogicalOutput{X: 77}
	*first.workspaces[0].ActiveWindowID = 77
	second := state.snapshot()
	if got := second.windows[0].Title; got == nil || *got != "before" {
		t.Fatalf("second snapshot title = %v, want before", got)
	}
	if got := second.outputs["DP-1"].Logical.X; got != 10 {
		t.Fatalf("second snapshot output x = %d, want 10", got)
	}
	if got := second.workspaces[0].Output; got == nil || *got != "DP-1" {
		t.Fatalf("second snapshot workspace output = %v, want DP-1", got)
	}
	if got := second.workspaces[0].ActiveWindowID; got == nil || *got != 42 {
		t.Fatalf("second snapshot active window = %v, want 42", got)
	}
}
