package config

import (
	"slices"
	"testing"
)

// TestOutputSectionIsParsed: every key of Niri's output section parses, and
// the deprecated background-color folds into the output's layout.
func TestOutputSectionIsParsed(t *testing.T) {
	cfg, err := Parse("outputs.kdl", []byte(`
output "DP-1" {
    mode "3440x1440@120.000"
    variable-refresh-rate
    scale 1
    transform "flipped-90"
    position x=2560 y=0
    focus-at-startup
}
output "Some Company Thing 0x1234" {
    off
    mode custom=true "1920x1080@75"
    variable-refresh-rate on-demand=true
    scale 1.5
    modeline 173.00 1920 2048 2248 2576 1080 1083 1088 1120 "-hsync" "+vsync"
    background-color "#112233"
    backdrop-color "#445566"
    hot-corners { top-left; }
    layout { gaps 4; }
}
output "HDMI-A-1" {
    mode "1280x720"
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outputs) != 3 {
		t.Fatalf("outputs = %d, want 3", len(cfg.Outputs))
	}
	first, second, third := cfg.Outputs[0], cfg.Outputs[1], cfg.Outputs[2]
	if first.Name != "DP-1" || first.Off || !first.FocusAtStartup {
		t.Errorf("DP-1 = %+v, want on and focused at startup", first)
	}
	if first.Mode == nil || *first.Mode != (OutputMode{Width: 3440, Height: 1440, Refresh: 120}) {
		t.Errorf("DP-1 mode = %v, want 3440x1440@120", first.Mode)
	}
	if first.VRR == nil || first.VRR.OnDemand {
		t.Errorf("DP-1 vrr = %v, want always on", first.VRR)
	}
	if first.Scale == nil || *first.Scale != 1 {
		t.Errorf("DP-1 scale = %v, want 1", first.Scale)
	}
	if first.Transform != TransformFlipped90 {
		t.Errorf("DP-1 transform = %v, want flipped-90", first.Transform)
	}
	if first.Position == nil || *first.Position != (OutputPosition{X: 2560}) {
		t.Errorf("DP-1 position = %v, want 2560,0", first.Position)
	}
	if !second.Off || second.Mode == nil || *second.Mode != (OutputMode{Width: 1920, Height: 1080, Refresh: 75, Custom: true}) {
		t.Errorf("second = off %t mode %v, want off and a custom 1920x1080@75", second.Off, second.Mode)
	}
	if second.VRR == nil || !second.VRR.OnDemand {
		t.Errorf("second vrr = %v, want on demand", second.VRR)
	}
	want := Modeline{Clock: 173, HDisplay: 1920, HSyncStart: 2048, HSyncEnd: 2248, HTotal: 2576,
		VDisplay: 1080, VSyncStart: 1083, VSyncEnd: 1088, VTotal: 1120, HSyncPositive: false, VSyncPositive: true}
	if second.Modeline == nil || *second.Modeline != want {
		t.Errorf("second modeline = %+v, want %+v", second.Modeline, want)
	}
	if second.BackdropColor == nil || second.HotCorners == nil || !second.HotCorners.TopLeft {
		t.Errorf("second backdrop %v hot corners %v, want both set", second.BackdropColor, second.HotCorners)
	}
	if second.Layout == nil || second.Layout.Gaps == nil || *second.Layout.Gaps != 4 ||
		second.Layout.BackgroundColor == nil || *second.Layout.BackgroundColor != (Color{0x11 / 255.0, 0x22 / 255.0, 0x33 / 255.0, 1}) {
		t.Errorf("second layout = %+v, want gaps 4 and the deprecated background-color", second.Layout)
	}
	if third.Mode == nil || *third.Mode != (OutputMode{Width: 1280, Height: 720}) || third.Transform != TransformNormal {
		t.Errorf("third = %+v, want 1280x720 with no refresh and the normal transform", third)
	}
	// The keys a later slice applies still warn.
	for _, key := range []string{"output.modeline", "output.backdrop-color", "output.hot-corners", "output.layout"} {
		if !slices.Contains(cfg.Warnings, key) {
			t.Errorf("warnings = %q, want %q", cfg.Warnings, key)
		}
	}
	if slices.Contains(cfg.Warnings, "output") {
		t.Errorf("warnings = %q, want the output section itself applied", cfg.Warnings)
	}
}

func TestOutputSectionRejectsBadValues(t *testing.T) {
	for _, src := range []string{
		`output { scale 1; }`,
		`output "DP-1" { scale 11; }`,
		`output "DP-1" { transform "sideways"; }`,
		`output "DP-1" { position x=1; }`,
		`output "DP-1" { mode "1920by1080"; }`,
		`output "DP-1" { mode custom=true "1920x1080"; }`,
		`output "DP-1" { mode custom=true "1920x1080@0"; }`,
		`output "DP-1" { modeline 173.00 1920 1900 2248 2576 1080 1083 1088 1120 "-hsync" "+vsync"; }`,
		`output "DP-1" { modeline 173.00 1920 2048 2248 2576 1080 1083 1088 1120 "hsync" "+vsync"; }`,
		`output "DP-1" { variable-refresh-rate sometimes=true; }`,
		`output "DP-1" { brightness 3; }`,
	} {
		if _, err := Parse("bad-output.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
	}
}

// TestOutputNameMatchesLikeNiri: the connector or "make model serial",
// ignoring ASCII case, with Unknown for a missing part; an output with no
// make, model or serial matches by connector only.
func TestOutputNameMatchesLikeNiri(t *testing.T) {
	name := OutputName{Connector: "DP-1", Make: "Dell Inc.", Model: "U2720Q", Serial: ""}
	for target, want := range map[string]bool{
		"dp-1": true, "DP-2": false,
		"dell inc. U2720Q Unknown": true, "Dell Inc. U2720Q": false, "Dell Inc. U2720Q X": false,
	} {
		if got := name.Matches(target); got != want {
			t.Errorf("Matches(%q) = %t, want %t", target, got, want)
		}
	}
	if (OutputName{Connector: "HDMI-A-1"}).Matches("Unknown Unknown Unknown") {
		t.Error("an output without make, model and serial matched them")
	}
	cfg := &Config{Outputs: []OutputConfig{{Name: "eDP-1"}, {Name: "Dell Inc. U2720Q Unknown"}, {Name: "DP-1"}}}
	if got := cfg.FindOutput(name); got == nil || got.Name != "Dell Inc. U2720Q Unknown" {
		t.Errorf("FindOutput = %v, want the first matching section", got)
	}
}

// TestOutputNamesSortLikeNiri: outputs with a make, model or serial come
// first, ordered by them and then by connector; the rest by connector.
func TestOutputNamesSortLikeNiri(t *testing.T) {
	names := []OutputName{
		{Connector: "HDMI-A-1"},
		{Connector: "DP-2", Make: "B"},
		{Connector: "DP-1", Make: "A", Model: "Z"},
		{Connector: "DP-3", Make: "A", Model: "Z"},
		{Connector: "eDP-1"},
	}
	slices.SortFunc(names, OutputName.Compare)
	var got []string
	for _, name := range names {
		got = append(got, name.Connector)
	}
	if want := []string{"DP-1", "DP-3", "DP-2", "HDMI-A-1", "eDP-1"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
