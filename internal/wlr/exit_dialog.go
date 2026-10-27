package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_seat.h>
#include <scenefx/types/wlr_scene.h>
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"os"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/ui"
)

// The exit confirmation dialog (Niri's exit_confirm_dialog.rs). Quit without
// skip-confirmation opens it over everything, the lock screen included; Enter
// then quits, and any other key press, button press or scroll closes it.
// While it is open no client has keyboard or pointer focus. It fades and
// zooms from 80% with the exit-confirmation-open-close animation.

type exitDialog struct {
	open     bool
	progress float64
	tree     *C.struct_wlr_scene_tree // nil once fully closed
	outputs  []exitDialogOutput
	panels   map[float64]*image.RGBA // by output scale
}

type exitDialogOutput struct {
	backdrop unsafe.Pointer
	panel    *C.struct_wlr_scene_buffer
	box      image.Rectangle // the output's layout box
	size     image.Point     // the panel's logical size
}

var exitDialogKey = animationKey{property: "exit-dialog"}

// ExitDialogOpen reports whether the exit confirmation dialog is up.
func (c *Compositor) ExitDialogOpen() bool { return c.exitDialog.open }

// Quit is Niri's Quit action: it asks first through the exit dialog unless
// skipConfirmation is set or the dialog cannot be drawn.
func (c *Compositor) Quit(skipConfirmation bool) {
	if !skipConfirmation && c.showExitDialog() {
		return
	}
	C.wl_display_terminate(c.display)
}

// exitDialogPanel is the dialog drawn for scale, cached per scale.
func (c *Compositor) exitDialogPanel(scale float64) (*image.RGBA, error) {
	d := &c.exitDialog
	if img := d.panels[scale]; img != nil {
		return img, nil
	}
	img, err := ui.ExitDialog(scale)
	if err != nil {
		return nil, err
	}
	if d.panels == nil {
		d.panels = map[float64]*image.RGBA{}
	}
	d.panels[scale] = img
	return img, nil
}

// showExitDialog opens the dialog, or reports false when it cannot be drawn.
func (c *Compositor) showExitDialog() bool {
	d := &c.exitDialog
	if d.open {
		return true
	}
	if _, err := c.exitDialogPanel(1); err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: exit dialog: %v\n", err)
		return false
	}
	if d.tree == nil && !c.buildExitDialog() {
		return false
	}
	d.open = true
	C.wlr_scene_node_raise_to_top(&d.tree.node)
	c.stopKeyboardRepeat()
	c.clearPointerGrab()
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	c.updateKeyboardFocus()
	c.animateExitDialog(1)
	return true
}

// hideExitDialog starts the close animation and gives focus back.
func (c *Compositor) hideExitDialog() {
	d := &c.exitDialog
	if !d.open {
		return
	}
	d.open = false
	c.updateKeyboardFocus()
	c.animateExitDialog(0)
}

func (c *Compositor) animateExitDialog(to float64) {
	d := &c.exitDialog
	c.animateValue(exitDialogKey, c.animationConfig.ExitConfirmationOpenClose, d.progress, to, 0,
		c.setExitDialogProgress, func() {
			if !d.open && d.tree != nil {
				C.wlr_scene_node_destroy(&d.tree.node)
				d.tree, d.outputs = nil, nil
				// The backdrops no longer cover the windows under the pointer.
				c.refocusPointer(nowMsec())
			}
		})
}

// buildExitDialog lays a backdrop and a centred panel over every enabled
// output.
func (c *Compositor) buildExitDialog() bool {
	d := &c.exitDialog
	d.tree = C.wlr_scene_tree_create(&c.scene.tree)
	if d.tree == nil {
		return false
	}
	for pointer := range c.sceneOutputs {
		if !c.outputIsEnabled(pointer) {
			continue
		}
		scale := float64((*C.struct_wlr_output)(pointer).scale)
		img, err := c.exitDialogPanel(scale)
		if err != nil {
			if img, err = c.exitDialogPanel(1); err != nil {
				continue
			}
			scale = 1
		}
		box := c.outputLayoutBox(pointer)
		out := exitDialogOutput{
			backdrop: sceneFXCreateRect(unsafe.Pointer(d.tree), box.Dx(), box.Dy(), ui.ExitDialogBackdrop),
			panel:    sceneImage(d.tree, img),
			box:      box,
			size: image.Pt(int(math.Round(float64(img.Rect.Dx())/scale)),
				int(math.Round(float64(img.Rect.Dy())/scale))),
		}
		if out.backdrop != nil {
			sceneFXNodeSetPosition(out.backdrop, box.Min.X, box.Min.Y)
		}
		d.outputs = append(d.outputs, out)
	}
	c.setExitDialogProgress(d.progress)
	return true
}

// setExitDialogProgress draws the dialog at an animation value: the backdrop
// and panel fade with it clamped to [0, 1], and the panel zooms from 80%
// about its centre with the unclamped value, so a spring overshoot shows.
func (c *Compositor) setExitDialogProgress(value float64) {
	d := &c.exitDialog
	if d.tree != nil {
		defer c.damageCaptureSources()
	}
	d.progress = value
	alpha := clampOpacity(value)
	zoom := max(value, 0)*0.2 + 0.8
	for _, out := range d.outputs {
		if out.backdrop != nil {
			color := ui.ExitDialogBackdrop
			color[3] *= float32(alpha)
			sceneRectSetColor(out.backdrop, color)
		}
		if out.panel == nil {
			continue
		}
		centre := out.box.Min.Add(image.Pt(max(0, (out.box.Dx()-out.size.X)/2), max(0, (out.box.Dy()-out.size.Y)/2))).
			Add(out.size.Div(2))
		w, h := int(math.Round(float64(out.size.X)*zoom)), int(math.Round(float64(out.size.Y)*zoom))
		box := image.Rect(centre.X-w/2, centre.Y-h/2, centre.X-w/2+w, centre.Y-h/2+h)
		sceneImagePlace(out.panel, box, alpha)
	}
}

// exitDialogKeyPress takes a user key press while the dialog is open: Enter quits
// and every other key closes the dialog. No client sees the press.
func (c *Compositor) exitDialogKeyPress(keycode uint32) bool {
	if !c.exitDialog.open {
		return false
	}
	if c.keysymIs(keycode, "Return") {
		C.wl_display_terminate(c.display)
		return true
	}
	c.hideExitDialog()
	return true
}
