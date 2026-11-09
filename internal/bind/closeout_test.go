package bind

import "testing"

func TestRemainingWindowActionsParse(t *testing.T) {
	for _, name := range []string{"center-window", "switch-preset-window-width", "switch-preset-window-width-back", "toggle-window-rule-opacity"} {
		if _, ok, err := ParseAction(name, nil); !ok || err != nil {
			t.Errorf("%s supported=%t err=%v", name, ok, err)
		}
		if _, ok, err := ParseAction(name, []string{"unexpected"}); !ok || err == nil {
			t.Errorf("%s accepted unexpected args", name)
		}
	}
	action, ok, err := ParseAction("set-window-width", []string{"-10%"})
	if !ok || err != nil || !action.Size.Relative || action.Size.Proportion != -10 {
		t.Errorf("width=%+v supported=%t err=%v", action, ok, err)
	}
}

// TestLayoutAndDebugBindsParse: Niri binds switch-layout "next", "prev" or a
// layout index, and the three debug toggles take no arguments.
func TestLayoutAndDebugBindsParse(t *testing.T) {
	for _, name := range []string{"toggle-debug-tint", "debug-toggle-opaque-regions", "debug-toggle-damage"} {
		if _, ok, err := ParseAction(name, nil); !ok || err != nil || !IsSupportedAction(name) {
			t.Errorf("%s supported=%t applied=%t err=%v", name, ok, IsSupportedAction(name), err)
		}
		if _, _, err := ParseAction(name, []string{"x"}); err == nil {
			t.Errorf("%s accepted an argument", name)
		}
	}
	if !IsSupportedAction(ActionSwitchLayout) {
		t.Error("switch-layout is not applied")
	}
	for arg, want := range map[string]Action{
		"next": {Name: ActionSwitchLayout, LayoutTarget: "Next"},
		"prev": {Name: ActionSwitchLayout, LayoutTarget: "Prev"},
		"2":    {Name: ActionSwitchLayout, LayoutTarget: "Index", Index: 2},
	} {
		action, ok, err := ParseAction(ActionSwitchLayout, []string{arg})
		if !ok || err != nil || action.Name != want.Name || action.LayoutTarget != want.LayoutTarget || action.Index != want.Index {
			t.Errorf("switch-layout %q = %+v, %t, %v", arg, action, ok, err)
		}
	}
	for _, args := range [][]string{nil, {"Next"}, {"256"}, {"-1"}, {"next", "prev"}} {
		if _, _, err := ParseAction(ActionSwitchLayout, args); err == nil {
			t.Errorf("switch-layout %q parsed", args)
		}
	}
}
