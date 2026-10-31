package layout

import (
	"slices"
	"testing"
)

func TestNewWindowGoesRightOfFocusedColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusLeft() {
		t.Fatal("FocusLeft() = false, want true")
	}
	ws.AddWindow(3)

	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 3 {
		t.Fatalf("FocusedWindow() = %d, %t, want 3, true", got, ok)
	}
}

func TestConsumeAdjacentWindowIntoFocusedColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(1) {
		t.Fatal("FocusWindow(1) = false, want true")
	}
	if !ws.ConsumeWindowIntoColumn() {
		t.Fatal("ConsumeWindowIntoColumn() = false, want to consume the adjacent right column")
	}

	first, firstOK := ws.Placement(1)
	consumed, consumedOK := ws.Placement(2)
	third, thirdOK := ws.Placement(3)
	if !firstOK || !consumedOK || !thirdOK {
		t.Fatalf("placements present = (%t, %t, %t), want all windows", firstOK, consumedOK, thirdOK)
	}
	if first.X != consumed.X || first.Width != consumed.Width {
		t.Errorf("focused and consumed windows occupy columns at x=%d and x=%d, want one column", first.X, consumed.X)
	}
	if third.X <= first.X {
		t.Errorf("following column x=%d, want it after the focused column at x=%d", third.X, first.X)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Errorf("FocusedWindow() = %d, %t, want focus to stay on window 1", got, ok)
	}
}

func TestExpelBottomWindowIntoColumnOnRight(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1400, Height: 800}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(1) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a two-window focused column")
	}
	if !ws.ExpelWindowFromColumn() {
		t.Fatal("ExpelWindowFromColumn() = false, want the bottom window to move right")
	}
	placements := ws.Placements()
	if len(placements) != 3 {
		t.Fatalf("placements = %d, want 3", len(placements))
	}
	for index, placement := range placements {
		if placement.Column != uint32(index) || placement.Tile != 0 {
			t.Errorf("placement %d = %+v, want a separate column at index %d", index, placement, index)
		}
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Errorf("FocusedWindow() = %d, %t, want focus to stay on window 1", got, ok)
	}
}

func TestConsumeOrExpelWindowRightUsesAdjacentColumnAndWorkspaceEdge(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1400, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusWindow(1) || !ws.ConsumeOrExpelWindowRight(nil) {
		t.Fatal("ConsumeOrExpelWindowRight() = false, want single window consumed into adjacent column")
	}
	if placements := ws.Placements(); len(placements) != 2 || placements[0].Column != placements[1].Column {
		t.Fatalf("placements after consume = %+v, want both windows in one column", placements)
	}
	if id, ok := ws.FocusedWindow(); !ok || id != 1 {
		t.Fatalf("focused window after consume = %d, %t, want moved window 1", id, ok)
	}
	if !ws.FocusWindow(2) || !ws.ConsumeOrExpelWindowRight(nil) {
		t.Fatal("ConsumeOrExpelWindowRight() = false, want active tile expelled into a new column")
	}
	placements := ws.Placements()
	if len(placements) != 2 || placements[0].Column == placements[1].Column {
		t.Fatalf("placements after expel = %+v, want two columns", placements)
	}
	if id, ok := ws.FocusedWindow(); !ok || id != 2 {
		t.Fatalf("focused window after expel = %d, %t, want moved window 2", id, ok)
	}

	edge := NewWorkspace(Bounds{Width: 800, Height: 600}, 400, 16)
	edge.AddWindow(9)
	if edge.ConsumeOrExpelWindowRight(nil) || !slices.Equal(edge.WindowOrder(), []uint64{9}) {
		t.Fatal("right edge action changed a lone window without an adjacent column")
	}
}

func TestConsumeOrExpelWindowLeftMergesIntoPreviousColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1800, Height: 800}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(3) || !ws.ConsumeOrExpelWindowLeft(nil) {
		t.Fatal("ConsumeOrExpelWindowLeft() = false, want single window consumed into previous column")
	}
	placements := ws.Placements()
	if len(placements) != 3 || placements[1].Column != placements[2].Column {
		t.Fatalf("placements after left consume = %+v, want windows 2 and 3 in one column", placements)
	}
	if id, ok := ws.FocusedWindow(); !ok || id != 3 {
		t.Fatalf("focused window after left consume = %d, %t, want moved window 3", id, ok)
	}
}

