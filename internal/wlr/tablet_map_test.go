package wlr

import (
	"math"
	"testing"
)

// TestTabletPointFollowsNiriMapping checks tabletPoint against Niri 26.04's
// compute_tablet_position, worked by hand.
func TestTabletPointFollowsNiriMapping(t *testing.T) {
	output := rect{100, 50, 1600, 900} // 16:9
	cases := []struct {
		name      string
		x, y      float64
		target    rect
		transform int
		mapped    bool
		aspect    float64
		wantX     float64
		wantY     float64
	}{
		{"matching aspect is direct", 0.5, 0.5, output, 0, true, 16.0 / 9, 900, 500},
		{"unknown size skips the ratio", 0.25, 0.5, output, 0, true, 0, 500, 500},
		// A 4:3 tablet on 16:9: ratio 0.75, so y is divided by it.
		{"narrower tablet stretches y", 0.5, 0.3, output, 0, true, 4.0 / 3, 900, 50 + 0.3/0.75*900},
		// A 2:1 tablet on 16:9: ratio 1.125, so x is multiplied by it.
		{"wider tablet stretches x", 0.4, 0.5, output, 0, true, 2, 100 + 0.4*1.125*1600, 500},
		{"clamped one pixel inside", 1, 1, output, 0, true, 16.0 / 9, 100 + 1600 - 1, 50 + 900 - 1},
		// 90°: the device spans the rotated 900x1600 size; (x, y) -> (w - y, x).
		{"rotated output", 0.25, 0.75, output, 1, true, 900.0 / 1600, 100 + 1600 - 0.75*1600, 50 + 0.25*900},
		{"unified box keeps no ratio", 0.5, 0.5, rect{0, 0, 3840, 1080}, 0, false, 4.0 / 3, 1920, 540},
	}
	for _, tc := range cases {
		x, y := tabletPoint(tc.x, tc.y, tc.target, tc.transform, tc.mapped, tc.aspect, 1)
		if math.Abs(x-tc.wantX) > 1e-9 || math.Abs(y-tc.wantY) > 1e-9 {
			t.Errorf("%s: (%v, %v), want (%v, %v)", tc.name, x, y, tc.wantX, tc.wantY)
		}
	}
}
