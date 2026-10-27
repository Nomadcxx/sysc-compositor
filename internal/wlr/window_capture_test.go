package wlr

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestCapturePrivacyChangesBeforeNextFrame(t *testing.T) {
	floating := true
	blurConfig := config.Blur{Passes: 1, Offset: 2, Saturation: 1}
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, Blur: blurConfig, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	done := make(chan error, 1)
	guard := time.AfterFunc(20*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer c.Terminate()
		done <- func() error {
			secret, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer secret.Close()
			if err := secret.Paint(0xff3498db, 200, 150); err != nil {
				return err
			}
			if err := secret.SetIdentity("secret", "secret"); err != nil {
				return err
			}
			public, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer public.Close()
			if err := public.Paint(0xff3498db, 200, 150); err != nil {
				return err
			}
			if err := public.SetIdentity("public", "public"); err != nil {
				return err
			}
			var privateView, publicView *toplevelView
			var output unsafe.Pointer
			var privateAt, publicAt image.Point
			var before color.NRGBA
			var clone, blur unsafe.Pointer
			onCompositor(t, c, func() {
				for _, v := range c.views {
					if cString(v.toplevel.app_id) == "secret" {
						privateView = v
					} else {
						publicView = v
					}
				}
				for p := range c.sceneOutputs {
					output = p
					break
				}
				if privateView == nil || publicView == nil {
					t.Error("test windows missing")
					return
				}
				c.MoveFloatingWindow(&privateView.id, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40}, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40})
				c.MoveFloatingWindow(&publicView.id, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 280}, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40})
				x, y := privateView.treePosition()
				privateAt = image.Pt(x+20, y+20)
				x, y = publicView.treePosition()
				publicAt = image.Pt(x+20, y+20)
				img, e := c.captureOutput(output)
				if e != nil {
					err = e
					return
				}
				before = img.NRGBAAt(privateAt.X, privateAt.Y)
				if before != (color.NRGBA{0x34, 0x98, 0xdb, 255}) {
					t.Errorf("private test pixel is %v, want test color", before)
				}
				clone, _ = sceneFXCloneView(unsafe.Pointer(c.windowAnimationTree), unsafe.Pointer(privateView.tree))
				if clone == nil {
					t.Error("capture clone missing")
					return
				}
				c.rememberCaptureTree(clone, privateView.captureIdentity)
				sceneFXTreeSetPosition(clone, 520, 40)
				sceneFXSetBlurData(unsafe.Pointer(c.scene), 1, 2, 0, 1)
				blur = sceneFXCreateBlurNode(unsafe.Pointer(c.layerTrees[layerWindows]), 180, 130)
				sceneFXNodeSetPosition(blur, privateAt.X-10, privateAt.Y-10)
				// Warm the output's blur state with the still-visible private image.
				_, err = c.captureOutputForTarget(output, config.CaptureDisplay)
			})
			defer onCompositor(t, c, func() {
				if clone != nil {
					sceneFXNodeDestroy(clone)
				}
				if blur != nil {
					sceneFXNodeDestroy(blur)
				}
			})
			if err != nil {
				return err
			}
			if privateView == nil || publicView == nil {
				return fmt.Errorf("test windows missing")
			}
			windowSource, err := testclient.StartCapture(c.SocketName(), strconv.FormatUint(privateView.id, 10))
			if err != nil {
				return err
			}
			defer windowSource.Close()
			outputSource, err := testclient.StartCapture(c.SocketName(), "")
			if err != nil {
				return err
			}
			defer outputSource.Close()
			if err = windowSource.Arm(false); err != nil {
				return err
			}
			if err = waitCapture(windowSource, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
				return err
			}
			if err = windowSource.Arm(false); err != nil {
				return err
			}
			if err = outputSource.Arm(false); err != nil {
				return err
			}
			if err = waitCapture(outputSource, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
				return err
			}
			if err = outputSource.Arm(false); err != nil {
				return err
			}
			block := "screen-capture"
			onCompositor(t, c, func() {
				err = c.Reconfigure(Options{Animations: config.Animations{Off: true}, Blur: blurConfig, WindowRules: []config.WindowRule{{Matches: []config.RuleMatch{{AppID: regexp.MustCompile("^secret$")}}, BlockOutFrom: &block}}})
				if err != nil {
					return
				}
				img, e := c.captureOutput(output)
				if e != nil {
					err = e
					return
				}
				if got := img.NRGBAAt(privateAt.X, privateAt.Y); got != (color.NRGBA{A: 255}) {
					t.Errorf("protected output pixel=%v, want black", got)
				}
				if got := img.NRGBAAt(publicAt.X, publicAt.Y); got != before {
					t.Errorf("public output pixel=%v, want unchanged %v", got, before)
				}
				if got := img.NRGBAAt(540, 60); got != (color.NRGBA{A: 255}) {
					t.Errorf("protected scene clone pixel=%v, want black", got)
				}
				display, e := c.captureOutputForTarget(output, config.CaptureDisplay)
				if e != nil {
					err = e
					return
				}
				if got := display.NRGBAAt(privateAt.X, privateAt.Y); got != before {
					t.Errorf("capture left physical scene masked: %v, want %v", got, before)
				}
				window, e := c.renderWindowCapture(privateView, config.CaptureScreen)
				if e != nil {
					err = e
					return
				}
				if got := color.NRGBAModel.Convert(window.At(20, 20)).(color.NRGBA); got != (color.NRGBA{A: 255}) {
					t.Errorf("protected window screenshot pixel=%v, want black", got)
				}
				// Masks used for off-screen rendering must leave client buffers intact.
				g := privateView.geometry()
				live, e := c.renderSurfaceTree(privateView.toplevel.base.surface, g.x, g.y, 1, 200, 150)
				if e != nil {
					err = e
					return
				}
				if got := color.NRGBAModel.Convert(live.At(20, 20)).(color.NRGBA); got != before {
					t.Errorf("capture changed display content: %v, want %v", got, before)
				}
			})
			if err != nil {
				return err
			}
			if err = waitCapture(windowSource, func(s testclient.CaptureState) bool { return s.Stopped == 1 }); err != nil {
				return err
			}
			if state := windowSource.State(); state.Ready != 1 {
				t.Errorf("blocked window delivered another frame: %+v", state)
			}
			if err = waitCapture(outputSource, func(s testclient.CaptureState) bool { return s.Ready >= 2 }); err != nil {
				return err
			}
			if got := outputSource.Image().NRGBAAt(privateAt.X, privateAt.Y); got != (color.NRGBA{A: 255}) {
				t.Errorf("ext output leaked protected pixel: %v", got)
			}
			legacy, err := testclient.StartLegacyCapture(c.SocketName(), nil)
			if err != nil {
				return err
			}
			defer legacy.Close()
			if err = legacy.ArmLegacy(false, false); err != nil {
				return err
			}
			if err = waitCapture(legacy, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
				return err
			}
			if got := legacy.Image().NRGBAAt(privateAt.X, privateAt.Y); got != (color.NRGBA{A: 255}) {
				t.Errorf("legacy screencopy leaked protected pixel: %v", got)
			}
			// A metadata change must re-evaluate the match before the next capture.
			if err := secret.SetIdentity("ordinary", "ordinary"); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				img, e := c.captureOutput(output)
				if e != nil {
					err = e
					return
				}
				if got := img.NRGBAAt(privateAt.X, privateAt.Y); got != before {
					t.Errorf("app-id rematch retained block: %v", got)
				}
				cast := "screencast"
				err = c.Reconfigure(Options{Animations: config.Animations{Off: true}, Blur: blurConfig, WindowRules: []config.WindowRule{{Matches: []config.RuleMatch{{AppID: regexp.MustCompile("^ordinary$")}}, BlockOutFrom: &cast}}})
				if err != nil {
					return
				}
				if err = c.Screenshot(false, ""); err != nil {
					return
				}
				defer c.closeScreenshotUI()
				img, e = c.captureOutputForTarget(output, config.CaptureCast)
				if e != nil {
					err = e
					return
				}
				if got := img.NRGBAAt(privateAt.X, privateAt.Y); got != (color.NRGBA{A: 255}) {
					t.Errorf("frozen screenshot leaked screencast pixel: %v", got)
				}
				if !c.ScreenshotUIOpen() {
					t.Error("cast closed the physical screenshot UI")
				}
			})
			return err
		}()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("privacy check did not finish")
	}
}

