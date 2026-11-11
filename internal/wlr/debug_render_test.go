package wlr

import (
	"image"
	"image/color"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestDebugVisualisationsReachTheDisplay: Niri's ToggleDebugTint washes each
// frame green, DebugToggleOpaqueRegions draws red over buffers that are not
// opaque, and DebugToggleDamage highlights damage in red; each toggles back
// off. They are drawn on the output's own frame, which a capture does not
// render, so the test reads what the output committed.
func TestDebugVisualisationsReachTheDisplay(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 0, func([]*testclient.ToplevelClient, []*toplevelView) {
		frames := make(chan color.NRGBA, 256)
		var stop func()
		var wallpaper unsafe.Pointer
		onCompositor(t, c, func() {
			var output unsafe.Pointer
			for output = range c.sceneOutputs {
				break
			}
			box := c.outputLayoutBox(output)
			img := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
			for i := 0; i < len(img.Pix); i += 4 {
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 51, 102, 153, 255
			}
			// An image buffer is not opaque by format, and is never scanned out.
			node := sceneImage(c.layerTrees[layerBackground], img)
			if node == nil {
				t.Fatal("could not create the wallpaper")
			}
			sceneImagePlace(node, box, 1)
			wallpaper = unsafe.Pointer(node)
			stop = c.probeOutputFrames(output, func(pixel color.NRGBA) {
				select {
				case frames <- pixel:
				default:
				}
			})
			c.debugFullDamage()
		})
		defer onCompositor(t, c, func() {
			stop()
			sceneFXNodeDestroy(wallpaper)
		})
		near := func(r, g, b int) func(color.NRGBA) bool {
			return func(p color.NRGBA) bool {
				d := func(a uint8, b int) bool { return int(a)-b <= 3 && b-int(a) <= 3 }
				return d(p.R, r) && d(p.G, g) && d(p.B, b)
			}
		}
		// expect waits for a committed frame that matches.
		expect := func(stage string, want func(color.NRGBA) bool) {
			t.Helper()
			var last color.NRGBA
			deadline := time.After(3 * time.Second)
			for {
				select {
				case last = <-frames:
					if want(last) {
						return
					}
				case <-deadline:
					t.Errorf("%s: the display shows %v", stage, last)
					return
				}
			}
		}
		toggle := func(action func()) {
			onCompositor(t, c, func() {
				action()
			})
		}
		plain := near(51, 102, 153)
		expect("before any toggle", plain)
		// 20% green over (0.2, 0.4, 0.6): (0.16, 0.52, 0.48).
		toggle(c.ToggleDebugTint)
		expect("tint", near(41, 133, 122))
		toggle(c.ToggleDebugTint)
		expect("tint off", plain)
		// 25% red over a buffer that is not opaque: (0.4, 0.3, 0.45).
		toggle(c.DebugToggleOpaqueRegions)
		expect("opaque regions", near(102, 77, 115))
		toggle(c.DebugToggleOpaqueRegions)
		expect("opaque regions off", plain)
		toggle(c.DebugToggleDamage)
		expect("damage", func(p color.NRGBA) bool { return int(p.R) > 51+20 })
		toggle(c.DebugToggleDamage)
		expect("damage off", plain)
		if t.Failed() {
			t.Logf("frames left unread: %d", len(frames))
		}
	})
}
