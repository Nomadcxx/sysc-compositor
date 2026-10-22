// Package text lays out and rasterizes the compositor's own UI text (the exit
// dialog, hotkey overlay, screenshot hint and recent-windows titles) in pure
// Go: go-text finds system fonts, falls back per rune and shapes with its
// HarfBuzz port, and x/image rasterizes the glyphs. It plays the part pango
// and cairo play in Niri. Nothing here touches wlroots; callers hand the
// finished image to the scene.
package text

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Font is a family and a size in logical pixels, as Niri's "sans 14px".
type Font struct {
	Family string
	Size   float64
}

// Align is a layout's horizontal alignment of its lines.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
)

// The font map is not safe for concurrent use, and neither is shaping with
// it, so one lock covers both. Loading scans the system font directories
// once; go-text keeps an index under the user cache directory after that.
var (
	fontsMu  sync.Mutex
	fontsErr error
	fonts    *fontscan.FontMap
	shaper   shaping.HarfbuzzShaper
	loadOnce sync.Once
)

func loadFonts() error {
	loadOnce.Do(func() {
		fonts = fontscan.NewFontMap(log.New(log.Writer(), "sysc-compositor: fonts: ", 0))
		fontsErr = fonts.UseSystemFonts("")
	})
	return fontsErr
}

// Preload scans the system fonts now, so the first UI that draws text does
// not wait for it. It is safe from any goroutine.
func Preload() error {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	return loadFonts()
}

// genericFamily maps Pango's and fontconfig's short generic names to the CSS
// generic names go-text's substitution table knows.
func genericFamily(family string) string {
	switch strings.ToLower(family) {
	case "sans", "sans-serif", "":
		return "sans-serif"
	case "mono", "monospace":
		return "monospace"
	case "serif":
		return "serif"
	}
	return family
}

// run is one shaped piece of a line, all in one face and style.
type run struct {
	out   shaping.Output
	style Style
}

type line struct {
	runs            []run
	width           fixed.Int26_6
	ascent, descent fixed.Int26_6 // descent is positive, down
}

// Layout is text shaped at one scale, ready to measure and draw.
type Layout struct {
	lines []line
	align Align
	size  image.Point
}

// NewLayout shapes spans in font at scale (physical pixels per logical
// pixel), with '\n' breaking lines. The font size is rounded to whole
// physical pixels, as Niri's to_physical_precise_round does.
func NewLayout(spans []Span, f Font, scale float64, align Align) (*Layout, error) {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	if err := loadFonts(); err != nil {
		return nil, err
	}
	px := math.Round(f.Size * scale)
	size := fixed.Int26_6(px * 64)
	// pangocairo's resolution is 96 dpi, so a point is 4/3 pixels.
	pointPx := 96.0 / 72.0 * scale

	l := &Layout{align: align}
	cur := line{}
	finish := func() {
		if cur.ascent == 0 && cur.descent == 0 {
			cur.ascent, cur.descent = emptyLineBounds(f.Family, size)
		}
		l.lines = append(l.lines, cur)
		cur = line{}
	}
	var seg shaping.Segmenter
	for _, span := range spans {
		parts := strings.Split(span.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				finish()
			}
			if part == "" {
				continue
			}
			family := f.Family
			if span.Style.Family != "" {
				family = span.Style.Family
			}
			aspect := font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal, Stretch: font.StretchNormal}
			if span.Style.Bold {
				aspect.Weight = font.WeightBold
			}
			if span.Style.Italic {
				aspect.Style = font.StyleItalic
			}
			fonts.SetQuery(fontscan.Query{Families: []string{genericFamily(family)}, Aspect: aspect})
			runes := []rune(part)
			input := shaping.Input{
				Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: size,
				Language: language.DefaultLanguage(),
			}
			inputs := seg.Split(input, fonts)
			outs := make([]shaping.Output, 0, len(inputs))
			for _, in := range inputs {
				outs = append(outs, shaper.Shape(in))
			}
			if span.Style.LetterSpacing != 0 {
				spacing := fixed.Int26_6(float64(span.Style.LetterSpacing) / 1024 * pointPx * 64)
				shaping.AddSpacing(outs, runes, 0, spacing)
			}
			for _, out := range outs {
				cur.runs = append(cur.runs, run{out: out, style: span.Style})
				cur.width += out.Advance
				cur.ascent = max(cur.ascent, out.LineBounds.Ascent)
				cur.descent = max(cur.descent, -out.LineBounds.Descent)
			}
		}
	}
	finish()

	for _, ln := range l.lines {
		l.size.X = max(l.size.X, ln.width.Ceil())
		l.size.Y += (ln.ascent + ln.descent).Ceil()
	}
	return l, nil
}

