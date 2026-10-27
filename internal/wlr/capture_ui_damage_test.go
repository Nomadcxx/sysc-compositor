package wlr

import (
	"bytes"
	"image/color"
	"os/exec"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestCaptureOutputFollowsCompositorUIDamage(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "ext-copy"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
			runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
				var client *testclient.CaptureClient
				var err error
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
				baseline := append([]byte(nil), client.Image().Pix...)
				steps := []struct {
					name     string
					change   func()
					baseline bool
				}{
					{"show hotkeys", c.ShowHotkeyOverlay, false},
					{"hide hotkeys", c.hideHotkeyOverlay, true},
					{"show exit dialog", func() {
						if !c.showExitDialog() {
							t.Error("exit dialog could not be drawn")
						}
					}, false},
					{"hide exit dialog", c.hideExitDialog, true},
				}
				for i, step := range steps {
					if err = arm(); err != nil {
						t.Error(err)
						return
					}
					if err = client.Wait(80 * time.Millisecond); err != nil {
						t.Error(err)
						return
					}
					if state := client.State(); state.Ready != i+1 {
						t.Errorf("%s: unchanged UI produced frame %+v", step.name, state)
						return
					}
					onCompositor(t, c, step.change)
					if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == i+2 }); err != nil {
						t.Errorf("%s: %v", step.name, err)
						return
					}
					if equal := bytes.Equal(client.Image().Pix, baseline); equal != step.baseline {
						t.Errorf("%s: captured image equals baseline=%v, want %v", step.name, equal, step.baseline)
						return
					}
				}
			})
		})
	}
}

func TestCaptureOutputFollowsLockSubsurfaceDamage(t *testing.T) {
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
				if err = lock.PaintSubsurface(0xff3498db); err != nil {
					t.Error(err)
					return
				}
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
				if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{0x34, 0x98, 0xdb, 255}) {
					t.Errorf("initial lock child pixel %v", got)
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
					t.Error("unchanged lock child produced a frame")
					return
				}
				if err = lock.PaintSubsurface(0xffdc2828); err != nil {
					t.Error(err)
					return
				}
				if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
					t.Error(err)
					return
				}
				if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{220, 40, 40, 255}) {
					t.Errorf("lock child capture pixel %v", got)
				}
			})
		})
	}
}
