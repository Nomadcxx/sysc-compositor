package ipc

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The lines are what Niri 26.04's `niri msg` writes (serde_json of
// niri_ipc::Request).
func TestDecodeRequest(t *testing.T) {
	workspaceIndex := uint8(2)
	for _, tc := range []struct {
		line string
		want Request
	}{
		{`"Version"`, Request{Name: "Version"}},
		{`"ReturnError"`, Request{Name: "ReturnError"}},
		{`"Outputs"`, Request{Name: "Outputs"}},
		{`{"Action":{"Quit":{"skip_confirmation":true}}}`,
			Request{Name: "Action", Action: &Action{Name: "Quit", SkipConfirmation: true}}},
		{`{"Action":{"Quit":{"skip_confirmation":false}}}`,
			Request{Name: "Action", Action: &Action{Name: "Quit"}}},
		{`{"Action":{"Spawn":{"command":["kitty","--hold"]}}}`,
			Request{Name: "Action", Action: &Action{Name: "Spawn", Command: []string{"kitty", "--hold"}}}},
		{`{"Action":{"SpawnSh":{"command":"echo hi; true"}}}`,
			Request{Name: "Action", Action: &Action{Name: "SpawnSh", ShellCommand: "echo hi; true"}}},
		{`{"Action":{"ToggleDebugTint":{}}}`,
			Request{Name: "Action", Action: &Action{Name: "ToggleDebugTint"}}},
		{`{"Action":{"ToggleOverview":{}}}`,
			Request{Name: "Action", Action: &Action{Name: "ToggleOverview"}}},
		{`{"Action":{"OpenOverview":{}}}`,
			Request{Name: "Action", Action: &Action{Name: "OpenOverview"}}},
		{`{"Action":{"CloseOverview":{}}}`,
			Request{Name: "Action", Action: &Action{Name: "CloseOverview"}}},
		{`{"Action":{"FocusWorkspace":{"reference":{"Index":2}}}}`,
			Request{Name: "Action", Action: &Action{Name: "FocusWorkspace", Workspace: &WorkspaceReference{Index: &workspaceIndex}}}},
	} {
		got, err := DecodeRequest([]byte(tc.line))
		if err != nil {
			t.Errorf("%s: %v", tc.line, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v / %+v, want %+v / %+v", tc.line, got, got.Action, tc.want, tc.want.Action)
		}
	}
}

func TestDecodeRequestRejects(t *testing.T) {
	for _, line := range []string{
		``, `{}`, `"Nope"`, `{"Version":null,"Outputs":null}`,
		`{"Action":{"Nope":{}}}`,
		`{"Action":{"Quit":{}}}`,                                   // serde: missing field
		`{"Action":{"Quit":{"skip_confirmation":true,"extra":1}}}`, // unknown field
		`{"Action":{"ToggleOverview":{"extra":true}}}`,
		`{"Action":{"OpenOverview":{"extra":true}}}`,
		`{"Action":{"CloseOverview":{"extra":true}}}`,
		`{"Action":{"Spawn":{"command":"kitty"}}}`, // not a list
		`{"Action":"Quit"}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("%q: err = %v, want %v", line, err, ErrParse)
		}
	}
}

func TestWindowActionsRoundTripNiriWire(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"FocusWindow":{"id":81}}}`,
		`{"Action":{"CloseWindow":{"id":81}}}`,
		`{"Action":{"CloseWindow":{"id":null}}}`,
		`{"Action":{"FocusWorkspace":{"reference":{"Id":7}}}}`,
		`{"Action":{"FocusWorkspace":{"reference":{"Index":2}}}}`,
		`{"Action":{"FocusWorkspace":{"reference":{"Name":"work"}}}}`,
	} {
		r, err := DecodeRequest([]byte(line))
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", line, err)
		}
		if got := string(EncodeRequest(r)); got != line {
			t.Errorf("round trip = %s, want %s", got, line)
		}
	}
}

