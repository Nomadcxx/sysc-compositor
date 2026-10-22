package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_session_lock_v1.h>

static struct wlr_scene_rect *sysc_lock_backdrop_create(struct wlr_scene_tree *parent) {
	const float black[4] = {0.0f, 0.0f, 0.0f, 1.0f};
	return wlr_scene_rect_create(parent, 0, 0, black);
}
*/
import "C"

import (
	"unsafe"
)

// The session lock (ext-session-lock-v1, sysc-1260). sysc-lock owns the lock
// UI and authentication; the compositor owns the seal:
//
//   - The session is sealed the moment a lock is accepted, before `locked` is
//     sent, and unsealed only by the locker's unlock_and_destroy. A locker
//     that dies without unlocking leaves the session sealed; a new locker may
//     then take over.
//   - While sealed, every output shows an opaque black backdrop above all
//     content, and the lock surface for that output above the backdrop once
//     it maps. User keyboard focus goes only to a lock surface, pointer
//     hit-testing reaches only lock surfaces, binds run only when marked
//     allow-when-locked, and injected input is refused.
//   - No control-socket or IPC request can unlock.

// sessionLock is the live locker's lock object. It is nil when no locker is
// connected, whether or not the session is sealed.
type sessionLock struct {
	lock      *C.struct_wlr_session_lock_v1
	listeners []*Listener
	surfaces  map[*C.struct_wlr_session_lock_surface_v1]*lockSurface
	unlocked  bool
}

type lockSurface struct {
	surface   *C.struct_wlr_session_lock_surface_v1
	tree      *C.struct_wlr_scene_tree
	output    unsafe.Pointer
	listeners []*Listener
}

// createSessionLock advertises ext-session-lock-v1 and makes the tree the
// backdrops and lock surfaces live in. The tree is made after every other
// tree under the scene root, so it draws above them all.
func (c *Compositor) createSessionLock() bool {
	manager := C.wlr_session_lock_manager_v1_create(c.display)
	if manager == nil || !c.createLockTree() {
		return false
	}
	c.keep(AddListener(unsafe.Pointer(&manager.events.new_lock), c.handleNewSessionLock))
	return true
}

func (c *Compositor) createLockTree() bool {
	c.lockTree = C.wlr_scene_tree_create(&c.scene.tree)
	if c.lockTree == nil {
		return false
	}
	C.wlr_scene_node_set_enabled(&c.lockTree.node, false)
	c.lockBackdrops = map[unsafe.Pointer]*C.struct_wlr_scene_rect{}
	return true
}

// handleNewSessionLock accepts a lock unless a live locker already holds one;
// that second lock is refused with `finished`.
func (c *Compositor) handleNewSessionLock(data unsafe.Pointer) {
	lock := (*C.struct_wlr_session_lock_v1)(data)
	if c.lock != nil {
		C.wlr_session_lock_v1_destroy(lock)
		return
	}
	l := &sessionLock{lock: lock, surfaces: map[*C.struct_wlr_session_lock_surface_v1]*lockSurface{}}
	c.lock = l
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		l.listeners = append(l.listeners, listener)
		c.keep(listener)
	}
	add(unsafe.Pointer(&lock.events.new_surface), c.handleNewLockSurface)
	add(unsafe.Pointer(&lock.events.unlock), func(unsafe.Pointer) {
		l.unlocked = true
		c.unsealSession()
	})
	add(unsafe.Pointer(&lock.events.destroy), func(unsafe.Pointer) {
		for _, listener := range l.listeners {
			c.drop(listener)
		}
		l.listeners = nil
		// The surfaces go with the lock. If the locker did not unlock, the
		// session stays sealed behind the black backdrops.
		for key, s := range l.surfaces {
			delete(l.surfaces, key)
			c.dropLockSurface(s)
		}
		if c.lock == l {
			c.lock = nil
		}
		c.updateKeyboardFocus()
	})

	// A session sealed by a locker that died is already behind the black
	// backdrops on every output, so the new locker is told at once.
	alreadySealed := c.sessionLocked
	c.sealSession()
	c.lockPendingOutputs = map[unsafe.Pointer]struct{}{}
	if !alreadySealed {
		for pointer := range c.sceneOutputs {
			if c.outputIsEnabled(pointer) {
				c.lockPendingOutputs[pointer] = struct{}{}
				// An idle output draws only when armed (sysc-1469); the
				// backdrop's damage does not reliably arm every output, so
				// each one is asked for the frame that shows the lock.
				C.wlr_output_schedule_frame((*C.struct_wlr_output)(pointer))
			}
		}
	}
	c.sendLockedIfPresented()
}

// sealSession closes the session to everything but lock surfaces.
func (c *Compositor) sealSession() {
	c.setSessionLocked(true)
	c.stopKeyboardRepeat()
	c.cancelTiledWindowMove()
	c.clearPointerGrab()
	c.dismissGrabbedPopups(c.seat)
	C.wlr_scene_node_raise_to_top(&c.lockTree.node)
	C.wlr_scene_node_set_enabled(&c.lockTree.node, true)
	for pointer := range c.sceneOutputs {
		c.arrangeLockOutput(pointer)
	}
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	c.updatePointerConstraint()
	c.updateKeyboardFocus()
	c.refocusPointer(nowMsec())
}

