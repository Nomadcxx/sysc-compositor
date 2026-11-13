package ui

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"image"
	"testing"
)

func TestGradientCoordinatesAndPremultipliedInterpolation(t *testing.T) {
	g := config.Gradient{From: config.Color{R: 1, A: 1}, To: config.Color{B: 1, A: 1}, Angle: 90}
	full := image.Rect(0, 0, 100, 100)
	horizontal := GradientImage(full, full, g)
	if a, b := horizontal.RGBAAt(0, 50), horizontal.RGBAAt(99, 50); a.R < 250 || a.B > 3 || b.B < 250 || b.R > 3 {
		t.Fatalf("horizontal endpoints: %v %v", a, b)
	}
	g.Angle = 180
	vertical := GradientImage(full, full, g)
	if a, b := vertical.RGBAAt(50, 0), vertical.RGBAAt(50, 99); a.R < 250 || b.B < 250 {
		t.Fatalf("vertical endpoints: %v %v", a, b)
	}
	g.Angle = 90
	sub := GradientImage(image.Rect(50, 0, 100, 100), full, g)
	if got := sub.RGBAAt(0, 50); got != horizontal.RGBAAt(50, 50) {
		t.Errorf("workspace-relative sample: %v", got)
	}
	g.From.A = 0
	got := gradientMixer(g)(.5)
	if got.R != 0 || got.B != 128 || got.A != 128 {
		t.Errorf("transparent red must not bleed into blue: %v", got)
	}
	g.From.A = 1
	g.Interpolation = "srgb-linear"
	got = gradientMixer(g)(.5)
	if got.R < 185 || got.R > 187 || got.B != got.R {
		t.Errorf("linear-light midpoint: %v", got)
	}
	for _, space := range []string{"srgb", "srgb-linear", "oklab", "oklch shorter hue", "oklch longer hue", "oklch increasing hue", "oklch decreasing hue"} {
		g.Interpolation = space
		a, b := gradientMixer(g)(0), gradientMixer(g)(1)
		if a.R < 254 || a.G > 1 || a.B > 1 || b.R > 1 || b.G > 1 || b.B < 254 {
			t.Errorf("%s changed endpoints: %v %v", space, a, b)
		}
	}
	g.Interpolation = "oklch shorter hue"
	shorter := gradientMixer(g)(.5)
	g.Interpolation = "oklch longer hue"
	longer := gradientMixer(g)(.5)
	if shorter.G >= longer.G || shorter.B <= longer.B {
		t.Errorf("hue routes did not follow opposite arcs: shorter=%v longer=%v", shorter, longer)
	}
}
