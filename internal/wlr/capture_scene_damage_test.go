package wlr

import (
	"image"
	"image/color"
	"os/exec"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestCaptureOutputFollowsLockSurfaceDamage(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "ext-copy"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
			runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
				lock, err := testclient.StartLockClient(c.SocketName(), true)
				if err != nil {
					t.Error(err)
					return
				}
				defer lock.Close()
				var client *testclient.CaptureClient
				if legacy {
					client, err = testclient.StartLegacyCapture(c.SocketName(), nil)
				} else {
					client, err = testclient.StartCapture(c.SocketName(), "")
				}
				if err != nil {
					t.Error(err)
					return
				}
				defer client.Close()
				arm := func() error {
					if legacy {
						return client.ArmLegacy(true, false)
					}
					return client.Arm(false)
				}
				if err = arm(); err != nil {
					t.Error(err)
					return
				}
				if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
					t.Error(err)
					return
				}
				if err = arm(); err != nil {
					t.Error(err)
					return
				}
				if err = client.Sync(); err != nil {
					t.Error(err)
					return
				}
				if err = client.Wait(80 * time.Millisecond); err != nil {
					t.Error(err)
					return
				}
				if client.State().Ready != 1 {
					t.Error("unchanged lock surface produced a frame")
					return
				}
				if err = lock.Paint(0xffdc2828); err != nil {
					t.Error(err)
					return
				}
				if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
					t.Error(err)
					return
				}
				if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{220, 40, 40, 255}) {
					t.Errorf("lock capture pixel %v", got)
				}
			})
		})
	}
}

func TestCaptureOutputFollowsScreenshotSelectionDamage(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "ext-copy"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			floating := true
			c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
			runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
				window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
				if err != nil {
					t.Error(err)
					return
				}
				defer window.Close()
				if err = window.Paint(0xffdc2828, 200, 150); err != nil {
					t.Error(err)
					return
				}
				onCompositor(t, c, func() {
					for _, v := range c.views {
						c.MoveFloatingWindow(&v.id, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40}, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40})
					}
					err = c.Screenshot(false, "")
					if err == nil {
						c.shotUI.selection = image.Rect(40, 40, 240, 190)
						c.drawShotSelection()
					}
				})
				if err != nil {
					t.Error(err)
					return
				}
				defer onCompositor(t, c, func() { c.closeScreenshotUI() })
				var client *testclient.CaptureClient
				if legacy {
					client, err = testclient.StartLegacyCapture(c.SocketName(), nil)
				} else {
					client, err = testclient.StartCapture(c.SocketName(), "")
				}
				if err != nil {
					t.Error(err)
					return
				}
				defer client.Close()
				arm := func() error {
					if legacy {
						return client.ArmLegacy(true, false)
					}
					return client.Arm(false)
				}
				if err = arm(); err != nil {
					t.Error(err)
					return
				}
				if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
					t.Error(err)
					return
				}
				if got := client.Image().NRGBAAt(80, 80); got != (color.NRGBA{220, 40, 40, 255}) {
					t.Errorf("initial selected pixel %v", got)
					return
				}
				if err = arm(); err != nil {
					t.Error(err)
					return
				}
				if err = client.Sync(); err != nil {
					t.Error(err)
					return
				}
				onCompositor(t, c, func() { c.shotUI.selection = image.Rect(140, 40, 240, 190); c.drawShotSelection() })
				if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
					t.Error(err)
					return
				}
				if got := client.Image().NRGBAAt(80, 80); got.R >= 200 || got.R < 100 || got.A != 255 {
					t.Errorf("selection failed to dim captured pixel: %v", got)
				}
			})
		})
	}
}
