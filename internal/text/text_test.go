package text

import (
	"image"
	"image/color"
	"testing"

	"github.com/go-text/typesetting/fontscan"
)

func TestParseMarkupTakesNiriStrings(t *testing.T) {
	spans, err := ParseMarkup("Press <span face='mono' bgcolor='#2C2C2C'> Space </span> to save &amp; quit.")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 || PlainText(spans) != "Press  Space  to save & quit." {
		t.Fatalf("spans = %+v", spans)
	}
	key := spans[1].Style
	if key.Family != "mono" || key.BG == nil || *key.BG != (color.NRGBA{0x2c, 0x2c, 0x2c, 0xff}) || key.FG != nil {
		t.Fatalf("key style = %+v", key)
	}
	if spans[0].Style != (Style{}) || spans[2].Style != (Style{}) {
		t.Fatalf("plain spans styled: %+v", spans)
	}

	spans, err = ParseMarkup("<span fgcolor='#999999'><span face='mono' letter_spacing='5000'><b>S</b></span>cope:</span>")
	if err != nil {
		t.Fatal(err)
	}
	s := spans[0].Style
	if len(spans) != 2 || !s.Bold || s.LetterSpacing != 5000 || s.Family != "mono" || s.FG == nil || s.FG.R != 0x99 {
		t.Fatalf("nested spans = %+v", spans)
	}
	if spans[1].Text != "cope:" || spans[1].Style.Bold || spans[1].Style.FG == nil {
		t.Fatalf("after nesting = %+v", spans[1])
	}

	for _, bad := range []string{"<u>x</u>", "<span size='large'>x</span>", "<span bgcolor='red'>x</span>", "a < b"} {
		if _, err := ParseMarkup(bad); err == nil {
			t.Errorf("ParseMarkup(%q) took it", bad)
		}
	}
}

// ink is the bounding box of the pixels drawn in img and how many there are.
func ink(img *image.RGBA, r image.Rectangle) (image.Rectangle, int) {
	var box image.Rectangle
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0x40 {
				n++
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box, n
}

func TestLayoutDrawsTextAtTheFontSizeAndScale(t *testing.T) {
	one, err := NewLayout(Plain("Are you sure you want to exit?"), Font{"sans", 14}, 1, AlignLeft)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewLayout(Plain("Are you sure you want to exit?"), Font{"sans", 14}, 2, AlignLeft)
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := one.Size(), two.Size()
	if s1.Y < 14 || s1.Y > 24 || s1.X < 100 {
		t.Fatalf("14px line is %v", s1)
	}
	if d := s2.Y - 2*s1.Y; d < -2 || d > 2 {
		t.Fatalf("scale 2 is %v, scale 1 is %v", s2, s1)
	}

	img := image.NewRGBA(image.Rectangle{Max: s1})
	one.Draw(img, image.Point{}, color.White)
	box, n := ink(img, img.Bounds())
	if n < 100 {
		t.Fatalf("only %d inked pixels", n)
	}
	if box.Dx() < s1.X*3/4 || box.Dy() < s1.Y/2 {
		t.Fatalf("ink %v does not fill the layout %v", box, s1)
	}
	if c := img.RGBAAt(box.Min.X+box.Dx()/2, 0); c.R != c.B {
		t.Fatalf("white text drew %v", c)
	}
}

func TestLayoutBreaksLinesAndCentresThem(t *testing.T) {
	l, err := NewLayout(Plain("I\nI am a much longer line\n\nI"), Font{"sans", 14}, 1, AlignCenter)
	if err != nil {
		t.Fatal(err)
	}
	size := l.Size()
	if len(l.lines) != 4 {
		t.Fatalf("%d lines", len(l.lines))
	}
	lineH := size.Y / 4
	img := image.NewRGBA(image.Rectangle{Max: size})
	l.Draw(img, image.Point{}, color.White)
	if _, n := ink(img, image.Rect(0, 2*lineH+2, size.X, 3*lineH-2)); n != 0 {
		t.Fatalf("the empty line drew %d pixels", n)
	}
	first, _ := ink(img, image.Rect(0, 0, size.X, lineH))
	if mid := (first.Min.X + first.Max.X) / 2; mid < size.X/2-3 || mid > size.X/2+3 {
		t.Fatalf("first line ink %v is not centred in width %d", first, size.X)
	}
}

func TestSpanBackgroundAndColourAreDrawn(t *testing.T) {
	spans, err := ParseMarkup("x<span bgcolor='#2C2C2C' fgcolor='#ff0000'> Enter </span>y")
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewLayout(spans, Font{"sans", 14}, 1, AlignLeft)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: l.Size()})
	l.Draw(img, image.Point{}, color.White)
	var bg, red int
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			switch c := img.RGBAAt(x, y); {
			case c == color.RGBA{0x2c, 0x2c, 0x2c, 0xff}:
				bg++
			case c.R > 0xc0 && c.G < 0x40 && c.A == 0xff:
				red++
			}
		}
	}
	if bg < 50 || red < 10 {
		t.Fatalf("background pixels %d, red text pixels %d", bg, red)
	}
}

func TestFallbackDrawsARuneTheFamilyLacks(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatal(err)
	}
	fontsMu.Lock()
	fonts.SetQuery(fontscan.Query{Families: []string{"monospace"}})
	face := fonts.ResolveFace('漢')
	fontsMu.Unlock()
	if face == nil {
		t.Skip("no installed font covers U+6F22")
	}
	l, err := NewLayout(Plain("漢"), Font{"mono", 14}, 1, AlignLeft)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: l.Size()})
	l.Draw(img, image.Point{}, color.White)
	if _, n := ink(img, img.Bounds()); n < 20 {
		t.Fatalf("fallback glyph drew %d pixels", n)
	}
}
