package wm

import "testing"

func niriStrip() MRUStrip {
	return MRUStrip{OutputWidth: 1920, OutputHeight: 1080, Scale: 1, Padding: 30, MaxHeight: 480, MaxScale: 0.5}
}

func TestMRUPreviewSizeFollowsNiri(t *testing.T) {
	s := niriStrip()
	for _, tc := range []struct{ w, h, wantW, wantH float64 }{
		{960, 1080, 427, 480}, // bounded by max-height
		{100, 50, 50, 25},     // bounded by max-scale
		{10, 10, 10, 10},      // windows up to 16px are not shrunk
		{3840, 100, 853, 22},  // bounded by the output's aspect ratio
	} {
		if w, h := s.PreviewSize(tc.w, tc.h); w != tc.wantW || h != tc.wantH {
			t.Errorf("preview of %vx%v = %vx%v, want %vx%v", tc.w, tc.h, w, h, tc.wantW, tc.wantH)
		}
	}
	s.Scale = 1.5
	if w, h := s.PreviewSize(960, 1080); w != 426+2/3. || h != 480 {
		t.Errorf("preview at scale 1.5 = %vx%v, want whole physical pixels", w, h)
	}
}

func TestMRUStripCentresOrScrollsToTheSelection(t *testing.T) {
	s := niriStrip()
	rects := s.Layout([][2]float64{{960, 1080}, {960, 1080}})
	// The gap is the highlight padding (30 + 2) on each side and 16 between.
	if rects[0] != (MRURect{0, 300, 427, 480}) || rects[1] != (MRURect{507, 300, 427, 480}) {
		t.Fatalf("strip = %+v", rects)
	}
	if pos := s.ViewPos(0, rects, 1); pos != -493 {
		t.Errorf("a strip that fits starts at %v, want it centred at -493", pos)
	}

	sizes := make([][2]float64, 6)
	for i := range sizes {
		sizes[i] = [2]float64{960, 1080}
	}
	rects = s.Layout(sizes)
	// The last preview is far right: the view moves so it ends at the strut.
	pos := s.ViewPos(0, rects, 5)
	if pos != 1250 {
		t.Fatalf("view for the last preview = %v, want 1250", pos)
	}
	// A preview already in view leaves the view where it is.
	if again := s.ViewPos(pos, rects, 4); again != pos {
		t.Errorf("selecting a visible preview moved the view from %v to %v", pos, again)
	}
	// The first preview is nearer the left: the view starts it at the strut.
	if first := s.ViewPos(pos, rects, 0); first != -208 {
		t.Errorf("view for the first preview = %v, want -208", first)
	}
	box := s.Highlight(rects[0], 17)
	if box != (MRURect{-32, 268, 491, 544 + 14 + 17 - 16}) {
		t.Errorf("highlight = %+v", box)
	}
	if !box.Contains(-32, 268) || box.Contains(459, 268) {
		t.Error("Contains does not take the left and top edges only")
	}
}

// TestMRURemovalMovesFollowNiri is Niri's WindowMruUi::remove_window: the
// previews beside a removed one slide into its gap; when the gap is left of
// the selection the left ones move and the view shifts, so the selection
// stays where it was drawn.
func TestMRURemovalMovesFollowNiri(t *testing.T) {
	s := niriStrip()
	listed := []uint64{1, 2, 3, 4}
	const gap = 507. // a 427px preview and Niri's 80px gap

	moves, view := s.RemovalMoves(listed, 3, 2, 427)
	if want := map[uint64]float64{1: -gap}; !equalMoves(moves, want) || view != -gap {
		t.Errorf("removing left of the selection: moves %v view %v, want %v and %v", moves, view, want, -gap)
	}
	moves, view = s.RemovalMoves(listed, 2, 2, 427)
	if want := map[uint64]float64{3: gap, 4: gap}; !equalMoves(moves, want) || view != 0 {
		t.Errorf("removing the selection: moves %v view %v, want %v and 0", moves, view, want)
	}
	moves, view = s.RemovalMoves(listed, 1, 3, 427)
	if want := map[uint64]float64{4: gap}; !equalMoves(moves, want) || view != 0 {
		t.Errorf("removing right of the selection: moves %v view %v, want %v and 0", moves, view, want)
	}
	if moves, view = s.RemovalMoves(listed, 1, 4, 427); len(moves) != 0 || view != 0 {
		t.Errorf("removing the last listed preview moved %v and the view by %v", moves, view)
	}
	// Niri slides every listed preview when an unlisted window closes; here
	// nothing listed moves.
	if moves, view = s.RemovalMoves(listed, 1, 9, 427); len(moves) != 0 || view != 0 {
		t.Errorf("removing an unlisted window moved %v and the view by %v", moves, view)
	}
}

// TestMRURelistMovesFollowNiri is Niri's animate_scope_filter_change: a
// preview listed before and after starts where it was, offset by the
// previews that left or joined between it and the selection, and the view
// shifts by what changed left of the selection.
func TestMRURelistMovesFollowNiri(t *testing.T) {
	s := niriStrip()
	windows := []uint64{1, 2, 3, 4, 5, 6}
	width := func(id uint64) float64 { return 100 * float64(id) }
	set := func(ids ...uint64) func(uint64) bool {
		return func(id uint64) bool {
			for _, i := range ids {
				if i == id {
					return true
				}
			}
			return false
		}
	}
	// Before: every window. After: 2 and 5 gone. The selection is 3.
	moves, view := s.RelistMoves(windows, set(1, 2, 3, 4, 5, 6), set(1, 3, 4, 6), 3, width)
	want := map[uint64]float64{
		4: 0,           // right next to the selection: nothing left between
		6: 500 + 80,    // 5 left the gap between
		1: -(200 + 80), // 2 left the gap between, on the left
	}
	if !equalMoves(moves, want) || view != -(200+80) {
		t.Errorf("narrowing: moves %v view %v, want %v and %v", moves, view, want, -(200 + 80.))
	}
	// Widening back: 2 and 5 join, so the others start nearer the selection.
	moves, view = s.RelistMoves(windows, set(1, 3, 4, 6), set(1, 2, 3, 4, 5, 6), 3, width)
	want = map[uint64]float64{4: 0, 6: -(500 + 80), 1: 200 + 80}
	if !equalMoves(moves, want) || view != 200+80 {
		t.Errorf("widening: moves %v view %v, want %v and %v", moves, view, want, 200+80.)
	}
	if moves, view = s.RelistMoves(windows, set(1), set(2), 9, width); len(moves) != 0 || view != 0 {
		t.Errorf("a relist with no selection moved %v and the view by %v", moves, view)
	}
}

func equalMoves(got, want map[uint64]float64) bool {
	if len(got) != len(want) {
		return false
	}
	for id, from := range want {
		if g, ok := got[id]; !ok || g != from {
			return false
		}
	}
	return true
}
