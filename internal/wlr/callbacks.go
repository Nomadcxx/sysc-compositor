package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#include <stdint.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);
*/
import "C"

import (
	"runtime/cgo"
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

// Register returns a token that the C trampoline can carry inside a
// wl_listener. The caller keeps the token and passes it to Unregister when the
// listener is removed, so a destroyed listener can never fire into freed Go
// memory and the handle cannot leak.
func Register(fn func(unsafe.Pointer)) uintptr {
	return uintptr(cgo.NewHandle(fn))
}

// Unregister releases a token from Register. It must be called exactly once,
// after wl_list_remove has unlinked the listener.
func Unregister(token uintptr) {
	if token != 0 {
		cgo.Handle(token).Delete()
	}
}