func TestWindowActionsRejectMalformedFields(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"FocusWindow":{}}}`,
		`{"Action":{"FocusWindow":{"id":null}}}`,
		`{"Action":{"FocusWindow":{"id":81,"extra":true}}}`,
		`{"Action":{"CloseWindow":{"id":"81"}}}`,
		`{"Action":{"CloseWindow":{"extra":true}}}`,
		`{"Action":{"FocusWorkspace":{}}}`,
		`{"Action":{"FocusWorkspace":{"reference":{"Id":7,"Index":1}}}}`,
		`{"Action":{"FocusWorkspace":{"reference":{"Other":7}}}}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("DecodeRequest(%s) error = %v, want %v", line, err, ErrParse)
		}
	}
}

func TestM6ActionsRoundTripNiriWire(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"MoveWindowToWorkspaceDown":{"focus":true}}}`,
		`{"Action":{"MoveWindowToWorkspaceUp":{"focus":false}}}`,
		`{"Action":{"MoveColumnToWorkspaceDown":{"focus":true}}}`,
		`{"Action":{"MoveColumnToWorkspaceUp":{"focus":false}}}`,
		`{"Action":{"FocusWindowDown":{}}}`,
		`{"Action":{"FocusWindowUp":{}}}`,
		`{"Action":{"MoveWindowDown":{}}}`,
		`{"Action":{"MoveWindowUp":{}}}`,
		`{"Action":{"FocusWindowInColumn":{"index":2}}}`,
		`{"Action":{"ConsumeWindowIntoColumn":{}}}`,
		`{"Action":{"ConsumeOrExpelWindowLeft":{"id":41}}}`,
		`{"Action":{"ConsumeOrExpelWindowRight":{"id":null}}}`,
		`{"Action":{"SetColumnDisplay":{"display":"Tabbed"}}}`,
		`{"Action":{"FullscreenWindow":{"id":null}}}`,
		`{"Action":{"MoveWindowToFloating":{"id":41}}}`,
		`{"Action":{"FocusFloating":{}}}`,
		`{"Action":{"SetWorkspaceName":{"name":"work","workspace":null}}}`,
		`{"Action":{"UnsetWorkspaceName":{"reference":{"Name":"work"}}}}`,
		`{"Action":{"MoveWindowToWorkspace":{"window_id":null,"reference":{"Name":"work"},"focus":true}}}`,
		`{"Action":{"MoveWindowToMonitor":{"id":null,"output":"DP-2"}}}`,
		`{"Action":{"MoveColumnToMonitor":{"output":"DP-2"}}}`,
		`{"Action":{"MoveWorkspaceToMonitor":{"output":"DP-2","reference":{"Name":"work"}}}}`,
		`{"Action":{"FocusMonitor":{"output":"DP-2"}}}`,
		`{"Action":{"ToggleWindowUrgent":{"id":41}}}`,
		`{"Action":{"SetWindowUrgent":{"id":41}}}`,
		`{"Action":{"UnsetWindowUrgent":{"id":41}}}`,
	} {
		request, err := DecodeRequest([]byte(line))
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", line, err)
		}
		if got := string(EncodeRequest(request)); got != line {
			t.Errorf("round trip = %s, want %s", got, line)
		}
	}
}

func TestMoveFloatingWindowPositionChangesRoundTripNiriWire(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"MoveFloatingWindow":{"id":41,"x":{"SetFixed":12.5},"y":{"SetProportion":75}}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":null,"x":{"AdjustFixed":40},"y":{"AdjustProportion":-10}}}}`,
	} {
		request, err := DecodeRequest([]byte(line))
		if err != nil {
			t.Fatalf("DecodeRequest(%s): %v", line, err)
		}
		if got := string(EncodeRequest(request)); got != line {
			t.Errorf("round trip = %s, want %s", got, line)
		}
	}
}

