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
		{arg: "NaN%", wantErr: true},
		{arg: "Inf%", wantErr: true},
		{arg: "-Inf%", wantErr: true},
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
	if placement.Width != 576 {
		t.Errorf("column width = %d, want 576", placement.Width)
	}

	// A relative change moves the width by that share of the space the gap
	// leaves, the way Niri measures it.
	if !ws.SetColumnWidth(SizeChange{Proportion: 10, Relative: true}) {
		t.Fatal("SetColumnWidth(+10%) = false, want true")
	}
	placement, _ = ws.Placement(3)
	if placement.Width != 694 {
		t.Errorf("column width after +10%% = %d, want 694", placement.Width)
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

	for _, want := range []int{576, 773, 379, 576} {
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
	if placement.Width != 379 {
		t.Errorf("preset column width after going back = %d, want 379", placement.Width)
	}
}

func TestSwitchPresetColumnWidthMixesFixedAndProportionalPresets(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.SetPresetSizes([]PresetSize{{Value: 0.25}, {Fixed: true, Value: 600}}, nil)
	ws.AddWindow(1)

	// The cycle mixes proportional and fixed sizes. From the default width
	// the first switch picks the first larger preset, then it wraps.
	for _, want := range []int{600, 230, 600} {
		if !ws.SwitchPresetColumnWidth(false) {
			t.Fatal("SwitchPresetColumnWidth = false, want true")
		}
		placement, ok := ws.Placement(1)
		if !ok {
			t.Fatal("Placement(1) missing")
		}
		if placement.Width != want {
			t.Fatalf("preset column width = %d, want %d", placement.Width, want)
		}
	}
	// Going backward steps back through the cycle.
	if !ws.SwitchPresetColumnWidth(true) {
		t.Fatal("SwitchPresetColumnWidth(back) = false, want true")
	}
	placement, _ := ws.Placement(1)
	if placement.Width != 230 {
		t.Errorf("preset column width after going back = %d, want 230", placement.Width)
	}
}

func TestSwitchPresetWindowHeightMixesFixedAndProportionalPresets(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.SetPresetSizes(nil, []PresetSize{{Value: 0.5}, {Fixed: true, Value: 700}})
	ws.AddWindow(1)

	for _, want := range []int{376, 700, 376} {
		if !ws.SwitchPresetWindowHeight(1, false) {
			t.Fatal("SwitchPresetWindowHeight = false, want true")
		}
		placement, ok := ws.Placement(1)
		if !ok {
			t.Fatal("Placement(1) missing")
		}
		if placement.Height != want {
			t.Fatalf("preset window height = %d, want %d", placement.Height, want)
		}
	}
}

func TestNewColumnsUseTheDefaultColumnDisplay(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.SetDefaultColumnDisplay(ColumnDisplayTabbed)
	ws.AddWindow(1)
	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.Display != ColumnDisplayTabbed {
		t.Errorf("the display of a new column = %v, want tabbed", placement.Display)
	}

	// A window that un-floats lands in a column of the default display too.
	ws.AddWindow(2)
	if !ws.SetWindowFloating(2, true, Bounds{X: 10, Y: 10, Width: 300, Height: 200}) {
		t.Fatal("SetWindowFloating(2, true) = false, want true")
	}
	if !ws.SetWindowFloating(2, false, Bounds{}) {
		t.Fatal("SetWindowFloating(2, false) = false, want true")
	}
	placement, _ = ws.Placement(2)
	if placement.Display != ColumnDisplayTabbed {
		t.Errorf("the display of the un-floated window's column = %v, want tabbed", placement.Display)
	}

	// A window that moves to another workspace lands in a column of that
	// workspace's default display, not the source's.
	target := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(3)
	if !ws.MoveWindowTo(target, 3) {
		t.Fatal("MoveWindowTo() = false, want true")
	}
	placement, _ = target.Placement(3)
	if placement.Display != ColumnDisplayNormal {
		t.Errorf("the display of the moved window's column = %v, want the target's normal", placement.Display)
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
	if placement.Width != 1168 {
		t.Errorf("maximized column width = %d, want 1168", placement.Width)
	}
	// Niri keeps a gap on either side of a column that fills the working area,
	// so the border and the shadow of the window stay visible.
	if placement.X != 16 {
		t.Errorf("maximized column x = %d, want 16", placement.X)
	}
	if placement.X+placement.Width != 1184 {
		t.Errorf("maximized column right edge = %d, want 1184", placement.X+placement.Width)
	}
	if !ws.ToggleMaximizeColumn() {
		t.Fatal("ToggleMaximizeColumn = false, want true")
	}
	placement, _ = ws.Placement(2)
	if placement.Width != 400 {
		t.Errorf("column width after unmaximizing = %d, want 400", placement.Width)
	}
}

func TestMaximizeWindowToEdgesExtractsTheWindow(t *testing.T) {
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
	if maximized.Height != 800 {
		t.Errorf("maximized window height = %d, want 800", maximized.Height)
	}
	hidden, ok := ws.Placement(2)
	if !ok {
		t.Fatal("Placement(2) missing")
	}
	if !hidden.Visible || hidden.Column == maximized.Column {
		t.Error("maximizing a normal group did not extract its target")
	}
	if !maximized.Maximized || hidden.Maximized {
		t.Errorf("Maximized = %v and %v, want only the maximized window marked", maximized.Maximized, hidden.Maximized)
	}

	if !ws.ToggleMaximizeWindowToEdges(0) {
		t.Fatal("ToggleMaximizeWindowToEdges = false, want true")
	}
	restored, _ := ws.Placement(2)
	if !restored.Visible {
		t.Error("the window behind the maximized one stayed hidden")
	}
	if unmaximized, _ := ws.Placement(1); unmaximized.Maximized {
		t.Error("the restored window is still marked maximized")
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
	if tall.Height != 376 {
		t.Errorf("window height = %d, want 376", tall.Height)
	}
	short, _ := ws.Placement(2)
	if short.Height != 376 {
		t.Errorf("the other window height = %d, want 376", short.Height)
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

	for _, want := range []int{507, 245, 376} {
		if !ws.SwitchPresetWindowHeight(0, false) {
			t.Fatal("SwitchPresetWindowHeight = false, want true")
		}
		placement, _ := ws.Placement(1)
		if placement.Height != want {
			t.Fatalf("preset window height = %d, want %d", placement.Height, want)
		}
	}
}

func TestMaximizeColumnFillsTheWorkingWidth(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)

	if !ws.MaximizeColumn(2) {
		t.Fatal("MaximizeColumn(2) = false, want true")
	}
	placement, ok := ws.Placement(2)
	if !ok {
		t.Fatal("Placement(2) missing")
	}
	if placement.Width != 1168 {
		t.Errorf("maximized column width = %d, want 1168", placement.Width)
	}
	if ws.MaximizeColumn(2) {
		t.Error("MaximizeColumn(2) = true on an already maximized column, want false")
	}
}

func TestMaximizeWindowToEdgesUsesTheParentArea(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusColumn(0) {
		t.Fatal("FocusColumn(0) = false, want true")
	}
	if !ws.ConsumeWindowIntoColumn() {
		t.Fatal("ConsumeWindowIntoColumn = false, want true")
	}

	if !ws.MaximizeWindowToEdges(1) {
		t.Fatal("MaximizeWindowToEdges(1) = false, want true")
	}
	placement, _ := ws.Placement(1)
	if placement.Height != 800 {
		t.Errorf("maximized window height = %d, want 800", placement.Height)
	}
	if placement.Width != 1200 {
		t.Errorf("maximized window width = %d, want 1200", placement.Width)
	}
	other, _ := ws.Placement(2)
	if other.Column == placement.Column || !other.Visible {
		t.Errorf("normal sibling was not retained in its own visible column: %+v", other)
	}
}

func TestSetColumnDisplayForSetsTheColumnOfTheWindow(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)

	if !ws.SetColumnDisplayFor(2, ColumnDisplayTabbed) {
		t.Fatal("SetColumnDisplayFor(2, tabbed) = false, want true")
	}
	placement, _ := ws.Placement(2)
	if placement.Display != ColumnDisplayTabbed {
		t.Errorf("column display = %v, want tabbed", placement.Display)
	}
	other, _ := ws.Placement(1)
	if other.Display != ColumnDisplayNormal {
		t.Errorf("other column display = %v, want normal", other.Display)
	}
}

func TestSetFloatingPositionPlacesTheWindow(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	if !ws.SetWindowFloating(1, true, Bounds{}) {
		t.Fatal("SetWindowFloating = false, want true")
	}
	placement, _ := ws.Placement(1)
	if placement.Width != 400 || placement.Height != 768 {
		t.Fatalf("floating window size = %dx%d, want 400x768", placement.Width, placement.Height)
	}

	for _, tc := range []struct {
		position FloatingPosition
		wantX    int
		wantY    int
	}{
		{position: FloatingPosition{X: 100, Y: 50, RelativeTo: RelativeToTopLeft}, wantX: 100, wantY: 50},
		{position: FloatingPosition{X: 100, Y: 10, RelativeTo: RelativeToBottomRight}, wantX: 700, wantY: 22},
		{position: FloatingPosition{X: 100, Y: 10, RelativeTo: RelativeToTop}, wantX: 500, wantY: 10},
		{position: FloatingPosition{X: 100, Y: 10, RelativeTo: RelativeToLeft}, wantX: 100, wantY: 26},
	} {
		if !ws.SetFloatingPosition(1, tc.position) {
			t.Fatalf("SetFloatingPosition(%+v) = false, want true", tc.position)
		}
		placement, _ := ws.Placement(1)
		if placement.X != tc.wantX || placement.Y != tc.wantY {
			t.Errorf("floating position for %+v = (%d,%d), want (%d,%d)",
				tc.position, placement.X, placement.Y, tc.wantX, tc.wantY)
		}
	}
}

func TestSetFloatingPositionKeepsThePositionForALaterFloat(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)

	if !ws.SetFloatingPosition(1, FloatingPosition{X: 120, Y: 60, RelativeTo: RelativeToTopLeft}) {
		t.Fatal("SetFloatingPosition = false, want true")
	}
	if !ws.SetWindowFloating(1, true, Bounds{}) {
		t.Fatal("SetWindowFloating = false, want true")
	}
	placement, _ := ws.Placement(1)
	if placement.X != 120 || placement.Y != 60 {
		t.Errorf("floating position = (%d,%d), want (120,60)", placement.X, placement.Y)
	}
}

func TestTabbedColumnKeepsRoomForItsTabIndicator(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	if !ws.ToggleColumnTabbedDisplay() {
		t.Fatal("ToggleColumnTabbedDisplay = false, want true")
	}
	plain, _ := ws.Placement(1)

	ws.SetTabSpace(TabSpace{Size: 9, Side: TabLeft})
	if got, _ := ws.Placement(1); got.X != plain.X+9 || got.Width != plain.Width-9 || got.Y != plain.Y || got.Height != plain.Height {
		t.Errorf("left tab space placement = %+v, want %+v moved right and narrowed by 9", got, plain)
	}
	ws.SetTabSpace(TabSpace{Size: 9, Side: TabBottom})
	if got, _ := ws.Placement(1); got.X != plain.X || got.Width != plain.Width || got.Y != plain.Y || got.Height != plain.Height-9 {
		t.Errorf("bottom tab space placement = %+v, want %+v shortened by 9", got, plain)
	}
	ws.SetTabSpace(TabSpace{Size: 9, Side: TabTop, HideWhenSingle: true})
	if got, _ := ws.Placement(1); got != plain {
		t.Errorf("a single hidden tab kept room: %+v, want %+v", got, plain)
	}

	ws.AddWindow(2)
	if !ws.FocusColumn(0) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not stack the second window in the tabbed column")
	}
	if got, _ := ws.Placement(1); got.Y != plain.Y+9 || got.Height != plain.Height-9 {
		t.Errorf("two tabs placement = %+v, want %+v moved down and shortened by 9", got, plain)
	}
	ws.SetTabSpace(TabSpace{})
	if !ws.ToggleColumnTabbedDisplay() {
		t.Fatal("ToggleColumnTabbedDisplay back = false, want true")
	}
	ws.SetTabSpace(TabSpace{Size: 9, Side: TabLeft})
	if got, _ := ws.Placement(1); got.X != plain.X {
		t.Errorf("a normal column took tab room: %+v", got)
	}
}

