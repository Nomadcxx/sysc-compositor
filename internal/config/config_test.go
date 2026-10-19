package config

import (
	"os"
	"path/filepath"
	"reflect"
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
	if len(cfg.Binds) != 4 || cfg.Binds[1].Actions[0].Args[0] != "kitty" || cfg.Binds[3].Actions[0].WorkspaceIndex != 1 {
		t.Errorf("applied binds = %+v", cfg.Binds)
	}
	if !reflect.DeepEqual(cfg.UnappliedActions, []string{"toggle-overview"}) {
		t.Errorf("unapplied actions = %q, want toggle-overview", cfg.UnappliedActions)
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
