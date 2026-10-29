package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_seat.h>
*/
import "C"

import (
	"errors"
	"image"
	"math"
)

type colorPicker struct{ done func([3]float64, error) }

// PickColor starts a pointer grab with Niri's crosshair and one correlated
// result. Its cancellation function, like this entry point, runs only on the
// compositor thread. Rendering reuses the privacy-aware output capture path.
func (c *Compositor) PickColor(done func([3]float64, error)) (func(), error) {
	if done == nil {
		return nil, errors.New("wlr: a color pick needs a result handler")
	}
	if c.sessionLocked || c.allOutputsOff() || c.pickColor != nil || c.shotUI != nil {
		return nil, errors.New("wlr: color picker unavailable")
	}
	pick := &colorPicker{done: done}
	c.pickColor = pick
	c.cancelTiledWindowMove()
	c.clearPointerGrab()
	if c.activeConstraint != nil {
		c.releasePointerConstraint(true)
	}
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	c.showCursorImage("crosshair")
	return func() {
		if c.pickColor == pick {
			c.finishColorPick([3]float64{}, errors.New("no color picked"))
		}
	}, nil
}
func (c *Compositor) finishColorPick(rgb [3]float64, err error) {
	pick := c.pickColor
	if pick == nil {
		return
	}
	c.pickColor = nil
	c.restoreCursorImage()
	c.refocusPointer(nowMsec())
	pick.done(rgb, err)
}
func (c *Compositor) pickColorButton(button uint32, pressed bool) bool {
	if c.pickColor == nil {
		return false
	}
	if !pressed {
		return true
	}
	c.consumedPointerButtons[button] = true
	var rgb [3]float64
	err := errors.New("no output under the pointer")
	for pointer := range c.sceneOutputs {
		box := c.outputLayoutBox(pointer)
		if !image.Pt(int(c.cursor.x), int(c.cursor.y)).In(box) {
			continue
		}
		output := (*C.struct_wlr_output)(pointer)
		scale := float64(output.scale)
		p := image.Pt(int(math.Floor((float64(c.cursor.x)-float64(box.Min.X))*scale)), int(math.Floor((float64(c.cursor.y)-float64(box.Min.Y))*scale)))
		w, h := outputPixelSize(pointer)
		p = outputPointToBuffer(p, image.Pt(w, h), uint32(output.transform))
		buffer, e := c.renderOutput(pointer)
		if e != nil {
			err = e
			break
		}
		pixel, e := c.readBuffer(buffer, image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
		C.wlr_buffer_unlock(buffer)
		err = e
		if e == nil {
			rgba := pixel.NRGBAAt(0, 0)
			rgb = [3]float64{float64(rgba.R) / 255, float64(rgba.G) / 255, float64(rgba.B) / 255}
		}
		break
	}
	c.finishColorPick(rgb, err)
	return true
}
