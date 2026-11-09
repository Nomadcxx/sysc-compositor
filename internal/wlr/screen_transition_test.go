package wlr

import (
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestScreenTransitionUsesUnadjustedDelayAndFade(t *testing.T) {
	start := time.Unix(1, 0)
	for _, tt := range []struct {
		elapsed time.Duration
		want    float64
	}{{-time.Millisecond, 1}, {0, 1}, {250 * time.Millisecond, .5}, {500 * time.Millisecond, 0}, {time.Second, 0}} {
		if got := screenTransitionOpacity(start.Add(tt.elapsed), start); got != tt.want {
			t.Errorf("elapsed=%v alpha=%g want=%g", tt.elapsed, got, tt.want)
		}
	}
}

func TestScreenTransitionRetainsPixelsAndClearsOnLock(t *testing.T) {
	c := newNested(t)
	var output unsafe.Pointer
	for output = range c.sceneOutputs {
		break
	}
	box := c.outputLayoutBox(output)
	// Desktop content: the overlay layer sits below the transition, as every
	// scene tree but the lock, exit dialog and config notification does.
	marker := sceneFXCreateRect(unsafe.Pointer(c.layerTrees[layerOverlay]), box.Dx(), box.Dy(), [4]float32{0, 1, 0, 1})
	defer sceneFXNodeDestroy(marker)
	sceneFXNodeSetPosition(marker, box.Min.X, box.Min.Y)
	delay := uint16(1000)
	if err := c.DoScreenTransition(&delay); err != nil {
		t.Fatal(err)
	}
	sceneFXRectUpdateFill(marker, box.Min.X, box.Min.Y, box.Dx(), box.Dy(), [4]uint16{}, [4]float32{1, 0, 0, 1})
	frame, err := c.captureOutputForTarget(output, config.CaptureScreen)
	if err != nil {
		t.Fatal(err)
	}
	center := frame.Bounds().Max.Div(2)
	if pixel := frame.NRGBAAt(center.X, center.Y); pixel.G < 240 || pixel.R > 10 {
		t.Fatalf("transition did not retain old pixels: %v", pixel)
	}
	c.sealSession()
	if len(c.screenTransitions) != 0 {
		t.Fatal("lock retained desktop transition")
	}
}