func TestTabbedColumnShowsOnlyItsActiveWindow(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusWindow(1) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a two-window focused column")
	}
	if !ws.ToggleColumnTabbedDisplay() {
		t.Fatal("ToggleColumnTabbedDisplay() = false, want true")
	}
	placements := ws.Placements()
	if len(placements) != 2 {
		t.Fatalf("placements = %d, want 2", len(placements))
	}
	if placements[0].X != placements[1].X || placements[0].Y != placements[1].Y ||
		placements[0].Width != placements[1].Width || placements[0].Height != placements[1].Height {
		t.Errorf("tab placements differ: %+v and %+v", placements[0], placements[1])
	}
	if !placements[0].Active || !placements[0].Visible || placements[1].Active || placements[1].Visible {
		t.Errorf("tab visibility = (%+v, %+v), want only window 1 active and visible", placements[0], placements[1])
	}
	if !ws.FocusWindow(2) {
		t.Fatal("FocusWindow(2) = false, want to activate the other tab")
	}
	placements = ws.Placements()
	if placements[0].Visible || !placements[1].Active || !placements[1].Visible {
		t.Errorf("tab visibility after focus = (%+v, %+v), want window 2 visible", placements[0], placements[1])
	}
}

func TestColumnDisplayAndWindowFocusActions(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusWindow(1) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a two-window focused column")
	}
	if !ws.SetColumnDisplay(ColumnDisplayTabbed) {
		t.Fatal("SetColumnDisplay(tabbed) = false, want true")
	}
	if !ws.FocusWindowInColumn(2) {
		t.Fatal("FocusWindowInColumn(2) = false, want to focus the second tile")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t, want 2", got, ok)
	}
	placements := ws.Placements()
	if placements[0].Visible || !placements[1].Visible || !placements[1].Active {
		t.Errorf("tab visibility = (%+v, %+v), want window 2 visible and active", placements[0], placements[1])
	}
	if !ws.SetColumnDisplay(ColumnDisplayNormal) {
		t.Fatal("SetColumnDisplay(normal) = false, want true")
	}
	placements = ws.Placements()
	if !placements[0].Visible || !placements[1].Visible || placements[0].Display != ColumnDisplayNormal {
		t.Errorf("normal placements = (%+v, %+v), want both tiles visible", placements[0], placements[1])
	}
}

func TestFocusWindowUpDownChangesTheActiveTile(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(1) || !ws.ConsumeWindowIntoColumn() || !ws.FocusWindow(2) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a three-window column")
	}
	if !ws.FocusWindowDown() {
		t.Fatal("FocusWindowDown() = false, want the next tile focused")
	}
	if got, _ := ws.FocusedWindow(); got != 3 {
		t.Fatalf("FocusedWindow() = %d after focus down, want 3", got)
	}
	if !ws.FocusWindowUp() {
		t.Fatal("FocusWindowUp() = false, want the previous tile focused")
	}
	if got, _ := ws.FocusedWindow(); got != 2 {
		t.Fatalf("FocusedWindow() = %d after focus up, want 2", got)
	}
	if !ws.FocusWindowInColumn(1) || ws.FocusWindowUp() {
		t.Fatal("focus up at the top tile should stay at the top")
	}
}

func TestMoveWindowUpDownReordersTilesAndKeepsFocus(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(1) || !ws.ConsumeWindowIntoColumn() || !ws.FocusWindow(2) || !ws.ConsumeWindowIntoColumn() {
		t.Fatal("could not form a three-window column")
	}
	if !ws.MoveWindowDown() {
		t.Fatal("MoveWindowDown() = false, want active tile moved down")
	}
	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after move down = %v, want %v", got, want)
	}
	if got, _ := ws.FocusedWindow(); got != 2 {
		t.Fatalf("FocusedWindow() after move down = %d, want moved window 2", got)
	}
	if !ws.MoveWindowUp() {
		t.Fatal("MoveWindowUp() = false, want active tile moved up")
	}
	if got, want := ws.WindowOrder(), []uint64{1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after move up = %v, want %v", got, want)
	}
}

func TestMoveFloatingWindowToOutputPreservesRelativePosition(t *testing.T) {
	source := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 500, 16)
	target := NewWorkspace(Bounds{X: 1200, Y: 300, Width: 2000, Height: 1200}, 800, 16)
	source.AddWindow(1)
	if !source.SetWindowFloating(1, true, Bounds{X: 300, Y: 250, Width: 400, Height: 300}) {
		t.Fatal("SetWindowFloating() = false, want source window floating")
	}

	if !source.MoveWindowTo(target, 1) {
		t.Fatal("MoveWindowTo() = false, want floating window moved to target workspace")
	}
	placement, ok := target.Placement(1)
	if !ok || !placement.Floating {
		t.Fatalf("target placement = %+v, %t, want floating window", placement, ok)
	}
	if got, want := [4]int{placement.X, placement.Y, placement.Width, placement.Height}, [4]int{1600, 600, 400, 300}; got != want {
		t.Errorf("target floating bounds = %v, want relative position and size %v", got, want)
	}
}