func TestWindowWidthAndCenterActionsTargetWithoutChangingFocus(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 30, Y: 40, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.SetWindowWidth(1, SizeChange{Fixed: 500}) {
		t.Fatal("targeted width was ignored")
	}
	first, _ := ws.Placement(1)
	second, _ := ws.Placement(2)
	if first.Width != 500 || second.Width != 400 {
		t.Fatalf("targeted widths = %d, %d", first.Width, second.Width)
	}
	if id, _ := ws.FocusedWindow(); id != 2 {
		t.Fatalf("width focused window %d", id)
	}
	if ws.CenterWindow(1) {
		t.Fatal("centering a non-active tiled column should do nothing")
	}
	if !ws.CenterWindow(2) {
		t.Fatal("centering active tiled column was ignored")
	}
	second, _ = ws.Placement(2)
	if second.X != 430 {
		t.Fatalf("centered x = %d, want 430", second.X)
	}
	if !ws.SwitchPresetWindowWidth(1, false) {
		t.Fatal("targeted preset was ignored")
	}
	first, _ = ws.Placement(1)
	if first.Width != 576 {
		t.Fatalf("targeted preset width = %d, want 576", first.Width)
	}
	if id, _ := ws.FocusedWindow(); id != 2 {
		t.Fatalf("preset focused window %d", id)
	}
}

