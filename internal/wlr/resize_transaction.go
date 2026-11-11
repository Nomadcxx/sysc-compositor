package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_xdg_shell.h>
static uint32_t sysc_resize_state(struct wlr_xdg_toplevel *toplevel) {
	struct wlr_xdg_toplevel_configure *s = &toplevel->scheduled;
	return s->tiled | s->constrained << 4 | s->maximized << 8 | s->fullscreen << 9 |
			s->resizing << 10 | s->activated << 11 | s->suspended << 12;
}
static bool sysc_resize_serial_reached(uint32_t current, uint32_t target) {
	return (int32_t)(current - target) >= 0;
}
static bool sysc_resize_unmapping(struct wlr_surface *surface) {
	return (surface->pending.committed & WLR_SURFACE_STATE_BUFFER) &&
			surface->pending.buffer == NULL;
}
*/
import "C"

import (
	"image"
	"time"
	"unsafe"
)

type resizeRequest struct {
	serial        uint32
	width, height int
	state         uint32
	sent          bool
	queued        *image.Point
}
type resizeTransactionMember struct {
	serial uint32
	ready  bool
	locked *uint32
}
type resizeTransaction struct {
	members       map[*toplevelView]*resizeTransactionMember
	timer         *time.Timer
	releaseQueued bool
}

// The pending state belongs to wlroots until the group is ready. Native cached
// states preserve the client's acquire wait, buffer lifetime and subsurfaces.
func (c *Compositor) watchResizeRequests(view *toplevelView) {
	add := func(signal unsafe.Pointer, callback func(unsafe.Pointer)) {
		listener := AddListener(signal, callback)
		view.listeners = append(view.listeners, listener)
		c.keep(listener)
	}
	add(unsafe.Pointer(&view.toplevel.base.events.configure), func(data unsafe.Pointer) {
		configure := (*C.struct_wlr_xdg_surface_configure)(data)
		if uint32(configure.serial) == view.sizeRequest.serial {
			view.sizeRequest.sent = true
		}
	})
	add(unsafe.Pointer(&view.toplevel.base.surface.events.client_commit), func(unsafe.Pointer) {
		if !view.mapped {
			return
		}
		surface := view.toplevel.base.surface
		if bool(C.sysc_resize_unmapping(surface)) {
			c.forgetResizeRequest(view)
			return
		}
		transaction := c.resizeTransaction
		if transaction == nil {
			return
		}
		member := transaction.members[view]
		if member == nil || member.ready || !bool(C.sysc_resize_serial_reached(view.toplevel.base.pending.configure_serial, C.uint32_t(member.serial))) {
			return
		}
		seq := uint32(C.wlr_surface_lock_pending(surface))
		member.locked = &seq
		member.ready = true
		c.maybeFinishResizeTransaction()
	})
	add(unsafe.Pointer(&view.toplevel.base.surface.events.commit), func(unsafe.Pointer) {
		request := &view.sizeRequest
		if request.serial == 0 || !bool(C.sysc_resize_serial_reached(view.toplevel.base.current.configure_serial, C.uint32_t(request.serial))) {
			return
		}
		request.serial = 0
		request.sent = false
		if request.queued != nil {
			size := *request.queued
			request.queued = nil
			_ = c.Post(func() {
				if view.mapped {
					c.requestWindowSize(view, size.X, size.Y)
				}
			})
		}
	})
}

func (c *Compositor) beginResizeBatch() func() {
	if c.debugConfig.DisableTransactions {
		c.finishResizeTransactions()
	}
	c.collectingResize++
	return func() {
		c.collectingResize--
		if c.collectingResize != 0 {
			return
		}
		if transaction := c.resizeTransaction; transaction != nil && len(transaction.members) < 2 {
			c.finishResizeTransactions()
		} else {
			c.maybeFinishResizeTransaction()
		}
	}
}
func (c *Compositor) requestWindowSize(view *toplevelView, width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	request := &view.sizeRequest
	state := uint32(C.sysc_resize_state(view.toplevel))
	if width == request.width && height == request.height && state == request.state {
		return
	}
	if request.sent && request.serial != 0 && !c.debugConfig.DisableResizeThrottling && state == request.state {
		next := image.Pt(width, height)
		request.queued = &next
		return
	}
	serial := uint32(C.wlr_xdg_toplevel_set_size(view.toplevel, C.int(width), C.int(height)))
	*request = resizeRequest{serial: serial, width: width, height: height, state: state}
	if c.debugConfig.DisableTransactions || view.floating || !view.openAnimationStarted || c.collectingResize == 0 {
		return
	}
	transaction := c.resizeTransaction
	if transaction == nil {
		transaction = &resizeTransaction{members: map[*toplevelView]*resizeTransactionMember{}}
		c.resizeTransaction = transaction
		transaction.timer = time.AfterFunc(300*time.Millisecond, func() {
			_ = c.Post(func() {
				if c.resizeTransaction == transaction {
					c.finishResizeTransactions()
				}
			})
		})
	}
	// A newer configure joins the same deadline; it must not leave an older
	// cached commit locked behind a member the group can no longer observe.
	if previous := transaction.members[view]; previous != nil && previous.locked != nil {
		C.wlr_surface_unlock_cached(view.toplevel.base.surface, C.uint32_t(*previous.locked))
	}
	transaction.members[view] = &resizeTransactionMember{serial: serial}
}
func (c *Compositor) maybeFinishResizeTransaction() {
	transaction := c.resizeTransaction
	if transaction == nil || c.collectingResize != 0 || transaction.releaseQueued {
		return
	}
	for _, member := range transaction.members {
		if !member.ready {
			return
		}
	}
	transaction.releaseQueued = true
	// Leave wl_surface.commit's signal stack: its pending state has not been
	// moved into the native cache until the client_commit signal returns.
	_ = c.Post(func() {
		if c.resizeTransaction == transaction {
			c.finishResizeTransactions()
		}
	})
}
func (c *Compositor) finishResizeTransactions() {
	transaction := c.resizeTransaction
	if transaction == nil {
		return
	}
	c.resizeTransaction = nil
	if transaction.timer != nil {
		transaction.timer.Stop()
	}
	for view, member := range transaction.members {
		if member.locked != nil {
			seq := *member.locked
			member.locked = nil
			C.wlr_surface_unlock_cached(view.toplevel.base.surface, C.uint32_t(seq))
		}
	}
}
func (c *Compositor) forgetResizeRequest(view *toplevelView) {
	view.sizeRequest = resizeRequest{}
	transaction := c.resizeTransaction
	if transaction == nil {
		return
	}
	member := transaction.members[view]
	delete(transaction.members, view)
	if member != nil && member.locked != nil {
		seq := *member.locked
		member.locked = nil
		C.wlr_surface_unlock_cached(view.toplevel.base.surface, C.uint32_t(seq))
	}
	c.maybeFinishResizeTransaction()
}
