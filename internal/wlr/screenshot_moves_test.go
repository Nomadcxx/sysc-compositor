package wlr

import (
	"image"
	"os/exec"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

const (
	keyLeft  = 105 // evdev KEY_LEFT
	keyRight = 106 // evdev KEY_RIGHT
	keyDown  = 108 // evdev KEY_DOWN
)

// TestScreenshotUIMovesTheSelectionByBinds is Niri's
// allowed_during_screenshot: column and window moves move the selection 50px
// within its output, and a monitor move carries it to the next output.
func TestScreenshotUIMovesTheSelectionByBinds(t *testing.T) {
	cfg, err := config.Parse("moves.kdl", []byte(`
binds {
    Ctrl+Left { move-column-left; }
    Ctrl+Down { move-window-down; }
    Ctrl+Right { move-column-to-monitor-next; }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{Binds: cfg.Binds})
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() { keyboard.remove() })
		onCompositor(t, c, func() {
			keyboard = c.addFakeKeyboard()
			press := func(keys ...uint32) {
				for _, k := range keys {
					keyboard.key(k, true)
				}
				for i := len(keys) - 1; i >= 0; i-- {
					keyboard.key(keys[i], false)
				}
			}
			if err := c.Screenshot(false, ""); err != nil {
				t.Fatal(err)
			}
			if len(c.shotUI.frames) != 2 {
				t.Fatalf("screenshot UI froze %d outputs, want 2", len(c.shotUI.frames))
			}
			ui := c.shotUI
			var first unsafe.Pointer
			for output := range ui.frames {
				first = output
				break
			}
			box := ui.frames[first].box
			ui.output = first
			ui.selection = image.Rect(box.Min.X+120, box.Min.Y+100, box.Min.X+220, box.Min.Y+150)
			c.drawShotSelection()

			press(keyLeftCtrl, keyLeft)
			if ui.selection.Min.X != box.Min.X+70 || ui.selection.Dx() != 100 {
				t.Errorf("after one move left the selection is %v", ui.selection)
			}
			press(keyLeftCtrl, keyLeft)
			press(keyLeftCtrl, keyLeft)
			if ui.selection.Min.X != box.Min.X || ui.selection.Dx() != 100 {
				t.Errorf("moving left did not stop at the output's edge: %v", ui.selection)
			}
			press(keyLeftCtrl, keyDown)
			if ui.selection.Min.Y != box.Min.Y+150 {
				t.Errorf("after a move down the selection is %v", ui.selection)
			}

			press(keyLeftCtrl, keyRight)
			if ui.output == first {
				t.Fatal("the monitor move left the selection on its output")
			}
			to := ui.frames[ui.output].box
			if !ui.selection.In(to) || ui.selection.Dx() != 100 || ui.selection.Dy() != 50 {
				t.Errorf("selection on the next output is %v in %v, want 100x50 inside", ui.selection, to)
			}
			if !c.ScreenshotUIOpen() {
				t.Error("a move closed the screenshot UI")
			}
			c.closeScreenshotUI()
		})
	})
}
