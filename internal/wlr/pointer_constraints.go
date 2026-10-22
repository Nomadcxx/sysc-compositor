package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server pixman-1
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#cgo LDFLAGS: -lm

#include <math.h>
#include <pixman.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_pointer_constraints_v1.h>
#include <wlr/types/wlr_relative_pointer_v1.h>
#include <wlr/types/wlr_seat.h>

static bool sysc_constraint_locked(struct wlr_pointer_constraint_v1 *constraint) {
	return constraint->type == WLR_POINTER_CONSTRAINT_V1_LOCKED;
}

// The constraint's region is wlroots' intersection of the requested region
// and the surface's input region, in surface coordinates.
static bool sysc_constraint_contains(struct wlr_pointer_constraint_v1 *constraint,
		double sx, double sy) {
	return pixman_region32_contains_point(&constraint->region,
		(int)floor(sx), (int)floor(sy), NULL);
}

static bool sysc_constraint_hint(struct wlr_pointer_constraint_v1 *constraint,
		double *sx, double *sy) {
	if (!constraint->current.cursor_hint.enabled) {
		return false;
	}
	*sx = constraint->current.cursor_hint.x;
	*sy = constraint->current.cursor_hint.y;
	return true;
}

static void sysc_absolute_to_layout(struct wlr_cursor *cursor, struct wlr_input_device *device,
		double x, double y, double *lx, double *ly) {
	wlr_cursor_absolute_to_layout_coords(cursor, device, x, y, lx, ly);
}
*/
import "C"

import "unsafe"

// Relative pointer and pointer constraints. Every user-seat pointer motion,
// relative or absolute (the nested backend and the test pointer report
// absolute positions), becomes a delta: it is sent as relative motion, then
// the active constraint decides how much of it moves the pointer. The active
// constraint is the one on the surface with pointer focus.

func (c *Compositor) createPointerConstraints() bool {
	if c.relativePointer = C.wlr_relative_pointer_manager_v1_create(c.display); c.relativePointer == nil {
		return false
	}
	if c.pointerConstraints = C.wlr_pointer_constraints_v1_create(c.display); c.pointerConstraints == nil {
		return false
	}
	c.keep(AddListener(unsafe.Pointer(&c.pointerConstraints.events.new_constraint), c.handleNewPointerConstraint))
	return true
}

func (c *Compositor) handleNewPointerConstraint(data unsafe.Pointer) {
	constraint := (*C.struct_wlr_pointer_constraint_v1)(data)
	var region, destroy *Listener
	region = AddListener(unsafe.Pointer(&constraint.events.set_region), func(unsafe.Pointer) {
		c.updatePointerConstraint()
	})
	destroy = AddListener(unsafe.Pointer(&constraint.events.destroy), func(unsafe.Pointer) {
		c.drop(region)
		c.drop(destroy)
		if c.activeConstraint == constraint {
			c.releasePointerConstraint(false)
		}
	})
	c.keep(region, destroy)
	c.updatePointerConstraint()
}

// updatePointerConstraint activates the constraint of the surface under the
// user seat's pointer, and deactivates any other.
func (c *Compositor) updatePointerConstraint() {
	var want *C.struct_wlr_pointer_constraint_v1
	if surface := c.seat.pointer_state.focused_surface; surface != nil {
		want = C.wlr_pointer_constraints_v1_constraint_for_surface(c.pointerConstraints, surface, c.seat)
	}
	if want == c.activeConstraint {
		return
	}
	if c.activeConstraint != nil {
		c.releasePointerConstraint(true)
	}
	if want != nil {
		c.activeConstraint = want
		C.wlr_pointer_constraint_v1_send_activated(want)
	}
}

// releasePointerConstraint ends the active constraint. A locked pointer
// moves to the client's hint, if it set one, as it is released.
func (c *Compositor) releasePointerConstraint(notify bool) {
	constraint := c.activeConstraint
	c.activeConstraint = nil
	var sx, sy C.double
	if bool(C.sysc_constraint_locked(constraint)) && bool(C.sysc_constraint_hint(constraint, &sx, &sy)) &&
		constraint.surface == c.seat.pointer_state.focused_surface {
		C.wlr_cursor_warp(c.cursor, nil, C.double(c.focusOriginX)+sx, C.double(c.focusOriginY)+sy)
	}
	if notify {
		C.wlr_pointer_constraint_v1_send_deactivated(constraint)
	}
}

// pointerMotion is the shared path for user-seat motion of dx, dy layout
// pixels (unaccelerated: udx, udy).
func (c *Compositor) pointerMotion(device *C.struct_wlr_input_device, timeMsec C.uint32_t,
	dx, dy, udx, udy float64) {
	C.wlr_relative_pointer_manager_v1_send_relative_motion(c.relativePointer, c.seat,
		C.uint64_t(timeMsec)*1000, C.double(dx), C.double(dy), C.double(udx), C.double(udy))
	dx, dy = c.constrainDelta(dx, dy)
	if dx != 0 || dy != 0 {
		C.wlr_cursor_move(c.cursor, device, C.double(dx), C.double(dy))
	}
	c.processCursorMotion(timeMsec)
	c.updatePointerConstraint()
}

// constrainDelta is how much of a motion the active constraint lets move the
// pointer: none when locked, and within the region, one axis at a time, when
// confined.
func (c *Compositor) constrainDelta(dx, dy float64) (float64, float64) {
	constraint := c.activeConstraint
	if constraint == nil {
		return dx, dy
	}
	if bool(C.sysc_constraint_locked(constraint)) {
		return 0, 0
	}
	sx, sy := float64(c.cursor.x)-float64(c.focusOriginX), float64(c.cursor.y)-float64(c.focusOriginY)
	inside := func(x, y float64) bool {
		return bool(C.sysc_constraint_contains(constraint, C.double(x), C.double(y)))
	}
	if inside(sx+dx, sy+dy) {
		return dx, dy
	}
	if inside(sx+dx, sy) {
		return dx, 0
	}
	if inside(sx, sy+dy) {
		return 0, dy
	}
	return 0, 0
}

// absoluteDelta turns an absolute device position into a layout delta from
// the pointer's position.
func (c *Compositor) absoluteDelta(device *C.struct_wlr_input_device, x, y C.double) (float64, float64) {
	var lx, ly C.double
	C.sysc_absolute_to_layout(c.cursor, device, x, y, &lx, &ly)
	return float64(lx - c.cursor.x), float64(ly - c.cursor.y)
}
