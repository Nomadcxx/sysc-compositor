package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_seat.h>
*/
import "C"

import "errors"

type windowPicker struct{ done func(uint64, error) }

// PickWindow owns one modal pointer grab and returns a mapped window ID, or
// zero for empty space/cancellation. Call it and its cancel function on the
// compositor thread, as with PickColor.
func (c *Compositor) PickWindow(done func(uint64, error)) (func(), error) {
	if done == nil {
		return nil, errors.New("wlr: a window pick needs a result handler")
	}
	if c.sessionLocked || c.allOutputsOff() || c.shotUI != nil {
		return nil, errors.New("wlr: window picker unavailable")
	}
	c.finishWindowPick(0, nil)
	c.finishColorPick([3]float64{}, errors.New("no color picked"))
	c.endTouchPoints(nowMsec())
	pick := &windowPicker{done: done}
	c.pickWindow = pick
	c.cancelTiledWindowMove()
	c.clearPointerGrab()
	if c.activeConstraint != nil {
		c.releasePointerConstraint(true)
	}
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	c.showCursorImage("crosshair")
	return func() {
		if c.pickWindow == pick {
			c.finishWindowPick(0, nil)
		}
	}, nil
}

func (c *Compositor) finishWindowPick(id uint64, err error) {
	pick := c.pickWindow
	if pick == nil {
		return
	}
	c.pickWindow = nil
	c.restoreCursorImage()
	c.refocusPointer(nowMsec())
	pick.done(id, err)
}

func (c *Compositor) pickWindowButton(button uint32, pressed bool) bool {
	if c.pickWindow == nil {
		return false
	}
	if pressed {
		c.consumedPointerButtons[button] = true
		var id uint64
		if view := c.viewAtCursor(); view != nil && view.mapped {
			id = view.id
		}
		c.finishWindowPick(id, nil)
	}
	return true
}
