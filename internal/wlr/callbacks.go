package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#include <stdint.h>
#include <wayland-server-core.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);
extern void sysc_post_fire(uintptr_t token);
extern void sysc_background_effect_get(uintptr_t handle, struct wl_resource *manager,
		uint32_t id, struct wl_resource *surface);
extern void sysc_background_effect_set_region(uintptr_t handle, struct wl_resource *resource,
		struct wl_resource *region);
extern void sysc_background_effect_resource_destroyed(uintptr_t handle);
*/
import "C"

import (
	"runtime/cgo"
	"sync/atomic"
	"unsafe"
)

// sysc_listener_fire is invoked from the C trampoline for every wlroots
// listener. This file may only declare C, never define it: a Go file carrying
// //export has its preamble copied into two translation units.
//
//export sysc_listener_fire
func sysc_listener_fire(handle C.uintptr_t, data unsafe.Pointer) {
	cgo.Handle(uintptr(handle)).Value().(func(unsafe.Pointer))(data)
}

// sysc_post_fire is invoked when the post eventfd is readable.
//
//export sysc_post_fire
func sysc_post_fire(token C.uintptr_t) {
	cgo.Handle(uintptr(token)).Value().(func(unsafe.Pointer))(nil)
}

//export sysc_background_effect_get
func sysc_background_effect_get(handle C.uintptr_t, manager *C.struct_wl_resource,
	id C.uint32_t, surface *C.struct_wl_resource) {
	cgo.Handle(uintptr(handle)).Value().(*Compositor).getBackgroundEffect(manager, uint32(id), surface)
}

//export sysc_background_effect_set_region
func sysc_background_effect_set_region(handle C.uintptr_t, resource,
	region *C.struct_wl_resource) {
	cgo.Handle(uintptr(handle)).Value().(*backgroundEffect).setRegion(resource, region)
}

//export sysc_background_effect_resource_destroyed
func sysc_background_effect_resource_destroyed(handle C.uintptr_t) {
	cgo.Handle(uintptr(handle)).Value().(*backgroundEffect).resourceDestroyed(uintptr(handle))
}

// liveHandles counts tokens Register has handed out and Unregister has not yet
// released. It is the M0 exit gate for handle ownership: a token that outlives
// its wl_signal is a Go handle leaked once per process run.
var liveHandles atomic.Int64

// LiveHandles reports the number of outstanding listener tokens.
func LiveHandles() int64 { return liveHandles.Load() }

// Register returns a token that the C trampoline can carry inside a
// wl_listener. The caller keeps the token and passes it to Unregister when the
// listener is removed, so a destroyed listener can never fire into freed Go
// memory and the handle cannot leak.
func Register(fn func(unsafe.Pointer)) uintptr {
	liveHandles.Add(1)
	return uintptr(cgo.NewHandle(fn))
}

// Unregister releases a token from Register. It must be called exactly once,
// after wl_list_remove has unlinked the listener.
func Unregister(token uintptr) {
	if token != 0 {
		cgo.Handle(token).Delete()
		liveHandles.Add(-1)
	}
}
