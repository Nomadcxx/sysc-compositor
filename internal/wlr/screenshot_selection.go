package wlr

import (
	"image"
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

// shotSelection preserves physical precision while the public UI rectangles
// stay logical. Integer logical rectangles cannot describe one pixel at scale2.
type shotSelection struct {
	logical, pixels, frameBox image.Rectangle
	scale                     float64
	transform                 uint32
}

func (f *frozenFrame) pixelBounds() image.Rectangle {
	size := f.pixelSize
	if f.transform&1 != 0 {
		size.X, size.Y = size.Y, size.X
	}
	return image.Rectangle{Max: size}
}

func (shot *screenshotUI) selectionPixels() image.Rectangle {
	frame := shot.frames[shot.output]
	if frame == nil {
		return image.Rectangle{}
	}
	if shot.preciseOutput == shot.output && shot.preciseSelection.logical == shot.selection {
		return shot.preciseSelection.pixels
	}
	selection := shot.selection.Intersect(frame.box)
	if shot.selection.Empty() {
		point := frame.physical(shot.selection.Min)
		return image.Rectangle{Min: point, Max: point}
	}
	return image.Rectangle{Min: frame.physical(selection.Min), Max: frame.physical(selection.Max)}.Intersect(frame.pixelBounds())
}

func (shot *screenshotUI) setSelectionPixels(selection image.Rectangle) {
	frame := shot.frames[shot.output]
	if frame == nil {
		return
	}
	selection = selection.Intersect(frame.pixelBounds())
	at := func(p image.Point, round func(float64) float64) image.Point {
		return frame.box.Min.Add(image.Pt(int(round(float64(p.X)/frame.scale)), int(round(float64(p.Y)/frame.scale))))
	}
	// The envelope remains nonempty even when the exact physical width is
	// smaller than one logical pixel. Captures and drawing use pixels below.
	logical := image.Rectangle{Min: at(selection.Min, math.Floor), Max: at(selection.Max, math.Ceil)}
	shot.selection = logical
	shot.preciseOutput = shot.output
	shot.preciseSelection = shotSelection{logical: logical, pixels: selection, frameBox: frame.box, scale: frame.scale, transform: frame.transform}
}

func (shot *screenshotUI) resizeSelection(change layout.SizeChange, width bool) bool {
	frame := shot.frames[shot.output]
	if frame == nil {
		return false
	}
	bounds := frame.pixelBounds()
	if bounds.Empty() {
		return false
	}
	selection := shot.selectionPixels()
	anchor := image.Pt(min(max(selection.Min.X, 0), bounds.Max.X-1), min(max(selection.Min.Y, 0), bounds.Max.Y-1))
	selection = selection.Add(anchor.Sub(selection.Min))
	current, available, remaining := selection.Dy(), bounds.Dy(), bounds.Max.Y-anchor.Y
	if width {
		current, available, remaining = selection.Dx(), bounds.Dx(), bounds.Max.X-anchor.X
	}
	value := float64(change.Fixed) * frame.scale
	if change.Fixed == 0 {
		value = float64(available) * change.Proportion / 100
	}
	if change.Relative {
		value += float64(current)
	}
	value = math.Max(1, math.Min(float64(remaining), math.Round(value)))
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	if width {
		selection.Max.X = anchor.X + int(value)
	} else {
		selection.Max.Y = anchor.Y + int(value)
	}
	if selection.Dx() < 1 {
		selection.Max.X = selection.Min.X + 1
	}
	if selection.Dy() < 1 {
		selection.Max.Y = selection.Min.Y + 1
	}
	shot.setSelectionPixels(selection)
	return true
}

func (c *Compositor) setShotSize(change layout.SizeChange, width bool) bool {
	if c.shotUI == nil || !c.shotUI.resizeSelection(change, width) {
		return false
	}
	c.drawShotSelection()
	return true
}

func (f *frozenFrame) physicalPointer(x, y float64) image.Point {
	bounds := f.pixelBounds()
	px := int(math.Round((x - float64(f.box.Min.X)) * f.scale))
	py := int(math.Round((y - float64(f.box.Min.Y)) * f.scale))
	return image.Pt(max(0, min(bounds.Max.X-1, px)), max(0, min(bounds.Max.Y-1, py)))
}
