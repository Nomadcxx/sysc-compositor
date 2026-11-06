package wlr

import (
	"image"
	"image/color"
	"os/exec"
	"regexp"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestBorderDrawsAroundTheWindowAndShrinksIt: Niri's border is drawn around
// every window in its urgent, active or inactive colour, a window rule's one
// colour keeps the layout's width, the window is configured two border widths
// smaller than its tile, and fullscreen and removal take the border away.
func TestBorderDrawsAroundTheWindowAndShrinksIt(t *testing.T) {
	defaults := config.Default().Layout
	red, green, blue, yellow := config.Color{R: 1, A: 1}, config.Color{G: 1, A: 1},
		config.Color{B: 1, A: 1}, config.Color{R: 1, G: 1, A: 1}
	layoutOptions := func(border config.FocusRing, shadow config.Shadow) *LayoutOptions {
		ring := defaults.FocusRing
		ring.Off = true
		return &LayoutOptions{Gaps: defaults.Gaps, ColumnWidthProportion: 0.5,
			InsertHint: defaults.InsertHint, FocusRing: ring, Border: border, Shadow: shadow}
	}
	border := config.FocusRing{Width: 8, ActiveColor: red, InactiveColor: green, UrgentColor: blue}
	shadow := defaults.Shadow
	shadow.On = true
	targetRule := func(withBackground *bool) []config.WindowRule {
		return []config.WindowRule{{
			Matches:                  []config.RuleMatch{{AppID: regexp.MustCompile(`^border-target$`)}},
			Decorations:              config.Decorations{Border: config.BorderRule{InactiveColor: &yellow}},
			DrawBorderWithBackground: withBackground,
		}}
	}
	animations := config.Animations{Off: true}
	c := newNestedWith(t, Options{Animations: animations, Layout: layoutOptions(border, shadow),
		WindowRules: targetRule(nil)})
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var output unsafe.Pointer
		var outputBox image.Rectangle
		var backgroundNode unsafe.Pointer
		onCompositor(t, c, func() {
			for output = range c.sceneOutputs {
				break
			}
			if output == nil {
				t.Error("no output for the border test")
				return
			}
			outputBox = c.outputLayoutBox(output)
			backgroundNode = sceneFXCreateRect(unsafe.Pointer(c.layerTrees[layerBackground]),
				outputBox.Dx(), outputBox.Dy(), [4]float32{0.2, 0.4, 0.6, 1})
			if backgroundNode == nil {
				t.Error("create background for the border test")
				return
			}
			sceneFXNodeSetPosition(backgroundNode, outputBox.Min.X, outputBox.Min.Y)
		})
		if output == nil || backgroundNode == nil {
			return
		}
		defer onCompositor(t, c, func() { sceneFXNodeDestroy(backgroundNode) })

		var clients []*testclient.ToplevelClient
		defer func() {
			for _, client := range clients {
				client.Close()
			}
		}()
		for _, appID := range []string{"border-target", "border-focus"} {
			client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				t.Error(err)
				return
			}
			clients = append(clients, client)
			if err := client.SetIdentity(appID, appID); err != nil {
				t.Error(err)
				return
			}
			if err := client.Paint(0xff3498db, 200, 150); err != nil {
				t.Error(err)
				return
			}
		}

		var view *toplevelView
		onCompositor(t, c, func() {
			for _, candidate := range c.views {
				if cString(candidate.toplevel.app_id) == "border-target" {
					view = candidate
				}
			}
		})
		if view == nil {
			t.Error("the target window did not map")
			return
		}

		// pixelLeftOf reads the output pixel dx to the left of the window's
		// left edge, halfway down.
		pixelLeftOf := func(dx int) color.NRGBA {
			var pixel color.NRGBA
			onCompositor(t, c, func() {
				frame, err := c.captureOutput(output)
				if err != nil {
					t.Error(err)
					return
				}
				x, y := view.treePosition()
				lx, ly := x-dx, y+view.geometry().h/2
				px := (lx - outputBox.Min.X) * frame.Bounds().Dx() / outputBox.Dx()
				py := (ly - outputBox.Min.Y) * frame.Bounds().Dy() / outputBox.Dy()
				if !image.Pt(px, py).In(frame.Bounds()) {
					t.Errorf("pixel (%d,%d) outside the rendered output %v", px, py, frame.Bounds())
					return
				}
				pixel = frame.NRGBAAt(px, py)
			})
			return pixel
		}
		background := color.NRGBA{R: 51, G: 102, B: 153, A: 255}
		nrgba := func(c config.Color) color.NRGBA {
			return color.NRGBA{R: uint8(c.R * 255), G: uint8(c.G * 255), B: uint8(c.B * 255), A: 255}
		}
		assertBorder := func(stage string, want color.NRGBA) {
			t.Helper()
			if got := pixelLeftOf(4); got != want {
				t.Errorf("%s: pixel inside the border = %v, want %v", stage, got, want)
			}
			if got := pixelLeftOf(12); got != background {
				t.Errorf("%s: pixel beyond the 8 px border = %v, want the background %v", stage, got, background)
			}
		}
		// configured compares the size sent to the client with the tile less
		// the border on each side.
		configured := func(stage string, border int) {
			t.Helper()
			onCompositor(t, c, func() {
				for _, window := range c.model.Snapshot().Windows {
					if window.ID != view.id {
						continue
					}
					gotW, gotH := int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height)
					wantW, wantH := window.Tile.Width-2*border, window.Tile.Height-2*border
					if gotW != wantW || gotH != wantH {
						t.Errorf("%s: configured %dx%d, want the %dx%d tile less %d px a side (%dx%d)",
							stage, gotW, gotH, window.Tile.Width, window.Tile.Height, border, wantW, wantH)
					}
				}
			})
		}

		assertBorder("inactive window with the rule's colour", nrgba(yellow))
		configured("inactive window", 8)
		onCompositor(t, c, func() {
			if !view.decorations.withBackground || view.borderNode == nil || sceneFXRectClipWidth(view.borderNode) != 0 {
				t.Error("a client-side decorated window's border does not fill its background")
			}
			if view.shadowNode == nil || sceneFXShadowClipWidth(view.shadowNode) != view.geometry().w+16 {
				t.Error("the shadow is not cut out from under the tile")
			}
		})

		onCompositor(t, c, func() {
			if !c.SetWindowUrgent(view.id, true) {
				t.Error("SetWindowUrgent(true) failed")
			}
		})
		assertBorder("urgent window", nrgba(blue))
		onCompositor(t, c, func() {
			if !c.SetWindowUrgent(view.id, false) {
				t.Error("SetWindowUrgent(false) failed")
			}
			if !c.FocusWindow(view.id) {
				t.Error("FocusWindow failed")
			}
		})
		assertBorder("active window", nrgba(red))

		onCompositor(t, c, func() {
			if !c.SetFullscreen(view.id, true) {
				t.Error("SetFullscreen(true) failed")
			}
			if sceneFXNodeEnabled(view.borderNode) {
				t.Error("a fullscreen window kept its border")
			}
			bounds, ok := c.outputLayoutBounds(view.outputName)
			if gotW, gotH := int(view.toplevel.scheduled.width), int(view.toplevel.scheduled.height); !ok ||
				gotW != bounds.Width || gotH != bounds.Height {
				t.Errorf("fullscreen window configured %dx%d, want the whole output %dx%d", gotW, gotH, bounds.Width, bounds.Height)
			}
		})
		onCompositor(t, c, func() {
			if !c.SetFullscreen(view.id, false) {
				t.Error("SetFullscreen(false) failed")
			}
		})
		assertBorder("fullscreen exit", nrgba(red))
		configured("fullscreen exit", 8)

		withoutBackground := false
		shadow.DrawBehindWindow = true
		onCompositor(t, c, func() {
			if err := c.Reconfigure(Options{Animations: animations, Layout: layoutOptions(border, shadow),
				WindowRules: targetRule(&withoutBackground)}); err != nil {
				t.Errorf("draw-border-with-background false: %v", err)
			}
			if view.decorations.withBackground || sceneFXRectClipWidth(view.borderNode) != view.geometry().w {
				t.Error("draw-border-with-background false did not cut the window out of the border")
			}
			if sceneFXShadowClipWidth(view.shadowNode) != 0 {
				t.Error("draw-behind-window did not remove the shadow's cut-out")
			}
		})
		assertBorder("border without background", nrgba(red))

		off := border
		off.Off = true
		onCompositor(t, c, func() {
			if err := c.Reconfigure(Options{Animations: animations, Layout: layoutOptions(off, defaults.Shadow)}); err != nil {
				t.Errorf("remove the border: %v", err)
			}
		})
		if got := pixelLeftOf(4); got != background {
			t.Errorf("border removal: pixel left of the window = %v, want the background %v", got, background)
		}
		configured("border removal", 0)
	})
}
