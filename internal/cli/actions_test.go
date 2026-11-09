package cli

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// The argument shapes and expected bodies follow niri-ipc 26.4.0's clap
// definitions, including non-default optional arguments, rather than the parser.
var actionExamples = []struct {
	name string
	args []string
	body string
}{
	{"Quit", []string{"quit", "-s"}, `{"skip_confirmation":true}`},
	{"PowerOffMonitors", []string{"power-off-monitors"}, `{}`},
	{"PowerOnMonitors", []string{"power-on-monitors"}, `{}`},
	{"Spawn", []string{"spawn", "--", "printf", "%s", "hello"}, `{"command":["printf","%s","hello"]}`},
	{"SpawnSh", []string{"spawn-sh", "--", "printf hello"}, `{"command":"printf hello"}`},
	{"DoScreenTransition", []string{"do-screen-transition", "--delay-ms=123"}, `{"delay_ms":123}`},
	{"Screenshot", []string{"screenshot", "--show-pointer=false", "--path", "/tmp/picked.png"}, `{"show_pointer":false,"path":"/tmp/picked.png"}`},
	{"ScreenshotScreen", []string{"screenshot-screen", "-d", "false", "-p", "false", "--path=/tmp/screen.png"}, `{"write_to_disk":false,"show_pointer":false,"path":"/tmp/screen.png"}`},
	{"ScreenshotWindow", []string{"screenshot-window", "--id", "7", "--write-to-disk=false", "--show-pointer=true", "--path", "/tmp/window.png"}, `{"id":7,"write_to_disk":false,"show_pointer":true,"path":"/tmp/window.png"}`},
	{"ToggleKeyboardShortcutsInhibit", []string{"toggle-keyboard-shortcuts-inhibit"}, `{}`},
	{"CloseWindow", []string{"close-window", "--id=7"}, `{"id":7}`},
	{"FullscreenWindow", []string{"fullscreen-window", "--id=7"}, `{"id":7}`},
	{"ToggleWindowedFullscreen", []string{"toggle-windowed-fullscreen", "--id=7"}, `{"id":7}`},
	{"FocusWindow", []string{"focus-window", "--id=7"}, `{"id":7}`},
	{"FocusWindowInColumn", []string{"focus-window-in-column", "2"}, `{"index":2}`},
	{"FocusWindowPrevious", []string{"focus-window-previous"}, `{}`},
	{"FocusColumnLeft", []string{"focus-column-left"}, `{}`},
	{"FocusColumnRight", []string{"focus-column-right"}, `{}`},
	{"FocusColumnFirst", []string{"focus-column-first"}, `{}`},
	{"FocusColumnLast", []string{"focus-column-last"}, `{}`},
	{"FocusColumnRightOrFirst", []string{"focus-column-right-or-first"}, `{}`},
	{"FocusColumnLeftOrLast", []string{"focus-column-left-or-last"}, `{}`},
	{"FocusColumn", []string{"focus-column", "2"}, `{"index":2}`},
	{"FocusWindowOrMonitorUp", []string{"focus-window-or-monitor-up"}, `{}`},
	{"FocusWindowOrMonitorDown", []string{"focus-window-or-monitor-down"}, `{}`},
	{"FocusColumnOrMonitorLeft", []string{"focus-column-or-monitor-left"}, `{}`},
	{"FocusColumnOrMonitorRight", []string{"focus-column-or-monitor-right"}, `{}`},
	{"FocusWindowDown", []string{"focus-window-down"}, `{}`},
	{"FocusWindowUp", []string{"focus-window-up"}, `{}`},
	{"FocusWindowDownOrColumnLeft", []string{"focus-window-down-or-column-left"}, `{}`},
	{"FocusWindowDownOrColumnRight", []string{"focus-window-down-or-column-right"}, `{}`},
	{"FocusWindowUpOrColumnLeft", []string{"focus-window-up-or-column-left"}, `{}`},
	{"FocusWindowUpOrColumnRight", []string{"focus-window-up-or-column-right"}, `{}`},
	{"FocusWindowOrWorkspaceDown", []string{"focus-window-or-workspace-down"}, `{}`},
	{"FocusWindowOrWorkspaceUp", []string{"focus-window-or-workspace-up"}, `{}`},
	{"FocusWindowTop", []string{"focus-window-top"}, `{}`},
	{"FocusWindowBottom", []string{"focus-window-bottom"}, `{}`},
	{"FocusWindowDownOrTop", []string{"focus-window-down-or-top"}, `{}`},
	{"FocusWindowUpOrBottom", []string{"focus-window-up-or-bottom"}, `{}`},
	{"MoveColumnLeft", []string{"move-column-left"}, `{}`},
	{"MoveColumnRight", []string{"move-column-right"}, `{}`},
	{"MoveColumnToFirst", []string{"move-column-to-first"}, `{}`},
	{"MoveColumnToLast", []string{"move-column-to-last"}, `{}`},
	{"MoveColumnLeftOrToMonitorLeft", []string{"move-column-left-or-to-monitor-left"}, `{}`},
	{"MoveColumnRightOrToMonitorRight", []string{"move-column-right-or-to-monitor-right"}, `{}`},
	{"MoveColumnToIndex", []string{"move-column-to-index", "2"}, `{"index":2}`},
	{"MoveWindowDown", []string{"move-window-down"}, `{}`},
	{"MoveWindowUp", []string{"move-window-up"}, `{}`},
	{"MoveWindowDownOrToWorkspaceDown", []string{"move-window-down-or-to-workspace-down"}, `{}`},
	{"MoveWindowUpOrToWorkspaceUp", []string{"move-window-up-or-to-workspace-up"}, `{}`},
	{"ConsumeOrExpelWindowLeft", []string{"consume-or-expel-window-left", "--id=7"}, `{"id":7}`},
	{"ConsumeOrExpelWindowRight", []string{"consume-or-expel-window-right", "--id=7"}, `{"id":7}`},
	{"ConsumeWindowIntoColumn", []string{"consume-window-into-column"}, `{}`},
	{"ExpelWindowFromColumn", []string{"expel-window-from-column"}, `{}`},
	{"SwapWindowRight", []string{"swap-window-right"}, `{}`},
	{"SwapWindowLeft", []string{"swap-window-left"}, `{}`},
	{"ToggleColumnTabbedDisplay", []string{"toggle-column-tabbed-display"}, `{}`},
	{"SetColumnDisplay", []string{"set-column-display", "tabbed"}, `{"display":"Tabbed"}`},
	{"CenterColumn", []string{"center-column"}, `{}`},
	{"CenterWindow", []string{"center-window", "--id=7"}, `{"id":7}`},
	{"CenterVisibleColumns", []string{"center-visible-columns"}, `{}`},
	{"FocusWorkspaceDown", []string{"focus-workspace-down"}, `{}`},
	{"FocusWorkspaceUp", []string{"focus-workspace-up"}, `{}`},
	{"FocusWorkspace", []string{"focus-workspace", "work"}, `{"reference":{"Name":"work"}}`},
	{"FocusWorkspacePrevious", []string{"focus-workspace-previous"}, `{}`},
	{"MoveWindowToWorkspaceDown", []string{"move-window-to-workspace-down", "--focus=false"}, `{"focus":false}`},
	{"MoveWindowToWorkspaceUp", []string{"move-window-to-workspace-up", "--focus=false"}, `{"focus":false}`},
	{"MoveWindowToWorkspace", []string{"move-window-to-workspace", "work", "--window-id", "7", "--focus", "false"}, `{"window_id":7,"reference":{"Name":"work"},"focus":false}`},
	{"MoveColumnToWorkspaceDown", []string{"move-column-to-workspace-down", "--focus=false"}, `{"focus":false}`},
	{"MoveColumnToWorkspaceUp", []string{"move-column-to-workspace-up", "--focus=false"}, `{"focus":false}`},
	{"MoveColumnToWorkspace", []string{"move-column-to-workspace", "--focus", "false", "2"}, `{"reference":{"Index":2},"focus":false}`},
	{"MoveWorkspaceDown", []string{"move-workspace-down"}, `{}`},
	{"MoveWorkspaceUp", []string{"move-workspace-up"}, `{}`},
	{"MoveWorkspaceToIndex", []string{"move-workspace-to-index", "2", "--reference", "work"}, `{"index":2,"reference":{"Name":"work"}}`},
	{"SetWorkspaceName", []string{"set-workspace-name", "new", "--workspace", "work"}, `{"name":"new","workspace":{"Name":"work"}}`},
	{"UnsetWorkspaceName", []string{"unset-workspace-name", "work"}, `{"reference":{"Name":"work"}}`},
	{"FocusMonitorLeft", []string{"focus-monitor-left"}, `{}`},
	{"FocusMonitorRight", []string{"focus-monitor-right"}, `{}`},
	{"FocusMonitorDown", []string{"focus-monitor-down"}, `{}`},
	{"FocusMonitorUp", []string{"focus-monitor-up"}, `{}`},
	{"FocusMonitorPrevious", []string{"focus-monitor-previous"}, `{}`},
	{"FocusMonitorNext", []string{"focus-monitor-next"}, `{}`},
	{"FocusMonitor", []string{"focus-monitor", "DP-1"}, `{"output":"DP-1"}`},
	{"MoveWindowToMonitorLeft", []string{"move-window-to-monitor-left"}, `{}`},
	{"MoveWindowToMonitorRight", []string{"move-window-to-monitor-right"}, `{}`},
	{"MoveWindowToMonitorDown", []string{"move-window-to-monitor-down"}, `{}`},
	{"MoveWindowToMonitorUp", []string{"move-window-to-monitor-up"}, `{}`},
	{"MoveWindowToMonitorPrevious", []string{"move-window-to-monitor-previous"}, `{}`},
	{"MoveWindowToMonitorNext", []string{"move-window-to-monitor-next"}, `{}`},
	{"MoveWindowToMonitor", []string{"move-window-to-monitor", "DP-1", "--id", "7"}, `{"id":7,"output":"DP-1"}`},
	{"MoveColumnToMonitorLeft", []string{"move-column-to-monitor-left"}, `{}`},
	{"MoveColumnToMonitorRight", []string{"move-column-to-monitor-right"}, `{}`},
	{"MoveColumnToMonitorDown", []string{"move-column-to-monitor-down"}, `{}`},
	{"MoveColumnToMonitorUp", []string{"move-column-to-monitor-up"}, `{}`},
	{"MoveColumnToMonitorPrevious", []string{"move-column-to-monitor-previous"}, `{}`},
	{"MoveColumnToMonitorNext", []string{"move-column-to-monitor-next"}, `{}`},
	{"MoveColumnToMonitor", []string{"move-column-to-monitor", "DP-1"}, `{"output":"DP-1"}`},
	{"SetWindowWidth", []string{"set-window-width", "-10%", "--id=7"}, `{"id":7,"change":{"AdjustProportion":-10}}`},
	{"SetWindowHeight", []string{"set-window-height", "--id", "7", "-20"}, `{"id":7,"change":{"AdjustFixed":-20}}`},
	{"ResetWindowHeight", []string{"reset-window-height", "--id=7"}, `{"id":7}`},
	{"SwitchPresetColumnWidth", []string{"switch-preset-column-width"}, `{}`},
	{"SwitchPresetColumnWidthBack", []string{"switch-preset-column-width-back"}, `{}`},
	{"SwitchPresetWindowWidth", []string{"switch-preset-window-width", "--id=7"}, `{"id":7}`},
	{"SwitchPresetWindowWidthBack", []string{"switch-preset-window-width-back", "--id=7"}, `{"id":7}`},
	{"SwitchPresetWindowHeight", []string{"switch-preset-window-height", "--id=7"}, `{"id":7}`},
	{"SwitchPresetWindowHeightBack", []string{"switch-preset-window-height-back", "--id=7"}, `{"id":7}`},
	{"MaximizeColumn", []string{"maximize-column"}, `{}`},
	{"MaximizeWindowToEdges", []string{"maximize-window-to-edges", "--id=7"}, `{"id":7}`},
	{"SetColumnWidth", []string{"set-column-width", "500"}, `{"change":{"SetFixed":500}}`},
	{"ExpandColumnToAvailableWidth", []string{"expand-column-to-available-width"}, `{}`},
	{"SwitchLayout", []string{"switch-layout", "prev"}, `{"layout":"Prev"}`},
	{"ShowHotkeyOverlay", []string{"show-hotkey-overlay"}, `{}`},
	{"MoveWorkspaceToMonitorLeft", []string{"move-workspace-to-monitor-left"}, `{}`},
	{"MoveWorkspaceToMonitorRight", []string{"move-workspace-to-monitor-right"}, `{}`},
	{"MoveWorkspaceToMonitorDown", []string{"move-workspace-to-monitor-down"}, `{}`},
	{"MoveWorkspaceToMonitorUp", []string{"move-workspace-to-monitor-up"}, `{}`},
	{"MoveWorkspaceToMonitorPrevious", []string{"move-workspace-to-monitor-previous"}, `{}`},
	{"MoveWorkspaceToMonitorNext", []string{"move-workspace-to-monitor-next"}, `{}`},
	{"MoveWorkspaceToMonitor", []string{"move-workspace-to-monitor", "DP-1", "--reference=work"}, `{"output":"DP-1","reference":{"Name":"work"}}`},
	{"ToggleDebugTint", []string{"toggle-debug-tint"}, `{}`},
	{"DebugToggleOpaqueRegions", []string{"debug-toggle-opaque-regions"}, `{}`},
	{"DebugToggleDamage", []string{"debug-toggle-damage"}, `{}`},
	{"ToggleWindowFloating", []string{"toggle-window-floating", "--id=7"}, `{"id":7}`},
	{"MoveWindowToFloating", []string{"move-window-to-floating", "--id=7"}, `{"id":7}`},
	{"MoveWindowToTiling", []string{"move-window-to-tiling", "--id=7"}, `{"id":7}`},
	{"FocusFloating", []string{"focus-floating"}, `{}`},
	{"FocusTiling", []string{"focus-tiling"}, `{}`},
	{"SwitchFocusBetweenFloatingAndTiling", []string{"switch-focus-between-floating-and-tiling"}, `{}`},
	{"MoveFloatingWindow", []string{"move-floating-window", "-x", "-10.5", "--y=25%", "--id=7"}, `{"id":7,"x":{"AdjustFixed":-10.5},"y":{"SetProportion":25}}`},
	{"ToggleWindowRuleOpacity", []string{"toggle-window-rule-opacity", "--id=7"}, `{"id":7}`},
	{"SetDynamicCastWindow", []string{"set-dynamic-cast-window", "--id=7"}, `{"id":7}`},
	{"SetDynamicCastMonitor", []string{"set-dynamic-cast-monitor", "DP-1"}, `{"output":"DP-1"}`},
	{"ClearDynamicCastTarget", []string{"clear-dynamic-cast-target"}, `{}`},
	{"StopCast", []string{"stop-cast", "--session-id=3"}, `{"session_id":3}`},
	{"ToggleOverview", []string{"toggle-overview"}, `{}`},
	{"OpenOverview", []string{"open-overview"}, `{}`},
	{"CloseOverview", []string{"close-overview"}, `{}`},
	{"ToggleWindowUrgent", []string{"toggle-window-urgent", "--id=7"}, `{"id":7}`},
	{"SetWindowUrgent", []string{"set-window-urgent", "--id=7"}, `{"id":7}`},
	{"UnsetWindowUrgent", []string{"unset-window-urgent", "--id=7"}, `{"id":7}`},
	{"LoadConfigFile", []string{"load-config-file", "--path=/tmp/config.kdl"}, `{"path":"/tmp/config.kdl"}`},
}

