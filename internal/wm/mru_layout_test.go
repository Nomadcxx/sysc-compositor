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
