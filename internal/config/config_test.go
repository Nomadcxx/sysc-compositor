package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestGreeterConfig is design §12: sysc-greet's greeter config, copied
// unmodified from sysc-greet ccfcdde (config/niri-greeter-config.kdl), must
// parse, and its startup commands and keyboard must come out as written.
func TestGreeterConfig(t *testing.T) {
	c, err := Load("testdata/niri-greeter-config.kdl")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SpawnAtStartup) != 0 {
		t.Errorf("spawn-at-startup %v, want none", c.SpawnAtStartup)
	}
	want := []string{
		"HOME=/var/lib/greeter /usr/local/bin/sysc-greet --wallpaper-daemon",
		"XDG_CACHE_HOME=/tmp/greeter-cache HOME=/var/lib/greeter kitty --start-as=fullscreen " +
			"--config=/etc/greetd/kitty.conf /usr/local/bin/sysc-greet; niri msg action quit --skip-confirmation",
	}
	if !reflect.DeepEqual(c.SpawnShAtStartup, want) {
		t.Errorf("spawn-sh-at-startup\n got %q\nwant %q", c.SpawnShAtStartup, want)
	}
	if kb := (Keyboard{Layout: "us", RepeatDelay: 400, RepeatRate: 40}); c.Keyboard != kb {
		t.Errorf("keyboard %+v, want %+v", c.Keyboard, kb)
	}
	if !c.Animations.Off {
		t.Error("greeter animations should be off")
	}
	if !c.HotkeyOverlay.SkipAtStartup {
		t.Error("the greeter must not show the hotkey overlay at startup")
	}
}

