package wlr

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

const (
	keyP        = 25 // evdev KEY_P
	keyC        = 46 // evdev KEY_C
	keyLeftCtrl = 29 // evdev KEY_LEFTCTRL
)

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// differs counts the pixels that differ between a and b inside r.
func differs(a, b image.Image, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				n++
			}
		}
	}
	return n
}

func waitReport(t *testing.T, reports chan *string) *string {
	t.Helper()
	select {
	case path := <-reports:
		return path
	case <-time.After(5 * time.Second):
		t.Error("no screenshot was reported")
		return nil
	}
}

// TestScreenshotScreenDrawsThePointerWhenAsked: show-pointer draws the theme
// image where the pointer is, and nothing else changes.
func TestScreenshotScreenDrawsThePointerWhenAsked(t *testing.T) {
	dir := t.TempDir()
	c := newNested(t)
	reports := make(chan *string, 2)
	c.SetScreenshotHandler(func(path *string) { reports <- path })
	var at image.Point
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var pointer *fakePointer
		defer onCompositor(t, c, func() { pointer.remove() })
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				at = box.Min.Add(image.Pt(box.Dx()/2, box.Dy()/2))
				pointer.moveTo(float64(at.X), float64(at.Y))
				w, h := outputPixelSize(output)
				at = image.Pt(w/2, h/2)
				break
			}
			if _, _, ok := c.pointerImage(1); !ok {
				t.Error("no pointer image to draw")
			}
			if err := c.ScreenshotScreen(true, false, filepath.Join(dir, "bare.png")); err != nil {
				t.Error(err)
			}
		})
		waitReport(t, reports)
		onCompositor(t, c, func() {
			if err := c.ScreenshotScreen(true, true, filepath.Join(dir, "pointer.png")); err != nil {
				t.Error(err)
			}
		})
		waitReport(t, reports)
	})
	bare, shown := decodePNG(t, filepath.Join(dir, "bare.png")), decodePNG(t, filepath.Join(dir, "pointer.png"))
	near := image.Rect(at.X-32, at.Y-32, at.X+32, at.Y+32).Intersect(bare.Bounds())
	if differs(bare, shown, near) < 20 {
		t.Error("show-pointer drew no pointer at the pointer")
	}
	if n := differs(bare, shown, image.Rect(0, 0, bare.Bounds().Dx(), max(0, near.Min.Y))); n != 0 {
		t.Errorf("%d pixels changed away from the pointer", n)
	}
}

// TestScreenshotUIHintTogglesThePointerAndCaptures: the help panel is up on
// every output; P shows the frozen pointer and swaps the panel; Ctrl+C
// copies to the clipboard only; a click on the capture button saves.
func TestScreenshotUIHintTogglesThePointerAndCaptures(t *testing.T) {
	dir := t.TempDir()
	c := newNested(t)
	reports := make(chan *string, 2)
	c.SetScreenshotHandler(func(path *string) { reports <- path })
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var pointer *fakePointer
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() {
			pointer.remove()
			keyboard.remove()
		})
		var button image.Point
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			keyboard = c.addFakeKeyboard()
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				pointer.moveTo(float64(box.Min.X+40), float64(box.Min.Y+40))
			}
			if err := c.Screenshot(false, filepath.Join(dir, "button.png")); err != nil {
				t.Error(err)
				return
			}
			for _, frame := range c.shotUI.frames {
				if frame.hint[0] == nil || frame.hint[1] == nil {
					t.Fatal("an output has no help panel")
				}
				if !sceneFXNodeEnabled(unsafe.Pointer(&frame.hint[0].node)) || sceneFXNodeEnabled(unsafe.Pointer(&frame.hint[1].node)) {
					t.Error("the panel does not offer to show the pointer")
				}
				if frame.pointer != nil && sceneFXNodeEnabled(unsafe.Pointer(&frame.pointerNode.node)) {
					t.Error("the pointer is shown with show-pointer off")
				}
			}
			keyboard.key(keyP, true)
			keyboard.key(keyP, false)
			if !c.shotUI.showPointer {
				t.Fatal("P did not show the pointer")
			}
			frozen := false
			for _, frame := range c.shotUI.frames {
				if frame.pointer != nil {
					frozen = sceneFXNodeEnabled(unsafe.Pointer(&frame.pointerNode.node))
				}
				if sceneFXNodeEnabled(unsafe.Pointer(&frame.hint[0].node)) || !sceneFXNodeEnabled(unsafe.Pointer(&frame.hint[1].node)) {
					t.Error("the panel did not switch to hiding the pointer")
				}
			}
			if !frozen {
				t.Error("no frozen pointer is drawn after P")
			}

			keyboard.key(keyLeftCtrl, true)
			keyboard.key(keyC, true)
			keyboard.key(keyC, false)
			keyboard.key(keyLeftCtrl, false)
			if c.ScreenshotUIOpen() {
				t.Fatal("Ctrl+C did not capture")
			}
		})
		if path := waitReport(t, reports); path != nil {
			t.Errorf("Ctrl+C wrote %q, want the clipboard only", *path)
		}

		onCompositor(t, c, func() {
			if err := c.Screenshot(true, filepath.Join(dir, "button.png")); err != nil {
				t.Error(err)
				return
			}
			for _, frame := range c.shotUI.frames {
				centre := frame.hintAt.Add(image.Pt(int(8*frame.scale)+int(16*frame.scale), frame.hintSize.Y/2))
				button = frame.logical(image.Rectangle{Min: centre, Max: centre}).Min
			}
			pointer.moveTo(float64(button.X), float64(button.Y))
			pointer.button(btnLeft, true)
			pointer.button(btnLeft, false)
			if c.ScreenshotUIOpen() {
				t.Error("a click on the capture button did not capture")
			}
		})
		if path := waitReport(t, reports); path == nil || *path != filepath.Join(dir, "button.png") {
			t.Errorf("the capture button reported %v, want the given path", path)
		}
	})
}
