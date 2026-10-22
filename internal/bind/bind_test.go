package bind

import (
	"reflect"
	"testing"
)

func TestParseChord(t *testing.T) {
	got, err := ParseChord("Mod+Ctrl+Page_Down")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "Page_Down" || got.Modifiers != Modifiers(ModifierCtrl) || !got.UsesMod {
		t.Fatalf("ParseChord() = %+v, want unresolved Mod and Ctrl+Page_Down", got)
	}
}

func TestChordResolveMod(t *testing.T) {
	chord, err := ParseChord("Mod+Ctrl+H")
	if err != nil {
		t.Fatal(err)
	}
	got := chord.ResolveMod(ModifierAlt)
	if got.UsesMod || got.Modifiers != Modifiers(ModifierAlt|ModifierCtrl) {
		t.Fatalf("ResolveMod() = %+v, want Alt+Ctrl", got)
	}
}

func TestResolveModKeyByBackend(t *testing.T) {
	tests := []struct {
		name, normal, nested string
		isNested             bool
		want                 Modifier
	}{
		{name: "native default", want: ModifierSuper},
		{name: "nested default", isNested: true, want: ModifierAlt},
		{name: "nested opposite of configured Alt", normal: "Alt", isNested: true, want: ModifierSuper},
		{name: "nested explicit", normal: "Alt", nested: "Ctrl", isNested: true, want: ModifierCtrl},
		{name: "native configured", normal: "Mod5", want: ModifierLevel3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveModKey(tt.normal, tt.nested, tt.isNested)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ResolveModKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAction(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		kind          string
		want          bool
		workspaceName string
		columnIndex   uint8
		columnDisplay string
	}{
		{name: "close", kind: ActionCloseWindow, want: true},
		{name: "spawn", args: []string{"kitty", "--class", "test"}, kind: ActionSpawn, want: true},
		{name: "spawn-sh", args: []string{"exec kitty"}, kind: ActionSpawnSh, want: true},
		{name: "workspace index", args: []string{"9"}, kind: ActionFocusWorkspace, want: true},
		{name: "workspace name", args: []string{"Work"}, kind: ActionFocusWorkspace, want: true, workspaceName: "Work"},
		{name: "consume into column", kind: ActionConsumeWindowIntoColumn, want: true},
		{name: "expel from column", kind: ActionExpelWindowFromColumn, want: true},
		{name: "consume or expel left", kind: ActionConsumeOrExpelWindowLeft, want: true},
		{name: "consume or expel right", kind: ActionConsumeOrExpelWindowRight, want: true},
		{name: "toggle tabbed display", kind: ActionToggleColumnTabbedDisplay, want: true},
		{name: "set tabbed display", args: []string{"tabbed"}, kind: ActionSetColumnDisplay, want: true, columnDisplay: "tabbed"},
		{name: "focus window in column", args: []string{"2"}, kind: ActionFocusWindowInColumn, want: true, columnIndex: 2},
		{name: "focus window down", kind: "focus-window-down", want: true},
		{name: "focus window up", kind: "focus-window-up", want: true},
		{name: "move window down", kind: "move-window-down", want: true},
		{name: "move window up", kind: "move-window-up", want: true},
		{name: "toggle floating", kind: ActionToggleWindowFloating, want: true},
		{name: "move to floating", kind: ActionMoveWindowToFloating, want: true},
		{name: "move to tiling", kind: ActionMoveWindowToTiling, want: true},
		{name: "focus floating", kind: ActionFocusFloating, want: true},
		{name: "focus tiling", kind: ActionFocusTiling, want: true},
		{name: "switch floating and tiling", kind: ActionSwitchFocusFloatingAndTiling, want: true},
		{name: "fullscreen window", kind: ActionFullscreenWindow, want: true},
		{name: "windowed fullscreen", kind: ActionToggleWindowedFullscreen, want: true},
		{name: "toggle overview", kind: "toggle-overview", want: true},
		{name: "open overview", kind: "open-overview", want: true},
		{name: "close overview", kind: "close-overview", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseAction(tt.kind, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tt.want {
				t.Fatalf("ParseAction() supported = %v, want %v", ok, tt.want)
			}
			if ok && got.Name != tt.kind {
				t.Fatalf("ParseAction() = %+v, want action %q", got, tt.kind)
			}
			if tt.kind == ActionFocusWorkspace && tt.workspaceName == "" && got.WorkspaceIndex != 9 {
				t.Fatalf("workspace index = %d, want 9", got.WorkspaceIndex)
			}
			if tt.workspaceName != "" && got.WorkspaceName != tt.workspaceName {
				t.Fatalf("workspace name = %q, want %q", got.WorkspaceName, tt.workspaceName)
			}
			if tt.columnIndex != 0 && got.ColumnIndex != tt.columnIndex {
				t.Fatalf("column index = %d, want %d", got.ColumnIndex, tt.columnIndex)
			}
			if tt.columnDisplay != "" && got.ColumnDisplay != tt.columnDisplay {
				t.Fatalf("column display = %q, want %q", got.ColumnDisplay, tt.columnDisplay)
			}
		})
	}
}

func TestParseRelativeWorkspaceMovesDefaultToFocus(t *testing.T) {
	for _, name := range []string{
		"move-window-to-workspace-down", "move-window-to-workspace-up",
		"move-column-to-workspace-down", "move-column-to-workspace-up",
	} {
		action, supported, err := ParseAction(name, nil)
		if err != nil || !supported || !action.Focus {
			t.Errorf("ParseAction(%q) = (%+v, %v, %v), want supported action with focus=true", name, action, supported, err)
		}
	}
}

func TestParseChordRejectsMalformedInput(t *testing.T) {
	for _, chord := range []string{"", "+H", "Mod+", "Mod++H", "Mod+Bogus+H", "Mod+Mod+H", "Mod+Ctrl"} {
		t.Run(chord, func(t *testing.T) {
			if _, err := ParseChord(chord); err == nil {
				t.Fatalf("ParseChord(%q) succeeded, want malformed bind error", chord)
			}
		})
	}
}

func TestParseActionRejectsBadArguments(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: ActionCloseWindow, args: []string{"1"}},
		{name: ActionSpawn},
		{name: ActionSpawnSh, args: []string{"sh", "-c", "false"}},
		{name: ActionFocusWorkspace, args: []string{"0"}},
		{name: ActionFocusWorkspace, args: []string{"256"}},
		{name: ActionSetColumnDisplay, args: []string{"stacked"}},
		{name: ActionFocusWindowInColumn, args: []string{"0"}},
		{name: ActionFocusWindowInColumn, args: []string{"256"}},
		{name: ActionMoveWorkspaceToMonitor},
		{name: ActionFocusMonitor, args: []string{"DP-1", "DP-2"}},
	} {
		if _, ok, err := ParseAction(tt.name, tt.args); !ok || err == nil {
			t.Errorf("ParseAction(%q, %v) = (%v, %v), want supported action with argument error", tt.name, tt.args, ok, err)
		}
	}
}