func TestRelativeWorkspaceMoveDefaultsFocusWhenEncoding(t *testing.T) {
	for _, name := range []string{
		"MoveWindowToWorkspaceDown", "MoveWindowToWorkspaceUp",
		"MoveColumnToWorkspaceDown", "MoveColumnToWorkspaceUp",
	} {
		got := string(EncodeRequest(Request{Name: "Action", Action: &Action{Name: name}}))
		if !strings.Contains(got, `"focus":true`) {
			t.Errorf("EncodeRequest(%s) = %s, want default focus=true", name, got)
		}
	}
}

func TestM6ActionsRejectMalformedFields(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"FocusWindowInColumn":{"index":256}}}`,
		`{"Action":{"SetColumnDisplay":{"display":"Tabbed","extra":true}}}`,
		`{"Action":{"SetColumnDisplay":{"display":"Mosaic"}}}`,
		`{"Action":{"MoveWindowToWorkspace":{"reference":{"Name":"work"},"focus":"yes"}}}`,
		`{"Action":{"MoveWindowToWorkspaceDown":{}}}`,
		`{"Action":{"MoveColumnToWorkspaceUp":{"focus":"no"}}}`,
		`{"Action":{"MoveWindowToWorkspaceDown":{"focus":true,"extra":1}}}`,
		`{"Action":{"MoveWindowToMonitor":{"output":4}}}`,
		`{"Action":{"FocusFloating":{"extra":true}}}`,
		`{"Action":{"SetWindowUrgent":{}}}`,
		`{"Action":{"ToggleWindowUrgent":{"id":null}}}`,
		`{"Action":{"ConsumeOrExpelWindowRight":{"id":"41"}}}`,
		`{"Action":{"ConsumeOrExpelWindowLeft":{"id":41,"extra":true}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":1,"x":{"AdjustFixed":1}}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":"1","x":{"AdjustFixed":1},"y":{"AdjustFixed":1}}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":null,"x":{"Unknown":1},"y":{"AdjustFixed":1}}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":null,"x":{"SetFixed":1,"AdjustFixed":2},"y":{"AdjustFixed":1}}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":null,"x":{"AdjustFixed":1},"y":{"AdjustFixed":1},"extra":true}}}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("DecodeRequest(%s) error = %v, want %v", line, err, ErrParse)
		}
	}
}

// EncodeRequest must write what serde writes, so Niri's own server would
// accept our `msg`, and ours accepts Niri's.
func TestEncodeRequestRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		r    Request
		want string
	}{
		{Request{Name: "Version"}, `"Version"`},
		{Request{Name: "Action", Action: &Action{Name: "Quit", SkipConfirmation: true}},
			`{"Action":{"Quit":{"skip_confirmation":true}}}`},
		{Request{Name: "Action", Action: &Action{Name: "Spawn", Command: []string{"a", "b"}}},
			`{"Action":{"Spawn":{"command":["a","b"]}}}`},
		{Request{Name: "Action", Action: &Action{Name: "SpawnSh", ShellCommand: "a && b"}},
			`{"Action":{"SpawnSh":{"command":"a && b"}}}`},
	} {
		got := string(EncodeRequest(tc.r))
		if got != tc.want {
			t.Errorf("EncodeRequest(%+v) = %s, want %s", tc.r, got, tc.want)
		}
		back, err := DecodeRequest([]byte(got))
		if err != nil || !reflect.DeepEqual(back, tc.r) {
			t.Errorf("%s decodes to %+v, %v", got, back, err)
		}
	}
}

func TestM9WireRequestsRoundTrip(t *testing.T) {
	for _, line := range []string{
		`{"Output":{"output":"eDP-1","action":"Off"}}`,
		`{"Action":{"SwitchLayout":{"layout":"Next"}}}`,
		`{"Action":{"SwitchLayout":{"layout":{"Index":2}}}}`,
		`{"Action":{"LoadConfigFile":{"path":null}}}`,
		`{"Action":{"LoadConfigFile":{"path":"/tmp/niri.kdl"}}}`,
	} {
		t.Run(line, func(t *testing.T) {
			request, err := DecodeRequest([]byte(line))
			if err != nil {
				t.Fatalf("DecodeRequest(%s): %v", line, err)
			}
			var gotValue, wantValue any
			if err := json.Unmarshal(EncodeRequest(request), &gotValue); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(line), &wantValue); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotValue, wantValue) {
				t.Fatalf("EncodeRequest(DecodeRequest(%s)) = %v, want %v", line, gotValue, wantValue)
			}
		})
	}
}

