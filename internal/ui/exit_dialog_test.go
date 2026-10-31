package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestExitDialogIsNiriPanel(t *testing.T) {
	one, err := ExitDialog(1)
	if err != nil {
		t.Fatal(err)
	}
	b := one.Bounds()
	if b.Dx() < 300 || b.Dy() < 3*14+32 || b.Dy() > 4*24+32 {
		t.Fatalf("panel is %v", b)
	}
	at := func(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }
	if at(one, 0, 0) != exitDialogBorderRGB || at(one, 3, b.Dy()/2) != exitDialogBorderRGB ||
		at(one, b.Dx()-1, b.Dy()-1) != exitDialogBorderRGB {
		t.Errorf("border is not Niri's 4px red frame")
	}
	if at(one, 4, b.Dy()/2) != exitDialogBackground || at(one, 15, 15) != exitDialogBackground {
		t.Errorf("inside the frame = %v, want the dark background", at(one, 4, b.Dy()/2))
	}
	var white, key int
	for y := 16; y < b.Dy()-16; y++ {
		for x := 16; x < b.Dx()-16; x++ {
			switch c := at(one, x, y); {
			case c.R > 0xe0 && c.G > 0xe0 && c.B > 0xe0:
				white++
			case c == color.RGBA{0x2c, 0x2c, 0x2c, 0xff}:
				key++
			}
		}
	}
	if white < 200 || key < 100 {
		t.Errorf("text pixels %d, key background pixels %d", white, key)
	}

	two, err := ExitDialog(2)
	if err != nil {
		t.Fatal(err)
	}
	if d := two.Bounds().Dx() - 2*b.Dx(); d < -4 || d > 4 {
		t.Errorf("scale 2 panel %v is not twice %v", two.Bounds(), b)
	}
	if at(two, 7, two.Bounds().Dy()/2) != exitDialogBorderRGB || at(two, 8, two.Bounds().Dy()/2) != exitDialogBackground {
		t.Errorf("scale 2 border is not 8px")
	}
}

// TestExitDialogTextIsTheDrawnText: the screen reader reads the dialog's
// drawn words without their markup.
func TestExitDialogTextIsTheDrawnText(t *testing.T) {
	want := "Are you sure you want to exit sysc-compositor?\n\nPress Enter to confirm."
	if got := ExitDialogText(); got != want {
		t.Errorf("ExitDialogText() = %q, want %q", got, want)
	}
}
