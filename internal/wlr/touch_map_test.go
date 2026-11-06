package wlr

import (
	"math"
	"testing"
)

// TestTouchTransformPointFollowsWlroots checks the touch point transform
// against wlroots' apply_output_transform, which works in the unit square
// because a touch reports a position normalized to 0..1.
func TestTouchTransformPointFollowsWlroots(t *testing.T) {
	const x, y = 0.2, 0.6
	cases := []struct {
		name      string
		transform int
		wantX     float64
		wantY     float64
	}{
		{"normal", 0, 0.2, 0.6},
		{"rotated 90", 1, 0.4, 0.2},
		{"rotated 180", 2, 0.8, 0.4},
		{"rotated 270", 3, 0.6, 0.8},
		{"flipped", 4, 0.8, 0.6},
		{"flipped 90", 5, 0.6, 0.2},
		{"flipped 180", 6, 0.2, 0.4},
		{"flipped 270", 7, 0.4, 0.8},
	}
	for _, tc := range cases {
		gotX, gotY := touchTransformPoint(x, y, tc.transform)
		if math.Abs(gotX-tc.wantX) > 1e-9 || math.Abs(gotY-tc.wantY) > 1e-9 {
			t.Errorf("%s: (%v, %v), want (%v, %v)", tc.name, gotX, gotY, tc.wantX, tc.wantY)
		}
	}
}

// TestTouchInverseTransformPointUndoesTheTransform checks that a layout
// point can be mapped back onto the device, which is what a test needs to
// aim a fake finger at a window.
func TestTouchInverseTransformPointUndoesTheTransform(t *testing.T) {
	for transform := 0; transform < 8; transform++ {
		for _, point := range [][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}, {0.2, 0.6}, {0.5, 0.5}} {
			tx, ty := touchTransformPoint(point[0], point[1], transform)
			gotX, gotY := touchInverseTransformPoint(tx, ty, transform)
			if math.Abs(gotX-point[0]) > 1e-9 || math.Abs(gotY-point[1]) > 1e-9 {
				t.Errorf("transform %d: (%v, %v) came back as (%v, %v)",
					transform, point[0], point[1], gotX, gotY)
			}
		}
	}
}
