package wlr

import (
	"image/color"
	"regexp"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// TestBackgroundColorFillsTheOutputBelowTheBackgroundLayer: background-color
// fills the output under its background layer surfaces, in the active
// workspace's colour over the layout's, and follows workspace switches and
// reloads, as Niri's workspace background does. A half-opaque swaybg shows
// the fill through it.
func TestBackgroundColorFillsTheOutputBelowTheBackgroundLayer(t *testing.T) {
	red, green := config.Color{R: 1, A: 1}, config.Color{G: 1, A: 1}
	half := 0.5
	options := func(background config.Color) Options {
		defaults := config.Default().Layout
		return Options{
			Animations: config.Animations{Off: true},
			Layout: &LayoutOptions{Gaps: defaults.Gaps, ColumnWidthProportion: 0.5,
				FocusRing: defaults.FocusRing, Border: defaults.Border, Shadow: defaults.Shadow,
				TabIndicator: defaults.TabIndicator, BackgroundColor: background,
				WorkspaceBackgroundColors: map[string]config.Color{config.WorkspaceKey("lit"): green}},
			Workspaces: []wm.WorkspaceConfig{{Name: "plain"}, {Name: "lit"}},
			LayerRules: []config.LayerRule{{
				Matches: []config.LayerMatch{{Namespace: regexp.MustCompile(`^wallpaper$`)}},
				Opacity: &half,
			}},
		}
	}
	c := newNestedWith(t, options(red))
	var checked bool
	pollThenStop(c, func() bool {
		if placed := mappedOn(c.layerPlacements(), layerBackground); placed == nil {
			return false
		}
		checked = true
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		pixel := func() color.NRGBA {
			frame, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return color.NRGBA{}
			}
			center := frame.Bounds().Min.Add(frame.Bounds().Max).Div(2)
			return frame.NRGBAAt(center.X, center.Y)
		}
		near := func(got, want color.NRGBA) bool {
			d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
			return d(got.R, want.R) <= 3 && d(got.G, want.G) <= 3 && d(got.B, want.B) <= 3
		}
		// #336699 at half opacity over the fill.
		if got, want := pixel(), (color.NRGBA{R: 153, G: 51, B: 77, A: 255}); !near(got, want) {
			t.Errorf("pixel over the layout's red = %v, want %v", got, want)
		}
		if !c.FocusWorkspaceName("lit") {
			t.Error("could not focus the workspace that sets its own background-color")
		} else if got, want := pixel(), (color.NRGBA{R: 26, G: 179, B: 77, A: 255}); !near(got, want) {
			t.Errorf("pixel over the workspace's green = %v, want %v", got, want)
		}
		c.FocusWorkspaceName("plain")
		if err := c.Reconfigure(options(config.Color{})); err != nil {
			t.Error(err)
		} else if got, want := pixel(), (color.NRGBA{R: 26, G: 51, B: 77, A: 255}); !near(got, want) {
			t.Errorf("pixel after a reload to transparent = %v, want %v over the black clear colour", got, want)
		}
		return true
	})
	runClient(t, c, []string{"swaybg", "-c", "#336699"}, nil, nil)
	if !checked {
		t.Fatal("swaybg never mapped a background layer surface")
	}
}
