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
		{`{"Action":{"FocusWorkspace":{"reference":{"Index":2}}}}`,
			Request{Name: "Action", Action: &Action{Name: "FocusWorkspace"}}},
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
		`{"Action":{"Spawn":{"command":"kitty"}}}`,                 // not a list
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
