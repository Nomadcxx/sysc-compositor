package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_output.h>
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"os"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ui"
)

type configNoticePanel struct {
	node   *C.struct_wlr_scene_buffer
	box    image.Rectangle
	scale  float64
	pixels image.Point
}
type configNotice struct {
	tree     *C.struct_wlr_scene_tree
	panels   []configNoticePanel
	progress float64
	timer    *time.Timer
}

var configNoticeKey = animationKey{property: "config-notification"}

func (c *Compositor) NotifyConfigLoadFailed(failed bool) {
	if !failed {
		c.hideConfigNotice()
		return
	}
	if c.configNotification.DisableFailed {
		return
	}
	c.clearConfigNotice()
	c.configNotice.progress = 0
	c.syncConfigNotice()
	if c.configNotice.tree == nil {
		return
	}
	c.animateValue(configNoticeKey, c.animationConfig.ConfigNotificationOpenClose, 0, 1, 0, c.placeConfigNotice, func() {
		c.configNotice.timer = time.AfterFunc(4*time.Second, func() { _ = c.Post(c.hideConfigNotice) })
	})
}
func (c *Compositor) clearConfigNotice() {
	c.cancelAnimation(configNoticeKey)
	if c.configNotice.timer != nil {
		c.configNotice.timer.Stop()
	}
	if c.configNotice.tree != nil {
		C.wlr_scene_node_destroy(&c.configNotice.tree.node)
	}
	c.configNotice = configNotice{}
}
func (c *Compositor) hideConfigNotice() {
	if c.configNotice.tree == nil {
		return
	}
	if c.configNotice.timer != nil {
		c.configNotice.timer.Stop()
		c.configNotice.timer = nil
	}
	c.animateValue(configNoticeKey, c.animationConfig.ConfigNotificationOpenClose, c.configNotice.progress, 0, 0, c.placeConfigNotice, c.clearConfigNotice)
}
func (c *Compositor) syncConfigNotice() {
	// New failure creates the tree; geometry changes rebuild only visible panels.
	notice := &c.configNotice
	if notice.tree != nil {
		C.wlr_scene_node_destroy(&notice.tree.node)
	}
	notice.tree = C.wlr_scene_tree_create(&c.scene.tree)
	notice.panels = nil
	if notice.tree == nil {
		return
	}
	if c.lockTree != nil {
		C.wlr_scene_node_place_below(&notice.tree.node, &c.lockTree.node)
	}
	for output := range c.sceneOutputs {
		scale := float64((*C.struct_wlr_output)(output).scale)
		img, err := ui.ConfigErrorNotification(scale)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: config notification: %v\n", err)
			continue
		}
		node := sceneImage(notice.tree, img)
		if node == nil {
			continue
		}
		sceneImageIgnoreInput(node)
		notice.panels = append(notice.panels, configNoticePanel{node, c.outputLayoutBox(output), scale, img.Bounds().Size()})
	}
	c.placeConfigNotice(notice.progress)
}
func (c *Compositor) placeConfigNotice(progress float64) {
	c.configNotice.progress = progress
	for _, panel := range c.configNotice.panels {
		width := int(math.Round(float64(panel.pixels.X) / panel.scale))
		height := int(math.Round(float64(panel.pixels.Y) / panel.scale))
		x := panel.box.Min.X + max(0, panel.box.Dx()-width)/2
		y := panel.box.Min.Y - height + int(math.Round(progress*float64(height+16)))
		sceneImagePlace(panel.node, image.Rect(x, y, x+width, y+height), 1)
	}
	c.damageCaptureSources()
}
func (c *Compositor) configNoticeGeometryChanged() {
	if c.configNotice.tree != nil {
		c.syncConfigNotice()
	}
}
