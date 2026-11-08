package wlr

import (
	"encoding/json"
	"image"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

func TestMixedDPIOutputsKeepScreenshotsAndTouchUpright(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	c.addFakePointer()
	two := 2.0
	if err := c.Reconfigure(Options{Animations: config.Animations{Off: true}, Touch: config.Touch{MapToOutput: name}, Outputs: []config.OutputConfig{{Name: name, Scale: &two, Transform: config.Transform90, Mode: &config.OutputMode{Width: 800, Height: 600, Refresh: 60, Custom: true}, Position: &config.OutputPosition{X: 5000, Y: 50}}}}); err != nil {
		t.Fatal(err)
	}
	var output unsafe.Pointer
	for pointer := range c.sceneOutputs {
		if outputName(pointer) == name {
			output = pointer
		}
	}
	box, transform := c.touchOutputBox()
	if box != (rect{5000, 50, 300, 400}) || transform != 1 {
		t.Fatalf("touch mapping = %+v transform %d", box, transform)
	}
	x, y := c.touchLayoutPoint(nil, 0.25, 0.75)
	if x != 5075 || y != 150 {
		t.Fatalf("rotated touch = %g,%g", x, y)
	}
	if !c.cursorLoadedAt(two) {
		t.Error("pointer theme was not loaded at the output's scale")
	}
	c.FocusMonitor(name)
	green := sceneFXCreateRect(unsafe.Pointer(c.layerTrees[layerBackground]), 100, 50, [4]float32{0, 1, 0, 1})
	if green == nil {
		t.Fatal("could not create screenshot marker")
	}
	sceneFXNodeSetPosition(green, 5010, 70)
	t.Cleanup(func() { sceneFXNodeDestroy(green) })
	saved := make(chan struct{}, 2)
	c.SetScreenshotHandler(func(*string) { saved <- struct{}{} })
	path := filepath.Join(t.TempDir(), "screen.png")
	if err := c.ScreenshotScreen(true, false, path); err != nil {
		t.Fatal(err)
	}
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		select {
		case <-saved:
		case <-time.After(3 * time.Second):
			t.Fatal("screenshot did not save")
		}
		if got := decodePNG(t, path).Bounds().Size(); got != image.Pt(600, 800) {
			t.Errorf("screenshot size=%v, want upright 600x800", got)
		}
		path = filepath.Join(t.TempDir(), "selection.png")
		onCompositor(t, c, func() {
			if err := c.Screenshot(false, path); err != nil {
				t.Fatal(err)
			}
			if len(c.shotUI.frames) != 2 {
				t.Fatal("screenshot UI omitted an output")
			}
			c.shotUI.output = output
			c.shotUI.selection = image.Rect(5010, 70, 5110, 120)
			c.confirmScreenshot(true)
		})
		select {
		case <-saved:
		case <-time.After(3 * time.Second):
			t.Fatal("selection did not save")
		}
		if got := decodePNG(t, path).Bounds().Size(); got != image.Pt(200, 100) {
			t.Errorf("selection size=%v, want 200x100", got)
		}
		r, g, b, _ := decodePNG(t, path).At(100, 50).RGBA()
		if g < 0xf000 || r > 0x1000 || b > 0x1000 {
			t.Errorf("rotated selection did not capture its green marker: %x %x %x", r, g, b)
		}
		onCompositor(t, c, func() {
			if err := c.Screenshot(false, ""); err != nil {
				t.Error(err)
				return
			}
			if _, err := c.ConfigureOutput(ipc.OutputRequest{Name: name, Action: json.RawMessage(`{"Scale":{"scale":{"Specific":1}}}`)}); err != nil {
				t.Error(err)
			}
			if c.ScreenshotUIOpen() {
				t.Error("output geometry change left the screenshot UI using a stale frame")
			}
		})
	})
}
