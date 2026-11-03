package layout

import (
	"testing"
)

func TestParseSizeChangeReadsNiriSizes(t *testing.T) {
	for _, tc := range []struct {
		arg     string
		want    SizeChange
		wantErr bool
	}{
		{arg: "50%", want: SizeChange{Proportion: 50}},
		{arg: "+10%", want: SizeChange{Proportion: 10, Relative: true}},
		{arg: "-10%", want: SizeChange{Proportion: -10, Relative: true}},
		{arg: "100", want: SizeChange{Fixed: 100}},
		{arg: "-40", want: SizeChange{Fixed: -40, Relative: true}},
		{arg: "", wantErr: true},
		{arg: "wide", wantErr: true},
		{arg: "50%%", wantErr: true},
	} {
		got, err := ParseSizeChange(tc.arg)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSizeChange(%q) = %+v, want an error", tc.arg, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSizeChange(%q): %v", tc.arg, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSizeChange(%q) = %+v, want %+v", tc.arg, got, tc.want)
		}
	}
}

func TestSetColumnWidthUsesTheWorkingWidth(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)

	if !ws.SetColumnWidth(SizeChange{Proportion: 50}) {
		t.Fatal("SetColumnWidth(50%) = false, want true")
	}
	placement, ok := ws.Placement(3)
	if !ok {
		t.Fatal("Placement(3) missing")
	}
	if placement.Width != 600 {
		t.Errorf("column width = %d, want 600", placement.Width)
	}

	// A relative change moves the width by that share of the working width.
	if !ws.SetColumnWidth(SizeChange{Proportion: 10, Relative: true}) {
		t.Fatal("SetColumnWidth(+10%) = false, want true")
	}
	placement, _ = ws.Placement(3)
	if placement.Width != 720 {
		t.Errorf("column width after +10%% = %d, want 720", placement.Width)
	}

	// A fixed size is a size in pixels.
	if !ws.SetColumnWidth(SizeChange{Fixed: 500}) {
		t.Fatal("SetColumnWidth(500) = false, want true")
	}
	placement, _ = ws.Placement(3)
	if placement.Width != 500 {
		t.Errorf("column width after 500px = %d, want 500", placement.Width)
	}
}

func TestSwitchPresetColumnWidthCyclesThePresets(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)

	for _, want := range []int{600, 800, 400, 600} {
		if !ws.SwitchPresetColumnWidth(false) {
			t.Fatal("SwitchPresetColumnWidth = false, want true")
		}
		placement, ok := ws.Placement(2)
		if !ok {
			t.Fatal("Placement(2) missing")
		}
		if placement.Width != want {
			t.Fatalf("preset column width = %d, want %d", placement.Width, want)
		}
	}
	// Going backward picks the smaller preset.
	if !ws.SwitchPresetColumnWidth(true) {
		t.Fatal("SwitchPresetColumnWidth(back) = false, want true")
	}
	placement, _ := ws.Placement(2)
	if placement.Width != 400 {
		t.Errorf("preset column width after going back = %d, want 400", placement.Width)
	}
}

func TestToggleMaximizeColumnFillsTheWidthAndRestoresIt(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)

	if !ws.ToggleMaximizeColumn() {
		t.Fatal("ToggleMaximizeColumn = false, want true")
	}
	placement, _ := ws.Placement(2)
	if placement.Width != 1200 {
		t.Errorf("maximized column width = %d, want 1200", placement.Width)
	}
	if !ws.ToggleMaximizeColumn() {
		t.Fatal("ToggleMaximizeColumn = false, want true")
	}
	placement, _ = ws.Placement(2)
	if placement.Width != 400 {
		t.Errorf("column width after unmaximizing = %d, want 400", placement.Width)
	}
}

