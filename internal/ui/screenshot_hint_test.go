package ui

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestScreenshotHintIsNiriPanel(t *testing.T) {
	show, err := ScreenshotHint(false, 1)
	if err != nil {
		t.Fatal(err)
	}
	hide, err := ScreenshotHint(true, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := show.Bounds()
	if b.Dy() != 2*16+2*8 && b.Dy() < 2*14+2+2*8 {
		t.Fatalf("panel is %v", b)
	}
	if bytes.Equal(show.Pix, hide.Pix) {
		t.Error("the show-pointer and hide-pointer panels are the same")
	}
	white := color.RGBA{255, 255, 255, 255}
	cy := b.Dy() / 2
	// The button: a white disc, a 2px dark ring inside its edge, a white centre.
	if show.RGBAAt(8+16, cy) != white || show.RGBAAt(8+1, cy) != white ||
		show.RGBAAt(8+3, cy) != exitDialogBackground || show.RGBAAt(8+6, cy) != white {
		t.Errorf("capture button is not Niri's ringed disc: %v %v %v %v", show.RGBAAt(8+16, cy),
			show.RGBAAt(8+1, cy), show.RGBAAt(8+3, cy), show.RGBAAt(8+6, cy))
	}
	if show.RGBAAt(0, 0) != shotPanelBorderRGB || show.RGBAAt(1, cy) != shotPanelBorderRGB || show.RGBAAt(2, 2) != exitDialogBackground {
		t.Error("border is not Niri's 2px grey frame")
	}
	size := b.Size()
	if !ScreenshotHintButton(size, image.Pt(8+16, cy), 1) || ScreenshotHintButton(size, image.Pt(8+16+15, cy+10), 1) ||
		ScreenshotHintButton(size, image.Pt(b.Dx()/2, cy), 1) {
		t.Error("capture button hit-test is not the inner circle")
	}
	if got := ScreenshotHintOrigin(image.Pt(1920, 1080), size, 1); got != image.Pt((1920-size.X)/2, 1080-size.Y-16) {
		t.Errorf("panel origin = %v", got)
	}
}