func TestFloatingSizingUsesItsOwnGeometryAndPresets(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 30, Y: 40, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.SetWindowFloating(2, true, Bounds{X: 100, Y: 120, Width: 300, Height: 200})
	if !ws.SetColumnWidth(SizeChange{Proportion: 50}) {
		t.Fatal("floating column-width route was ignored")
	}
	floating, _ := ws.Placement(2)
	if floating.Width != 600 || floating.Height != 200 || floating.X != 100 || floating.Y != 120 {
		t.Fatalf("floating width geometry = %+v", floating)
	}
	if !ws.SetWindowHeight(0, SizeChange{Proportion: 50}) {
		t.Fatal("floating height route was ignored")
	}
	floating, _ = ws.Placement(2)
	if floating.Height != 400 || floating.Width != 600 {
		t.Fatalf("floating height geometry = %+v", floating)
	}
	if ws.ResetWindowHeight(0) || ws.ToggleMaximizeColumn() {
		t.Fatal("floating reset or maximize changed the tiled column")
	}
	if !ws.CenterColumn() {
		t.Fatal("floating center-column route was ignored")
	}
	floating, _ = ws.Placement(2)
	if floating.X != 330 || floating.Y != 240 {
		t.Fatalf("floating center geometry = %+v", floating)
	}
	ws.SetPresetSizes([]PresetSize{{Fixed: true, Value: 900}, {Fixed: true, Value: 300}, {Value: 0.5}}, nil)
	for _, width := range []int{900, 300, 600, 900} {
		if !ws.SwitchPresetColumnWidth(false) {
			t.Fatal("floating preset route was ignored")
		}
		floating, _ = ws.Placement(2)
		if floating.Width != width {
			t.Fatalf("floating preset width = %d, want %d", floating.Width, width)
		}
	}
	ws.SetWindowSizeHints(2, 500, 700, 150, 500)
	ws.SetWindowWidth(2, SizeChange{Fixed: 1000})
	floating, _ = ws.Placement(2)
	if floating.Width != 700 {
		t.Fatalf("floating max-width = %d", floating.Width)
	}
	if !ws.SetWindowWidth(1, SizeChange{Fixed: 500}) {
		t.Fatal("tiled target while floating was ignored")
	}
	tiled, _ := ws.Placement(1)
	if tiled.Width != 500 {
		t.Fatalf("targeted tiled width = %d", tiled.Width)
	}
	if id, _ := ws.FocusedWindow(); id != 2 {
		t.Fatalf("targeted width changed floating focus to %d", id)
	}
}

