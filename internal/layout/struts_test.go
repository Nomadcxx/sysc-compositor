package layout

import "testing"

// Struts shrink the layout area from each edge; a negative value opens it
// back up, and the area never shrinks past zero.
func TestStrutsInset(t *testing.T) {
	full := Bounds{X: 100, Y: 200, Width: 1000, Height: 800}
	got := Struts{Left: 10, Right: 20, Top: 30, Bottom: 40}.Inset(full)
	want := Bounds{X: 110, Y: 230, Width: 970, Height: 730}
	if got != want {
		t.Errorf("Inset() = %+v, want %+v", got, want)
	}
	// The zero value leaves the area alone.
	if got := (Struts{}).Inset(full); got != full {
		t.Errorf("the zero struts changed the area to %+v", got)
	}
	// Negative edges expand the area.
	got = Struts{Left: -50, Bottom: -25}.Inset(full)
	want = Bounds{X: 50, Y: 200, Width: 1050, Height: 825}
	if got != want {
		t.Errorf("negative Inset() = %+v, want %+v", got, want)
	}
	// Struts wider than the area clamp the size to zero, keeping the inset
	// edge position.
	got = Struts{Left: 900, Right: 900}.Inset(full)
	want = Bounds{X: 1000, Y: 200, Width: 0, Height: 800}
	if got != want {
		t.Errorf("overflowing Inset() = %+v, want %+v", got, want)
	}
}
