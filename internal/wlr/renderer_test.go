package wlr

import (
	"image/color"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestRendererLossRecreatesAndRepaintsExistingBuffers(t *testing.T) {
	t.Setenv("WLR_RENDERER", "gles2")
	t.Setenv("LIBGL_ALWAYS_SOFTWARE", "1")
	c := newNested(t)
	original := c.renderer
	var output unsafe.Pointer
	for output = range c.sceneOutputs {
		break
	}
	lost, checked := false, false
	var frames int
	pollThenStop(c, func() bool {
		ready := false
		for _, layer := range c.layerOrder {
			if width, _, mapped := layer.mappedSize(); mapped && width > 0 {
				ready = true
			}
		}
		if !ready {
			return false
		}
		if !lost {
			frames = c.scenesDrawn()
			lost = true
			forceRendererLossForTest(c)
			return false
		}
		if c.renderer == original || c.scenesDrawn() <= frames {
			return false
		}
		frame, err := c.captureOutput(output)
		if err != nil {
			t.Error(err)
			return true
		}
		center := frame.Bounds().Max.Div(2)
		if pixel := frame.NRGBAAt(center.X, center.Y); pixel.G < 240 || pixel.R > 10 || pixel.B > 10 {
			t.Errorf("existing buffer after recovery = %v", pixel)
		}
		checked = true
		return true
	})
	runClient(t, c, []string{"swaybg", "-m", "fill", "-i", solidPNG(t, color.NRGBA{G: 255, A: 255})}, nil, nil)
	if !checked {
		t.Fatal("renderer never recovered and repainted")
	}
}

func TestRendererLossRestoresLatestSHMCommit(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		if err := clients[0].Paint(0x00ff0000, 80, 60); err != nil {
			t.Fatal(err)
		}
		if err := clients[0].Paint(0x0000ff00, 80, 60); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() { forceRendererLossForTest(c) })
		onCompositor(t, c, func() {
			frame, err := c.renderWindowCapture(views[0], config.CaptureScreen)
			if err != nil {
				t.Error(err)
				return
			}
			center := frame.Bounds().Max.Div(2)
			r, g, b, _ := frame.At(center.X, center.Y).RGBA()
			if g < 0xf000 || r > 0x1000 || b > 0x1000 {
				t.Errorf("recovered stale SHM pixels: %x %x %x", r, g, b)
			}
		})
	})
}