func TestFloatingPresetHeightAndOversizedCenter(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 30, Y: 40, Width: 1000, Height: 700}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowFloating(1, true, Bounds{X: 100, Y: 120, Width: 300, Height: 200})
	ws.SetPresetSizes(nil, []PresetSize{{Fixed: true, Value: 500}, {Fixed: true, Value: 200}})
	for _, want := range []int{500, 200, 500} {
		ws.SwitchPresetWindowHeight(0, false)
		got, _ := ws.Placement(1)
		if got.Height != want {
			t.Fatalf("floating preset height = %d, want %d", got.Height, want)
		}
	}
	ws.SetWindowWidth(1, SizeChange{Fixed: 1500})
	ws.SetWindowHeight(1, SizeChange{Fixed: 1000})
	ws.CenterWindow(1)
	got, _ := ws.Placement(1)
	if got.X != 30 || got.Y != 40 || got.Width != 1500 || got.Height != 1000 {
		t.Fatalf("oversized floating center = %+v", got)
	}
}

func TestSetMaximizedExtractsWindowsAndRestoresFloating(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 10, Y: 20, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusColumn(0)
	ws.ConsumeWindowIntoColumn()
	if !ws.SetMaximized(1, true) || ws.SetMaximized(1, true) {
		t.Fatal("maximize must apply once")
	}
	first, _ := ws.Placement(1)
	second, _ := ws.Placement(2)
	if !first.Maximized || first.Column == second.Column || second.Maximized || first.Width != 1200 || first.Height != 800 || first.Y != 20 {
		t.Fatalf("extracted maximize: first=%+v second=%+v", first, second)
	}
	if !ws.SetMaximized(1, false) || ws.SetMaximized(1, false) {
		t.Fatal("unmaximize must apply once")
	}
	first, _ = ws.Placement(1)
	if first.Maximized || first.Width != 400 {
		t.Fatalf("unmaximized geometry = %+v", first)
	}
	ws.SetWindowFloating(2, true, Bounds{X: 100, Y: 120, Width: 300, Height: 200})
	if !ws.SetMaximized(2, true) {
		t.Fatal("floating maximize was ignored")
	}
	second, _ = ws.Placement(2)
	if second.Floating || !second.Maximized {
		t.Fatalf("maximized floating target = %+v", second)
	}
	if !ws.SetMaximized(2, false) {
		t.Fatal("floating unmaximize was ignored")
	}
	second, _ = ws.Placement(2)
	if !second.Floating || second.Maximized || second.X != 100 || second.Y != 120 || second.Width != 300 || second.Height != 200 {
		t.Fatalf("restored floating target = %+v", second)
	}
}

