package bind

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestParseSizeActions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want layout.SizeChange
	}{
		{name: ActionSetColumnWidth, args: []string{"50%"}, want: layout.SizeChange{Proportion: 50}},
		{name: ActionSetColumnWidth, args: []string{"-10%"}, want: layout.SizeChange{Proportion: -10, Relative: true}},
		{name: ActionSetColumnWidth, args: []string{"+10%"}, want: layout.SizeChange{Proportion: 10, Relative: true}},
		{name: ActionSetColumnWidth, args: []string{"640"}, want: layout.SizeChange{Fixed: 640}},
		{name: ActionSetWindowHeight, args: []string{"30%"}, want: layout.SizeChange{Proportion: 30}},
		{name: ActionSetWindowHeight, args: []string{"-20"}, want: layout.SizeChange{Fixed: -20, Relative: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" "+tt.args[0], func(t *testing.T) {
			action, supported, err := ParseAction(tt.name, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !supported {
				t.Fatalf("ParseAction(%q) is not supported", tt.name)
			}
			if action.Size != tt.want {
				t.Fatalf("size = %+v, want %+v", action.Size, tt.want)
			}
		})
	}
}

func TestParseSizeActionsRejectBadArguments(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: ActionSetColumnWidth},
		{name: ActionSetColumnWidth, args: []string{"wide"}},
		{name: ActionSetColumnWidth, args: []string{"50%%"}},
		{name: ActionSetWindowHeight},
		{name: ActionSetWindowHeight, args: []string{"tall"}},
	} {
		if _, ok, err := ParseAction(tt.name, tt.args); !ok || err == nil {
			t.Errorf("ParseAction(%q, %v) = (%v, %v), want supported action with argument error", tt.name, tt.args, ok, err)
		}
	}
}

func TestSizeActionsAreSupported(t *testing.T) {
	for _, name := range []string{
		ActionSetColumnWidth, ActionSwitchPresetColumnWidth, ActionSwitchPresetColumnWidthBack,
		ActionMaximizeColumn, ActionMaximizeWindowToEdges, ActionCenterColumn,
		ActionCenterVisibleColumns, ActionExpandColumnToAvailableWidth, ActionSetWindowHeight,
		ActionResetWindowHeight, ActionSwitchPresetWindowHeight, ActionSwitchPresetWindowHeightBack,
	} {
		if !IsSupportedAction(name) {
			t.Errorf("IsSupportedAction(%q) = false, want true", name)
		}
	}
	// The two size actions take a size, so they are read with one.
	for _, name := range []string{ActionSetColumnWidth, ActionSetWindowHeight} {
		if _, supported, err := ParseAction(name, []string{"50%"}); !supported || err != nil {
			t.Errorf("ParseAction(%q) = (%v, %v), want supported action", name, supported, err)
		}
	}
	for _, name := range []string{
		ActionSwitchPresetColumnWidth, ActionSwitchPresetColumnWidthBack, ActionMaximizeColumn,
		ActionMaximizeWindowToEdges, ActionCenterColumn, ActionCenterVisibleColumns,
		ActionExpandColumnToAvailableWidth, ActionResetWindowHeight, ActionSwitchPresetWindowHeight,
		ActionSwitchPresetWindowHeightBack,
	} {
		if _, supported, err := ParseAction(name, nil); !supported || err != nil {
			t.Errorf("ParseAction(%q) = (%v, %v), want supported action", name, supported, err)
		}
	}
}
