package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/text"
)

// Niri's mru.rs constants.
const (
	mruPanelPadding = 12
	mruPanelBorder  = 4
	mruTitleMax     = 16383
	// mruTitleFade is how much of a clipped title fades out, in logical
	// pixels (Niri's GradientFadeTextureRenderElement).
	mruTitleFade = 18
)

var (
	mruFont           = text.Font{Family: "sans", Size: 14}
	mruPanelBorderRGB = color.RGBA{128, 128, 128, 255} // cairo rgb(0.5, 0.5, 0.5)
	// MRUBackdrop is the colour laid over every output while the switcher
	// is shown.
	MRUBackdrop = [4]float32{0, 0, 0, 0.8}
)

// MRUTitle draws a window title in white on transparent at scale, one line
// as Pango's single-paragraph mode lays it out, at most maxWidth physical
// pixels wide: a longer title is cut on the right and fades out there.
func MRUTitle(title string, maxWidth int, scale float64) (*image.RGBA, error) {
	title = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(title)
	layout, err := text.NewLayout(text.Plain(title), mruFont, scale, text.AlignLeft)
	if err != nil {
		return nil, err
	}
	size := layout.Size()
	full := image.NewRGBA(image.Rect(0, 0, min(size.X, mruTitleMax), min(size.Y, mruTitleMax)))
	layout.Draw(full, image.Point{}, color.White)
	if maxWidth <= 0 || full.Rect.Dx() <= maxWidth {
		return full, nil
	}
	img := full.SubImage(image.Rect(0, 0, maxWidth, full.Rect.Dy())).(*image.RGBA)
	// The fade covers the last 18 logical pixels, or all of a narrower cut.
	fade := min(float64(maxWidth), mruTitleFade*scale)
	start := float64(maxWidth) - fade
	for x := int(start); x < maxWidth; x++ {
		k := math.Min(1, math.Max(0, (float64(maxWidth)-float64(x))/fade))
		for y := range img.Rect.Dy() {
			i := img.PixOffset(x, y)
			for c := range 4 {
				img.Pix[i+c] = uint8(math.Round(float64(img.Pix[i+c]) * k))
			}
		}
	}
	return img, nil
}

// MRUScopes is the order of the scope panel's names: All, Workspace and
// Output, Niri's SCOPE_CYCLE.
const (
	MRUScopeAll = iota
	MRUScopeWorkspace
	MRUScopeOutput
)

// MRUScopePanel draws the switcher's scope panel with selected (an index
// into All, Workspace, Output) in white and the rest grey, each with its key
// letter on a key background, as Niri's generate_scope_panels.
func MRUScopePanel(selected int, scale float64) (*image.RGBA, error) {
	const (
		unselected  = "<span fgcolor='#999999'>"
		end         = "</span>"
		shortcut    = "<span face='mono' bgcolor='#2C2C2C' letter_spacing='5000'><b>"
		shortcutEnd = "</b></span>"
	)
	// A zero-width space first, so the letter spacing shows on the left.
	markup := "​" + unselected + shortcut + "S" + shortcutEnd + "cope:" + end
	for i, name := range []string{"All", "Workspace", "Output"} {
		markup += "  "
		if i != selected {
			markup += unselected
		}
		markup += shortcut + name[:1] + shortcutEnd + name[1:]
		if i != selected {
			markup += end
		}
	}
	spans, err := text.ParseMarkup(markup)
	if err != nil {
		return nil, err
	}
	layout, err := text.NewLayout(spans, mruFont, scale, text.AlignLeft)
	if err != nil {
		return nil, err
	}
	padding := physical(scale, mruPanelPadding)
	size := layout.Size().Add(image.Pt(2*padding, 2*padding))
	img := image.NewRGBA(image.Rectangle{Max: size})
	draw.Draw(img, img.Bounds(), image.NewUniform(exitDialogBackground), image.Point{}, draw.Src)
	layout.Draw(img, image.Pt(padding, padding), color.White)
	strokeBorder(img, int(math.Round(mruPanelBorder/2*scale)), mruPanelBorderRGB)
	return img, nil
}

// MRUPanelPadding is the scope panel's padding in logical pixels; the panel
// sits twice that below the output's top edge.
const MRUPanelPadding = mruPanelPadding
