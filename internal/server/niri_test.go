package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
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
	placement := wm.WindowSnapshot{ID: 12, Output: "DP-1", WorkspaceID: 7, Column: 3, IsUrgent: true,
		Tile: layout.Placement{ID: 12, Column: 3, Tile: 2, X: -1900, Y: 50, Width: 1000, Height: 800}}
	got := toNiriWindow(wlr.WindowInfo{
		ID: 12, Title: &title, AppID: &appID, PID: 123,
		X: -1890, Y: 60, Width: 800, Height: 600, Output: "DP-1", IsFocused: true,
		FocusTimestamp: 8*time.Second + 23*time.Nanosecond, HasFocusTimestamp: true,
	}, placement, ipc.Output{Name: "DP-1", Logical: logical})
	workspaceID := uint64(7)
	pid := int32(123)
	focusTimestamp := &ipc.Timestamp{Secs: 8, Nanos: 23}
	position := [2]float64{20, 40}
	columnPosition := [2]uint32{4, 3}
	want := ipc.Window{
		ID: 12, Title: &title, AppID: &appID, PID: &pid, WorkspaceID: &workspaceID,
		IsFocused: true, IsUrgent: true,
		Layout: ipc.WindowLayout{
			PosInScrollingLayout: &columnPosition,
			TileSize:             [2]float64{1000, 800}, WindowSize: [2]int32{800, 600},
			TilePosInWorkspaceView: &position, WindowOffsetInTile: [2]float64{10, 10},
		},
		FocusTimestamp: focusTimestamp,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toNiriWindow() = %+v, want %+v", got, want)
	}

	got = toNiriWindow(wlr.WindowInfo{ID: 13, Output: "DP-1"}, wm.WindowSnapshot{ID: 13, WorkspaceID: 7}, ipc.Output{Name: "DP-1"})
	if got.Title != nil || got.AppID != nil || got.PID != nil || got.FocusTimestamp != nil {
		t.Fatalf("missing window metadata must stay null: %+v", got)
	}
}

func TestNiriFloatingWindowSnapshotConversion(t *testing.T) {
	got := toNiriWindow(wlr.WindowInfo{ID: 5, Width: 400, Height: 300}, wm.WindowSnapshot{
		ID: 5, WorkspaceID: 9, Tile: layout.Placement{ID: 5, Floating: true, X: 90, Y: 70, Width: 400, Height: 300},
	}, ipc.Output{Name: "DP-1"})
	if !got.IsFloating {
		t.Errorf("IsFloating = false, want true for a floating placement")
	}
	if got.Layout.PosInScrollingLayout != nil {
		t.Errorf("PosInScrollingLayout = %v, want nil for a floating placement", got.Layout.PosInScrollingLayout)
	}
	if got.Layout.TilePosInWorkspaceView == nil || *got.Layout.TilePosInWorkspaceView != [2]float64{90, 70} {
		t.Errorf("TilePosInWorkspaceView = %v, want [90 70]", got.Layout.TilePosInWorkspaceView)
	}
}

func TestNiriWorkspaceSnapshotConversion(t *testing.T) {
	output := "DP-1"
	workspaceName := "Work"
	activeWindow := uint64(12)
	got := toNiriWorkspaces(wm.Snapshot{Workspaces: []wm.WorkspaceSnapshot{
		{ID: 41, Index: 1, Output: output},
		{ID: 42, Index: 2, Output: output, Name: &workspaceName, IsActive: true, IsFocused: true, ActiveWindowID: &activeWindow, IsUrgent: true},
		{ID: 43, Index: 3, Output: output},
	}})
	want := []ipc.Workspace{
		{ID: 41, Idx: 1, Output: &output},
		{ID: 42, Idx: 2, Name: &workspaceName, Output: &output, IsActive: true, IsFocused: true, ActiveWindowID: &activeWindow, IsUrgent: true},
		{ID: 43, Idx: 3, Output: &output},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toNiriWorkspaces() = %+v, want %+v", got, want)
	}
}