func TestDecodeOutputRequestRejectsUnknownAction(t *testing.T) {
	if _, err := DecodeRequest([]byte(`{"Output":{"output":"eDP-1","action":"Unknown"}}`)); err != ErrParse {
		t.Fatalf("DecodeRequest unknown output action error = %v, want %v", err, ErrParse)
	}
}

func TestDecodeOutputRequestRejectsMalformedAction(t *testing.T) {
	for _, line := range []string{
		`{"Output":{"output":"eDP-1","action":{"Mode":{}}}}`,
		`{"Output":{"output":"eDP-1","action":{"Vrr":{"vrr":true}}}}`,
		`{"Output":{"output":"eDP-1","action":{"Transform":"Sideways"}}}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("DecodeRequest(%s) error = %v, want %v", line, err, ErrParse)
		}
	}
}

func TestReplyWire(t *testing.T) {
	for _, tc := range []struct {
		r    Reply
		want string
	}{
		{Reply{Ok: Handled}, `{"Ok":"Handled"}`},
		{Reply{Ok: VersionResponse("x 1")}, `{"Ok":{"Version":"x 1"}}`},
		{Unimplemented("Outputs"), `{"Err":"unimplemented: Outputs"}`},
	} {
		out, err := json.Marshal(tc.r)
		if err != nil || string(out) != tc.want {
			t.Errorf("Marshal(%+v) = %s, %v; want %s", tc.r, out, err, tc.want)
		}
		var back Reply
		if err := json.Unmarshal(out, &back); err != nil || string(back.Ok) != string(tc.r.Ok) || back.Err != tc.r.Err {
			t.Errorf("%s reads back as %+v, %v", out, back, err)
		}
	}
}

func TestStateResponseWire(t *testing.T) {
	output := Output{
		Name: "DP-1", Make: "Dell Inc.", Model: "AW3423DWF", Modes: []Mode{},
	}
	window := Window{
		ID:     9,
		Layout: WindowLayout{TileSize: [2]float64{}, WindowSize: [2]int32{}, WindowOffsetInTile: [2]float64{}},
	}
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"Outputs", map[string]Output{"DP-1": output}, `{"Ok":{"Outputs":{"DP-1":{"name":"DP-1","make":"Dell Inc.","model":"AW3423DWF","serial":null,"physical_size":null,"modes":[],"current_mode":null,"is_custom_mode":false,"vrr_supported":false,"vrr_enabled":false,"logical":null}}}}`},
		{"Workspaces", []Workspace{{ID: 7, Idx: 1, Output: stringPtr("DP-1"), IsActive: true, IsFocused: true}}, `{"Ok":{"Workspaces":[{"id":7,"idx":1,"name":null,"output":"DP-1","is_urgent":false,"is_active":true,"is_focused":true,"active_window_id":null}]}}`},
		{"Windows", []Window{window}, `{"Ok":{"Windows":[{"id":9,"title":null,"app_id":null,"pid":null,"workspace_id":null,"is_focused":false,"is_floating":false,"is_urgent":false,"layout":{"pos_in_scrolling_layout":null,"tile_size":[0,0],"window_size":[0,0],"tile_pos_in_workspace_view":null,"window_offset_in_tile":[0,0]},"focus_timestamp":null}]}}`},
		{"Layers", []LayerSurface{{Namespace: "sysc-shell:bar", Output: "DP-1", Layer: LayerTop, KeyboardInteractivity: LayerKeyboardNone}}, `{"Ok":{"Layers":[{"namespace":"sysc-shell:bar","output":"DP-1","layer":"Top","keyboard_interactivity":"None"}]}}`},
		{"FocusedOutput", (*Output)(nil), `{"Ok":{"FocusedOutput":null}}`},
		{"OverviewState", OverviewState{IsOpen: true}, `{"Ok":{"OverviewState":{"is_open":true}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := json.Marshal(Reply{Ok: SuccessValue(tc.name, tc.value)})
			if err != nil || string(out) != tc.want {
				t.Fatalf("response = %s, %v; want %s", out, err, tc.want)
			}
		})
	}
}