func TestEveryActionCLIProducesNiriWire(t *testing.T) {
	if len(actionExamples) != len(ipc.Actions) {
		t.Fatalf("examples=%d, actions=%d", len(actionExamples), len(ipc.Actions))
	}
	seen := map[string]bool{}
	for _, example := range actionExamples {
		t.Run(example.name, func(t *testing.T) {
			seen[example.name] = true
			request, err := parseMsg(append([]string{"action"}, example.args...))
			if err != nil {
				t.Fatal(err)
			}
			want := `{"Action":{"` + example.name + `":` + example.body + `}}`
			assertJSON(t, ipc.EncodeRequest(request), []byte(want))
			decoded, err := ipc.DecodeRequest(ipc.EncodeRequest(request))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(request, decoded) {
				t.Fatalf("wire lost action fields: before=%+v after=%+v", request.Action, decoded.Action)
			}
		})
	}
	for _, name := range ipc.Actions {
		if !seen[name] {
			t.Errorf("no CLI example for %s", name)
		}
	}
}

func assertJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("invalid JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("wire=%s, want %s", got, want)
	}
}

func TestActionCLIRejectsMalformedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"not-an-action"}, {"power-off-monitors", "--id", "1"}, {"focus-window"},
		{"focus-window", "--id", "-1"}, {"close-window", "--id", "18446744073709551616"},
		{"close-window", "--id", "1", "--id", "2"}, {"close-window", "--id"},
		{"focus-window-in-column", "256"}, {"focus-column", "18446744073709551616"},
		{"set-column-display", "bad"}, {"focus-workspace", "256"},
		{"move-window-to-workspace", "2", "--id", "1"}, {"move-column-to-workspace", "2", "--focus", "maybe"},
		{"set-window-width", "2147483648"}, {"set-window-height", "-2147483649"},
		{"set-window-width", "NaN%"}, {"set-column-width", "Inf%"}, {"set-window-height", "10%junk"},
		{"move-floating-window", "--x", "NaN"}, {"move-floating-window", "--y", "Inf%"},
		{"screenshot", "--write-to-disk", "false"}, {"screenshot-screen", "--show-pointer"},
		{"screenshot-window", "-p", "yes"}, {"do-screen-transition", "--delay-ms", "65536"},
		{"do-screen-transition", "--delay-ms", "-1"}, {"quit", "--skip-confirmation=true", "--skip-confirmation"},
	} {
		t.Run(args[0], func(t *testing.T) {
			if _, err := parseAction(args); err == nil {
				t.Fatalf("accepted %v", args)
			}
		})
	}
}

