package wlr

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// ghostConfig is archThink's configuration where it touches drawing
// (sysc-2294): a transparent background with no wallpaper, translucent
// windows (kitty's background_opacity 0.75, then 0.9 when inactive), rounded
// clipped windows, a frosted top bar and the owner's springs.
const ghostConfig = `
layout {
    gaps 5
    background-color "transparent"
    default-column-width { proportion 0.5; }
    border {
        off
    }
    focus-ring {
        width 2
        active-color "#ffffff"
        inactive-color "#505050"
    }
}
overview {
    workspace-shadow {
        off
    }
}
animations {
    workspace-switch { spring damping-ratio=0.80 stiffness=523 epsilon=0.0001; }
    window-open { duration-ms 150; curve "ease-out-expo"; }
    window-close { duration-ms 150; curve "ease-out-quad"; }
    horizontal-view-movement { spring damping-ratio=0.85 stiffness=423 epsilon=0.0001; }
    window-movement { spring damping-ratio=0.75 stiffness=323 epsilon=0.0001; }
    window-resize { spring damping-ratio=0.85 stiffness=423 epsilon=0.0001; }
    overview-open-close { spring damping-ratio=0.85 stiffness=800 epsilon=0.0001; }
}
window-rule {
    match is-active=true
    opacity 0.75
}
window-rule {
    match is-active=false
    opacity 0.9
}
window-rule {
    geometry-corner-radius 12
    clip-to-geometry true
}
layer-rule {
    match namespace="^sysc-test-bar$"
    background-effect {
        blur true
    }
}
`

func ghostOptions(t *testing.T) Options {
	t.Helper()
	cfg, err := config.Parse("ghost.kdl", []byte(ghostConfig))
	if err != nil {
		t.Fatal(err)
	}
	return Options{
		Layout: LayoutFromConfig(cfg), Animations: cfg.Animations, Overview: cfg.Overview,
		Blur: cfg.Blur, WindowRules: cfg.WindowRules, LayerRules: cfg.LayerRules, PreferNoCSD: true,
	}
}

// ghostRun drives steps on their own goroutine against a running compositor
// that keeps each output's committed buffer, and stops it afterwards.
func ghostRun(t *testing.T, c *Compositor, steps func()) {
	t.Helper()
	onReady := make(chan struct{})
	c.committedBuffers = map[unsafe.Pointer]unsafe.Pointer{}
	watchdog := time.AfterFunc(90*time.Second, c.Terminate)
	defer watchdog.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		close(onReady)
		steps()
	}()
	<-onReady
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}

// settle waits until no animation runs and every output has committed its
// damage, twice in a row a frame apart.
func settle(t *testing.T, c *Compositor, step string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	calm := 0
	for calm < 2 {
		if time.Now().After(deadline) {
			t.Errorf("%s: the outputs never settled", step)
			return false
		}
		time.Sleep(40 * time.Millisecond)
		settled := false
		onCompositor(t, c, func() { settled = c.outputsSettled() })
		if settled {
			calm++
		} else {
			calm = 0
		}
	}
	return true
}

// checkGhosts compares what each output shows, its last committed buffer,
// with a fresh render of its scene. A difference is a stale pixel the
// output's buffers kept: sysc-2294's ghost.
func checkGhosts(t *testing.T, c *Compositor, step string) {
	t.Helper()
	if !settle(t, c, step) {
		return
	}
	onCompositor(t, c, func() {
		for output := range c.sceneOutputs {
			shown, err := c.committedImage(output)
			if err != nil {
				t.Errorf("%s: %v", step, err)
				continue
			}
			fresh, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Errorf("%s: %v", step, err)
				continue
			}
			if count, box, first := imageDifference(shown, fresh, 3); count > 0 {
				t.Errorf("%s: output %s shows %d stale pixels in %v; first at %v shown %v, scene %v",
					step, outputName(output), count, box, first, shown.NRGBAAt(first.X, first.Y), fresh.NRGBAAt(first.X, first.Y))
				dumpGhost(t, step, shown, fresh)
			}
		}
	})
}