func TestSetMaximizedRetainsTabbedGroupAndIgnoresStruts(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 110, Y: 120, Width: 1000, Height: 600}, 400, 16)
	ws.SetMaximizedBounds(Bounds{X: 10, Y: 20, Width: 1200, Height: 800})
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusColumn(0)
	ws.ConsumeWindowIntoColumn()
	ws.SetColumnDisplay(ColumnDisplayTabbed)
	ws.SetTabSpace(TabSpace{Size: 40, Side: TabLeft})
	if !ws.SetMaximized(1, true) {
		t.Fatal("tabbed maximize was ignored")
	}
	first, _ := ws.Placement(1)
	second, _ := ws.Placement(2)
	if first.Column != second.Column || !first.Maximized || !second.Maximized || first.X != 10 || first.Y != 20 || first.Width != 1200 || first.Height != 800 {
		t.Fatalf("tabbed maximize: first=%+v second=%+v", first, second)
	}
}

func TestMaximizedGeometryTracksWorkspaceResize(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 10, Y: 20, Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetMaximized(1, true)
	ws.SetGeometry(Bounds{X: 30, Y: 40, Width: 900, Height: 600}, 400, 16)
	p, _ := ws.Placement(1)
	if p.X != 30 || p.Y != 40 || p.Width != 900 || p.Height != 600 {
		t.Fatalf("resized maximized placement = %+v", p)
	}
}

func TestMaximizeAnInactiveTilePreservesFocusAndGroup(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusColumn(0)
	ws.ConsumeWindowIntoColumn()
	ws.FocusWindow(2)
	if !ws.SetMaximized(1, true) {
		t.Fatal("inactive maximize was ignored")
	}
	if id, _ := ws.FocusedWindow(); id != 2 {
		t.Fatalf("maximize changed focus to %d", id)
	}
	first, _ := ws.Placement(1)
	second, _ := ws.Placement(2)
	if first.Column == second.Column || second.Maximized {
		t.Fatalf("maximize changed sibling group: %+v %+v", first, second)
	}
	if !ws.SetMaximized(1, false) {
		t.Fatal("inactive unmaximize was ignored")
	}
	if id, _ := ws.FocusedWindow(); id != 2 {
		t.Fatalf("unmaximize changed focus to %d", id)
	}
}
