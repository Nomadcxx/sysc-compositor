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

func TestConfiguredLaptopNiriM7Values(t *testing.T) {
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