func TestCaptureRuleReloadDropsRetiredFrozenPixels(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer window.Close()
		if err = window.Paint(0xff3498db, 200, 150); err != nil {
			t.Error(err)
			return
		}
		if err = window.SetIdentity("retired", "retired"); err != nil {
			t.Error(err)
			return
		}
		var id uint64
		onCompositor(t, c, func() {
			for _, v := range c.views {
				id = v.id
			}
			err = c.Screenshot(false, "")
		})
		if err != nil {
			t.Error(err)
			return
		}
		observer, err := testclient.StartCapture(c.SocketName(), strconv.FormatUint(id, 10))
		if err != nil {
			t.Error(err)
			return
		}
		defer observer.Close()
		window.Close()
		if err = waitCapture(observer, func(s testclient.CaptureState) bool { return s.Stopped == 1 }); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if len(c.views) != 0 || len(c.captureNodes) != 0 {
				t.Errorf("retired source retained views=%d buffers=%d", len(c.views), len(c.captureNodes))
			}
			if !c.ScreenshotUIOpen() {
				t.Error("frozen screenshot vanished before policy change")
				return
			}
			block := "screen-capture"
			if err = c.Reconfigure(Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{BlockOutFrom: &block}}}); err != nil {
				t.Error(err)
				return
			}
			if c.ScreenshotUIOpen() {
				t.Error("privacy reload retained pixels of a destroyed window")
			}
		})
	})
}

