package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_output.h>
*/
import "C"

import (
	"errors"
	"image"
	"image/draw"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

type screenTransition struct {
	tree   *C.struct_wlr_scene_tree
	frames [3]*C.struct_wlr_scene_buffer
	start  time.Time
}

func screenTransitionOpacity(now, start time.Time) float64 {
	return 1 - min(1, max(0, float64(now.Sub(start))/float64(500*time.Millisecond)))
}

// DoScreenTransition freezes each render target independently. The old frame
// stays opaque through the delay, then crossfades over 500 ms of real time.
func (c *Compositor) DoScreenTransition(delayMS *uint16) error {
	if c.sessionLocked {
		return errors.New("wlr: no screen transition while locked")
	}
	c.clearScreenTransitions()
	delay := 250 * time.Millisecond
	if delayMS != nil {
		delay = time.Duration(*delayMS) * time.Millisecond
	}
	start := time.Now().Add(delay)
	c.screenTransitions = map[unsafe.Pointer]*screenTransition{}
	for output := range c.sceneOutputs {
		transition := &screenTransition{start: start}
		transition.tree = C.wlr_scene_tree_create(&c.scene.tree)
		if transition.tree == nil {
			c.clearScreenTransitions()
			return errors.New("wlr: cannot allocate screen transition")
		}
		// Niri draws the config notification, exit dialog and lock over it.
		above := &c.lockTree.node
		if c.configNotice.tree != nil {
			above = &c.configNotice.tree.node
		}
		C.wlr_scene_node_place_below(&transition.tree.node, above)
		C.wlr_scene_node_set_enabled(&transition.tree.node, false)
		for _, target := range []config.CaptureTarget{config.CaptureDisplay, config.CaptureScreen, config.CaptureCast} {
			pixels, err := c.captureOutputForTarget(output, target)
			if err != nil {
				C.wlr_scene_node_destroy(&transition.tree.node)
				c.clearScreenTransitions()
				return err
			}
			pixels = uprightCastFrame(pixels, uint32((*C.struct_wlr_output)(output).transform))
			rgba := image.NewRGBA(pixels.Bounds())
			draw.Draw(rgba, rgba.Bounds(), pixels, pixels.Bounds().Min, draw.Src)
			node := sceneImage(transition.tree, rgba)
			if node == nil {
				C.wlr_scene_node_destroy(&transition.tree.node)
				c.clearScreenTransitions()
				return errors.New("wlr: cannot retain screen transition")
			}
			sceneImageIgnoreInput(node)
			transition.frames[int(target)] = node
			C.wlr_scene_node_set_enabled(&node.node, target == config.CaptureDisplay)
		}
		c.screenTransitions[output] = transition
		c.placeScreenTransition(output, transition, 1)
		C.wlr_scene_node_set_enabled(&transition.tree.node, true)
	}
	c.scheduleScreenTransitionTick()
	return nil
}

func (c *Compositor) placeScreenTransition(output unsafe.Pointer, transition *screenTransition, opacity float64) {
	box := c.outputLayoutBox(output)
	for _, node := range transition.frames {
		if node != nil {
			sceneImagePlace(node, box, opacity)
		}
	}
}
func (c *Compositor) screenTransitionGeometryChanged(output unsafe.Pointer) {
	if transition := c.screenTransitions[output]; transition != nil {
		c.placeScreenTransition(output, transition, screenTransitionOpacity(time.Now(), transition.start))
	}
}
func (c *Compositor) finishScreenTransition(output unsafe.Pointer) {
	if transition := c.screenTransitions[output]; transition != nil {
		C.wlr_scene_node_destroy(&transition.tree.node)
		delete(c.screenTransitions, output)
	}
}
func (c *Compositor) clearScreenTransitions() {
	c.screenTransitionGeneration++
	if c.screenTransitionTimer != nil {
		c.screenTransitionTimer.Stop()
		c.screenTransitionTimer = nil
	}
	for output := range c.screenTransitions {
		c.finishScreenTransition(output)
	}
}
func (c *Compositor) scheduleScreenTransitionTick() {
	if len(c.screenTransitions) == 0 || c.screenTransitionTimer != nil {
		return
	}
	generation := c.screenTransitionGeneration
	c.screenTransitionTimer = time.AfterFunc(animationFrame, func() {
		_ = c.Post(func() {
			if generation != c.screenTransitionGeneration {
				return
			}
			c.screenTransitionTimer = nil
			now := time.Now()
			for output, transition := range c.screenTransitions {
				opacity := screenTransitionOpacity(now, transition.start)
				if opacity <= 0 {
					c.finishScreenTransition(output)
				} else {
					c.placeScreenTransition(output, transition, opacity)
				}
			}
			c.damageCaptureSources()
			c.scheduleScreenTransitionTick()
		})
	})
}
func (c *Compositor) selectScreenTransitionTarget(target config.CaptureTarget) func() {
	for _, transition := range c.screenTransitions {
		for i, node := range transition.frames {
			C.wlr_scene_node_set_enabled(&node.node, i == int(target))
		}
	}
	return func() {
		for _, transition := range c.screenTransitions {
			for i, node := range transition.frames {
				C.wlr_scene_node_set_enabled(&node.node, i == int(config.CaptureDisplay))
			}
		}
	}
}
