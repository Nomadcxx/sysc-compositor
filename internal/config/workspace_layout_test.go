package config

import (
	"reflect"
	"testing"
)

func TestWorkspaceLayoutIsParsed(t *testing.T) {
	cfg, err := Parse("workspaces.kdl", []byte(`
workspace "wide" {
    open-on-output "eDP-1"
    layout {
        gaps 8
        default-column-width { proportion 0.75; }
        preset-column-widths { proportion 0.25; fixed 600; }
        preset-window-heights { proportion 0.5; }
        default-column-display "tabbed"
        center-focused-column "on-overflow"
    }
}
workspace "fixed" {
    layout {
        default-column-width { fixed 400; }
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 2 {
		t.Fatalf("workspaces = %+v, want two", cfg.Workspaces)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none", cfg.Warnings)
	}
	wide := cfg.Workspaces[0]
	if wide.Layout == nil {
		t.Fatalf("the first workspace has no layout")
	}
	if wide.Layout.Gaps == nil || *wide.Layout.Gaps != 8 {
		t.Errorf("the gaps of the first workspace = %+v, want 8", wide.Layout.Gaps)
	}
	if wide.Layout.ColumnWidthProportion == nil || *wide.Layout.ColumnWidthProportion != 0.75 {
		t.Errorf("the column width proportion = %+v, want 0.75", wide.Layout.ColumnWidthProportion)
	}
	if wide.Layout.FixedColumnWidth != nil {
		t.Errorf("the first workspace has a fixed column width: %+v", wide.Layout.FixedColumnWidth)
	}
	if len(wide.Layout.PresetColumnWidths) != 2 ||
		wide.Layout.PresetColumnWidths[0] != (PresetSize{Value: 0.25}) ||
		wide.Layout.PresetColumnWidths[1] != (PresetSize{Fixed: true, Value: 600}) {
		t.Errorf("the preset column widths = %+v, want 0.25 and fixed 600", wide.Layout.PresetColumnWidths)
	}
	if len(wide.Layout.PresetWindowHeights) != 1 ||
		wide.Layout.PresetWindowHeights[0] != (PresetSize{Value: 0.5}) {
		t.Errorf("the preset window heights = %+v, want 0.5", wide.Layout.PresetWindowHeights)
	}
	if wide.Layout.DefaultColumnDisplay == nil || *wide.Layout.DefaultColumnDisplay != ColumnDisplayTabbed {
		t.Errorf("the default column display = %+v, want tabbed", wide.Layout.DefaultColumnDisplay)
	}
	if wide.Layout.CenterFocusedColumn == nil || *wide.Layout.CenterFocusedColumn != CenterFocusedColumnOnOverflow {
		t.Errorf("the centering policy = %+v, want on-overflow", wide.Layout.CenterFocusedColumn)
	}
	fixed := cfg.Workspaces[1]
	if fixed.Layout == nil || fixed.Layout.FixedColumnWidth == nil || *fixed.Layout.FixedColumnWidth != 400 {
		t.Fatalf("the fixed column width = %+v, want 400", fixed.Layout)
	}
	if fixed.Layout.Gaps != nil {
		t.Errorf("the second workspace has gaps of its own: %+v", fixed.Layout.Gaps)
	}
	if fixed.Layout.PresetColumnWidths != nil || fixed.Layout.PresetWindowHeights != nil ||
		fixed.Layout.DefaultColumnDisplay != nil || fixed.Layout.CenterFocusedColumn != nil {
		t.Errorf("the second workspace overrides the presets, the display or the centering policy: %+v", fixed.Layout)
	}
}

// TestWorkspaceLayoutWarnsForTheKeysTheCompositorCannotApply: Niri parses
// empty-workspace-above-first inside a workspace's layout but reads it only
// per output, so it does nothing there; sysc names it instead of ignoring it.
func TestWorkspaceLayoutWarnsForTheKeysTheCompositorCannotApply(t *testing.T) {
	cfg, err := Parse("workspaces.kdl", []byte(`
workspace "wide" {
    layout {
        empty-workspace-above-first
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Warnings, []string{"workspace.layout.empty-workspace-above-first"}) {
		t.Fatalf("warnings = %q, want the unapplied key to be named", cfg.Warnings)
	}
}

// TestLayoutFlagsAreParsed: always-center-single-column and
// empty-workspace-above-first are Niri flags, set by the bare key and unset
// with false, and a workspace may unset the first one.
func TestLayoutFlagsAreParsed(t *testing.T) {
	cfg, err := Parse("layout.kdl", []byte(`
layout {
    always-center-single-column
    empty-workspace-above-first
}
workspace "fit" {
    layout {
        always-center-single-column false
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none", cfg.Warnings)
	}
	if !cfg.Layout.AlwaysCenterSingleColumn || !cfg.Layout.EmptyWorkspaceAboveFirst {
		t.Errorf("the layout flags = %t %t, want both set", cfg.Layout.AlwaysCenterSingleColumn, cfg.Layout.EmptyWorkspaceAboveFirst)
	}
	override := cfg.Workspaces[0].Layout.AlwaysCenterSingleColumn
	if override == nil || *override {
		t.Errorf("the workspace's always-center-single-column = %v, want false", override)
	}
	if _, err := Parse("layout.kdl", []byte(`layout { always-center-single-column "yes"; }`)); err == nil {
		t.Error("a string flag value parsed, want an error")
	}
}

func TestWorkspaceLayoutRejectsBadValues(t *testing.T) {
	for _, src := range []string{
		`workspace "wide" { layout { gaps -4; } }`,
		`workspace "wide" { layout { default-column-width { fixed 0; } } }`,
		`workspace "wide" { layout { default-column-width { proportion 200; } } }`,
		`workspace "wide" { layout { preset-column-widths { weird 3; } } }`,
		`workspace "wide" { layout { preset-column-widths { fixed 0; } } }`,
		`workspace "wide" { layout { preset-window-heights { proportion "lots"; } } }`,
		`workspace "wide" { layout { default-column-display "weird"; } }`,
		`workspace "wide" { layout { center-focused-column "sometimes"; } }`,
		`workspace "wide" { layout { struts { left 70000; } } }`,
		`workspace "wide" { layout { struts { right -70000; } } }`,
		`workspace "wide" { layout { struts { west 3; } } }`,
		`workspace "wide" { layout { struts { left "ten"; } } }`,
	} {
		if _, err := Parse("bad-layout.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
	}
}

// The layout section and a workspace both take a struts block, and the edges
// accept Niri's range, negatives included.
func TestStrutsAreParsed(t *testing.T) {
	cfg, err := Parse("struts.kdl", []byte(`
layout {
    struts { left 10; right 10; top 0; bottom 0; }
}
workspace "wide" {
    layout {
        struts { left -20; bottom 5; }
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none", cfg.Warnings)
	}
	if cfg.Layout.Struts != (Struts{Left: 10, Right: 10}) {
		t.Errorf("the config struts = %+v, want left and right 10", cfg.Layout.Struts)
	}
	wide := cfg.Workspaces[0]
	if wide.Layout == nil || wide.Layout.Struts == nil {
		t.Fatalf("the workspace has no struts of its own: %+v", wide.Layout)
	}
	if *wide.Layout.Struts != (Struts{Left: -20, Bottom: 5}) {
		t.Errorf("the workspace struts = %+v, want left -20 and bottom 5", *wide.Layout.Struts)
	}
}

// The layout section takes the preset cycles and the default column display,
// and the empty preset list means Niri's default presets.
func TestLayoutPresetsAreParsed(t *testing.T) {
	cfg, err := Parse("layout.kdl", []byte(`
layout {
    preset-column-widths { proportion 0.25; fixed 600; }
    preset-window-heights {}
    default-column-display "tabbed"
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none", cfg.Warnings)
	}
	if len(cfg.Layout.PresetColumnWidths) != 2 ||
		cfg.Layout.PresetColumnWidths[0] != (PresetSize{Value: 0.25}) ||
		cfg.Layout.PresetColumnWidths[1] != (PresetSize{Fixed: true, Value: 600}) {
		t.Errorf("the preset column widths = %+v, want 0.25 and fixed 600", cfg.Layout.PresetColumnWidths)
	}
	if cfg.Layout.PresetWindowHeights == nil || len(cfg.Layout.PresetWindowHeights) != 0 {
		t.Errorf("the preset window heights = %+v, want an empty list for the defaults", cfg.Layout.PresetWindowHeights)
	}
	if cfg.Layout.DefaultColumnDisplay != ColumnDisplayTabbed {
		t.Errorf("the default column display = %+v, want tabbed", cfg.Layout.DefaultColumnDisplay)
	}
}

func TestLayoutRejectsBadPresetValues(t *testing.T) {
	for _, src := range []string{
		`layout { preset-column-widths { fixed 0; } }`,
		`layout { preset-column-widths { weird 3; } }`,
		`layout { preset-window-heights 3; }`,
		`layout { default-column-display "centered"; }`,
		`layout { center-focused-column "centered"; }`,
		`layout { center-focused-column 3; }`,
		`layout { struts { left 70000; } }`,
		`layout { struts { top -70000; } }`,
		`layout { struts { west 3; } }`,
		`layout { struts "flat"; }`,
	} {
		if _, err := Parse("bad-layout.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
	}
}

// The layout section takes the centering policy of the focused column.
func TestCenterFocusedColumnIsParsed(t *testing.T) {
	cfg, err := Parse("layout.kdl", []byte(`
layout {
    center-focused-column "on-overflow"
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none", cfg.Warnings)
	}
	if cfg.Layout.CenterFocusedColumn != CenterFocusedColumnOnOverflow {
		t.Errorf("the centering policy = %+v, want on-overflow", cfg.Layout.CenterFocusedColumn)
	}
}
