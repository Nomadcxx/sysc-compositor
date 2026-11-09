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
