package wlr

import (
	"image"
	"slices"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// TestGuessOutputScaleMatchesNiri: Niri's own cases for guess_monitor_scale
// (src/utils/scale.rs). Pure Go, no compositor.
func TestGuessOutputScaleMatchesNiri(t *testing.T) {
	for _, tc := range []struct {
		name               string
		mmW, mmH, pxW, pxH int
		want               float64
	}{
		{"Librem 5", 65, 129, 720, 1440, 1.5},
		{"OnePlus 6", 68, 144, 1080, 2280, 2.5},
		{"Pixel 6a", 64, 142, 1080, 2400, 2.5},
		{"13in MacBook Retina", 286, 179, 2560, 1600, 1.75},
		{"Surface Laptop Studio", 303, 202, 2400, 1600, 1.5},
		{"Dell XPS 9320", 290, 180, 3840, 2400, 2.5},
		{"X1 Yoga Gen 6", 300, 190, 3840, 2400, 2.5},
		{"23in 1080p", 509, 286, 1920, 1080, 1},
		{"23in 4K", 509, 286, 3840, 2160, 1.75},
		{"27in 4K", 598, 336, 3840, 2160, 1.5},
		{"32in 4K", 708, 398, 3840, 2160, 1.25},
		{"25in 4K", 554, 312, 3840, 2160, 1.5},
		{"23.5in 4K", 522, 294, 3840, 2160, 1.75},
		{"Legion 7 16in", 340, 210, 2560, 1600, 1.5},
		{"Acer 31.5in", 700, 390, 2560, 1440, 1},
		{"Surface Pro 6", 260, 170, 2736, 1824, 2},
		{"unknown size", 0, 0, 1920, 1080, 1},
	} {
		if got := guessOutputScale(tc.mmW, tc.mmH, tc.pxW, tc.pxH); got != tc.want {
			t.Errorf("%s: guess = %v, want %v", tc.name, got, tc.want)
		}
	}
	// Niri's closest_representable_scale cases, through a configured scale.
	for in, want := range map[float64]float64{1.3: 1.3, 1.31: 157.0 / 120, 1.33: 160.0 / 120, 1.35: 1.35, 0: 0.1, 12: 10} {
		if got := outputScale(&in, 0, 0, 1920, 1080); got != want {
			t.Errorf("outputScale(%v) = %v, want %v", in, got, want)
		}
	}
}

// TestPickOutputModeMatchesNiri: Niri's pick_mode. An exact refresh in mHz,
// else the fastest mode of the size; a miss falls back to the fastest
// preferred mode, then the first.
func TestPickOutputModeMatchesNiri(t *testing.T) {
	modes := []outputModeChoice{
		{1920, 1080, 60000, true},
		{1920, 1080, 144000, false},
		{1920, 1080, 120000, false},
		{2560, 1440, 59951, false},
		{1280, 720, 60000, true},
	}
	for _, tc := range []struct {
		name     string
		target   *config.OutputMode
		index    int
		fallback bool
	}{
		{"no target, fastest preferred", nil, 0, false},
		{"exact refresh", &config.OutputMode{Width: 1920, Height: 1080, Refresh: 120}, 2, false},
		{"refresh to the mHz", &config.OutputMode{Width: 2560, Height: 1440, Refresh: 59.951}, 3, false},
		{"no refresh, fastest of the size", &config.OutputMode{Width: 1920, Height: 1080}, 1, false},
		{"refresh missing", &config.OutputMode{Width: 1920, Height: 1080, Refresh: 75}, 0, true},
		{"size missing", &config.OutputMode{Width: 800, Height: 600}, 0, true},
	} {
		index, fallback := pickOutputMode(modes, tc.target)
		if index != tc.index || fallback != tc.fallback {
			t.Errorf("%s: pick = %d (fallback %t), want %d (fallback %t)", tc.name, index, fallback, tc.index, tc.fallback)
		}
	}
	if index, _ := pickOutputMode([]outputModeChoice{{800, 600, 60000, false}}, nil); index != 0 {
		t.Errorf("no preferred mode: pick = %d, want the first", index)
	}
	if index, _ := pickOutputMode(nil, nil); index != -1 {
		t.Errorf("no modes: pick = %d, want -1", index)
	}
}

// TestPlaceOutputsMatchesNiri: Niri's reposition_outputs. Configured
// positions first unless they overlap, then the rest left to right in name
// order, whatever order they connected in.
func TestPlaceOutputsMatchesNiri(t *testing.T) {
	at := func(x, y int) *config.OutputPosition { return &config.OutputPosition{X: x, Y: y} }
	size := image.Pt(1920, 1080)
	for _, tc := range []struct {
		name    string
		outputs []outputPlacement
		want    []image.Point
	}{
		{"the owner's desktop", []outputPlacement{
			{config.OutputName{Connector: "DP-1"}, image.Pt(3440, 1440), at(2560, 0)},
			{config.OutputName{Connector: "DP-3"}, image.Pt(2560, 1440), at(0, 0)},
		}, []image.Point{{2560, 0}, {0, 0}}},
		{"automatic, in connector order", []outputPlacement{
			{config.OutputName{Connector: "HDMI-A-1"}, size, nil},
			{config.OutputName{Connector: "DP-2"}, image.Pt(1280, 720), nil},
		}, []image.Point{{1280, 0}, {0, 0}}},
		{"make, model and serial before connector", []outputPlacement{
			{config.OutputName{Connector: "DP-1"}, size, nil},
			{config.OutputName{Connector: "DP-2", Make: "Dell"}, size, nil},
		}, []image.Point{{1920, 0}, {0, 0}}},
		{"an overlap falls back to automatic", []outputPlacement{
			{config.OutputName{Connector: "DP-1"}, size, at(0, 0)},
			{config.OutputName{Connector: "DP-2"}, size, at(100, 100)},
		}, []image.Point{{0, 0}, {1920, 0}}},
		{"configured first, then automatic right of them", []outputPlacement{
			{config.OutputName{Connector: "DP-1"}, size, nil},
			{config.OutputName{Connector: "DP-2"}, size, at(0, 2000)},
		}, []image.Point{{1920, 0}, {0, 2000}}},
	} {
		if got := placeOutputs(tc.outputs); !slices.Equal(got, tc.want) {
			t.Errorf("%s: positions = %v, want %v", tc.name, got, tc.want)
		}
	}
}