// unsealSession runs on a valid unlock_and_destroy, and only then.
func (c *Compositor) unsealSession() {
	c.lockPendingOutputs = nil
	for pointer, backdrop := range c.lockBackdrops {
		C.wlr_scene_node_destroy(&backdrop.node)
		delete(c.lockBackdrops, pointer)
	}
	C.wlr_scene_node_set_enabled(&c.lockTree.node, false)
	c.setSessionLocked(false)
	c.updateKeyboardFocus()
	c.refocusPointer(nowMsec())
}

func (c *Compositor) handleNewLockSurface(data unsafe.Pointer) {
	surface := (*C.struct_wlr_session_lock_surface_v1)(data)
	l := c.lock
	if l == nil {
		return
	}
	tree := C.wlr_scene_subsurface_tree_create(c.lockTree, surface.surface)
	if tree == nil {
		C.wl_resource_post_no_memory(surface.resource)
		return
	}
	s := &lockSurface{surface: surface, tree: tree, output: unsafe.Pointer(surface.output)}
	l.surfaces[surface] = s
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		s.listeners = append(s.listeners, listener)
		c.keep(listener)
	}
	add(unsafe.Pointer(&surface.surface.events._map), func(unsafe.Pointer) {
		c.updateKeyboardFocus()
		c.refocusPointer(nowMsec())
	})
	add(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
		delete(l.surfaces, surface)
		c.dropLockSurface(s)
		c.updateKeyboardFocus()
		c.refocusPointer(nowMsec())
	})
	c.arrangeLockOutput(s.output)
}

// dropLockSurface forgets a lock surface whose role object is gone. Its
// wl_surface can outlive the role, and its scene tree with it, so the tree is
// destroyed here: a later lock must not show an old locker's buffers.
func (c *Compositor) dropLockSurface(s *lockSurface) {
	for _, listener := range s.listeners {
		c.drop(listener)
	}
	s.listeners = nil
	if s.tree != nil {
		C.wlr_scene_node_destroy(&s.tree.node)
		s.tree = nil
	}
}

// arrangeLockOutput sizes the black backdrop and the lock surfaces of one
// output to its layout box. It runs on lock and whenever an output appears or
// changes geometry while the session is sealed.
func (c *Compositor) arrangeLockOutput(pointer unsafe.Pointer) {
	if !c.sessionLocked {
		return
	}
	if _, live := c.sceneOutputs[pointer]; !live {
		return
	}
	var box C.struct_wlr_box
	C.wlr_output_layout_get_box(c.layout, (*C.struct_wlr_output)(pointer), &box)
	backdrop := c.lockBackdrops[pointer]
	if backdrop == nil {
		if backdrop = C.sysc_lock_backdrop_create(c.lockTree); backdrop == nil {
			return
		}
		c.lockBackdrops[pointer] = backdrop
		C.wlr_scene_node_lower_to_bottom(&backdrop.node)
	}
	C.wlr_scene_rect_set_size(backdrop, box.width, box.height)
	C.wlr_scene_node_set_position(&backdrop.node, box.x, box.y)
	if c.lock == nil {
		return
	}
	for _, s := range c.lock.surfaces {
		if s.output != pointer {
			continue
		}
		C.wlr_scene_node_set_position(&s.tree.node, box.x, box.y)
		C.wlr_session_lock_surface_v1_configure(s.surface, C.uint32_t(box.width), C.uint32_t(box.height))
	}
}

// lockOutputGone forgets an output that was destroyed or turned off, so
// `locked` does not wait for a frame it will never draw.
func (c *Compositor) lockOutputGone(pointer unsafe.Pointer) {
	if backdrop := c.lockBackdrops[pointer]; backdrop != nil && c.sceneOutputs[pointer] == nil {
		C.wlr_scene_node_destroy(&backdrop.node)
		delete(c.lockBackdrops, pointer)
	}
	if _, pending := c.lockPendingOutputs[pointer]; pending {
		delete(c.lockPendingOutputs, pointer)
		c.sendLockedIfPresented()
	}
}

// lockFrameDone runs after an output repaints. Its first repaint after a
// lock was accepted has drawn the backdrop.
func (c *Compositor) lockFrameDone(pointer unsafe.Pointer) {
	if _, pending := c.lockPendingOutputs[pointer]; pending {
		delete(c.lockPendingOutputs, pointer)
		c.sendLockedIfPresented()
	}
}

// sendLockedIfPresented sends `locked` once every enabled output has drawn a
// frame with the backdrop, so no unlocked content is on screen when the
// locker hears the session is locked.
func (c *Compositor) sendLockedIfPresented() {
	l := c.lock
	if l == nil || l.unlocked || c.lockPendingOutputs == nil || len(c.lockPendingOutputs) > 0 {
		return
	}
	c.lockPendingOutputs = nil
	C.wlr_session_lock_v1_send_locked(l.lock)
}

// lockKeyboardSurface is where user keyboard focus goes while sealed: the
// mapped lock surface on the active output, else any mapped one, else none.
func (c *Compositor) lockKeyboardSurface() *C.struct_wlr_surface {
	if c.lock == nil {
		return nil
	}
	active := c.FocusedOutputName()
	var fallback *C.struct_wlr_surface
	for _, s := range c.lock.surfaces {
		if !bool(s.surface.surface.mapped) {
			continue
		}
		if C.GoString((*C.struct_wlr_output)(s.output).name) == active {
			return s.surface.surface
		}
		if fallback == nil {
			fallback = s.surface.surface
		}
	}
	return fallback
}
