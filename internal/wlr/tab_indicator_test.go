package wlr

import (
	"image"
	"slices"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// TestTabIndicatorRectsFollowNiri: the tabs share Niri's length, sit beside
// the column at the gap and spread the leftover pixel over the first tabs.
// Pure Go, no compositor.
func TestTabIndicatorRectsFollowNiri(t *testing.T) {
	left := config.Default().Layout.TabIndicator
	top := left
	top.Position, top.Length, top.GapsBetweenTabs = config.TabIndicatorTop, 1, 2
	for _, tc := range []struct {
		name      string
		indicator config.TabIndicator
		area      image.Rectangle
		count     int
		want      []image.Rectangle
	}{
		{"left, half the height", left, image.Rect(100, 50, 300, 250), 2,
			[]image.Rectangle{image.Rect(91, 100, 95, 150), image.Rect(91, 150, 95, 200)}},
		{"top, whole width, gaps between", top, image.Rect(10, 20, 110, 220), 3,
			[]image.Rectangle{image.Rect(10, 11, 42, 15), image.Rect(44, 11, 76, 15), image.Rect(78, 11, 110, 15)}},
		{"left, odd length", left, image.Rect(0, 0, 50, 101), 2,
			[]image.Rectangle{image.Rect(-9, 25, -5, 51), image.Rect(-9, 51, -5, 76)}},
	} {
		if got := tabIndicatorRects(tc.indicator, tc.area, tc.count); !slices.Equal(got, tc.want) {
			t.Errorf("%s: rects = %v, want %v", tc.name, got, tc.want)
		}
	}

	rounded := left
	rounded.CornerRadius = 3
	if got := tabIndicatorRadii(rounded, 0, 2); got != (config.CornerRadius{3, 3, 0, 0}) {
		t.Errorf("first of two joined vertical tabs radii = %v, want the top corners", got)
	}
	if got := tabIndicatorRadii(rounded, 1, 3); got != (config.CornerRadius{}) {
		t.Errorf("middle joined tab radii = %v, want none", got)
	}
}
