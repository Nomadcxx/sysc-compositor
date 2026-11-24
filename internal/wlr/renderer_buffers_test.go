package wlr

import "testing"

// The SHM backup patches in only what a commit damaged, in buffer pixels,
// and falls back to a full copy when the buffer changed shape.
func TestRendererBackupCopiesOnlyDamage(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		width, sourceWidth, scale int
		buffer, surface           rect
		want                      int
	}{
		{"buffer damage", 8, 8, 1, rect{1, 0, 2, 1}, rect{}, 2},
		{"surface damage at scale 2", 8, 8, 2, rect{}, rect{0, 1, 1, 1}, 4},
		{"no damage", 8, 8, 1, rect{}, rect{}, 0},
		{"damage past the buffer", 8, 8, 1, rect{6, 3, 5, 5}, rect{}, 2},
		{"resized buffer", 8, 6, 1, rect{0, 0, 1, 1}, rect{}, -1},
	} {
		if got := patchBufferPixelsForTest(tc.width, 4, tc.sourceWidth, tc.scale, tc.buffer, tc.surface); got != tc.want {
			t.Errorf("%s: %d pixels changed, want %d", tc.name, got, tc.want)
		}
	}
}
