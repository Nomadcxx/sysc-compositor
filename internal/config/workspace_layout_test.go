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
	fixed := cfg.Workspaces[1]
	if fixed.Layout == nil || fixed.Layout.FixedColumnWidth == nil || *fixed.Layout.FixedColumnWidth != 400 {
		t.Fatalf("the fixed column width = %+v, want 400", fixed.Layout)
	}
	if fixed.Layout.Gaps != nil {
		t.Errorf("the second workspace has gaps of its own: %+v", fixed.Layout.Gaps)
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
	} {
		if _, err := Parse("bad-layout.kdl", []byte(src)); err == nil {
			t.Errorf("Parse(%q) error = nil, want a rejection", src)
		}
	}
}
