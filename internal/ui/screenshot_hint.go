package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/text"
	"golang.org/x/image/vector"
)

// Niri's screenshot_ui.rs panel constants.
const (
	shotPanelPadding = 8
	shotPanelRadius  = 16
	shotPanelBorder  = 4
	shotTextHideP    = "Press <span face='mono' bgcolor='#2C2C2C'> Space </span> to save the screenshot.\n" +
		"Press <span face='mono' bgcolor='#2C2C2C'> P </span> to hide the pointer."
	shotTextShowP = "Press <span face='mono' bgcolor='#2C2C2C'> Space </span> to save the screenshot.\n" +
		"Press <span face='mono' bgcolor='#2C2C2C'> P </span> to show the pointer."
	// ScreenshotHintOpacity is the panel's alpha; ScreenshotHintDragOpacity
	// is its alpha while a selection is being dragged.
	ScreenshotHintOpacity     = 0.9
	ScreenshotHintDragOpacity = 0.3
)

var shotPanelBorderRGB = color.RGBA{77, 77, 77, 255} // cairo rgb(0.3, 0.3, 0.3)

// ScreenshotHint draws the screenshot UI's help panel at scale: the capture
// button, then how to save and how to toggle the pointer, which pointerShown
// picks.
func ScreenshotHint(pointerShown bool, scale float64) (*image.RGBA, error) {
	markup := shotTextShowP
	if pointerShown {
		markup = shotTextHideP
	}
	spans, err := text.ParseMarkup(markup)
	if err != nil {
		return nil, err
	}
	layout, err := text.NewLayout(spans, hotkeyFont, scale, text.AlignLeft)
	if err != nil {
		return nil, err
	}
	// 2px between lines keeps the Space and P key backgrounds apart.
	layout.SetLineSpacing(physical(scale, 2))
	padding := physical(scale, shotPanelPadding)
	halfBorder := int(math.Round(shotPanelBorder / 2 * scale))
	radius := physical(scale, shotPanelRadius)
	stroke := float64(physical(scale, 2))

	size := layout.Size()
	width := size.X + padding + 2*radius + padding - halfBorder + padding
	height := max(size.Y, 2*radius) + 2*padding
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(exitDialogBackground), image.Point{}, draw.Src)

	cx, cy, r := float64(padding+radius), float64(height/2), float64(radius)
	fillCircle(img, cx, cy, r, color.White)
	fillCircle(img, cx, cy, r-stroke, exitDialogBackground)
	fillCircle(img, cx, cy, r-2*stroke, color.White)

	layout.Draw(img, image.Pt(padding+2*radius+padding-halfBorder, padding), color.White)
	strokeBorder(img, halfBorder, shotPanelBorderRGB)
	return img, nil
}

// ScreenshotHintButton reports whether p, in the panel's physical pixels,
// is on the capture button, as Niri's is_within_capture_button.
func ScreenshotHintButton(panel image.Point, p image.Point, scale float64) bool {
	padding := physical(scale, shotPanelPadding)
	r := physical(scale, shotPanelRadius) - 2
	dx, dy := p.X-(padding+r), p.Y-panel.Y/2
	return dx*dx+dy*dy <= r*r
}

// ScreenshotHintOrigin is where the panel goes on an output of size, both in
// physical pixels: centred, one padding's gap above the bottom twice over.
func ScreenshotHintOrigin(output, panel image.Point, scale float64) image.Point {
	padding := physical(scale, shotPanelPadding)
	return image.Pt(max(0, (output.X-panel.X)/2), max(0, output.Y-panel.Y-2*padding))
}

// fillCircle paints an antialiased disc.
func fillCircle(img *image.RGBA, cx, cy, r float64, c color.Color) {
	if r <= 0 {
		return
	}
	box := image.Rect(int(math.Floor(cx-r)), int(math.Floor(cy-r)), int(math.Ceil(cx+r)), int(math.Ceil(cy+r))).Intersect(img.Bounds())
	if box.Empty() {
		return
	}
	var z vector.Rasterizer
	z.Reset(box.Dx(), box.Dy())
	x, y := float32(cx)-float32(box.Min.X), float32(cy)-float32(box.Min.Y)
	rr := float32(r)
	k := float32(0.5522847498) * rr // cubic Bézier quarter circle
	z.MoveTo(x+rr, y)
	z.CubeTo(x+rr, y+k, x+k, y+rr, x, y+rr)
	z.CubeTo(x-k, y+rr, x-rr, y+k, x-rr, y)
	z.CubeTo(x-rr, y-k, x-k, y-rr, x, y-rr)
	z.CubeTo(x+k, y-rr, x+rr, y-k, x+rr, y)
	z.ClosePath()
	z.DrawOp = draw.Over
	z.Draw(img, box, image.NewUniform(c), image.Point{})
}