func TestNiriHandler(t *testing.T) {
	var quits []bool
	h := niriHandler{quit: func(skip bool) { quits = append(quits, skip) }, spawn: newSpawner("w", "", nil, config.Default().Cursor, "")}
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
		{`{"Action":{"Quit":{"skip_confirmation":false}}}`, `{"Ok":"Handled"}`, true},
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
	if !reflect.DeepEqual(quits, []bool{true, false}) {
		t.Errorf("quit ran with %v, want skip_confirmation true then false, each after its reply", quits)
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

func TestNiriKeyboardLayoutsQuery(t *testing.T) {
	h := niriHandler{snapshot: func() niriSnapshot {
		return niriSnapshot{keyboardLayouts: &ipc.KeyboardLayouts{
			Names: []string{"English (US)", "Russian"}, CurrentIdx: 1,
		}}
	}}
	reply, _ := h.handle(ipc.Request{Name: "KeyboardLayouts"})
	got, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"Ok":{"KeyboardLayouts":{"names":["English (US)","Russian"],"current_idx":1}}}`
	if string(got) != want {
		t.Fatalf("KeyboardLayouts = %s, want %s", got, want)
	}
}

func TestCastActionsArePosted(t *testing.T) {
	var posted []ipc.Action
	h := niriHandler{postAction: func(action ipc.Action, _ uint64) error {
		posted = append(posted, action)
		return nil
	}}
	for _, name := range []string{"SetDynamicCastWindow", "SetDynamicCastMonitor", "ClearDynamicCastTarget", "StopCast"} {
		reply, after := h.action(ipc.Action{Name: name, SessionID: 7})
		if reply.Err != "" || after == nil {
			t.Fatalf("%s was not accepted for compositor dispatch: %+v", name, reply)
		}
		after()
		if len(posted) == 0 || posted[len(posted)-1].Name != name {
			t.Fatalf("%s was not posted", name)
		}
	}
}

func TestNiriSwitchLayoutActionIsPosted(t *testing.T) {
	index := uint8(2)
	var posted ipc.Action
	h := niriHandler{postAction: func(action ipc.Action, _ uint64) error {
		posted = action
		return nil
	}}
	reply, after := h.action(ipc.Action{Name: "SwitchLayout", Layout: "Index", LayoutIndex: &index})
	if string(reply.Ok) != `"Handled"` || after == nil {
		t.Fatalf("SwitchLayout reply = %+v, after=%v", reply, after != nil)
	}
	after()
	if posted.Name != "SwitchLayout" || posted.Layout != "Index" || posted.LayoutIndex == nil || *posted.LayoutIndex != index {
		t.Fatalf("posted SwitchLayout = %+v, want Index %d", posted, index)
	}
}

func TestNiriLoadConfigFileActionValidatesAndPosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	if err := os.WriteFile(path, []byte("layout { gaps 20 }"), 0o600); err != nil {
		t.Fatal(err)
	}
	posted := false
	h := niriHandler{postAction: func(action ipc.Action, _ uint64) error {
		posted = action.ConfigPath != nil && *action.ConfigPath == path
		return nil
	}}
	reply, after := h.action(ipc.Action{Name: "LoadConfigFile", ConfigPath: &path})
	if string(reply.Ok) != `"Handled"` || after == nil {
		t.Fatalf("LoadConfigFile reply = %+v, after=%v", reply, after != nil)
	}
	after()
	if !posted {
		t.Fatal("LoadConfigFile action was not posted")
	}

	reply, _ = h.action(ipc.Action{Name: "LoadConfigFile", ConfigPath: ptr("relative.kdl")})
	if reply.Err != "path must be absolute: relative.kdl" {
		t.Fatalf("relative LoadConfigFile path error = %q", reply.Err)
	}
}

func TestNiriWindowActionsArePosted(t *testing.T) {
	output1, output2 := "DP-1", "HDMI-A-1"
	focusedOutput := ipc.Output{Name: output1}
	id1, id2 := uint64(7), uint64(8)
	var posted []struct {
		action      ipc.Action
		workspaceID uint64
	}
	h := niriHandler{
		snapshot: func() niriSnapshot {
			return niriSnapshot{
				focusedOutput: &focusedOutput,
				workspaces: []ipc.Workspace{
					{ID: id1, Idx: 1, Output: &output1},
					{ID: id2, Idx: 2, Output: &output1},
					{ID: 9, Idx: 1, Output: &output2},
				},
			}
		},
		postAction: func(action ipc.Action, workspaceID uint64) error {
			posted = append(posted, struct {
				action      ipc.Action
				workspaceID uint64
			}{action, workspaceID})
			return nil
		},
	}
	for _, tc := range []struct {
		line, name  string
		workspaceID uint64
	}{
		{`{"Action":{"FocusWindow":{"id":42}}}`, "FocusWindow", 0},
		{`{"Action":{"CloseWindow":{"id":42}}}`, "CloseWindow", 0},
		{`{"Action":{"CloseWindow":{"id":null}}}`, "CloseWindow", 0},
		{`{"Action":{"FocusWorkspace":{"reference":{"Id":8}}}}`, "FocusWorkspace", 8},
		{`{"Action":{"FocusWorkspace":{"reference":{"Index":2}}}}`, "FocusWorkspace", 8},
		{`{"Action":{"FocusWorkspace":{"reference":{"Index":1}}}}`, "FocusWorkspace", 7},
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
		if got.action.Name != tc.name || got.workspaceID != tc.workspaceID {
			t.Errorf("posted = %+v, want action %s workspace ID %d", got, tc.name, tc.workspaceID)
		}
	}
	request, err := ipc.DecodeRequest([]byte(`{"Action":{"FocusWorkspace":{"reference":{"Name":"missing"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reply, after := h.handle(request)
	if string(reply.Ok) != string(ipc.Handled) || after != nil || len(posted) != 6 {
		t.Fatalf("unmatched workspace = %+v, after %v, posted %d; want accepted no-op", reply, after != nil, len(posted))
	}
}

func TestNiriOverviewStateAndActions(t *testing.T) {
	h := niriHandler{
		snapshot: func() niriSnapshot { return niriSnapshot{overviewOpen: true} },
	}
	reply, after := h.handle(ipc.Request{Name: "OverviewState"})
	if after != nil || string(reply.Ok) != `{"OverviewState":{"is_open":true}}` {
		t.Fatalf("OverviewState = %+v, after %v", reply, after != nil)
	}

	var posted []string
	h.snapshot = func() niriSnapshot { return niriSnapshot{} }
	h.postAction = func(action ipc.Action, _ uint64) error {
		posted = append(posted, action.Name)
		return nil
	}
	for _, name := range []string{"ToggleOverview", "OpenOverview", "CloseOverview"} {
		line := []byte(`{"Action":{"` + name + `":{}}}`)
		request, err := ipc.DecodeRequest(line)
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", line, err)
		}
		reply, after = h.handle(request)
		if string(reply.Ok) != string(ipc.Handled) || after == nil {
			t.Fatalf("%s reply = %+v, after %v; want Handled and queued action", name, reply, after != nil)
		}
		after()
	}
	if !reflect.DeepEqual(posted, []string{"ToggleOverview", "OpenOverview", "CloseOverview"}) {
		t.Fatalf("posted overview actions = %v", posted)
	}
}

func TestNiriM6ActionsArePosted(t *testing.T) {
	output := "DP-1"
	focusedOutput := ipc.Output{Name: output}
	name := "work"
	selectedWindowID := uint64(41)
	var posted []struct {
		action      ipc.Action
		workspaceID uint64
	}
	h := niriHandler{
		snapshot: func() niriSnapshot {
			return niriSnapshot{
				focusedOutput: &focusedOutput,
				workspaces: []ipc.Workspace{
					{ID: 7, Idx: 1, Output: &output, IsFocused: true, Name: &name},
					{ID: 8, Idx: 2, Output: &output},
				},
			}
		},
		postAction: func(action ipc.Action, workspaceID uint64) error {
			posted = append(posted, struct {
				action      ipc.Action
				workspaceID uint64
			}{action, workspaceID})
			return nil
		},
	}
	for _, tc := range []struct {
		line        string
		name        string
		workspaceID uint64
	}{
		{`{"Action":{"ConsumeWindowIntoColumn":{}}}`, "ConsumeWindowIntoColumn", 0},
		{`{"Action":{"MoveWindowToWorkspaceDown":{"focus":true}}}`, "MoveWindowToWorkspaceDown", 0},
		{`{"Action":{"MoveWindowToWorkspaceUp":{"focus":false}}}`, "MoveWindowToWorkspaceUp", 0},
		{`{"Action":{"MoveColumnToWorkspaceDown":{"focus":true}}}`, "MoveColumnToWorkspaceDown", 0},
		{`{"Action":{"MoveColumnToWorkspaceUp":{"focus":false}}}`, "MoveColumnToWorkspaceUp", 0},
		{`{"Action":{"FocusWindowDown":{}}}`, "FocusWindowDown", 0},
		{`{"Action":{"FocusWindowUp":{}}}`, "FocusWindowUp", 0},
		{`{"Action":{"FocusWindowPrevious":{}}}`, "FocusWindowPrevious", 0},
		{`{"Action":{"MoveWindowDown":{}}}`, "MoveWindowDown", 0},
		{`{"Action":{"MoveWindowUp":{}}}`, "MoveWindowUp", 0},
		{`{"Action":{"ConsumeOrExpelWindowLeft":{"id":41}}}`, "ConsumeOrExpelWindowLeft", 0},
		{`{"Action":{"ConsumeOrExpelWindowRight":{"id":null}}}`, "ConsumeOrExpelWindowRight", 0},
		{`{"Action":{"FocusWindowInColumn":{"index":2}}}`, "FocusWindowInColumn", 0},
		{`{"Action":{"SetColumnDisplay":{"display":"Tabbed"}}}`, "SetColumnDisplay", 0},
		{`{"Action":{"FullscreenWindow":{"id":null}}}`, "FullscreenWindow", 0},
		{`{"Action":{"ToggleWindowFloating":{"id":41}}}`, "ToggleWindowFloating", 0},
		{`{"Action":{"SetWorkspaceName":{"name":"dev","workspace":null}}}`, "SetWorkspaceName", 7},
		{`{"Action":{"MoveWindowToWorkspace":{"window_id":null,"reference":{"Name":"WORK"},"focus":true}}}`, "MoveWindowToWorkspace", 7},
		{`{"Action":{"MoveWorkspaceToMonitor":{"output":"HDMI-A-1","reference":{"Name":"work"}}}}`, "MoveWorkspaceToMonitor", 7},
		{`{"Action":{"MoveWorkspaceToMonitor":{"output":"HDMI-A-1","reference":null}}}`, "MoveWorkspaceToMonitor", 7},
		{`{"Action":{"MoveWorkspaceToMonitorNext":{}}}`, "MoveWorkspaceToMonitorNext", 0},
		{`{"Action":{"MoveColumnToMonitor":{"output":"HDMI-A-1"}}}`, "MoveColumnToMonitor", 0},
		{`{"Action":{"FocusMonitor":{"output":"HDMI-A-1"}}}`, "FocusMonitor", 0},
		{`{"Action":{"FocusMonitorNext":{}}}`, "FocusMonitorNext", 0},
		{`{"Action":{"ToggleWindowUrgent":{"id":41}}}`, "ToggleWindowUrgent", 0},
		{`{"Action":{"SetWindowUrgent":{"id":41}}}`, "SetWindowUrgent", 0},
		{`{"Action":{"UnsetWindowUrgent":{"id":41}}}`, "UnsetWindowUrgent", 0},
	} {
		request, err := ipc.DecodeRequest([]byte(tc.line))
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", tc.line, err)
		}
		reply, after := h.handle(request)
		if string(reply.Ok) != string(ipc.Handled) || after == nil {
			t.Fatalf("%s reply = %+v, after = %v; want Handled and posted operation", tc.name, reply, after != nil)
		}
		after()
		got := posted[len(posted)-1]
		if got.action.Name != tc.name || got.workspaceID != tc.workspaceID {
			t.Errorf("posted = %+v, want action %s workspace ID %d", got, tc.name, tc.workspaceID)
		}
		if (tc.name == "MoveWorkspaceToMonitor" || tc.name == "FocusMonitor") && got.action.Output != "HDMI-A-1" {
			t.Errorf("posted output = %q, want HDMI-A-1", got.action.Output)
		}
		if tc.name == "ConsumeOrExpelWindowLeft" && (got.action.WindowID == nil || *got.action.WindowID != selectedWindowID) {
			t.Errorf("posted window ID = %v, want %d", got.action.WindowID, selectedWindowID)
		}
		if tc.name == "ConsumeOrExpelWindowRight" && got.action.WindowID != nil {
			t.Errorf("posted window ID = %d, want nil", *got.action.WindowID)
		}
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

func TestNiriStatePreservesModelWorkspaces(t *testing.T) {
	state := newNiriState(false)
	output := ipc.Output{Name: "DP-1"}
	focused := output
	outputName := "DP-1"
	active := uint64(12)
	workspaces := []ipc.Workspace{
		{ID: 41, Idx: 1, Output: &outputName},
		{ID: 42, Idx: 2, Output: &outputName, IsActive: true, IsFocused: true, ActiveWindowID: &active},
		{ID: 43, Idx: 3, Output: &outputName},
	}
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused, workspaces: workspaces,
	})
	if got := state.snapshot().workspaces; !reflect.DeepEqual(got, workspaces) {
		t.Fatalf("Niri workspaces = %+v, want compositor model %+v", got, workspaces)
	}
}

func TestNiriWorkspacesReportTheirMostRecentWindow(t *testing.T) {
	state := newNiriState(false)
	output := ipc.Output{Name: "DP-1"}
	focused := output
	workspaceID := uint64(1)
	outputName := "DP-1"
	activeWindowID := uint64(12)
	state.replace(niriSnapshot{
		outputs: map[string]ipc.Output{"DP-1": output}, focusedOutput: &focused,
		workspaces: []ipc.Workspace{{ID: workspaceID, Idx: 1, Output: &outputName, ActiveWindowID: &activeWindowID}},
		windows: []ipc.Window{
			{ID: 12, WorkspaceID: &workspaceID},
			{ID: 11, WorkspaceID: &workspaceID},
		},
	})
	workspace := state.snapshot().workspaces[0]
	if workspace.ActiveWindowID == nil || *workspace.ActiveWindowID != 12 {
		t.Fatalf("active_window_id = %v, want model-selected window 12", workspace.ActiveWindowID)
	}
}

func TestEventStreamReportsWindowLayoutChanges(t *testing.T) {
	id, workspaceID := uint64(42), uint64(7)
	before := niriSnapshot{windows: []ipc.Window{{
		ID: id, WorkspaceID: &workspaceID,
		Layout: ipc.WindowLayout{TileSize: [2]float64{600, 800}, TilePosInWorkspaceView: ptr([2]float64{0, 0})},
	}}}
	after := niriSnapshot{windows: []ipc.Window{{
		ID: id, WorkspaceID: &workspaceID,
		Layout: ipc.WindowLayout{TileSize: [2]float64{600, 800}, TilePosInWorkspaceView: ptr([2]float64{616, 0})},
	}}}

	var found bool
	for _, line := range eventStreamChanges(before, after) {
		if strings.Contains(string(line), `"WindowLayoutsChanged"`) && strings.Contains(string(line), `"changes":[[42,`) {
			found = true
		}
	}
	if !found {
		t.Fatal("eventStreamChanges() did not report the changed window tile position")
	}
}

func TestEventStreamReportsUrgencyTransitions(t *testing.T) {
	windowID, workspaceID := uint64(42), uint64(7)
	before := niriSnapshot{
		workspaces: []ipc.Workspace{{ID: workspaceID, Idx: 1}},
		windows:    []ipc.Window{{ID: windowID, WorkspaceID: &workspaceID}},
	}
	after := niriSnapshot{
		workspaces: []ipc.Workspace{{ID: workspaceID, Idx: 1, IsUrgent: true}},
		windows:    []ipc.Window{{ID: windowID, WorkspaceID: &workspaceID, IsUrgent: true}},
	}
	want := [][]byte{
		eventLine("WorkspaceUrgencyChanged", struct {
			ID     uint64 `json:"id"`
			Urgent bool   `json:"urgent"`
		}{ID: workspaceID, Urgent: true}),
		eventLine("WindowUrgencyChanged", struct {
			ID     uint64 `json:"id"`
			Urgent bool   `json:"urgent"`
		}{ID: windowID, Urgent: true}),
	}
	if got := eventStreamChanges(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("urgency events = %q, want %q", got, want)
	}
	if got := eventStreamChanges(after, before); len(got) != 2 ||
		!strings.Contains(string(got[0]), `"WorkspaceUrgencyChanged":{"id":7,"urgent":false}`) ||
		!strings.Contains(string(got[1]), `"WindowUrgencyChanged":{"id":42,"urgent":false}`) {
		t.Fatalf("cleared urgency events = %q, want workspace then window clear events", got)
	}
}

func ptr[T any](value T) *T { return &value }

func TestNiriSnapshotsDoNotSharePointerFields(t *testing.T) {
	state := newNiriState(false)
	output := ipc.Output{Name: "DP-1", Logical: &ipc.LogicalOutput{X: 10}}
	title := "before"
	workspaceID := uint64(1)
	outputName := "DP-1"
	activeWindowID := uint64(42)
	state.replace(niriSnapshot{
		outputs:    map[string]ipc.Output{"DP-1": output},
		workspaces: []ipc.Workspace{{ID: 1, Idx: 1, Output: &outputName, ActiveWindowID: &activeWindowID}},
		windows:    []ipc.Window{{ID: 42, Title: &title, WorkspaceID: &workspaceID}},
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

func TestNavigationActionsAreHandled(t *testing.T) {
	var posted []string
	h := niriHandler{postAction: func(action ipc.Action, _ uint64) error {
		posted = append(posted, action.Name)
		return nil
	}}
	names := []string{
		"FocusWorkspaceDown", "FocusWorkspaceUp", "FocusWorkspacePrevious", "MoveWorkspaceDown", "MoveWorkspaceUp",
		"FocusColumnRightOrFirst", "FocusColumnLeftOrLast", "FocusWindowOrMonitorUp", "FocusWindowOrMonitorDown",
		"FocusColumnOrMonitorLeft", "FocusColumnOrMonitorRight", "FocusWindowDownOrColumnLeft",
		"FocusWindowDownOrColumnRight", "FocusWindowUpOrColumnLeft", "FocusWindowUpOrColumnRight",
		"FocusWindowOrWorkspaceDown", "FocusWindowOrWorkspaceUp", "FocusWindowTop", "FocusWindowBottom",
		"FocusWindowDownOrTop", "FocusWindowUpOrBottom", "MoveColumnLeftOrToMonitorLeft",
		"MoveColumnRightOrToMonitorRight", "MoveWindowDownOrToWorkspaceDown", "MoveWindowUpOrToWorkspaceUp",
		"SwapWindowLeft", "SwapWindowRight",
	}
	lines := make([]string, 0, len(names)+3)
	for _, name := range names {
		lines = append(lines, `{"Action":{"`+name+`":{}}}`)
	}
	lines = append(lines, `{"Action":{"FocusColumn":{"index":2}}}`, `{"Action":{"MoveColumnToIndex":{"index":1}}}`,
		`{"Action":{"MoveWorkspaceToIndex":{"index":1,"reference":null}}}`)
	if len(lines) != 30 {
		t.Fatalf("test covers %d actions, want 30", len(lines))
	}
	for _, line := range lines {
		r, err := ipc.DecodeRequest([]byte(line))
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		reply, after := h.handle(r)
		out, _ := json.Marshal(reply)
		if string(out) != `{"Ok":"Handled"}` || after == nil {
			t.Errorf("%s: reply %s (after hook %v), want Handled with a posted action", line, out, after != nil)
			continue
		}
		after()
	}
	if len(posted) != 30 {
		t.Errorf("posted %d actions, want 30: %v", len(posted), posted)
	}
}
