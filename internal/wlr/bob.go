package wlr

import (
	"math"
	"time"
)

// bobOffset is Niri's baba_is_float_offset: a surface with baba-is-float
// floats up to a 48th of its output's height above its place and back, on a
// 3.6 second cycle.
func bobOffset(now time.Time, outputHeight int) int {
	seconds := float64(now.UnixNano()) / float64(time.Second)
	amplitude := float64(outputHeight) / 96
	return int(math.Round(amplitude * (math.Sin(2*math.Pi*seconds/3.6) - 1)))
}

// bobbing reports whether a mapped window or layer surface floats, so the
// animation clock keeps ticking for it.
func (c *Compositor) bobbing() bool {
	for _, view := range c.views {
		if view.mapped && view.appearance.BabaIsFloat != nil && *view.appearance.BabaIsFloat {
			return true
		}
	}
	for _, layer := range c.layerOrder {
		if _, _, mapped := layer.mappedSize(); mapped && layer.rules.BabaIsFloat {
			return true
		}
	}
	return false
}

// syncBobbing moves every floating window and layer surface to its bob
// offset at now, and back to its place once its rule stops floating it. The
// offset moves input with the drawing, as Niri's hit test does.
func (c *Compositor) syncBobbing(now time.Time) {
	var outputs map[uint64]string
	for _, view := range c.views {
		bob := 0
		if view.mapped && view.appearance.BabaIsFloat != nil && *view.appearance.BabaIsFloat {
			if outputs == nil && c.model != nil {
				outputs = make(map[uint64]string)
				for _, window := range c.model.Snapshot().Windows {
					outputs[window.ID] = window.Output
				}
			}
			if bounds, ok := c.outputLayoutBounds(outputs[view.id]); ok {
				bob = bobOffset(now, bounds.Height)
			}
		}
		if bob != view.bobY {
			x, y := view.layoutPosition()
			view.bobY = bob
			c.setViewScenePosition(view, x, y)
		}
	}
	for _, layer := range c.layerOrder {
		bob := 0
		if _, _, mapped := layer.mappedSize(); mapped && layer.rules.BabaIsFloat {
			if bounds, ok := c.outputLayoutBounds(layer.outputName); ok {
				bob = bobOffset(now, bounds.Height)
			}
		}
		layer.setLayerBob(bob)
	}
	c.scheduleAnimationTick()
}
