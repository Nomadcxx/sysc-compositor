package config

import "testing"

func TestBlockOutFromMatchesNiriCaptureTargets(t *testing.T) {
	cfg, err := Parse("capture.kdl", []byte(`
window-rule { match app-id="^secret$"; block-out-from "screen-capture"; }
window-rule { match app-id="^secret$" title="^share$"; block-out-from "screencast"; }
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		app, title string
		target     CaptureTarget
		blocked    bool
	}{
		{"secret", "private", CaptureDisplay, false},
		{"secret", "private", CaptureScreen, true},
		{"secret", "private", CaptureCast, true},
		{"secret", "share", CaptureDisplay, false},
		{"secret", "share", CaptureScreen, false},
		{"secret", "share", CaptureCast, true},
		{"ordinary", "private", CaptureScreen, false},
		{"ordinary", "private", CaptureCast, false},
	} {
		if got := cfg.ResolveWindowRules(tc.app, tc.title).BlocksCapture(tc.target); got != tc.blocked {
			t.Errorf("%s/%s target %d blocked=%t, want %t", tc.app, tc.title, tc.target, got, tc.blocked)
		}
	}
	clean, err := Parse("capture.kdl", []byte(`window-rule { match app-id="^secret$"; }`))
	if err != nil {
		t.Fatal(err)
	}
	if clean.ResolveWindowRules("secret", "private").BlocksCapture(CaptureScreen) {
		t.Error("removed rule kept capture blocked")
	}
	for _, invalid := range []string{`"anything"`, `true`, `1`, `"screen-capture" "screencast"`} {
		if _, err := Parse("invalid.kdl", []byte(`window-rule { block-out-from `+invalid+`; }`)); err == nil {
			t.Errorf("accepted invalid block-out-from %s", invalid)
		}
	}
}
