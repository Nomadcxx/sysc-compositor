package wm

import "math"

// Niri's mru.rs layout constants, in logical pixels.
const (
	mruPreviewMinSize = 16.
	// MRUBorder is the selected preview's border width.
	MRUBorder = 2.
	// MRUTitleGap is the gap from a preview to its title.
	MRUTitleGap = 14.
	mruGap      = 16.
	mruStrut    = 192.
)

// MRUStrip lays the switcher's previews out in a row on one output, as
// Niri's Inner::thumbnails and compute_view_pos do. Lengths are logical and
// rounded to the output's physical pixels.
type MRUStrip struct {
	OutputWidth, OutputHeight float64
	Scale                     float64
	// Padding is the highlight's padding; MaxHeight and MaxScale bound the
	// previews.
	Padding, MaxHeight, MaxScale float64
}

// MRURect is a rectangle in an output's logical coordinates.
type MRURect struct{ X, Y, W, H float64 }

// Round is Niri's round_logical_in_physical.
func (s MRUStrip) Round(logical float64) float64 {
	return math.Round(logical*s.Scale) / s.Scale
}

// HighlightPadding is how far the highlight reaches past a preview.
func (s MRUStrip) HighlightPadding() float64 {
	return s.Round(s.Padding) + s.Round(MRUBorder)
}

func (s MRUStrip) gap() float64 {
	padding := s.HighlightPadding()
	return padding + s.Round(mruGap) + padding
}

// PreviewSize is Niri's Thumbnail::preview_size for a window of w×h.
func (s MRUStrip) PreviewSize(w, h float64) (float64, float64) {
	if w <= 0 || h <= 0 || s.OutputHeight <= 0 {
		return 0, 0
	}
	maxScale := math.Max(0.001, s.MaxScale)
	maxHeight := math.Min(math.Max(1, s.MaxHeight), s.OutputHeight*maxScale)
	maxWidth := maxHeight * s.OutputWidth / s.OutputHeight
	minScale := math.Min(1, mruPreviewMinSize/math.Max(w, h))
	scale := math.Min(maxWidth/w, maxHeight/h)
	scale = math.Max(minScale, math.Min(maxScale, scale))
	return s.Round(w * scale), s.Round(h * scale)
}

// Layout places previews of the given window sizes left to right from x 0,
// each centred vertically.
func (s MRUStrip) Layout(sizes [][2]float64) []MRURect {
	rects := make([]MRURect, len(sizes))
	x := 0.
	for i, size := range sizes {
		w, h := s.PreviewSize(size[0], size[1])
		rects[i] = MRURect{X: x, Y: s.Round((s.OutputHeight - h) / 2), W: w, H: h}
		x += w + s.gap()
	}
	return rects
}

// ViewPos is Niri's compute_view_pos: where the view starts in strip
// coordinates, from the previous one, so the selected preview stays in view
// with the least motion, or the whole strip centred when it fits.
func (s MRUStrip) ViewPos(previous float64, rects []MRURect, current int) float64 {
	if current < 0 || current >= len(rects) {
		return 0
	}
	workingX := mruStrut + mruGap
	workingWidth := math.Max(0, s.OutputWidth-2*workingX)
	last := rects[len(rects)-1]
	if stripWidth := last.X + last.W; stripWidth <= workingWidth {
		return -(s.OutputWidth - stripWidth) / 2
	}
	cur := rects[current]
	return viewOffset(previous+workingX, workingWidth, cur.X, cur.W) + cur.X - workingX
}

// viewOffset is Niri's mru compute_view_offset.
func viewOffset(curX, workingWidth, x, width float64) float64 {
	if curX <= x && x+width <= curX+workingWidth {
		return -(x - curX)
	}
	if math.Abs(curX-x) <= math.Abs(curX+workingWidth-(x+width)) {
		return 0
	}
	return -(workingWidth - width)
}

// Highlight is the box behind a preview at r whose title, if any, is
// titleHeight tall.
func (s MRUStrip) Highlight(r MRURect, titleHeight float64) MRURect {
	padding := s.HighlightPadding()
	box := MRURect{X: r.X - padding, Y: r.Y - padding, W: r.W + 2*padding, H: r.H + 2*padding}
	if titleHeight > 0 {
		// Niri takes half the padding back so it looks balanced.
		box.H += s.Round(MRUTitleGap) + titleHeight - s.Round(padding/2)
	}
	return box
}

func (r MRURect) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}