// emptyLineBounds is the height an empty line takes in family.
func emptyLineBounds(family string, size fixed.Int26_6) (fixed.Int26_6, fixed.Int26_6) {
	fonts.SetQuery(fontscan.Query{Families: []string{genericFamily(family)}})
	face := fonts.ResolveFace(' ')
	if face == nil {
		return size, 0
	}
	ext, ok := face.FontHExtents()
	if !ok {
		return size, 0
	}
	scale := float64(size) / float64(face.Upem())
	return fixed.Int26_6(float64(ext.Ascender) * scale), fixed.Int26_6(-float64(ext.Descender) * scale)
}

// Size is the layout's extent in physical pixels.
func (l *Layout) Size() image.Point { return l.size }

// Draw paints the layout into dst with its top-left corner at at, in fg
// unless a span sets its own colour. Span backgrounds are drawn first.
func (l *Layout) Draw(dst *image.RGBA, at image.Point, fg color.Color) {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	y := fixed.I(at.Y)
	for _, ln := range l.lines {
		x := fixed.I(at.X)
		if l.align == AlignCenter {
			x += (fixed.I(l.size.X) - ln.width) / 2
		}
		baseline := y + ln.ascent
		pen := x
		for _, r := range ln.runs {
			if r.style.BG != nil {
				box := image.Rect(pen.Floor(), y.Floor(), (pen + r.out.Advance).Ceil(), (baseline + ln.descent).Ceil())
				xdraw.Draw(dst, box, image.NewUniform(*r.style.BG), image.Point{}, xdraw.Over)
			}
			pen += r.out.Advance
		}
		pen = x
		for _, r := range ln.runs {
			src := image.NewUniform(fg)
			if r.style.FG != nil {
				src = image.NewUniform(*r.style.FG)
			}
			for _, g := range r.out.Glyphs {
				drawGlyph(dst, r.out.Face, r.out.Size, g, pen+g.XOffset, baseline-g.YOffset, src)
				pen += g.Advance
			}
		}
		y = baseline + ln.descent
	}
}

// drawGlyph rasterizes one glyph with its origin at (x, baseline).
func drawGlyph(dst *image.RGBA, face *font.Face, size fixed.Int26_6, g shaping.Glyph, x, baseline fixed.Int26_6, src image.Image) {
	if bitmap, ok := face.GlyphDataBitmap(g.GlyphID); ok && bitmap.Format == font.PNG {
		img, err := png.Decode(bytes.NewReader(bitmap.Data))
		if err == nil {
			box := image.Rect((x + g.XBearing).Floor(), (baseline - g.YBearing).Floor(),
				(x + g.XBearing + g.Width).Ceil(), (baseline - g.YBearing - g.Height).Ceil())
			xdraw.ApproxBiLinear.Scale(dst, box, img, img.Bounds(), xdraw.Over, nil)
			return
		}
	}
	outline, ok := face.GlyphDataOutline(g.GlyphID)
	if !ok || len(outline.Segments) == 0 {
		return
	}
	scale := float32(size) / 64 / float32(face.Upem())
	ox, oy := float32(x)/64, float32(baseline)/64
	pt := func(p ot.SegmentPoint) (float32, float32) { return ox + p.X*scale, oy - p.Y*scale }

	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, s := range outline.Segments {
		for _, p := range s.ArgsSlice() {
			px, py := pt(p)
			minX, minY, maxX, maxY = min(minX, px), min(minY, py), max(maxX, px), max(maxY, py)
		}
	}
	box := image.Rect(int(math.Floor(float64(minX))), int(math.Floor(float64(minY))),
		int(math.Ceil(float64(maxX))), int(math.Ceil(float64(maxY)))).Intersect(dst.Bounds())
	if box.Empty() {
		return
	}
	var r vector.Rasterizer
	r.Reset(box.Dx(), box.Dy())
	bx, by := float32(box.Min.X), float32(box.Min.Y)
	for i, s := range outline.Segments {
		a := s.Args
		x0, y0 := pt(a[0])
		switch s.Op {
		case ot.SegmentOpMoveTo:
			// The rasterizer's MoveTo leaves the previous contour open.
			if i > 0 {
				r.ClosePath()
			}
			r.MoveTo(x0-bx, y0-by)
		case ot.SegmentOpLineTo:
			r.LineTo(x0-bx, y0-by)
		case ot.SegmentOpQuadTo:
			x1, y1 := pt(a[1])
			r.QuadTo(x0-bx, y0-by, x1-bx, y1-by)
		case ot.SegmentOpCubeTo:
			x1, y1 := pt(a[1])
			x2, y2 := pt(a[2])
			r.CubeTo(x0-bx, y0-by, x1-bx, y1-by, x2-bx, y2-by)
		}
	}
	r.ClosePath()
	r.DrawOp = xdraw.Over
	r.Draw(dst, box, src, image.Point{})
}