func TestMoveFloatingWindowToSmallerOutputKeepsItVisible(t *testing.T) {
	source := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 500, 16)
	target := NewWorkspace(Bounds{X: 1200, Y: 300, Width: 640, Height: 480}, 320, 16)
	source.AddWindow(1)
	if !source.SetWindowFloating(1, true, Bounds{X: 1050, Y: 800, Width: 400, Height: 300}) {
		t.Fatal("SetWindowFloating() = false, want source window floating")
	}

	if !source.MoveWindowTo(target, 1) {
		t.Fatal("MoveWindowTo() = false, want floating window moved to target workspace")
	}
	placement, ok := target.Placement(1)
	if !ok || !placement.Floating {
		t.Fatalf("target placement = %+v, %t, want floating window", placement, ok)
	}
	if got, want := [2]int{placement.X, placement.Y}, [2]int{1765, 705}; got != want {
		t.Errorf("target floating position = %v, want edge-clamped position %v", got, want)
	}
}

func TestFloatingFocusAndMoveWindowUpDownFollowNiri(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	for _, window := range []struct {
		id uint64
		y  int
	}{{1, 100}, {2, 300}, {3, 450}} {
		if !ws.SetWindowFloating(window.id, true, Bounds{X: 100, Y: window.y, Width: 360, Height: 200}) {
			t.Fatalf("SetWindowFloating(%d) = false, want floating window", window.id)
		}
	}
	if !ws.FocusWindow(2) {
		t.Fatal("FocusWindow(2) = false, want floating window 2 focused")
	}
	if !ws.FocusWindowDown() {
		t.Fatal("FocusWindowDown() = false, want nearest lower floating window focused")
	}
	if got, _ := ws.FocusedWindow(); got != 3 {
		t.Fatalf("FocusedWindow() after focus down = %d, want 3", got)
	}
	if !ws.FocusWindowUp() {
		t.Fatal("FocusWindowUp() = false, want nearest upper floating window focused")
	}
	if got, _ := ws.FocusedWindow(); got != 2 {
		t.Fatalf("FocusedWindow() after focus up = %d, want 2", got)
	}
	if !ws.MoveWindowDown() {
		t.Fatal("MoveWindowDown() = false, want focused floating window moved")
	}
	if got, ok := ws.Placement(2); !ok || got.Y != 350 {
		t.Fatalf("floating placement after move down = %+v, %t, want y=350", got, ok)
	}
	if !ws.MoveWindowUp() {
		t.Fatal("MoveWindowUp() = false, want focused floating window moved")
	}
	if got, ok := ws.Placement(2); !ok || got.Y != 300 {
		t.Fatalf("floating placement after move up = %+v, %t, want y=300", got, ok)
	}
}

func TestFloatingWindowRetainsGeometryAndTiledPositionAcrossToggles(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusWindow(1) {
		t.Fatal("FocusWindow(1) = false, want window 1 focused")
	}
	if !ws.SetWindowFloating(1, true, Bounds{X: 90, Y: 70, Width: 360, Height: 240}) {
		t.Fatal("SetWindowFloating(1, true) = false, want to float the window")
	}
	placement, ok := ws.Placement(1)
	if !ok || !placement.Floating || [4]int{placement.X, placement.Y, placement.Width, placement.Height} != [4]int{90, 70, 360, 240} {
		t.Fatalf("floating placement = %+v, %t, want rectangle (90,70,360,240)", placement, ok)
	}
	if got, focused := ws.FocusedWindow(); !focused || got != 1 {
		t.Errorf("FocusedWindow() = %d, %t, want floating window 1 to stay focused", got, focused)
	}
	if !ws.SetFloatingGeometry(1, Bounds{X: 140, Y: 110, Width: 420, Height: 300}) {
		t.Fatal("SetFloatingGeometry() = false, want to retain a moved and resized rectangle")
	}
	if !ws.SetWindowFloating(1, false, Bounds{}) {
		t.Fatal("SetWindowFloating(1, false) = false, want to restore tiled placement")
	}
	tiled, ok := ws.Placement(1)
	if !ok || tiled.Floating || tiled.Column != 0 {
		t.Fatalf("retiled placement = %+v, %t, want the original leading column", tiled, ok)
	}
	if !ws.SetWindowFloating(1, true, Bounds{}) {
		t.Fatal("SetWindowFloating(1, true) = false, want to float the window again")
	}
	placement, ok = ws.Placement(1)
	if !ok || [4]int{placement.X, placement.Y, placement.Width, placement.Height} != [4]int{140, 110, 420, 300} {
		t.Errorf("remembered floating placement = %+v, %t, want rectangle (140,110,420,300)", placement, ok)
	}
}

