package wlr

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestOutputPointerTransformsPreserveAlphaAndClip(t *testing.T) {
	icon := image.NewRGBA(image.Rect(0, 0, 3, 2))
	icon.SetRGBA(0, 0, color.RGBA{R: 128, A: 128})
	icon.SetRGBA(2, 1, color.RGBA{B: 255, A: 255})
	for transform, points := range [][2]image.Point{
		{{1, 1}, {3, 2}}, {{1, 2}, {2, 0}},
		{{3, 2}, {1, 1}}, {{3, 1}, {2, 3}},
		{{3, 1}, {1, 2}}, {{3, 2}, {2, 0}},
		{{1, 2}, {3, 1}}, {{1, 1}, {2, 3}},
	} {
		dst := image.NewRGBA(image.Rect(0, 0, 5, 4))
		draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
		drawTransformedPointer(dst, icon, image.Pt(1, 1), uint32(transform))
		if got := dst.RGBAAt(points[0].X, points[0].Y); got != (color.RGBA{255, 127, 127, 255}) {
			t.Errorf("transform %d alpha pixel=%v", transform, got)
		}
		if got := dst.RGBAAt(points[1].X, points[1].Y); got != (color.RGBA{B: 255, A: 255}) {
			t.Errorf("transform %d corner=%v", transform, got)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, 5, 4))
	drawTransformedPointer(dst, icon, image.Pt(-1, 0), 0)
	if got := dst.RGBAAt(1, 1); got != (color.RGBA{B: 255, A: 255}) {
		t.Errorf("partially visible pointer clipped away: %v", got)
	}
}
