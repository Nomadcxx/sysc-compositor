package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/text"
)

// ConfigErrorNotification uses Niri's 14px font, 8px padding and red frame.
func ConfigErrorNotification(scale float64) (*image.RGBA, error) {
	spans, err := text.ParseMarkup("Failed to parse the config file. Please run <span face='monospace' bgcolor='#000000'>sysc-compositor validate</span> to see the errors.")
	if err != nil {
		return nil, err
	}
	layout, err := text.NewLayout(spans, text.Font{Family: "sans", Size: 14}, scale, text.AlignCenter)
	if err != nil {
		return nil, err
	}
	padding := physical(scale, 8)
	img := image.NewRGBA(image.Rectangle{Max: layout.Size().Add(image.Pt(2*padding, 2*padding))})
	draw.Draw(img, img.Bounds(), image.NewUniform(exitDialogBackground), image.Point{}, draw.Src)
	layout.Draw(img, image.Pt(padding, padding), color.White)
	strokeBorder(img, int(math.Round(2*scale)), exitDialogBorderRGB)
	return img, nil
}