func TestReturningUnfocusedFloatingTabKeepsFocusOnRightColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusWindow(2) || !ws.ConsumeOrExpelWindowRight(nil) {
		t.Fatal("could not tab window 3 into window 2's column")
	}
	ws.AddWindow(4)
	if !ws.SetWindowFloating(3, true, Bounds{X: 500, Y: 80, Width: 300, Height: 200}) {
		t.Fatal("SetWindowFloating(3, true) = false, want inactive tab floated")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 4 {
		t.Fatalf("FocusedWindow() after floating tab = %d, %t, want right column window 4", got, ok)
	}

	if !ws.SetWindowFloating(3, false, Bounds{}) {
		t.Fatal("SetWindowFloating(3, false) = false, want inactive tab restored")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 4 {
		t.Errorf("FocusedWindow() after restoring tab = %d, %t, want right column window 4", got, ok)
	}
}

func TestMoveFloatingWindowPositionChangesAndRetainsTiledPosition(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	tiledBefore, _ := ws.Placement(1)
	if ws.MoveFloatingWindow(1, PositionChange{Kind: PositionAdjustFixed, Value: 1}, PositionChange{Kind: PositionAdjustFixed, Value: 1}) {
		t.Fatal("MoveFloatingWindow() adjusted an unset tiled position")
	}
	if !ws.MoveFloatingWindow(1, PositionChange{Kind: PositionSetFixed, Value: 120}, PositionChange{Kind: PositionSetFixed, Value: 80}) {
		t.Fatal("MoveFloatingWindow() = false, want to store the initial floating position")
	}
	tiled, ok := ws.Placement(1)
	if !ok || tiled.Floating || tiled.X != tiledBefore.X || tiled.Y != tiledBefore.Y {
		t.Fatalf("tiled placement after storing float position = %+v, want tiled geometry unchanged", tiled)
	}
	if !ws.SetWindowFloating(1, true, Bounds{X: 700, Y: 600, Width: tiled.Width, Height: tiled.Height}) {
		t.Fatal("SetWindowFloating() = false, want window 1 floating")
	}
	placement, ok := ws.Placement(1)
	if !ok || !placement.Floating || [2]int{placement.X, placement.Y} != [2]int{220, 130} {
		t.Fatalf("floating placement = %+v, want saved position (220,130)", placement)
	}
	if !ws.MoveFloatingWindow(1, PositionChange{Kind: PositionSetProportion, Value: 50}, PositionChange{Kind: PositionSetProportion, Value: 25}) {
		t.Fatal("MoveFloatingWindow() = false, want proportional move")
	}
	if !ws.MoveFloatingWindow(1, PositionChange{Kind: PositionAdjustProportion, Value: 10}, PositionChange{Kind: PositionAdjustProportion, Value: -5}) {
		t.Fatal("MoveFloatingWindow() = false, want proportional adjustment")
	}
	if !ws.MoveFloatingWindow(1, PositionChange{Kind: PositionAdjustFixed, Value: -20}, PositionChange{Kind: PositionAdjustFixed, Value: 10}) {
		t.Fatal("MoveFloatingWindow() = false, want fixed adjustment")
	}
	placement, ok = ws.Placement(1)
	if !ok || [2]int{placement.X, placement.Y} != [2]int{680, 220} {
		t.Errorf("floating placement after position changes = %+v, want (680,220)", placement)
	}
}

func TestFloatingResizeHonorsSizeHintsAndKeepsAnchoredEdges(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	ws.AddWindow(1)
	if !ws.SetWindowSizeHints(1, 300, 500, 200, 400) {
		t.Fatal("SetWindowSizeHints() = false, want to set floating bounds")
	}
	if !ws.SetWindowFloating(1, true, Bounds{X: 100, Y: 100, Width: 400, Height: 300}) {
		t.Fatal("SetWindowFloating() = false, want window 1 floating")
	}
	if !ws.ResizeWindow(1, 400, 300, 100, 100, ResizeOptions{
		ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
	}) {
		t.Fatal("ResizeWindow() = false, want to resize the floating window")
	}
	got, ok := ws.Placement(1)
	if !ok || [4]int{got.X, got.Y, got.Width, got.Height} != [4]int{200, 200, 300, 200} {
		t.Errorf("floating resize = %+v, want clamped rectangle (200,200,300,200)", got)
	}
}

func TestFocusSwitchBetweenFloatingAndTiledRestoresEachActiveWindow(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.FocusWindow(1) || !ws.SetWindowFloating(1, true, Bounds{X: 90, Y: 70, Width: 360, Height: 240}) {
		t.Fatal("could not float focused window 1")
	}
	if !ws.FocusTiling() {
		t.Fatal("FocusTiling() = false, want tiled layer focused")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("tiled FocusedWindow() = %d, %t, want 2", got, ok)
	}
	if !ws.FocusFloating() {
		t.Fatal("FocusFloating() = false, want previously focused float restored")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Fatalf("floating FocusedWindow() = %d, %t, want 1", got, ok)
	}
	if !ws.SwitchFocusFloatingAndTiling() {
		t.Fatal("SwitchFocusFloatingAndTiling() = false, want switch back to tiling")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Errorf("switched FocusedWindow() = %d, %t, want tiled window 2", got, ok)
	}
}

func TestMoveColumnKeepsWindowFocused(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)
	if !ws.FocusLeft() || !ws.MoveRight() {
		t.Fatal("expected to focus and move the middle column right")
	}

	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t, want moved window 2", got, ok)
	}
}

func TestFocusWindowSelectsItsColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusLeft()

	if !ws.FocusWindow(2) {
		t.Fatal("FocusWindow(2) = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t, want 2, true", got, ok)
	}
}

func TestFocusedColumnStaysInsideViewport(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 600, 20)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.AddWindow(3)
	ws.FocusRight()
	ws.FocusRight()

	placement, ok := ws.Placement(3)
	if !ok {
		t.Fatal("Placement(3) missing")
	}
	if placement.X < 0 || placement.X+placement.Width > 1000 {
		t.Fatalf("focused column %+v is outside the viewport", placement)
	}
}

func TestFocusedColumnKeepsConfiguredGapsAtOutputEdges(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 700, 20)
	ws.AddWindow(1)

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 120 || placement.Y != 70 || placement.Width != 700 || placement.Height != 760 {
		t.Fatalf("placement = %+v, want 20-pixel outer gaps around the tile", placement)
	}
}

func TestOversizedColumnRemainsScrollable(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 1200, 16)
	ws.AddWindow(1)

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 0 || placement.Width != 1200 {
		t.Fatalf("placement = %+v, want an oversized column left-aligned in the viewport", placement)
	}
}

func TestScrollByMovesViewportWithoutChangingFocusedColumn(t *testing.T) {
	w := NewWorkspace(Bounds{Width: 200, Height: 100}, 80, 10)
	for id := uint64(1); id <= 4; id++ {
		w.AddWindow(id)
	}
	w.FocusFirst()
	focused, _ := w.FocusedWindow()
	if w.ScrollBy(-1) {
		t.Fatal("ScrollBy moved left of the first column's initial padding")
	}
	if !w.ScrollBy(100) {
		t.Fatal("ScrollBy(100) did not move the viewport")
	}
	placement, _ := w.Placement(1)
	if placement.X != -90 {
		t.Fatalf("first column x = %d, want -90 after scrolling", placement.X)
	}
	if got, _ := w.FocusedWindow(); got != focused {
		t.Fatalf("focused window = %d, want unchanged %d", got, focused)
	}
	if !w.ScrollBy(100) {
		t.Fatal("ScrollBy did not clamp to the remaining content")
	}
	if w.ScrollBy(1) {
		t.Fatal("ScrollBy moved past the last column")
	}
	if !w.ScrollBy(-1000) || w.ScrollBy(-1) {
		t.Fatal("ScrollBy did not clamp at the first column")
	}
}

func TestInsertionPositionTargetsColumnAndTabbedTileBoundaries(t *testing.T) {
	w := NewWorkspace(Bounds{X: 10, Y: 20, Width: 400, Height: 300}, 100, 20)
	for id := uint64(1); id <= 3; id++ {
		w.AddWindow(id)
	}
	if got := w.InsertionPositionAt(12, 150); got != (InsertPosition{Column: 0}) {
		t.Fatalf("before-first insertion = %+v, want column 0", got)
	}
	if got := w.InsertionPositionAt(225, 150); got != (InsertPosition{Column: 2}) {
		t.Fatalf("between-column insertion = %+v, want column 2", got)
	}
	if got := w.InsertionPositionAt(500, 150); got != (InsertPosition{Column: 3}) {
		t.Fatalf("after-last insertion = %+v, want column 3", got)
	}

	tabbed := NewWorkspace(Bounds{X: 10, Y: 20, Width: 400, Height: 300}, 100, 20)
	tabbed.AddWindow(1)
	tabbed.AddWindow(2)
	tabbed.FocusLeft()
	if !tabbed.ConsumeWindowIntoColumn() || !tabbed.ToggleColumnTabbedDisplay() {
		t.Fatal("could not create a tabbed column")
	}
	active, _ := tabbed.Placement(1)
	upper := tabbed.InsertionPositionAt(float64(active.X+20), float64(active.Y+1))
	lower := tabbed.InsertionPositionAt(float64(active.X+20), float64(active.Y+active.Height-1))
	if !upper.InColumn || upper.Tile != 0 || !lower.InColumn || lower.Tile != 1 {
		t.Fatalf("tabbed insertion positions = upper %+v lower %+v, want before and after active tile", upper, lower)
	}
}

func TestMoveWindowToPositionReordersAndKeepsTabbedMembership(t *testing.T) {
	w := NewWorkspace(Bounds{Width: 500, Height: 300}, 100, 20)
	for id := uint64(1); id <= 3; id++ {
		w.AddWindow(id)
	}
	if !w.MoveWindowToPosition(w, 1, InsertPosition{Column: 3}) {
		t.Fatal("MoveWindowToPosition did not move window to the last column")
	}
	placements := w.Placements()
	if placements[0].ID != 2 || placements[1].ID != 3 || placements[2].ID != 1 {
		t.Fatalf("column order after move = %+v, want [2,3,1]", placements)
	}
}

func TestRemovingWindowRemovesItsColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 500, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.RemoveWindow(2)

	if got, want := ws.WindowOrder(), []uint64{1}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Fatalf("FocusedWindow() = %d, %t, want 1, true", got, ok)
	}
}

func TestFocusFirstAndLastColumn(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 250, Height: 100}, 100, 10)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	if !ws.FocusFirst() {
		t.Fatal("FocusFirst() = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 1 {
		t.Fatalf("FocusedWindow() = %d, %t after FocusFirst(), want 1, true", got, ok)
	}
	if !ws.FocusLast() {
		t.Fatal("FocusLast() = false, want true")
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 3 {
		t.Fatalf("FocusedWindow() = %d, %t after FocusLast(), want 3, true", got, ok)
	}
}

func TestMoveFocusedColumnToFirstAndLast(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 250, Height: 100}, 100, 10)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	ws.FocusWindow(2)
	if !ws.MoveFirst() {
		t.Fatal("MoveFirst() = false, want true")
	}
	if got, want := ws.WindowOrder(), []uint64{2, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after MoveFirst() = %v, want %v", got, want)
	}
	if got, ok := ws.FocusedWindow(); !ok || got != 2 {
		t.Fatalf("FocusedWindow() = %d, %t after MoveFirst(), want 2, true", got, ok)
	}
	if !ws.MoveLast() {
		t.Fatal("MoveLast() = false, want true")
	}
	if got, want := ws.WindowOrder(), []uint64{1, 3, 2}; !slices.Equal(got, want) {
		t.Fatalf("WindowOrder() after MoveLast() = %v, want %v", got, want)
	}
}

func TestResizeRetainsColumnWidthAndWindowHeightAcrossLayoutUpdates(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	if !ws.ResizeWindow(1, 16, 16, 560, 420, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}
	if ws.ResizeWindow(1, 0, 0, 0, 0, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow with an empty size = true, want false")
	}
	if !ws.FocusWindow(2) || !ws.FocusWindow(1) {
		t.Fatal("could not move focus away and back to the resized column")
	}
	ws.SetGeometry(Bounds{Width: 1200, Height: 900}, 600, 16)

	first, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if first.Width != 560 || first.Height != 420 {
		t.Fatalf("resized placement = %+v, want retained size 560x420", first)
	}
	second, ok := ws.Placement(2)
	if !ok {
		t.Fatal("Placement(2) missing")
	}
	if second.X != first.X+first.Width+16 {
		t.Fatalf("second column x = %d, want after resized first column at %d", second.X, first.X+first.Width+16)
	}
}

func TestVerticalResizePreservesConstrainedColumnWidth(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1200, Height: 800}, 400, 16)
	ws.AddWindow(1)
	if !ws.ResizeWindow(1, 16, 16, 900, 500, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("initial ResizeWindow() failed")
	}
	ws.SetWindowSizeHints(1, 0, 500, 0, 0)

	placement, ok := ws.Placement(1)
	if !ok || placement.Width != 500 {
		t.Fatalf("constrained placement = %+v, %t, want width 500", placement, ok)
	}
	if !ws.ResizeWindow(1, placement.X, placement.Y, placement.Width, 420, ResizeOptions{ResizeHeight: true}) {
		t.Fatal("vertical ResizeWindow() failed")
	}
	ws.SetWindowSizeHints(1, 0, 0, 0, 0)

	placement, _ = ws.Placement(1)
	if placement.Width != 900 || placement.Height != 420 {
		t.Fatalf("placement after vertical resize and hint removal = %+v, want 900x420", placement)
	}
}

func TestResizeClampsAndKeepsLeftAndTopAnchorsFixed(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 500, 0, 400, 0)

	// The pointer requested a 300x300 tile at (316, 166). Its right and bottom
	// edges were anchored at (616, 466), so size-hint clamping must move the
	// tile origin back as the tile grows to 500x400.
	if !ws.ResizeWindow(1, 316, 166, 300, 300, ResizeOptions{
		ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
	}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 116 || placement.Y != 66 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (116, 66), size 500x400", placement)
	}
	if right, bottom := placement.X+placement.Width, placement.Y+placement.Height; right != 616 || bottom != 466 {
		t.Fatalf("anchored edges = (%d, %d), want (616, 466)", right, bottom)
	}
}

func TestResizeClampsRightAndBottomEdgesWithoutMovingOrigin(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 500, 0, 400, 0)

	if !ws.ResizeWindow(1, 200, 100, 300, 300, ResizeOptions{ResizeWidth: true, ResizeHeight: true}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 200 || placement.Y != 100 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (200, 100), size 500x400", placement)
	}
}

