package wlr

// touchTransformPoint applies an output transform to a position normalized to
// [0, 1] over the glass, as wlroots' apply_output_transform does. The turn
// happens in the unit square, before the position is scaled to the output's
// size, so a quarter turn swaps the sides of the output.
//
// The transform numbers are wl_output_transform's: 0 normal, 1 90, 2 180,
// 3 270, 4 flipped, 5 flipped 90, 6 flipped 180, 7 flipped 270.
func touchTransformPoint(x, y float64, transform int) (float64, float64) {
	switch transform {
	case 1: // 90
		return 1 - y, x
	case 2: // 180
		return 1 - x, 1 - y
	case 3: // 270
		return y, 1 - x
	case 4: // flipped
		return 1 - x, y
	case 5: // flipped 90
		return y, x
	case 6: // flipped 180
		return x, 1 - y
	case 7: // flipped 270
		return 1 - y, 1 - x
	default: // normal
		return x, y
	}
}

// touchInverseTransformPoint undoes touchTransformPoint: it takes a position
// on the turned output and gives back the position over the glass.
func touchInverseTransformPoint(x, y float64, transform int) (float64, float64) {
	switch transform {
	case 1: // 90
		return y, 1 - x
	case 2: // 180
		return 1 - x, 1 - y
	case 3: // 270
		return 1 - y, x
	case 4: // flipped
		return 1 - x, y
	case 5: // flipped 90
		return y, x
	case 6: // flipped 180
		return x, 1 - y
	case 7: // flipped 270
		return 1 - y, 1 - x
	default: // normal
		return x, y
	}
}