// A popup can disappear after a close/resize snapshot copies its buffers.
// Exercise that scene lifetime directly, without relying on popup timing.
func TestCapturePrivacyFollowsDetachedSnapshotBuffers(t *testing.T) {
	floating := true
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer window.Close()
		onCompositor(t, c, func() {
			var view *toplevelView
			var output unsafe.Pointer
			for _, candidate := range c.views {
				view = candidate
			}
			for pointer := range c.sceneOutputs {
				output = pointer
				break
			}
			if view == nil || output == nil {
				t.Error("missing capture target")
				return
			}
			block := "screen-capture"
			if err = c.Reconfigure(Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{BlockOutFrom: &block}}}); err != nil {
				t.Error(err)
				return
			}
			pixels := image.NewRGBA(image.Rect(0, 0, 60, 40))
			draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{220, 40, 40, 255}), image.Point{}, draw.Src)
			transient := sceneImage(view.tree, pixels)
			if transient == nil {
				t.Error("transient scene buffer allocation failed")
				return
			}
			sceneFXNodeSetPosition(unsafe.Pointer(&transient.node), 220, 0)
			c.animationConfig.Off = false
			c.animationConfig.WindowClose = testSpringAnimation()
			c.captureCloseSnapshot(view)
			sceneFXNodeDestroy(unsafe.Pointer(&transient.node))
			if view.closeSnapshot == nil {
				t.Error("close snapshot missing")
				return
			}
			sceneFXTreeSetPosition(view.closeSnapshot, 350, 40)
			sceneFXTreeSetEnabled(view.closeSnapshot, true)
			display, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return
			}
			if got := display.NRGBAAt(590, 60); got != (color.NRGBA{220, 40, 40, 255}) {
				t.Errorf("retained test pixel=%v", got)
				return
			}
			capture, err := c.captureOutput(output)
			if err != nil {
				t.Error(err)
				return
			}
			if got := capture.NRGBAAt(590, 60); got != (color.NRGBA{A: 255}) {
				t.Errorf("detached snapshot leaked protected pixel: %v", got)
			}
			display, err = c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return
			}
			if got := display.NRGBAAt(590, 60); got != (color.NRGBA{220, 40, 40, 255}) {
				t.Errorf("capture changed retained display pixel: %v", got)
			}
		})
	})
}
