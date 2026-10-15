package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <string.h>

#include <wlr/backend.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_primary_selection_v1.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_shm.h>
#include <wlr/types/wlr_subcompositor.h>
#include <wlr/types/wlr_viewporter.h>
#include <wlr/types/wlr_xdg_shell.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);

// A sysc_listener carries a Go token next to the wayland listener. The
// wl_notify_func_t contract hands the callback the struct wl_listener* itself,
// and `listener` is the first member, so the two pointers coincide and the
// token is recoverable without wl_container_of.
struct sysc_listener {
	struct wl_listener listener;
	uintptr_t handle;
};

static void sysc_listener_bridge(struct wl_listener *l, void *data) {
	sysc_listener_fire(((struct sysc_listener *)l)->handle, data);
}

static struct sysc_listener *sysc_listener_add(struct wl_signal *signal,
		uintptr_t handle) {
	struct sysc_listener *sl = calloc(1, sizeof(*sl));
	if (sl == NULL) {
		abort();
	}
	sl->listener.notify = sysc_listener_bridge;
	sl->handle = handle;
	wl_signal_add(signal, &sl->listener);
	return sl;
}

static void sysc_listener_remove(struct sysc_listener *sl) {
	wl_list_remove(&sl->listener.link);
	free(sl);
}
*/
import "C"

import "unsafe"

// Listener is a wlroots listener owned by Go. Token is the Go callback token;
// it is unlinked and released exactly once by Destroy.
type Listener struct {
	sl    *C.struct_sysc_listener
	token uintptr
}

// Signal is the wlr_signal* a Listener attaches to. Opaque on the Go side so
// callers cannot reach into wlroots internals by accident.
type Signal = unsafe.Pointer

// AddListener registers fn on signal. The token returned must reach Destroy, or
// a destroyed signal will call into freed Go memory.
func AddListener(signal Signal, fn func(unsafe.Pointer)) *Listener {
	token := Register(fn)
	return &Listener{sl: C.sysc_listener_add((*C.struct_wl_signal)(signal), C.uintptr_t(token)), token: token}
}

// Native returns the listener's own pointer. The wayland notify contract hands
// the callback this pointer, so it is the only stable key available when a
// signal carries no payload of its own — wlroots' output frame signal passes
// NULL, and a per-listener map is then the only way back to its subject.
func (l *Listener) Native() Signal { return unsafe.Pointer(l.sl) }

// Destroy unlinks the listener and releases its token. Safe to call once only.
func (l *Listener) Destroy() {
	if l.sl == nil {
		return
	}
	C.sysc_listener_remove(l.sl)
	Unregister(l.token)
	l.sl = nil
}
