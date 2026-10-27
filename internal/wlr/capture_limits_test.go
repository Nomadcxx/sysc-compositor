package wlr

import (
	"math"
	"testing"
)

func TestCaptureAllocationRejectsUntrustedDimensions(t *testing.T) {
	for _, tc := range []struct {
		w, h, stride int
		valid        bool
	}{
		{1, 1, 4, true}, {4096, 4096, 16384, true}, {4096, 4097, 16384, false},
		{0, 1, 4, false}, {-1, 1, 4, false}, {1, 0, 4, false}, {1, -1, 4, false},
		{2, 1, 7, false}, {1, 1, -4, false}, {math.MaxInt, 2, math.MaxInt, false},
		{1, math.MaxInt, 4, false}, {1, 2, math.MaxInt, false},
	} {
		_, err := captureAllocation(tc.w, tc.h, tc.stride)
		if (err == nil) != tc.valid {
			t.Errorf("allocation %dx%d stride %d err=%v, valid=%t", tc.w, tc.h, tc.stride, err, tc.valid)
		}
	}
}
