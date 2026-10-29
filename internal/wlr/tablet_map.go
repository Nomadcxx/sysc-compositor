package wlr

import "math"

// tabletPoint maps a tablet tool's position, normalized to [0, 1] on the
// device, into layout coordinates as Niri 26.04's compute_tablet_position
// does. A mapped tablet covers target, an output's logical box under its
// wl_output transform, keeping the device's aspect ratio as Mutter does
// (aspect is width over height; 0 when the device has no size). An unmapped
// tablet covers target, the bounding box of every output, with no transform
// or ratio correction, which is what OpenTabletDriver expects. px is one
// logical pixel of the mapped (or first) output: the point stays that far
// inside the right and bottom edges.
func tabletPoint(x, y float64, target rect, transform int, mapped bool, aspect, px float64) (float64, float64) {
	if !mapped {
		transform = 0
	}
	w, h := float64(target.w), float64(target.h)
	// The device spans the output's size before its transform: rotations
	// by 90 or 270 degrees swap the sides.
	sw, sh := w, h
	if transform%2 == 1 {
		sw, sh = h, w
	}
	px0, py0 := x*sw, y*sh
	switch transform {
	case 1: // 90
		px0, py0 = sh-py0, px0
	case 2: // 180
		px0, py0 = sw-px0, sh-py0
	case 3: // 270
		px0, py0 = py0, sw-px0
	case 4: // flipped
		px0 = sw - px0
	case 5: // flipped 90
		px0, py0 = py0, px0
	case 6: // flipped 180
		py0 = sh - py0
	case 7: // flipped 270
		px0, py0 = sh-py0, sw-px0
	}
	if mapped && aspect > 0 {
		px0, py0 = px0/w, py0/h
		if ratio := aspect / (sw / sh); ratio > 1 {
			px0 *= ratio
		} else {
			py0 /= ratio
		}
		px0, py0 = px0*w, py0*h
	}
	px0 = math.Max(0, math.Min(px0, w-px))
	py0 = math.Max(0, math.Min(py0, h-px))
	return px0 + float64(target.x), py0 + float64(target.y)
}
