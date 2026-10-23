package ui

import (
	"image/color"
	"strings"
	"testing"
)

func TestMRUTitleFadesOutWhenCut(t *testing.T) {
	full, err := MRUTitle("A window title that runs on", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h := full.Bounds().Dy(); h < 14 || h > 24 {
		t.Fatalf("title is %v tall, want one 14px line", full.Bounds())
	}
	if two, _ := MRUTitle("two\nlines", 0, 1); two.Bounds().Dy() != full.Bounds().Dy() {
		t.Errorf("a title with a line break is %v, want one line", two.Bounds())
	}
	cut, err := MRUTitle("A window title that runs on", 60, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cut.Bounds().Dx() != 60 {
		t.Fatalf("cut title is %v wide, want 60", cut.Bounds())
	}
	// Over the last 18 pixels the text fades: the brightest pixel per column
	// falls off towards the right edge.
	brightest := func(x int) uint8 {
		var m uint8
		for y := cut.Bounds().Min.Y; y < cut.Bounds().Max.Y; y++ {
			m = max(m, cut.RGBAAt(cut.Bounds().Min.X+x, y).A)
		}
		return m
	}
	var left, right uint8
	for x := range 40 {
		left = max(left, brightest(x))
	}
	for x := 57; x < 60; x++ {
		right = max(right, brightest(x))
	}
	if left < 0xe0 || right > left/5 {
		t.Errorf("unfaded text alpha %d, last 3 columns %d", left, right)
	}
}

func TestMRUScopePanelIsNiriPanel(t *testing.T) {
	panels := make([]string, 3)
	for selected := range 3 {
		img, err := MRUScopePanel(selected, 1)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if b.Dy() < 14+24 || b.Dy() > 24+24 || b.Dx() < 200 {
			t.Fatalf("panel %d is %v", selected, b)
		}
		if img.RGBAAt(0, 0) != mruPanelBorderRGB || img.RGBAAt(1, b.Dy()/2) != mruPanelBorderRGB ||
			img.RGBAAt(2, b.Dy()/2) != exitDialogBackground {
			t.Errorf("panel %d border is not Niri's 2px grey frame", selected)
		}
		var white, grey, key int
		for y := 12; y < b.Dy()-12; y++ {
			for x := 12; x < b.Dx()-12; x++ {
				switch c := img.RGBAAt(x, y); {
				case c.R > 0xe0 && c.G > 0xe0:
					white++
				case c.R > 0x90 && c.R < 0xa0 && c.G == c.R:
					grey++
				case c == (color.RGBA{0x2c, 0x2c, 0x2c, 0xff}):
					key++
				}
			}
		}
		if white < 30 || grey < 100 || key < 100 {
			t.Errorf("panel %d: white %d, grey %d, key %d pixels", selected, white, grey, key)
		}
		panels[selected] = string(img.Pix)
	}
	if panels[0] == panels[1] || panels[1] == panels[2] || strings.Compare(panels[0], panels[2]) == 0 {
		t.Error("the three scopes draw the same panel")
	}
}