func TestActionCLIDefaultsAndOptionOrdering(t *testing.T) {
	for _, example := range []struct {
		args []string
		body string
	}{
		{[]string{"screenshot"}, `{"Screenshot":{"show_pointer":true,"path":null}}`},
		{[]string{"screenshot", "-pfalse"}, `{"Screenshot":{"show_pointer":false,"path":null}}`},
		{[]string{"do-screen-transition", "-d65535"}, `{"DoScreenTransition":{"delay_ms":65535}}`},
		{[]string{"focus-window", "--id", "18446744073709551615"}, `{"FocusWindow":{"id":18446744073709551615}}`},
		{[]string{"focus-window-in-column", "255"}, `{"FocusWindowInColumn":{"index":255}}`},
		{[]string{"set-column-width", "-2147483648"}, `{"SetColumnWidth":{"change":{"AdjustFixed":-2147483648}}}`},
		{[]string{"set-column-width", "0%"}, `{"SetColumnWidth":{"change":{"SetProportion":0}}}`},
		{[]string{"move-floating-window", "-x-10.5"}, `{"MoveFloatingWindow":{"id":null,"x":{"AdjustFixed":-10.5},"y":{"AdjustFixed":0}}}`},
		{[]string{"screenshot-screen"}, `{"ScreenshotScreen":{"write_to_disk":true,"show_pointer":true,"path":null}}`},
		{[]string{"screenshot-window"}, `{"ScreenshotWindow":{"id":null,"write_to_disk":true,"show_pointer":false,"path":null}}`},
		{[]string{"move-window-to-workspace", "2"}, `{"MoveWindowToWorkspace":{"window_id":null,"reference":{"Index":2},"focus":true}}`},
		{[]string{"move-column-to-workspace-up"}, `{"MoveColumnToWorkspaceUp":{"focus":true}}`},
		{[]string{"move-floating-window"}, `{"MoveFloatingWindow":{"id":null,"x":{"AdjustFixed":0},"y":{"AdjustFixed":0}}}`},
		{[]string{"do-screen-transition"}, `{"DoScreenTransition":{"delay_ms":null}}`},
		{[]string{"center-window"}, `{"CenterWindow":{"id":null}}`},
		{[]string{"set-window-width", "--id", "7", "-10%"}, `{"SetWindowWidth":{"id":7,"change":{"AdjustProportion":-10}}}`},
		{[]string{"set-workspace-name", "--", "--literal"}, `{"SetWorkspaceName":{"name":"--literal","workspace":null}}`},
	} {
		t.Run(example.args[0], func(t *testing.T) {
			action, err := parseAction(example.args)
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, ipc.EncodeRequest(ipc.Request{Name: "Action", Action: &action}), []byte(`{"Action":`+example.body+`}`))
		})
	}
}

func TestNewActionCLIReachesTheUnixSocket(t *testing.T) {
	socket, received := fakeNiri(t)
	code, _, errOut := runMsg(socket, "action", "set-window-width", "-10%", "--id", "7")
	if code != 0 {
		t.Fatalf("exit=%d: %s", code, errOut)
	}
	got := received()
	if len(got) != 1 {
		t.Fatalf("requests=%+v", got)
	}
	assertJSON(t, ipc.EncodeRequest(got[0]), []byte(`{"Action":{"SetWindowWidth":{"id":7,"change":{"AdjustProportion":-10}}}}`))
}