func TestResizeClampsAtMaximumAndKeepsLeftAndTopAnchorsFixed(t *testing.T) {
	ws := NewWorkspace(Bounds{X: 100, Y: 50, Width: 1000, Height: 800}, 400, 16)
	ws.AddWindow(1)
	ws.SetWindowSizeHints(1, 0, 500, 0, 400)

	// The requested 600x500 tile exceeds the client maximum. The grabbed
	// right and bottom edges still need to stay at (616, 466) after clamping.
	if !ws.ResizeWindow(1, 16, -34, 600, 500, ResizeOptions{
		ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
	}) {
		t.Fatal("ResizeWindow(1) = false, want true")
	}

	placement, ok := ws.Placement(1)
	if !ok {
		t.Fatal("Placement(1) missing")
	}
	if placement.X != 116 || placement.Y != 66 || placement.Width != 500 || placement.Height != 400 {
		t.Fatalf("clamped placement = %+v, want origin (116, 66), size 500x400", placement)
	}
	if right, bottom := placement.X+placement.Width, placement.Y+placement.Height; right != 616 || bottom != 466 {
		t.Fatalf("anchored edges = (%d, %d), want (616, 466)", right, bottom)
	}
}

func focusedID(t *testing.T, ws *Workspace) uint64 {
	t.Helper()
	id, ok := ws.FocusedWindow()
	if !ok {
		t.Fatal("no focused window")
	}
	return id
}

func TestStripIndexActionsClampLikeNiri(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1800, Height: 800}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	ws.FocusWindow(3)
	if !ws.FocusColumn(1) || focusedID(t, ws) != 1 {
		t.Fatalf("FocusColumn(1) focused %d, want 1", focusedID(t, ws))
	}
	ws.FocusWindow(2)
	if !ws.FocusColumn(0) || focusedID(t, ws) != 1 {
		t.Fatalf("FocusColumn(0) focused %d, want 1", focusedID(t, ws))
	}
	if !ws.FocusColumn(9) || focusedID(t, ws) != 3 {
		t.Fatalf("FocusColumn(9) focused %d, want 3", focusedID(t, ws))
	}
	ws.FocusWindow(1)
	if !ws.MoveColumnToIndex(3) || !slices.Equal(ws.WindowOrder(), []uint64{2, 3, 1}) || focusedID(t, ws) != 1 {
		t.Fatalf("MoveColumnToIndex(3): order %v focus %d, want [2 3 1] focus 1", ws.WindowOrder(), focusedID(t, ws))
	}
	if !ws.MoveColumnToIndex(0) || !slices.Equal(ws.WindowOrder(), []uint64{1, 2, 3}) {
		t.Fatalf("MoveColumnToIndex(0): order %v, want [1 2 3]", ws.WindowOrder())
	}

	ws.AddWindow(4)
	if !ws.SetWindowFloating(4, true, Bounds{X: 100, Y: 100, Width: 200, Height: 200}) || !ws.FocusFloating() {
		t.Fatal("could not focus a floating window")
	}
	if ws.MoveColumnToIndex(2) {
		t.Fatal("MoveColumnToIndex moved a column while floating was focused")
	}
	if !ws.FocusColumn(2) || focusedID(t, ws) != 2 {
		t.Fatalf("FocusColumn(2) from floating focused %d, want tiled 2", focusedID(t, ws))
	}
}

