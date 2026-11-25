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
// reloads, as Niri's workspace background does. A half-opaque wallpaper
// shows the fill through it.
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
		wallpaper := false
		for _, layer := range c.layerOrder {
			if width, _, mapped := layer.mappedSize(); layer.namespace() == "wallpaper" && mapped && width > 0 {
				wallpaper = true
			}
		}
		if !wallpaper {
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
	// An image, not swaybg -c: opacity does not reach the colour of the
	// single-pixel buffer that draws (see TestLayerRuleAppearanceAndBlockOut).
	runClient(t, c, []string{"swaybg", "-m", "fill", "-i", solidPNG(t, color.NRGBA{R: 0x33, G: 0x66, B: 0x99, A: 255})}, nil, nil)
	if !checked {
		t.Fatal("swaybg never mapped a background layer surface")
	}
}

// TestPlaceWithinBackdropDrawsUnderTheWorkspaceColour: a place-within-backdrop
// wallpaper draws under the workspace background colour, as in Niri's
// render_inner, so an opaque colour hides it outside the overview and a
// transparent one shows it. It takes no pointer input. Without the rule it
// draws over the colour again.
func TestPlaceWithinBackdropDrawsUnderTheWorkspaceColour(t *testing.T) {
	red := config.Color{R: 1, A: 1}
	options := func(background config.Color, backdrop bool) Options {
		defaults := config.Default().Layout
		return Options{
			Animations: config.Animations{Off: true},
			Layout: &LayoutOptions{Gaps: defaults.Gaps, ColumnWidthProportion: 0.5,
				FocusRing: defaults.FocusRing, Border: defaults.Border, Shadow: defaults.Shadow,
				TabIndicator: defaults.TabIndicator, BackgroundColor: background},
			LayerRules: []config.LayerRule{{
				Matches:             []config.LayerMatch{{Namespace: regexp.MustCompile(`^wallpaper$`)}},
				PlaceWithinBackdrop: &backdrop,
			}},
		}
	}
	c := newNestedWith(t, options(red, true))
	var checked bool
	pollThenStop(c, func() bool {
		var wallpaper *layerSurface
		for _, layer := range c.layerOrder {
			if width, _, mapped := layer.mappedSize(); layer.namespace() == "wallpaper" && mapped && width > 0 {
				wallpaper = layer
			}
		}
		if wallpaper == nil {
			return false
		}
		checked = true
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		box := c.outputLayoutBox(output)
		x, y := float64(box.Min.X+box.Dx()/2), float64(box.Min.Y+box.Dy()/2)
		pixel := func() color.NRGBA {
			frame, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return color.NRGBA{}
			}
			center := frame.Bounds().Min.Add(frame.Bounds().Max).Div(2)
			got := frame.NRGBAAt(center.X, center.Y)
			got.A = 255
			return got
		}
		blue, opaqueRed := color.NRGBA{B: 255, A: 255}, color.NRGBA{R: 255, A: 255}
		if !wallpaper.withinBackdrop() || wallpaper.tree.node.parent != c.backdropLayerTree {
			t.Error("the place-within-backdrop wallpaper is not in the backdrop tree")
		}
		if got := pixel(); got != opaqueRed {
			t.Errorf("pixel over an opaque background-color = %v, want its red over the backdrop", got)
		}
		if surface, _, _ := c.surfaceAt(x, y); surface != nil {
			t.Error("the backdrop wallpaper took pointer input")
		}
		if err := c.Reconfigure(options(config.Color{}, true)); err != nil {
			t.Error(err)
		} else if got := pixel(); got != blue {
			t.Errorf("pixel over a transparent background-color = %v, want the wallpaper's blue", got)
		}
		if err := c.Reconfigure(options(red, false)); err != nil {
			t.Error(err)
		} else {
			if got := pixel(); got != blue {
				t.Errorf("pixel without the rule = %v, want the wallpaper over the colour", got)
			}
			if wallpaper.tree.node.parent != c.layerTrees[layerBackground] {
				t.Error("without the rule the wallpaper did not return to the background layer")
			}
		}
		return true
	})
	runClient(t, c, []string{"swaybg", "-m", "fill", "-i", solidPNG(t, color.NRGBA{B: 255, A: 255})}, nil, nil)
	if !checked {
		t.Fatal("swaybg never mapped a background layer surface")
	}
}
