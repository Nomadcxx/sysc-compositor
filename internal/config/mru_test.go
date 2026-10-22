package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
)

func TestRecentWindowsDefaultsToNiri(t *testing.T) {
	r := Default().RecentWindows
	if r.Off || r.DebounceMS != 750 || r.OpenDelayMS != 150 {
		t.Errorf("recent-windows = %+v", r)
	}
	if r.Highlight != (MRUHighlight{ActiveColor: Color{0.6, 0.6, 0.6, 1}, UrgentColor: Color{1, 0.6, 0.6, 1}, Padding: 30}) {
		t.Errorf("highlight = %+v", r.Highlight)
	}
	if r.Previews != (MRUPreviews{MaxHeight: 480, MaxScale: 0.5}) {
		t.Errorf("previews = %+v", r.Previews)
	}
	alt, shift := bind.Modifiers(bind.ModifierAlt), bind.Modifiers(bind.ModifierShift)
	want := []MRUBind{
		{Chord: bind.Chord{Key: "Tab", Modifiers: alt}},
		{Chord: bind.Chord{Key: "Tab", Modifiers: alt | shift}, Backward: true},
		{Chord: bind.Chord{Key: "grave", Modifiers: alt}, FilterAppID: true},
		{Chord: bind.Chord{Key: "grave", Modifiers: alt | shift}, Backward: true, FilterAppID: true},
		{Chord: bind.Chord{Key: "Tab", UsesMod: true}},
		{Chord: bind.Chord{Key: "Tab", Modifiers: shift, UsesMod: true}, Backward: true},
		{Chord: bind.Chord{Key: "grave", UsesMod: true}, FilterAppID: true},
		{Chord: bind.Chord{Key: "grave", Modifiers: shift, UsesMod: true}, Backward: true, FilterAppID: true},
	}
	if !slices.Equal(r.Binds, want) {
		t.Errorf("binds = %+v\nwant %+v", r.Binds, want)
	}
	spring := Default().Animations.RecentWindowsClose.Spring
	if spring == nil || *spring != (SpringConfig{DampingRatio: 1, Stiffness: 800, Epsilon: 0.001}) {
		t.Errorf("recent-windows-close = %+v", spring)
	}
}

func TestRecentWindowsSectionMergesAsNiri(t *testing.T) {
	dir := t.TempDir()
	// A later section, here in an include, merges into the earlier one.
	if err := os.WriteFile(filepath.Join(dir, "more.kdl"), []byte(`
recent-windows {
    on
    binds {
        Alt+Tab { next-window scope="workspace"; }
    }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(filepath.Join(dir, "config.kdl"), []byte(`
recent-windows {
    off
    debounce-ms 0
    open-delay-ms 40
    highlight {
        padding 15
        active-color "#00ff00"
        corner-radius 4.5
    }
    previews {
        max-height 960
    }
    binds {
        Alt+Tab { next-window; }
        Alt+grave { next-window filter="app-id"; }
        Super+Tab allow-inhibiting=false hotkey-overlay-title=null { previous-window scope="output"; }
    }
}
include "more.kdl"
animations {
    recent-windows-close { spring damping-ratio=0.5 stiffness=300 epsilon=0.01; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.RecentWindows
	if r.Off || r.DebounceMS != 0 || r.OpenDelayMS != 40 {
		t.Errorf("recent-windows = %+v", r)
	}
	if r.Highlight != (MRUHighlight{ActiveColor: Color{0, 1, 0, 1}, UrgentColor: Color{1, 0.6, 0.6, 1}, Padding: 15, CornerRadius: 4.5}) {
		t.Errorf("highlight = %+v", r.Highlight)
	}
	if r.Previews != (MRUPreviews{MaxHeight: 960, MaxScale: 0.5}) {
		t.Errorf("previews = %+v", r.Previews)
	}
	alt := bind.Modifiers(bind.ModifierAlt)
	want := []MRUBind{
		{Chord: bind.Chord{Key: "grave", Modifiers: alt}, FilterAppID: true},
		{Chord: bind.Chord{Key: "Tab", Modifiers: bind.Modifiers(bind.ModifierSuper)}, Backward: true,
			Scope: MRUScopeOutput, IgnoresInhibit: true},
		{Chord: bind.Chord{Key: "Tab", Modifiers: alt}, Scope: MRUScopeWorkspace},
	}
	if !slices.Equal(r.Binds, want) {
		t.Errorf("binds = %+v\nwant %+v", r.Binds, want)
	}
	if spring := cfg.Animations.RecentWindowsClose.Spring; spring == nil || spring.Stiffness != 300 {
		t.Errorf("recent-windows-close = %+v", cfg.Animations.RecentWindowsClose)
	}
	if slices.Contains(cfg.Warnings, "recent-windows") || slices.Contains(cfg.Warnings, "animations.recent-windows-close") {
		t.Errorf("recent-windows is still reported as not applied: %v", cfg.Warnings)
	}
	for _, bad := range []string{
		"recent-windows { debounce-ms 70000; }",
		"recent-windows { previews { max-scale 2; }; }",
		"recent-windows { previews { max-height 0; }; }",
		"recent-windows { highlight { padding -1; }; }",
		"recent-windows { binds { Tab { next-window; }; }; }",
		"recent-windows { binds { Alt+MouseLeft { next-window; }; }; }",
		"recent-windows { binds { Alt+Tab { focus-column-left; }; }; }",
		"recent-windows { binds { Alt+Tab { next-window filter=\"all\"; }; }; }",
		"recent-windows { binds { Alt+Tab { next-window scope=\"monitor\"; }; }; }",
		"recent-windows { binds { Alt+Tab { next-window; previous-window; }; }; }",
		"recent-windows { binds { Alt+Tab repeat=false { next-window; }; }; }",
		"recent-windows { binds { Alt+Tab { next-window; }; Alt+Tab { previous-window; }; }; }",
		"recent-windows { show-always; }",
	} {
		if _, err := Parse("mru.kdl", []byte(bad+"\n")); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