func TestParse(t *testing.T) {
	c, err := Parse("t", []byte(`
spawn-at-startup "kitty" "-e" "htop"
/-spawn-at-startup "skipped"
environment {
    QT_QPA_PLATFORM "wayland"
    DISPLAY null
}
input { keyboard { xkb { layout "us,de"; options "grp:alt_shift_toggle"; }; }; }
binds { }
`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.SpawnAtStartup, [][]string{{"kitty", "-e", "htop"}}) {
		t.Errorf("spawn-at-startup %q", c.SpawnAtStartup)
	}
	if v, ok := c.Environment["QT_QPA_PLATFORM"]; !ok || v == nil || *v != "wayland" {
		t.Errorf("QT_QPA_PLATFORM = %v", v)
	}
	if v, ok := c.Environment["DISPLAY"]; !ok || v != nil {
		t.Errorf("DISPLAY null should unset, got %v (present %v)", v, ok)
	}
	if c.Keyboard.Layout != "us,de" || c.Keyboard.Options != "grp:alt_shift_toggle" || c.Keyboard.RepeatRate != 25 {
		t.Errorf("keyboard %+v", c.Keyboard)
	}
}

func TestParseReportsUnappliedBindActions(t *testing.T) {
	cfg, err := Parse("bindings.kdl", []byte(`
binds {
    Mod+Q { close-window; spawn "kitty"; }
    Mod+W { focus-workspace-down; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.UnappliedActions, []string{"focus-workspace-down"}) {
		t.Fatalf("unapplied actions = %q, want unique action names in source order", cfg.UnappliedActions)
	}
	if len(cfg.Binds) != 1 || len(cfg.Binds[0].Actions) != 2 {
		t.Fatalf("applied binds = %+v, want only the supported close and spawn actions", cfg.Binds)
	}
}

func TestParseM5SettingsAndBinds(t *testing.T) {
	cfg, err := Parse("m5.kdl", []byte(`
input {
    mod-key "Alt"
    mod-key-nested "Super"
    keyboard { xkb { layout "us"; }; }
}

layout {
    gaps 16
    default-column-width { proportion 0.5; }
}
cursor {
    xcursor-theme "Adwaita"
    xcursor-size 28
}
binds {
    Mod+Q repeat=false { close-window; }
    Mod+Return { spawn "kitty" "--class" "test"; }
    Mod+Ctrl+L { move-column-right; }
    Mod+1 { focus-workspace 1; }
    Mod+O { toggle-overview; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Input.ModKey != "Alt" || cfg.Input.ModKeyNested != "Super" {
		t.Errorf("input modifier keys = %+v", cfg.Input)
	}
	if cfg.Layout.Gaps != 16 || cfg.Layout.ColumnWidthProportion != 0.5 {
		t.Errorf("layout sizing = %+v", cfg.Layout)
	}
	if cfg.Cursor.Theme != "Adwaita" || cfg.Cursor.Size != 28 {
		t.Errorf("cursor = %+v", cfg.Cursor)
	}
	if len(cfg.Binds) != 5 || cfg.Binds[1].Actions[0].Args[0] != "kitty" || cfg.Binds[3].Actions[0].WorkspaceIndex != 1 ||
		cfg.Binds[4].Actions[0].Name != "toggle-overview" {
		t.Errorf("applied binds = %+v", cfg.Binds)
	}
	if len(cfg.UnappliedActions) != 0 {
		t.Errorf("unapplied actions = %q, want none", cfg.UnappliedActions)
	}
}

func TestBindRepeatDefaultsTrueAndCanBeDisabled(t *testing.T) {
	cfg, err := Parse("bind-repeat.kdl", []byte(`
binds {
    Mod+Q { close-window; }
    Mod+W repeat=false { close-window; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 2 {
		t.Fatalf("binds = %d, want 2", len(cfg.Binds))
	}
	if !cfg.Binds[0].Repeat {
		t.Error("repeat defaults to false, want true")
	}
	if cfg.Binds[1].Repeat {
		t.Error("repeat=false was not retained")
	}
}

func TestBindRepeatRequiresBoolean(t *testing.T) {
	if _, err := Parse("bind-repeat.kdl", []byte(`
binds {
    Mod+Q repeat="false" { close-window; }
}
`)); err == nil {
		t.Fatal("string repeat property parsed, want boolean validation error")
	}
}

func TestBindAllowWhenLockedDefaultsFalse(t *testing.T) {
	cfg, err := Parse("bind-locked.kdl", []byte(`
binds {
    Mod+Q { close-window; }
    XF86AudioMute allow-when-locked=true { spawn "wpctl" "set-mute" "@DEFAULT_AUDIO_SINK@" "toggle"; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 2 {
		t.Fatalf("binds = %d, want 2", len(cfg.Binds))
	}
	if cfg.Binds[0].AllowWhenLocked {
		t.Error("allow-when-locked defaults to true, want false")
	}
	if !cfg.Binds[1].AllowWhenLocked {
		t.Error("allow-when-locked=true was not retained")
	}
	if _, err := Parse("bind-locked.kdl", []byte(`
binds {
    Mod+Q allow-when-locked="yes" { close-window; }
}
`)); err == nil {
		t.Error("string allow-when-locked parsed, want boolean validation error")
	}
}

func TestM7OverviewGesturesAndInsertHintConfig(t *testing.T) {
	cfg, err := Parse("m7.kdl", []byte(`
overview {
    zoom 0.6
    backdrop-color "#11223380"
}
gestures {
    hot-corners {
        top-right
        bottom-left
    }
    dnd-edge-view-scroll {
        trigger-width 45
        delay-ms 150
        max-speed 900
    }
}
layout {
    insert-hint {
        off
        color 20 30 40 50
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Overview.Zoom != 0.6 || cfg.Overview.BackdropColor != (Color{17.0 / 255, 34.0 / 255, 51.0 / 255, 128.0 / 255}) {
		t.Errorf("overview config = %+v, want zoom 0.6 and #11223380", cfg.Overview)
	}
	if cfg.Gestures.HotCorners != (HotCorners{TopRight: true, BottomLeft: true}) {
		t.Errorf("hot corners = %+v", cfg.Gestures.HotCorners)
	}
	if cfg.Gestures.DNDEdgeViewScroll != (DNDEdgeViewScroll{TriggerWidth: 45, DelayMS: 150, MaxSpeed: 900}) {
		t.Errorf("DND edge view scroll = %+v", cfg.Gestures.DNDEdgeViewScroll)
	}
	if cfg.Layout.InsertHint != (InsertHint{Off: true, Color: Color{20.0 / 255, 30.0 / 255, 40.0 / 255, 50.0 / 255}}) {
		t.Errorf("insert hint = %+v", cfg.Layout.InsertHint)
	}
	if slices.Contains(cfg.Warnings, "overview") || slices.Contains(cfg.Warnings, "gestures") || slices.Contains(cfg.Warnings, "layout.insert-hint") {
		t.Errorf("M7 config sections remain unapplied: %v", cfg.Warnings)
	}
}

func TestM7ConfigDefaultsAndRangeValidation(t *testing.T) {
	cfg := Default()
	if cfg.Overview.Zoom != 0.5 || cfg.Overview.BackdropColor != (Color{0.15, 0.15, 0.15, 1}) {
		t.Errorf("overview defaults = %+v", cfg.Overview)
	}
	if cfg.Gestures.DNDEdgeViewScroll != (DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 100, MaxSpeed: 1500}) {
		t.Errorf("DND edge scroll defaults = %+v", cfg.Gestures.DNDEdgeViewScroll)
	}
	if cfg.Layout.InsertHint != (InsertHint{Color: Color{127.0 / 255, 200.0 / 255, 1, 128.0 / 255}}) {
		t.Errorf("insert hint defaults = %+v", cfg.Layout.InsertHint)
	}

	for src, want := range map[string]string{
		`overview { zoom -0.1; }`:                                   "zoom",
		`overview { zoom 1.1; }`:                                    "zoom",
		`overview { backdrop-color "#12xz"; }`:                      "color",
		`overview { backdrop-color 0 0 256 255; }`:                  "color",
		`gestures { dnd-edge-view-scroll { trigger-width -1; }; }`:  "trigger-width",
		`gestures { dnd-edge-view-scroll { delay-ms 65536; }; }`:    "delay-ms",
		`gestures { dnd-edge-view-scroll { max-speed 1000001; }; }`: "max-speed",
		`layout { insert-hint { color 1 2 3 256; }; }`:              "color",
	} {
		if _, err := Parse("bad-m7.kdl", []byte(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want containing %q", src, err, want)
		}
	}
}

func TestConfiguredLaptopNiriM8Values(t *testing.T) {
	path := os.Getenv("SYSC_NIRI_CONFIG")
	if path == "" {
		t.Skip("SYSC_NIRI_CONFIG is not set")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Overview.Zoom != 0.5 || cfg.Overview.BackdropColor != (Color{0, 0, 0, 1}) {
		t.Errorf("laptop overview = %+v, want zoom 0.5 and #000", cfg.Overview)
	}
	if !cfg.Gestures.HotCorners.Off {
		t.Errorf("laptop hot corners = %+v, want off", cfg.Gestures.HotCorners)
	}
	if cfg.Gestures.DNDEdgeViewScroll != (DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 100, MaxSpeed: 1500}) {
		t.Errorf("laptop DND edge scroll = %+v, want Niri defaults", cfg.Gestures.DNDEdgeViewScroll)
	}
	if cfg.Layout.InsertHint.Off || cfg.Layout.InsertHint.Color != (Color{1, 1, 1, 34.0 / 255}) {
		t.Errorf("laptop insert hint = %+v, want #ffffff22 enabled", cfg.Layout.InsertHint)
	}
	if !cfg.PreferNoCSD {
		t.Error("laptop prefer-no-csd was not applied")
	}
	if got := cfg.Layout.FocusRing; got.Off || got.Width != 4 ||
		got.ActiveColor != (Color{55.0 / 255, 244.0 / 255, 153.0 / 255, 1}) ||
		got.InactiveColor != (Color{80.0 / 255, 80.0 / 255, 80.0 / 255, 1}) {
		t.Errorf("laptop focus ring = %+v", got)
	}
	if got := cfg.Layout.Shadow; !got.On || got.Softness != 20 || got.Spread != 5 || got.OffsetX != 0 || got.OffsetY != 5 {
		t.Errorf("laptop shadow = %+v", got)
	}
	wantSprings := []SpringConfig{
		{DampingRatio: 1, Stiffness: 700, Epsilon: 0.0001},
		{DampingRatio: 1, Stiffness: 700, Epsilon: 0.1},
		{DampingRatio: 1, Stiffness: 800, Epsilon: 0.001},
		{DampingRatio: 1, Stiffness: 600, Epsilon: 0.0001},
		{DampingRatio: 1, Stiffness: 900, Epsilon: 0.0001},
		{DampingRatio: 1, Stiffness: 800, Epsilon: 0.00001},
		{DampingRatio: 1, Stiffness: 700, Epsilon: 0.0001},
	}
	gotAnimations := []Animation{
		cfg.Animations.WindowOpen, cfg.Animations.WindowClose, cfg.Animations.WindowResize,
		cfg.Animations.WindowMovement, cfg.Animations.OverviewOpenClose,
		cfg.Animations.WorkspaceSwitch, cfg.Animations.HorizontalViewMovement,
	}
	for i, want := range wantSprings {
		if got := gotAnimations[i]; got.Spring == nil || *got.Spring != want || got.Off {
			t.Errorf("laptop animation %d = %+v, want spring %+v", i, got, want)
		}
	}
	appearance := cfg.ResolveWindowRules("test-app", "test-title")
	if appearance.GeometryCornerRadius == nil || *appearance.GeometryCornerRadius != (CornerRadius{20, 20, 20, 20}) ||
		appearance.ClipToGeometry == nil || !*appearance.ClipToGeometry {
		t.Errorf("laptop window geometry appearance = %+v", appearance)
	}
	for _, warning := range cfg.Warnings {
		if strings.HasPrefix(warning, "animations") {
			t.Errorf("laptop's supported spring settings produced warning %q", warning)
		}
	}
}

// TestBindsAreNotReportedUnapplied: binds have been applied since M5, so
// validate warns only about the actions not built yet.
func TestBindsAreNotReportedUnapplied(t *testing.T) {
	cfg, err := Parse("binds.kdl", []byte("binds {\n    Mod+T { spawn \"foot\"; }\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cfg.Warnings, "binds") {
		t.Errorf("binds reported as not applied: %v", cfg.Warnings)
	}
}

func TestM6WorkspaceAndRuleSectionsAreApplied(t *testing.T) {
	cfg, err := Parse("m6.kdl", []byte(`
workspace "Work" { open-on-output "DP-1"; }
window-rule {
    match app-id="terminal"
    exclude title="hidden"
    open-floating true
    open-on-output "DP-1"
    default-column-width { proportion 0.4; }
}
window-rule {
    match app-id="terminal" title="dialog"
    open-floating false
    open-fullscreen true
    open-on-workspace "Work"
    default-window-height { fixed 480; }
}
layer-rule {
    match namespace="sysc-shell"
    place-within-backdrop true
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("M6 sections remain unapplied: %v", cfg.Warnings)
	}
	if !reflect.DeepEqual(cfg.Workspaces, []WorkspaceConfig{{Name: "Work", OpenOnOutput: "DP-1"}}) {
		t.Errorf("workspaces = %+v", cfg.Workspaces)
	}
	resolved := cfg.ResolveWindowRules("x-terminal-test", "open dialog")
	if resolved.OpenFloating == nil || *resolved.OpenFloating || resolved.OpenFullscreen == nil || !*resolved.OpenFullscreen ||
		resolved.OpenOnWorkspace == nil || *resolved.OpenOnWorkspace != "Work" || resolved.OpenOnOutput == nil || *resolved.OpenOnOutput != "DP-1" {
		t.Errorf("ordered window rule result = %+v", resolved)
	}
	if resolved.DefaultColumnWidth == nil || resolved.DefaultColumnWidth.Fixed || resolved.DefaultColumnWidth.Value != 0.4 ||
		resolved.DefaultWindowHeight == nil || !resolved.DefaultWindowHeight.Fixed || resolved.DefaultWindowHeight.Value != 480 {
		t.Errorf("default sizes = %+v, %+v", resolved.DefaultColumnWidth, resolved.DefaultWindowHeight)
	}
	if got := cfg.ResolveWindowRules("x-terminal-test", "hidden"); got.OpenFloating != nil {
		t.Errorf("excluded window got rules = %+v", got)
	}
	if !cfg.ResolveLayerRules("sysc-shell-panel") || cfg.ResolveLayerRules("other") {
		t.Error("layer namespace rule did not match only sysc-shell namespaces")
	}
}

func TestWindowRuleProportionClampsLikeNiri(t *testing.T) {
	cfg, err := Parse("zero-proportion.kdl", []byte(`
window-rule {
    default-column-width { proportion 0.0; }
    default-window-height { proportion 0.0; }
}
window-rule {
    default-column-width { proportion -1.0; }
    default-window-height { proportion 10001.0; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	result := cfg.ResolveWindowRules("app", "title")
	if result.DefaultColumnWidth == nil || result.DefaultColumnWidth.Value != 0 ||
		result.DefaultWindowHeight == nil || result.DefaultWindowHeight.Value != 10000 {
		t.Fatalf("clamped proportion rules = %+v", result)
	}
}

func TestM6RulesRejectBadRegexDuplicateNamesAndValues(t *testing.T) {
	for src, want := range map[string]string{
		`window-rule { match app-id="["; }`:                       "regular expression",
		`workspace "Work"; workspace "work";`:                     "duplicate workspace name",
		`window-rule { open-floating "yes"; }`:                    "boolean",
		`layer-rule { match namespace="("; }`:                     "regular expression",
		`window-rule { default-column-width { fixed -1; }; }`:     "whole number",
		`window-rule { default-column-width { proportion 1; }; }`: "decimal number",
	} {
		if _, err := Parse("bad-m6.kdl", []byte(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want containing %q", src, err, want)
		}
	}
}

func TestM6WindowRulesAcceptEmptyDefaultSizes(t *testing.T) {
	cfg, err := Parse("empty-default-size.kdl", []byte(`
window-rule {
    match app-id="test"
    default-column-width {}
    default-window-height {}
}
`))
	if err != nil {
		t.Fatalf("Parse() rejected Niri's empty default sizes: %v", err)
	}
	if len(cfg.WindowRules) != 1 || cfg.WindowRules[0].DefaultColumnWidth == nil || cfg.WindowRules[0].DefaultWindowHeight == nil {
		t.Fatalf("empty size overrides = %+v, want both overrides preserved", cfg.WindowRules)
	}
	if !cfg.WindowRules[0].DefaultColumnWidth.WindowChooses || !cfg.WindowRules[0].DefaultWindowHeight.WindowChooses {
		t.Fatalf("empty size overrides = %+v, want client-chosen dimensions", cfg.WindowRules[0])
	}
}

func TestM6WorkspaceAndMonitorBindsAreApplied(t *testing.T) {
	cfg, err := Parse("m6-binds.kdl", []byte(`
binds {
    Mod+1 { focus-workspace "Work"; }
    Mod+N { set-workspace-name "Work"; }
    Mod+Shift+N { unset-workspace-name; }
    Mod+Alt+Right { move-column-to-monitor "DP-2"; }
    Mod+Alt+Left { move-column-to-monitor-left; }
    Mod+Alt+W { move-window-to-workspace "Work" focus=false; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnappliedActions) != 0 {
		t.Fatalf("unapplied actions = %v, want none", cfg.UnappliedActions)
	}
	if len(cfg.Binds) != 6 {
		t.Fatalf("bind count = %d, want 6: %+v", len(cfg.Binds), cfg.Binds)
	}
	if got := cfg.Binds[1].Actions[0].WorkspaceName; got != "Work" {
		t.Errorf("set-workspace-name = %q, want Work", got)
	}
	if got := reflect.ValueOf(cfg.Binds[3].Actions[0]).FieldByName("Output"); !got.IsValid() || got.String() != "DP-2" {
		t.Errorf("move-column-to-monitor output = %v, want DP-2", got)
	}
	if got := cfg.Binds[4].Actions[0].Name; got != "move-column-to-monitor-left" {
		t.Errorf("directional move = %q, want move-column-to-monitor-left", got)
	}
	if got := cfg.Binds[5].Actions[0]; got.WorkspaceName != "Work" ||
		!reflect.ValueOf(got).FieldByName("Focus").IsValid() || reflect.ValueOf(got).FieldByName("Focus").Bool() {
		t.Errorf("move-window-to-workspace = %+v, want Work with focus=false", got)
	}
}

func TestM6RelativeWorkspaceMoveBindsAreApplied(t *testing.T) {
	cfg, err := Parse("relative-workspace-binds.kdl", []byte(`
binds {
    Mod+Ctrl+Page_Down { move-column-to-workspace-down focus=false; }
    Mod+Ctrl+Page_Up { move-window-to-workspace-up; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnappliedActions) != 0 || len(cfg.Binds) != 2 {
		t.Fatalf("relative workspace binds = %+v, unapplied = %v; want two applied actions", cfg.Binds, cfg.UnappliedActions)
	}
	if got := cfg.Binds[0].Actions[0]; got.Name != "move-column-to-workspace-down" || got.Focus {
		t.Errorf("move-column-to-workspace-down = %+v, want focus=false", got)
	}
	if got := cfg.Binds[1].Actions[0]; got.Name != "move-window-to-workspace-up" || !got.Focus {
		t.Errorf("move-window-to-workspace-up = %+v, want default focus=true", got)
	}
}

func TestParseRejectsMalformedM5Binds(t *testing.T) {
	for src, want := range map[string]string{
		`binds {
    Mod+Bogus+H { close-window; }
}`: "invalid modifier",
		`binds {
    Mod+Q { spawn; }
}`: "spawn needs a command",
		`input {
    mod-key "Meta"
}`: "invalid modifier",
		`layout {
    gaps -1
}`: "from 0 to",
		`cursor {
    xcursor-size 256
}`: "from 0 to 255",
	} {
		if _, err := Parse("m5-invalid.kdl", []byte(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want %q", src, err, want)
		}
	}
}

func TestParseKeepsSupportedActionsOnUnsupportedTriggersVisible(t *testing.T) {
	cfg, err := Parse("scroll.kdl", []byte(`
binds {
    Mod+WheelScrollLeft { focus-column-left; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 0 || !reflect.DeepEqual(cfg.UnappliedActions, []string{"focus-column-left"}) {
		t.Fatalf("binds = %+v, unapplied = %q; want no applied wheel bind and a visible action warning", cfg.Binds, cfg.UnappliedActions)
	}
}

func TestParseM8SettingsAreApplied(t *testing.T) {
	cfg, err := Parse("m8.kdl", []byte(`
prefer-no-csd
layout {
    focus-ring {
        width 4
        active-color "#7fc8ff"
        inactive-color "#505050"
    }
    shadow {
        on
        softness 20
        spread 5
        offset x=0 y=5
    }
}
animations {
    window-open { spring damping-ratio=1.0 stiffness=700 epsilon=0.0001; }
    window-close { spring damping-ratio=1.0 stiffness=700 epsilon=0.1; }
    window-resize { spring damping-ratio=1.0 stiffness=800 epsilon=0.001; }
    window-movement { spring damping-ratio=1.0 stiffness=600 epsilon=0.0001; }
    overview-open-close { spring damping-ratio=1.0 stiffness=900 epsilon=0.0001; }
    workspace-switch { spring damping-ratio=1.0 stiffness=800 epsilon=0.00001; }
    horizontal-view-movement { spring damping-ratio=1.0 stiffness=700 epsilon=0.0001; }
}
window-rule {
    geometry-corner-radius 20
    clip-to-geometry true
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("M8 settings were reported unapplied: %v", cfg.Warnings)
	}
	if !cfg.PreferNoCSD {
		t.Error("prefer-no-csd was not applied")
	}
	if got := cfg.Layout.FocusRing; got.Off || got.Width != 4 ||
		got.ActiveColor != (Color{127.0 / 255, 200.0 / 255, 1, 1}) ||
		got.InactiveColor != (Color{80.0 / 255, 80.0 / 255, 80.0 / 255, 1}) {
		t.Errorf("focus ring = %+v", got)
	}
	if got := cfg.Layout.Shadow; !got.On || got.Softness != 20 || got.Spread != 5 || got.OffsetX != 0 || got.OffsetY != 5 {
		t.Errorf("shadow = %+v", got)
	}
	want := map[string]SpringConfig{
		"window-open":              {DampingRatio: 1, Stiffness: 700, Epsilon: 0.0001},
		"window-close":             {DampingRatio: 1, Stiffness: 700, Epsilon: 0.1},
		"window-resize":            {DampingRatio: 1, Stiffness: 800, Epsilon: 0.001},
		"window-movement":          {DampingRatio: 1, Stiffness: 600, Epsilon: 0.0001},
		"overview-open-close":      {DampingRatio: 1, Stiffness: 900, Epsilon: 0.0001},
		"workspace-switch":         {DampingRatio: 1, Stiffness: 800, Epsilon: 0.00001},
		"horizontal-view-movement": {DampingRatio: 1, Stiffness: 700, Epsilon: 0.0001},
	}
	got := map[string]Animation{
		"window-open": cfg.Animations.WindowOpen, "window-close": cfg.Animations.WindowClose,
		"window-resize": cfg.Animations.WindowResize, "window-movement": cfg.Animations.WindowMovement,
		"overview-open-close": cfg.Animations.OverviewOpenClose, "workspace-switch": cfg.Animations.WorkspaceSwitch,
		"horizontal-view-movement": cfg.Animations.HorizontalViewMovement,
	}
	for name, params := range want {
		if got[name].Off || got[name].Spring == nil || *got[name].Spring != params {
			t.Errorf("%s animation = %+v, want spring %+v", name, got[name], params)
		}
	}
	if got := cfg.ResolveWindowRules("app", "title"); got.GeometryCornerRadius == nil || *got.GeometryCornerRadius != (CornerRadius{20, 20, 20, 20}) ||
		got.ClipToGeometry == nil || !*got.ClipToGeometry {
		t.Errorf("window appearance rule = %+v", got)
	}
}

func TestM8WindowRuleCornersUseCSSOrderAndLastMatch(t *testing.T) {
	cfg, err := Parse("m8-rules.kdl", []byte(`
window-rule {
    geometry-corner-radius 1 2 3 4
    clip-to-geometry false
}
window-rule {
    match app-id="special"
    geometry-corner-radius 20
    clip-to-geometry true
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ResolveWindowRules("other", "title"); got.GeometryCornerRadius == nil || *got.GeometryCornerRadius != (CornerRadius{1, 2, 3, 4}) ||
		got.ClipToGeometry == nil || *got.ClipToGeometry {
		t.Errorf("general window appearance = %+v", got)
	}
	if got := cfg.ResolveWindowRules("special", "title"); got.GeometryCornerRadius == nil || *got.GeometryCornerRadius != (CornerRadius{20, 20, 20, 20}) ||
		got.ClipToGeometry == nil || !*got.ClipToGeometry {
		t.Errorf("last matching window appearance = %+v", got)
	}
}

func TestPreferNoCSDAcceptsExplicitFalse(t *testing.T) {
	cfg, err := Parse("prefer-no-csd.kdl", []byte("prefer-no-csd false"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PreferNoCSD {
		t.Fatal("prefer-no-csd false enabled the preference")
	}
}

func TestM8AnimationOffAndUnsupportedSettings(t *testing.T) {
	// Niri rejects a repeated `animations` within one file, so the later
	// block that turns animations back on comes from an include.
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	for path, content := range map[string]string{
		root: `animations { off; }
include "animations.kdl"
`,
		filepath.Join(dir, "animations.kdl"): `animations {
    on
    window-open { off; }
    window-close {
        duration-ms 150
        curve "ease-out-expo"
    }
    slowdown 2
}
`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Animations.Off || !cfg.Animations.WindowOpen.Off {
		t.Errorf("global/per-kind animation off = %v/%v", cfg.Animations.Off, cfg.Animations.WindowOpen.Off)
	}
	if cfg.Animations.WindowClose.Spring != nil || !cfg.Animations.WindowClose.Unsupported {
		t.Errorf("unsupported easing was applied: %+v", cfg.Animations.WindowClose)
	}
	for _, warning := range []string{"animations.slowdown", "animations.window-close.easing"} {
		if !slices.Contains(cfg.Warnings, warning) {
			t.Errorf("warnings %v do not contain %q", cfg.Warnings, warning)
		}
	}
}

func TestParseRejectsInvalidM8Settings(t *testing.T) {
	for src, want := range map[string]string{
		`animations { window-open { spring damping-ratio=0.0 stiffness=700 epsilon=0.0001; }; }`: "damping ratio",
		`animations { window-open { spring damping-ratio=1.0 stiffness=0 epsilon=0.0001; }; }`:   "stiffness",
		`animations { window-open { spring damping-ratio=1.0 stiffness=700 epsilon=0.2; }; }`:    "epsilon",
		`layout { focus-ring { width -1; }; }`:                                                   "width",
		`layout { shadow { softness 1025; }; }`:                                                  "softness",
		`layout { focus-ring 1 {}; }`:                                                            "takes child",
		`layout { focus-ring { width 4 unit="px"; }; }`:                                          "width",
		`layout { shadow extra=true {}; }`:                                                       "takes child",
		`layout { shadow { softness 20 extra=true; }; }`:                                         "softness",
		`animations { slowdown 2 extra=true; }`:                                                  "slowdown",
		`animations { window-open { duration-ms 150 extra=true; }; }`:                            "duration-ms",
		`animations { window-open { custom-shader "shader" extra=true; }; }`:                     "custom-shader",
		`window-rule { geometry-corner-radius -1; }`:                                             "radius",
		`window-rule { clip-to-geometry "yes"; }`:                                                "boolean",
	} {
		if _, err := Parse("bad-m8.kdl", []byte(src)); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Errorf("Parse(%q) error = %v, want containing %q", src, err, want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for src, want := range map[string]string{
		`frobnicate 1`:                                 "unexpected node `frobnicate`",
		`spawn-at-startup`:                             "wrong number of arguments",
		`spawn-sh-at-startup "a" "b"`:                  "wrong number of arguments",
		`spawn-at-startup 1`:                           "not a string",
		`input { keyboard { repeat-rate 300; }; }`:     "from 0 to 255",
		`input { keyboard { xkb { colour "x"; }; }; }`: "unexpected node `colour`",
		`layout {`: "parse failed",
	} {
		_, err := Parse("t", []byte(src))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want it to mention %q", src, err, want)
		}
	}
}

func TestLoadMergesNestedIncludesAtTheirSourcePosition(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	outer := filepath.Join(dir, "parts", "outer.kdl")
	inner := filepath.Join(dir, "parts", "nested", "inner.kdl")
	for path, content := range map[string]string{
		root: `spawn-at-startup "root-before"
include "parts/outer.kdl"
include "missing-optional.kdl" optional=true
spawn-at-startup "root-after"
`,
		outer: `spawn-at-startup "outer-before"
include "nested/inner.kdl"
spawn-at-startup "outer-after"
`,
		inner: `spawn-at-startup "inner"
`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"root-before"}, {"outer-before"}, {"inner"}, {"outer-after"}, {"root-after"}}
	if !reflect.DeepEqual(c.SpawnAtStartup, want) {
		t.Errorf("spawn-at-startup = %q, want positional merge %q", c.SpawnAtStartup, want)
	}
}

func TestLoadLaterBindsOverrideIncludedBinds(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	base := filepath.Join(dir, "base.kdl")
	if err := os.WriteFile(base, []byte("binds {\n    Mod+Q { spawn \"old\"; }\n}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("include \"base.kdl\"\nbinds {\n    Mod+Q { spawn \"new\"; }\n}"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Binds) != 1 || len(c.Binds[0].Actions) != 1 || !reflect.DeepEqual(c.Binds[0].Actions[0].Args, []string{"new"}) {
		t.Fatalf("binds = %+v, want the later bind only", c.Binds)
	}
}

func TestParseRejectsDuplicateBindsWithinSection(t *testing.T) {
	source := []byte(`binds {
    Mod+Q { spawn "one"; }
    Mod+Q { spawn "two"; }
}`)
	if _, err := Parse("duplicate-binds.kdl", source); err == nil {
		t.Fatal("Parse accepted duplicate binds in one section")
	}
}

func TestLoadWithSourcesTracksNestedAndOptionalIncludes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	child := filepath.Join(dir, "parts", "child.kdl")
	grandchild := filepath.Join(dir, "parts", "nested", "inner.kdl")
	missing := filepath.Join(dir, "parts", "optional.kdl")
	for path, source := range map[string]string{
		root:       "include \"parts/child.kdl\"\ninclude \"parts/optional.kdl\" optional=true",
		child:      "include \"nested/inner.kdl\"",
		grandchild: "spawn-at-startup \"nested\"",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := LoadWithSources(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Root != filepath.Clean(root) {
		t.Fatalf("root = %q, want absolute root %q", result.Root, filepath.Clean(root))
	}
	want := []string{root, child, grandchild, missing}
	for i := range want {
		want[i] = filepath.Clean(want[i])
	}
	if !reflect.DeepEqual(result.Sources, want) {
		t.Fatalf("sources = %q, want %q", result.Sources, want)
	}
	if result.Config == nil || !reflect.DeepEqual(result.Config.SpawnAtStartup, [][]string{{"nested"}}) {
		t.Fatalf("loaded config = %+v, want nested config", result.Config)
	}
}

func TestLoadWithSourcesReportsMissingIncludeOnReadFailure(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	missing := filepath.Join(dir, "missing.kdl")
	if err := os.WriteFile(root, []byte("include \"missing.kdl\""), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := LoadWithSources(root)
	if err == nil {
		t.Fatal("LoadWithSources() unexpectedly loaded a missing required include")
	}
	want := []string{filepath.Clean(root), filepath.Clean(missing)}
	if !reflect.DeepEqual(result.Sources, want) {
		t.Fatalf("sources after failed include read = %q, want %q", result.Sources, want)
	}
}

func TestLoadWithSourcesReportsIncludeOnParseFailure(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	child := filepath.Join(dir, "broken.kdl")
	if err := os.WriteFile(root, []byte("include \"broken.kdl\""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("binds {"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := LoadWithSources(root)
	if err == nil {
		t.Fatal("LoadWithSources() unexpectedly parsed a malformed include")
	}
	want := []string{filepath.Clean(root), filepath.Clean(child)}
	if !reflect.DeepEqual(result.Sources, want) {
		t.Fatalf("sources after include parse failure = %q, want %q", result.Sources, want)
	}
}

func TestLoadSwitchEventsMergeIncludedConfig(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.kdl")
	root := filepath.Join(dir, "config.kdl")
	if err := os.WriteFile(base, []byte("switch-events { lid-close { spawn \"old\"; }; tablet-mode-on { spawn \"tablet\" \"on\"; }; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("include \"base.kdl\"\nswitch-events { lid-close { spawn \"new\"; }; lid-open { spawn \"open\"; }; tablet-mode-off { spawn \"tablet\" \"off\"; }; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(c.Warnings, "switch-events") {
		t.Fatalf("switch-events was left unapplied: %v", c.Warnings)
	}
	for name, want := range map[string][]string{
		"LidClose": {"new"}, "LidOpen": {"open"}, "TabletModeOn": {"tablet", "on"}, "TabletModeOff": {"tablet", "off"},
	} {
		var got []string
		switch name {
		case "LidOpen":
			got = c.SwitchEvents.LidOpen
		case "LidClose":
			got = c.SwitchEvents.LidClose
		case "TabletModeOn":
			got = c.SwitchEvents.TabletModeOn
		case "TabletModeOff":
			got = c.SwitchEvents.TabletModeOff
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("SwitchEvents.%s = %v, want %v", name, got, want)
		}
	}
}

func TestParseRejectsDuplicateSwitchEventsSections(t *testing.T) {
	source := []byte(`switch-events {
    lid-open { spawn "one"; }
}
switch-events {
    lid-close { spawn "two"; }
}`)
	if _, err := Parse("duplicate-switch-events.kdl", source); err == nil {
		t.Fatal("Parse accepted duplicate switch-events sections in one file")
	} else if !strings.Contains(err.Error(), "duplicate node") {
		t.Fatalf("Parse failed for the wrong reason: %v", err)
	}
}

func TestParseRejectsMalformedSwitchEvents(t *testing.T) {
	for _, source := range []string{
		"switch-events { lid-open { spawn; } }",
		"switch-events { power-button { spawn \"power\"; } }",
		"switch-events { lid-open { spawn \"one\"; }; lid-open { spawn \"two\"; } }",
	} {
		if _, err := Parse("switch-events.kdl", []byte(source)); err == nil {
			t.Errorf("Parse(%q) succeeded, want malformed switch-events error", source)
		}
	}
}

func TestLoadRejectsRecursiveIncludes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.kdl")
	child := filepath.Join(dir, "child.kdl")
	if err := os.WriteFile(root, []byte(`include "child.kdl"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte(`include "root.kdl"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "recursive include") {
		t.Fatalf("Load() error = %v, want recursive include", err)
	}
}

func TestLocate(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "niri", "config.kdl")
	env := map[string]string{"XDG_CONFIG_HOME": dir, "HOME": "/nonexistent"}
	getenv := func(k string) string { return env[k] }
	if got := Locate("", getenv); got != "" && got != "/etc/niri/config.kdl" {
		t.Errorf("no user config: %q", got)
	}
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Locate("", getenv); got != user {
		t.Errorf("user config: %q, want %q", got, user)
	}
	env["NIRI_CONFIG"] = "/from/env.kdl"
	if got := Locate("", getenv); got != "/from/env.kdl" {
		t.Errorf("NIRI_CONFIG: %q", got)
	}
	if got := Locate("/from/flag.kdl", getenv); got != "/from/flag.kdl" {
		t.Errorf("-c: %q", got)
	}
}

func TestBlurDefaultsMatchNiriAndUnnamedFieldsStay(t *testing.T) {
	cfg, err := Parse("blur.kdl", []byte(``))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Blur{Passes: 3, Offset: 3, Noise: 0.02, Saturation: 1.5}); cfg.Blur != want {
		t.Errorf("default blur = %+v, want Niri's %+v", cfg.Blur, want)
	}
	cfg, err = Parse("blur.kdl", []byte(`
blur {
    passes 2
    noise 0
    off
    saturation 1.2
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Blur{Off: true, Passes: 2, Offset: 3, Noise: 0, Saturation: 1.2}); cfg.Blur != want {
		t.Errorf("partial blur = %+v, want %+v (offset keeps its default)", cfg.Blur, want)
	}
	for _, warning := range cfg.Warnings {
		if warning == "blur" {
			t.Error("blur is still reported as accepted but not applied")
		}
	}
	if _, err := Parse("blur.kdl", []byte("blur {\n    noise 2\n}\n")); err == nil {
		t.Error("noise 2 parsed, want a range error")
	}
}

func TestBindAllowInhibitingAndToggleShortcutsInhibit(t *testing.T) {
	cfg, err := Parse("bind-inhibit.kdl", []byte(`
binds {
    Mod+Q { close-window; }
    Mod+Escape allow-inhibiting=false { toggle-keyboard-shortcuts-inhibit; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 2 || len(cfg.UnappliedActions) != 0 {
		t.Fatalf("binds = %+v, unapplied = %q", cfg.Binds, cfg.UnappliedActions)
	}
	if cfg.Binds[0].IgnoresInhibit {
		t.Error("a bind without allow-inhibiting ignores inhibitors, want it inhibitable")
	}
	if !cfg.Binds[1].IgnoresInhibit || cfg.Binds[1].Actions[0].Name != "toggle-keyboard-shortcuts-inhibit" {
		t.Errorf("toggle bind = %+v, want allow-inhibiting=false kept", cfg.Binds[1])
	}
}

func TestScreenshotPathDefaultsToNiriAndTakesNull(t *testing.T) {
	cfg, err := Parse("shot.kdl", []byte(``))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ScreenshotPath == nil || *cfg.ScreenshotPath != DefaultScreenshotPath {
		t.Errorf("default screenshot-path = %v, want Niri's %q", cfg.ScreenshotPath, DefaultScreenshotPath)
	}
	cfg, err = Parse("shot.kdl", []byte("screenshot-path \"~/Shots/%Y.png\"\n"))
	if err != nil || cfg.ScreenshotPath == nil || *cfg.ScreenshotPath != "~/Shots/%Y.png" {
		t.Errorf("screenshot-path = %v, %v", cfg.ScreenshotPath, err)
	}
	cfg, err = Parse("shot.kdl", []byte("screenshot-path null\n"))
	if err != nil || cfg.ScreenshotPath != nil {
		t.Errorf("screenshot-path null = %v, %v; want nil", cfg.ScreenshotPath, err)
	}
	for _, w := range cfg.Warnings {
		if w == "screenshot-path" {
			t.Error("screenshot-path is still reported as not applied")
		}
	}
}

func TestScreenshotScreenBindProperties(t *testing.T) {
	cfg, err := Parse("shot.kdl", []byte(`
binds {
    Print { screenshot-screen; }
    Ctrl+Print { screenshot-screen write-to-disk=false show-pointer=false; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 2 {
		t.Fatalf("binds = %+v, unapplied = %q", cfg.Binds, cfg.UnappliedActions)
	}
	if a := cfg.Binds[0].Actions[0]; !a.WriteToDisk || !a.ShowPointer {
		t.Errorf("default screenshot-screen = %+v, want write-to-disk and show-pointer true", a)
	}
	if a := cfg.Binds[1].Actions[0]; a.WriteToDisk || a.ShowPointer {
		t.Errorf("screenshot-screen with both false = %+v", a)
	}
}

func TestScreenshotWindowBindDefaultsHideThePointer(t *testing.T) {
	cfg, err := Parse("shot.kdl", []byte("binds {\n    Alt+Print { screenshot-window; }\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 1 {
		t.Fatalf("binds = %+v, unapplied = %q", cfg.Binds, cfg.UnappliedActions)
	}
	if a := cfg.Binds[0].Actions[0]; !a.WriteToDisk || a.ShowPointer {
		t.Errorf("screenshot-window = %+v, want write-to-disk true and show-pointer false as in Niri", a)
	}
}

func TestScreenshotBindTakesShowPointerOnly(t *testing.T) {
	cfg, err := Parse("shot.kdl", []byte("binds {\n    Print { screenshot show-pointer=false; }\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 1 || cfg.Binds[0].Actions[0].ShowPointer {
		t.Fatalf("binds = %+v, unapplied = %q", cfg.Binds, cfg.UnappliedActions)
	}
	if _, err := Parse("shot.kdl", []byte("binds {\n    Print { screenshot write-to-disk=false; }\n}\n")); err == nil {
		t.Error("screenshot took write-to-disk, which Niri's screenshot action does not have")
	}
}

func TestQuitBindTakesSkipConfirmation(t *testing.T) {
	cfg, err := Parse("quit.kdl", []byte(`
binds {
    Mod+Shift+E { quit; }
    Ctrl+Alt+Delete { quit skip-confirmation=true; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Binds) != 2 || len(cfg.UnappliedActions) != 0 {
		t.Fatalf("binds = %+v, unapplied = %q", cfg.Binds, cfg.UnappliedActions)
	}
	if a := cfg.Binds[0].Actions[0]; a.Name != "quit" || a.SkipConfirmation {
		t.Errorf("quit = %+v, want the confirmation", a)
	}
	if a := cfg.Binds[1].Actions[0]; !a.SkipConfirmation {
		t.Errorf("quit skip-confirmation=true = %+v", a)
	}
	for _, bad := range []string{"quit force=true", "quit skip-confirmation=1", "quit \"now\""} {
		if _, err := Parse("quit.kdl", []byte("binds {\n    Mod+Q { "+bad+"; }\n}\n")); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
}

func TestExitConfirmationAnimationDefaultsToNiri(t *testing.T) {
	cfg, err := Parse("exit.kdl", []byte(``))
	if err != nil {
		t.Fatal(err)
	}
	if s := cfg.Animations.ExitConfirmationOpenClose.Spring; s == nil || *s != (SpringConfig{DampingRatio: 0.6, Stiffness: 500, Epsilon: 0.01}) {
		t.Errorf("default exit-confirmation-open-close = %+v", cfg.Animations.ExitConfirmationOpenClose)
	}
	cfg, err = Parse("exit.kdl", []byte(`
animations {
    exit-confirmation-open-close { spring damping-ratio=1.0 stiffness=800 epsilon=0.001; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if s := cfg.Animations.ExitConfirmationOpenClose.Spring; s == nil || *s != (SpringConfig{DampingRatio: 1, Stiffness: 800, Epsilon: 0.001}) {
		t.Errorf("exit-confirmation-open-close = %+v", cfg.Animations.ExitConfirmationOpenClose)
	}
	for _, w := range cfg.Warnings {
		if w == "animations.exit-confirmation-open-close" {
			t.Error("exit-confirmation-open-close is still reported as not applied")
		}
	}
}

func TestHotkeyOverlaySectionAndBindTitles(t *testing.T) {
	cfg, err := Parse("overlay.kdl", []byte(`
hotkey-overlay {
    skip-at-startup
    hide-not-bound
}
binds {
    Mod+T hotkey-overlay-title="Terminal" { spawn "foot"; }
    Mod+R hotkey-overlay-title=null { switch-preset-column-width; }
    Ctrl+Alt+Delete { quit skip-confirmation=true; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HotkeyOverlay != (HotkeyOverlay{SkipAtStartup: true, HideNotBound: true}) {
		t.Errorf("hotkey-overlay = %+v", cfg.HotkeyOverlay)
	}
	if len(cfg.OverlayBinds) != 3 {
		t.Fatalf("overlay binds = %+v", cfg.OverlayBinds)
	}
	if b := cfg.OverlayBinds[0]; b.Action != "spawn" || len(b.Args) != 1 || b.Title == nil || *b.Title != "Terminal" {
		t.Errorf("titled spawn = %+v", b)
	}
	// An action this compositor does not apply still reaches the overlay.
	if b := cfg.OverlayBinds[1]; b.Action != "switch-preset-column-width" || !b.TitleHidden || b.Title != nil {
		t.Errorf("null-titled bind = %+v", b)
	}
	if b := cfg.OverlayBinds[2]; b.Action != "quit" || !b.SkipConfirmation {
		t.Errorf("quit = %+v", b)
	}
	for _, w := range cfg.Warnings {
		if w == "hotkey-overlay" {
			t.Error("hotkey-overlay is still reported as not applied")
		}
	}
	for _, bad := range []string{"hotkey-overlay { show-always; }", "hotkey-overlay { skip-at-startup true; }",
		"binds {\n    Mod+T hotkey-overlay-title=1 { spawn \"foot\"; }\n}"} {
		if _, err := Parse("overlay.kdl", []byte(bad+"\n")); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