func TestParseM6WorkspaceAndMonitorActions(t *testing.T) {
	tests := []struct {
		name, action string
		args         []string
		workspace    string
		index        uint8
		output       string
	}{
		{name: "set workspace name", action: "set-workspace-name", args: []string{"Work"}, workspace: "Work"},
		{name: "unset workspace name", action: "unset-workspace-name"},
		{name: "move column to named workspace", action: "move-column-to-workspace", args: []string{"Work"}, workspace: "Work"},
		{name: "move window to indexed workspace", action: "move-window-to-workspace", args: []string{"3"}, index: 3},
		{name: "move column to named monitor", action: "move-column-to-monitor", args: []string{"DP-2"}, output: "DP-2"},
		{name: "move window to named monitor", action: "move-window-to-monitor", args: []string{"DP-2"}, output: "DP-2"},
		{name: "move workspace to named monitor", action: "move-workspace-to-monitor", args: []string{"DP-2"}, output: "DP-2"},
		{name: "focus named monitor", action: "focus-monitor", args: []string{"DP-2"}, output: "DP-2"},
		{name: "move column to monitor direction", action: "move-column-to-monitor-left"},
		{name: "move window to monitor direction", action: "move-window-to-monitor-right"},
		{name: "move workspace to monitor direction", action: "move-workspace-to-monitor-next"},
		{name: "focus monitor direction", action: "focus-monitor-left"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, supported, err := ParseAction(tt.action, tt.args)
			if err != nil || !supported {
				t.Fatalf("ParseAction() = (%+v, %v, %v), want a supported action", got, supported, err)
			}
			if !IsSupportedAction(tt.action) {
				t.Fatalf("IsSupportedAction(%q) = false", tt.action)
			}
			if got.WorkspaceName != tt.workspace || got.WorkspaceIndex != tt.index {
				t.Fatalf("ParseAction() = %+v, want workspace=%q index=%d", got, tt.workspace, tt.index)
			}
			if tt.output != "" {
				output := reflect.ValueOf(got).FieldByName("Output")
				if !output.IsValid() || output.String() != tt.output {
					t.Fatalf("ParseAction() output field = %v, want %q", output, tt.output)
				}
			}
			if tt.action == "move-column-to-workspace" || tt.action == "move-window-to-workspace" {
				focus := reflect.ValueOf(got).FieldByName("Focus")
				if !focus.IsValid() || !focus.Bool() {
					t.Fatal("workspace move defaults to focus=false, want Niri's focus=true")
				}
			}
		})
	}
}
