// Package ui draws the compositor's own panels (Niri's src/ui) as images:
// the exit confirmation dialog, and later the hotkey overlay, the screenshot
// hint and the recent-windows panel. It is pure Go; the wlr package puts the
// images in the scene.
package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/text"
)

// Niri's exit_confirm_dialog.rs constants.
const (
	exitDialogPadding = 16
	exitDialogBorder  = 8
	exitDialogMarkup  = "Are you sure you want to exit sysc-compositor?\n\n" +
		"Press <span face='mono' bgcolor='#2C2C2C'> Enter </span> to confirm."
)

var (
	exitDialogFont       = text.Font{Family: "sans", Size: 14}
	exitDialogBackground = color.RGBA{26, 26, 26, 255}  // cairo rgb(0.1, 0.1, 0.1)
	exitDialogBorderRGB  = color.RGBA{255, 77, 77, 255} // cairo rgb(1, 0.3, 0.3)
	// ExitDialogBackdrop is the colour laid over each output behind the
	// dialog, before the open animation's opacity.
	ExitDialogBackdrop = [4]float32{0, 0, 0, 0.4}
)

// ExitDialog draws the exit confirmation panel at scale, in physical pixels.
func ExitDialog(scale float64) (*image.RGBA, error) {
	spans, err := text.ParseMarkup(exitDialogMarkup)
	if err != nil {
		return nil, err
	}
	layout, err := text.NewLayout(spans, exitDialogFont, scale, text.AlignCenter)
	if err != nil {
		return nil, err
	}
	padding := physical(scale, exitDialogPadding)
	size := layout.Size().Add(image.Pt(2*padding, 2*padding))
	img := image.NewRGBA(image.Rectangle{Max: size})
	draw.Draw(img, img.Bounds(), image.NewUniform(exitDialogBackground), image.Point{}, draw.Src)
	layout.Draw(img, image.Pt(padding, padding), color.White)
	// Niri strokes an even-width line along the edges, so half of it falls
	// inside the panel.
	strokeBorder(img, int(math.Round(exitDialogBorder/2*scale)), exitDialogBorderRGB)
	return img, nil
}

// physical is Niri's to_physical_precise_round for a logical length.
func physical(scale float64, logical int) int {
	return int(math.Round(float64(logical) * scale))
}

// strokeBorder paints a width-pixel frame inside img's edges.
func strokeBorder(img *image.RGBA, width int, c color.Color) {
	b := img.Bounds()
	src := image.NewUniform(c)
	for _, r := range []image.Rectangle{
		image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+width),
		image.Rect(b.Min.X, b.Max.Y-width, b.Max.X, b.Max.Y),
		image.Rect(b.Min.X, b.Min.Y, b.Min.X+width, b.Max.Y),
		image.Rect(b.Max.X-width, b.Min.Y, b.Max.X, b.Max.Y),
	} {
		draw.Draw(img, r, src, image.Point{}, draw.Src)
	}
}
