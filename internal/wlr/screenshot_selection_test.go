package wlr

import (
	"image"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestScreenshotSizingPreservesPhysicalPixels(t *testing.T) {
	identity := 1
	output := unsafe.Pointer(&identity)
	frame := &frozenFrame{box: image.Rect(50, 80, 850, 680), scale: 2, pixelSize: image.Pt(1600, 1200)}
	shot := &screenshotUI{output: output, frames: map[unsafe.Pointer]*frozenFrame{output: frame}}
	shot.setSelectionPixels(image.Rect(101, 99, 401, 299))
	if !shot.resizeSelection(layout.SizeChange{Fixed: 1}, true) || shot.selectionPixels() != image.Rect(101, 99, 103, 299) {
		t.Fatalf("fixed logical pixel width=%v", shot.selectionPixels())
	}
	shot.resizeSelection(layout.SizeChange{Proportion: 0}, true)
	shot.resizeSelection(layout.SizeChange{Proportion: 0}, false)
	if got := shot.selectionPixels(); got != image.Rect(101, 99, 102, 100) {
		t.Fatalf("one physical pixel=%v", got)
	}
	shot.resizeSelection(layout.SizeChange{Proportion: 50}, true)
	if got := shot.selectionPixels(); got.Min != image.Pt(101, 99) || got.Dx() != 800 {
		t.Fatalf("proportion size lost anchor/full-output basis: %v", got)
	}
	shot.resizeSelection(layout.SizeChange{Fixed: 100000}, true)
	if got := shot.selectionPixels(); got.Max.X != 1600 {
		t.Fatalf("size did not clamp to remaining output: %v", got)
	}
	// A direct logical selection edit, as used by pointer drags, invalidates
	// the precise cache instead of reusing stale pixels.
	shot.selection = image.Rect(60, 90, 80, 100)
	if got := shot.selectionPixels(); got != image.Rect(20, 20, 60, 40) {
		t.Fatalf("logical selection change retained stale pixels: %v", got)
	}
}

func TestScreenshotSizingUsesUprightFractionalOutput(t *testing.T) {
	identity := 1
	output := unsafe.Pointer(&identity)
	frame := &frozenFrame{box: image.Rect(0, 0, 480, 640), scale: 1.25, transform: 1, pixelSize: image.Pt(800, 600)}
	shot := &screenshotUI{output: output, frames: map[unsafe.Pointer]*frozenFrame{output: frame}, selection: image.Rect(10, 10, 110, 110)}
	shot.resizeSelection(layout.SizeChange{Proportion: 50}, true)
	shot.resizeSelection(layout.SizeChange{Proportion: 50}, false)
	if got := shot.selectionPixels(); got.Dx() != 300 || got.Dy() != 400 {
		t.Fatalf("rotated fractional size=%v want300x400", got)
	}
	shot.resizeSelection(layout.SizeChange{Fixed: -100000, Relative: true}, true)
	if got := shot.selectionPixels(); got.Dx() != 1 {
		t.Fatalf("negative relative width=%v", got)
	}
}
