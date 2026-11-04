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
	fixed := cfg.Workspaces[1]
	if fixed.Layout == nil || fixed.Layout.FixedColumnWidth == nil || *fixed.Layout.FixedColumnWidth != 400 {
		t.Fatalf("the fixed column width = %+v, want 400", fixed.Layout)
	}
	if fixed.Layout.Gaps != nil {
		t.Errorf("the second workspace has gaps of its own: %+v", fixed.Layout.Gaps)
	}
	if fixed.Layout.PresetColumnWidths != nil || fixed.Layout.PresetWindowHeights != nil ||
		fixed.Layout.DefaultColumnDisplay != nil {
		t.Errorf("the second workspace overrides the presets or the display: %+v", fixed.Layout)
	}
}

func TestWorkspaceLayoutWarnsForTheKeysTheCompositorCannotApply(t *testing.T) {
	cfg, err := Parse("workspaces.kdl", []byte(`
workspace "wide" {
    layout {
        struts { left 10; }
    }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Warnings, []string{"workspace.layout.struts"}) {
		t.Fatalf("warnings = %q, want the struts key to be named", cfg.Warnings)
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
	} {
		if _, err := Parse("bad-layout.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
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
	} {
		if _, err := Parse("bad-layout.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
	}
}