func TestMaximizeWindowToEdgesFillsTheColumnAndHidesTheRest(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusColumn(0) {
		t.Fatal("FocusColumn(0) = false, want true")
	}
	if !ws.ConsumeWindowIntoColumn() {
		t.Fatal("ConsumeWindowIntoColumn = false, want true")
	}

	if !ws.ToggleMaximizeWindowToEdges(0) {
		t.Fatal("ToggleMaximizeWindowToEdges = false, want true")
	}
	maximized, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if maximized.Width != 1200 {
		t.Errorf("maximized window width = %d, want 1200", maximized.Width)
	}
	if maximized.Height != 768 {
		t.Errorf("maximized window height = %d, want 768", maximized.Height)
	}
	hidden, ok := ws.Placement(2)
	if !ok {
		t.Fatal("Placement(2) missing")
	}
	if hidden.Visible {
		t.Error("the window behind the maximized one is still visible")
	}

	if !ws.ToggleMaximizeWindowToEdges(0) {
		t.Fatal("ToggleMaximizeWindowToEdges = false, want true")
	}
	restored, _ := ws.Placement(2)
	if !restored.Visible {
		t.Error("the window behind the maximized one stayed hidden")
	}
}

func TestCenterColumnCentersTheFocusedColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)

	if !ws.CenterColumn() {
		t.Fatal("CenterColumn = false, want true")
	}
	placement, _ := ws.Placement(3)
	if placement.X != 400 {
		t.Errorf("focused column x = %d, want 400", placement.X)
	}
}

func TestScrollingUsesContentCoordinatesNotOutputCoordinates(t *testing.T) {
	// A workspace that does not start at the origin, as a second output does.
	ws := NewWorkspace(Bounds{X: 1920, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)

	if !ws.CenterColumn() {
		t.Fatal("CenterColumn = false, want true")
	}
	placement, _ := ws.Placement(3)
	if placement.X != 2320 {
		t.Errorf("focused column x = %d, want 2320", placement.X)
	}
}

func TestExpandColumnToAvailableWidthOnASecondOutput(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 1920, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	// The newest window is the focused one, so the second column is focused.

	if !ws.ExpandColumnToAvailableWidth() {
		t.Fatal("ExpandColumnToAvailableWidth = false, want true")
	}
	expanded, _ := ws.Placement(2)
	if expanded.Width != 752 {
		t.Errorf("expanded column width = %d, want 752", expanded.Width)
	}
	if expanded.X < 1920 || expanded.X+expanded.Width > 1920+1200 {
		t.Errorf("expanded column x = %d, want it inside the output", expanded.X)
	}
}

func TestExpandColumnToAvailableWidthUsesTheFreeSpace(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)

	if !ws.ExpandColumnToAvailableWidth() {
		t.Fatal("ExpandColumnToAvailableWidth = false, want true")
	}
	placement, _ := ws.Placement(2)
	if placement.Width != 752 {
		t.Errorf("expanded column width = %d, want 752", placement.Width)
	}
	other, _ := ws.Placement(1)
	if other.Width != 400 {
		t.Errorf("the other column width = %d, want 400", other.Width)
	}
}

func TestSetWindowHeightSplitsTheColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusColumn(0) {
		t.Fatal("FocusColumn(0) = false, want true")
	}
	if !ws.ConsumeWindowIntoColumn() {
		t.Fatal("ConsumeWindowIntoColumn = false, want true")
	}

	if !ws.SetWindowHeight(0, SizeChange{Proportion: 50}) {
		t.Fatal("SetWindowHeight(50%) = false, want true")
	}
	tall, _ := ws.Placement(1)
	if tall.Height != 400 {
		t.Errorf("window height = %d, want 400", tall.Height)
	}
	short, _ := ws.Placement(2)
	if short.Height != 352 {
		t.Errorf("the other window height = %d, want 352", short.Height)
	}

	if !ws.ResetWindowHeight(0) {
		t.Fatal("ResetWindowHeight = false, want true")
	}
	reset, _ := ws.Placement(1)
	if reset.Height != 376 {
		t.Errorf("window height after the reset = %d, want 376", reset.Height)
	}
}

func TestSwitchPresetWindowHeightCyclesThePresets(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusColumn(0) {
		t.Fatal("FocusColumn(0) = false, want true")
	}
	if !ws.ConsumeWindowIntoColumn() {
		t.Fatal("ConsumeWindowIntoColumn = false, want true")
	}

	for _, want := range []int{400, 533, 267} {
		if !ws.SwitchPresetWindowHeight(0, false) {
			t.Fatal("SwitchPresetWindowHeight = false, want true")
		}
		placement, _ := ws.Placement(1)
		if placement.Height != want {
			t.Fatalf("preset window height = %d, want %d", placement.Height, want)
		}
	}
}