func stringPtr(s string) *string { return &s }

// TestNamesMatchNiri compares the request and action tables with niri-ipc
// 26.4.0 at the pinned reference checkout, when it is present.
func TestNamesMatchNiri(t *testing.T) {
	src, err := os.ReadFile("/home/nomadx/references/niri/niri-ipc/src/lib.rs")
	if err != nil {
		t.Skipf("reference checkout unavailable: %v", err)
	}
	for _, tc := range []struct {
		enum string
		got  []string
	}{{"Request", Requests}, {"Action", Actions}} {
		want := enumVariants(t, string(src), tc.enum)
		if !reflect.DeepEqual(tc.got, want) {
			t.Errorf("%s: ours %d names, niri-ipc %d; ours %v\nniri-ipc %v", tc.enum, len(tc.got), len(want), tc.got, want)
		}
	}
}

var reVariant = regexp.MustCompile(`(?m)^    ([A-Z][A-Za-z0-9]*)\s*[{(,]`)

func enumVariants(t *testing.T, src, enum string) []string {
	start := strings.Index(src, "pub enum "+enum+" {")
	if start < 0 {
		t.Fatalf("no enum %s in niri-ipc", enum)
	}
	open := start + strings.Index(src[start:], "{")
	depth, end := 0, open
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			end = i
			break
		}
	}
	var names []string
	for _, m := range reVariant.FindAllStringSubmatch(src[open+1:end], -1) {
		names = append(names, m[1])
	}
	return names
}

func TestScreenshotScreenActionRoundTrips(t *testing.T) {
	line := []byte(`{"Action":{"ScreenshotScreen":{"write_to_disk":false,"show_pointer":true,"path":"/tmp/x.png"}}}`)
	req, err := DecodeRequest(line)
	if err != nil {
		t.Fatal(err)
	}
	a := req.Action
	if a == nil || a.Name != "ScreenshotScreen" || a.WriteToDisk || !a.ShowPointer || a.Path == nil || *a.Path != "/tmp/x.png" {
		t.Fatalf("decoded %+v", a)
	}
	if _, err := DecodeRequest([]byte(`{"Action":{"ScreenshotScreen":{"show_pointer":true}}}`)); err == nil {
		t.Error("ScreenshotScreen without write_to_disk decoded, want a parse error as niri-ipc gives")
	}
	noPath, err := DecodeRequest([]byte(`{"Action":{"ScreenshotScreen":{"write_to_disk":true,"show_pointer":false}}}`))
	if err != nil || noPath.Action.Path != nil {
		t.Errorf("ScreenshotScreen without path = %+v, %v; want a nil path", noPath.Action, err)
	}
}

func TestScreenshotWindowActionRoundTrips(t *testing.T) {
	req, err := DecodeRequest([]byte(`{"Action":{"ScreenshotWindow":{"id":7,"write_to_disk":true,"show_pointer":false,"path":null}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := req.Action
	if a.Name != "ScreenshotWindow" || a.WindowID == nil || *a.WindowID != 7 || !a.WriteToDisk || a.ShowPointer || a.Path != nil {
		t.Fatalf("decoded %+v", a)
	}
	again, err := DecodeRequest(EncodeRequest(req))
	if err != nil || again.Action.WindowID == nil || *again.Action.WindowID != 7 {
		t.Errorf("re-decoded %+v, %v", again.Action, err)
	}
}