// imageDifference counts the pixels whose channels differ by more than
// tolerance, with their bounding box and the first of them.
func imageDifference(a, b *image.NRGBA, tolerance int) (int, image.Rectangle, image.Point) {
	count := 0
	var box image.Rectangle
	var first image.Point
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			for ch := 0; ch < 3; ch++ {
				d := int(a.Pix[i+ch]) - int(b.Pix[i+ch])
				if d > tolerance || d < -tolerance {
					if count == 0 {
						first, box = image.Pt(x, y), image.Rect(x, y, x+1, y+1)
					}
					box = box.Union(image.Rect(x, y, x+1, y+1))
					count++
					break
				}
			}
		}
	}
	return count, box, first
}

// dumpGhost writes both images when SYSC_GHOST_DUMP names a directory.
func dumpGhost(t *testing.T, step string, shown, fresh *image.NRGBA) {
	dir := os.Getenv("SYSC_GHOST_DUMP")
	if dir == "" {
		return
	}
	for name, img := range map[string]*image.NRGBA{"shown": shown, "scene": fresh} {
		file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s.png", step, name)))
		if err != nil {
			t.Log(err)
			return
		}
		_ = png.Encode(file, img)
		_ = file.Close()
	}
}

// TestCommittedFramesMatchTheScene replays the owner's ghosting report as a
// script on archThink's settings and checks, after each step settles, that
// the output shows what its scene draws.
func TestCommittedFramesMatchTheScene(t *testing.T) {
	c := newNestedWith(t, ghostOptions(t))
	ghostRun(t, c, func() {
		do := func(step string, action func()) {
			onCompositor(t, c, action)
			checkGhosts(t, c, step)
		}
		mapped := func(n int) bool {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				count := 0
				onCompositor(t, c, func() {
					for _, view := range c.views {
						if view != nil && view.mapped {
							count++
						}
					}
				})
				if count == n {
					return true
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Errorf("%d windows never mapped", n)
			return false
		}
		open := func(color uint32) *testclient.ToplevelClient {
			client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				t.Error(err)
				return nil
			}
			if err = client.Paint(color, 600, 400); err != nil {
				t.Error(err)
			}
			return client
		}
		var clients []*testclient.ToplevelClient
		defer func() {
			for _, client := range clients {
				if client != nil {
					client.Close()
				}
			}
		}()
		for i, color := range []uint32{0xffd04040, 0xff40d040, 0xff4040d0} {
			clients = append(clients, open(color))
			if !mapped(i + 1) {
				return
			}
		}
		checkGhosts(t, c, "open")
		do("focus-left", func() { c.FocusColumnLeft() })
		do("focus-left-again", func() { c.FocusColumnLeft() })
		do("move-right", func() { c.MoveColumnRight() })
		do("move-left", func() { c.MoveColumnLeft() })
		// Two windows close together, each with its own close fade.
		clients[1].Close()
		clients[2].Close()
		clients[1], clients[2] = nil, nil
		if !mapped(1) {
			return
		}
		checkGhosts(t, c, "close-two")
		clients = append(clients, open(0xff40d040))
		if !mapped(2) {
			return
		}
		checkGhosts(t, c, "reopen")
		do("window-to-workspace-down", func() { c.MoveWindowToWorkspaceDown(true) })
		do("window-to-workspace-up", func() { c.MoveWindowToWorkspaceUp(true) })
		do("workspace-down", func() {
			c.model.FocusWorkspaceDown()
			c.applyWorkspaceModel()
		})
		do("workspace-up", func() {
			c.model.FocusWorkspaceUp()
			c.applyWorkspaceModel()
		})
		do("overview-open", func() { c.OpenOverview() })
		do("overview-close", func() { c.CloseOverview() })
		var pointer *fakePointer
		onCompositor(t, c, func() { pointer = c.addFakePointer() })
		defer onCompositor(t, c, func() { pointer.remove() })
		bar, err := testclient.StartLayerPopupClient(c.SocketName())
		if err != nil {
			t.Error(err)
			return
		}
		checkGhosts(t, c, "bar-mapped")
		onCompositor(t, c, func() {
			blur := 0
			for _, effect := range c.backgroundEffects {
				blur += len(effect.blurNodes)
			}
			if blur == 0 {
				t.Error("the bar's layer rule gave it no blur")
			}
		})
		do("overview-open-with-bar", func() { c.OpenOverview() })
		do("overview-close-with-bar", func() { c.CloseOverview() })
		bar.Close()
		checkGhosts(t, c, "bar-gone")
		do("overview-open-after-bar", func() { c.OpenOverview() })
		do("overview-close-after-bar", func() { c.CloseOverview() })
	})
}