func TestStripWrapAndOrFallbacks(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1800, Height: 900}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		ws.AddWindow(id)
	}
	ws.FocusWindow(3)
	if !ws.FocusColumnRightOrFirst() || focusedID(t, ws) != 1 {
		t.Fatalf("FocusColumnRightOrFirst on the last column focused %d, want 1", focusedID(t, ws))
	}
	if !ws.FocusColumnLeftOrLast() || focusedID(t, ws) != 3 {
		t.Fatalf("FocusColumnLeftOrLast on the first column focused %d, want 3", focusedID(t, ws))
	}

	// Column 1 becomes [1, 2, 3]; column 2 holds 4.
	col := NewWorkspace(Bounds{Width: 1800, Height: 900}, 400, 16)
	for _, id := range []uint64{1, 2, 3, 4} {
		col.AddWindow(id)
	}
	col.FocusWindow(1)
	col.ConsumeWindowIntoColumn()
	col.ConsumeWindowIntoColumn()
	col.FocusWindow(1)
	if !col.FocusWindowBottom() || focusedID(t, col) != 3 {
		t.Fatalf("FocusWindowBottom focused %d, want 3", focusedID(t, col))
	}
	if !col.FocusWindowTop() || focusedID(t, col) != 1 {
		t.Fatalf("FocusWindowTop focused %d, want 1", focusedID(t, col))
	}
	if !col.FocusWindowUpOrBottom() || focusedID(t, col) != 3 {
		t.Fatalf("FocusWindowUpOrBottom from the top focused %d, want 3", focusedID(t, col))
	}
	if !col.FocusWindowDownOrTop() || focusedID(t, col) != 1 {
		t.Fatalf("FocusWindowDownOrTop from the bottom focused %d, want 1", focusedID(t, col))
	}
	if !col.FocusWindowDownOrColumnRight() || focusedID(t, col) != 2 {
		t.Fatalf("FocusWindowDownOrColumnRight mid-column focused %d, want 2", focusedID(t, col))
	}
	col.FocusWindow(3)
	if !col.FocusWindowDownOrColumnRight() || focusedID(t, col) != 4 {
		t.Fatalf("FocusWindowDownOrColumnRight at the bottom focused %d, want 4", focusedID(t, col))
	}
	if !col.FocusWindowUpOrColumnLeft() || focusedID(t, col) != 3 {
		t.Fatalf("FocusWindowUpOrColumnLeft on a single tile focused %d, want column 1's active 3", focusedID(t, col))
	}
	if !col.FocusWindowUpOrColumnRight() || focusedID(t, col) != 2 {
		t.Fatalf("FocusWindowUpOrColumnRight mid-column focused %d, want 2", focusedID(t, col))
	}
	col.FocusWindow(1)
	if col.FocusWindowUpOrColumnLeft() {
		t.Fatal("FocusWindowUpOrColumnLeft at the top of the first column reported a change")
	}
	col.FocusWindow(3)
	if col.FocusWindowDownOrColumnLeft() || focusedID(t, col) != 3 {
		t.Fatalf("FocusWindowDownOrColumnLeft at the first column's bottom changed focus to %d", focusedID(t, col))
	}

	col.AddWindow(5)
	col.SetWindowFloating(5, true, Bounds{X: 100, Y: 500, Width: 100, Height: 100})
	col.AddWindow(6)
	col.SetWindowFloating(6, true, Bounds{X: 300, Y: 50, Width: 100, Height: 100})
	col.FocusWindow(5)
	if !col.FocusWindowTop() || focusedID(t, col) != 6 {
		t.Fatalf("FocusWindowTop with floating focused %d, want topmost float 6", focusedID(t, col))
	}
	if !col.FocusWindowBottom() || focusedID(t, col) != 5 {
		t.Fatalf("FocusWindowBottom with floating focused %d, want bottommost float 5", focusedID(t, col))
	}
}

func TestSwapWindowMatchesNiri(t *testing.T) {
	ws := NewWorkspace(Bounds{Width: 1800, Height: 900}, 400, 16)
	ws.AddWindow(1)
	ws.AddWindow(2)
	ws.FocusWindow(1)
	ws.ResizeWindow(1, 0, 0, 600, 0, ResizeOptions{ResizeWidth: true})
	if !ws.SwapWindowRight() || !slices.Equal(ws.WindowOrder(), []uint64{2, 1}) || focusedID(t, ws) != 1 {
		t.Fatalf("single-tile swap: order %v focus %d, want [2 1] focus 1", ws.WindowOrder(), focusedID(t, ws))
	}
	if placement, _ := ws.Placement(1); placement.Width != 600 {
		t.Fatalf("single-tile swap left window 1 %d wide, want its column's 600 (a column move)", placement.Width)
	}
	if ws.SwapWindowRight() {
		t.Fatal("SwapWindowRight on the last column reported a change")
	}

	col := NewWorkspace(Bounds{Width: 1800, Height: 900}, 400, 16)
	for _, id := range []uint64{1, 2, 3} {
		col.AddWindow(id)
	}
	col.FocusWindow(1)
	col.ConsumeWindowIntoColumn() // column A = [1, 2], column B = [3]
	col.FocusWindow(1)
	if !col.SwapWindowRight() {
		t.Fatal("SwapWindowRight() = false")
	}
	placements := map[uint64]Placement{}
	for _, placement := range col.Placements() {
		placements[placement.ID] = placement
	}
	if placements[3].Column != 0 || placements[3].Tile != 0 || placements[2].Column != 0 || placements[2].Tile != 1 ||
		placements[1].Column != 1 || placements[1].Tile != 0 {
		t.Fatalf("after swap: %+v, want A=[3 2] B=[1]", col.Placements())
	}
	if focusedID(t, col) != 1 {
		t.Fatalf("focus after swap = %d, want the moved window 1", focusedID(t, col))
	}
	if !col.SwapWindowLeft() || focusedID(t, col) != 1 {
		t.Fatal("SwapWindowLeft did not swap back")
	}
	col.AddWindow(4)
	col.SetWindowFloating(4, true, Bounds{X: 10, Y: 10, Width: 100, Height: 100})
	col.FocusWindow(4)
	if col.SwapWindowLeft() || col.SwapWindowRight() {
		t.Fatal("swap acted while floating was focused")
	}
}
