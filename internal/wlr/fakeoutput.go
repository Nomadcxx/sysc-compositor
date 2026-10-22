package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend/headless.h>
#include <wlr/types/wlr_xdg_shell.h>

static void sysc_fake_fullscreen_request(struct wlr_xdg_toplevel *toplevel,
		struct wlr_output *output) {
	toplevel->requested.fullscreen = true;
	toplevel->requested.fullscreen_output = output;
}

static void sysc_fake_clear_fullscreen_output(struct wlr_xdg_toplevel *toplevel) {
	toplevel->requested.fullscreen_output = NULL;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// headlessTestBackend adds deterministic outputs to compositor tests that
// need more than the nested Wayland backend's single output.
type headlessTestBackend struct {
	c         *Compositor
	backend   *C.struct_wlr_backend
	newOutput *Listener
}

func addHeadlessTestOutput(c *Compositor, width, height uint32) (*headlessTestBackend, error) {
	backend := C.wlr_headless_backend_create(c.loop)
	if backend == nil {
		return nil, errors.New("wlr_headless_backend_create failed")
	}
	listener := AddListener(unsafe.Pointer(&backend.events.new_output), c.handleNewOutput)
	c.keep(listener)
	destroy := func() {
		c.drop(listener)
		C.wlr_backend_destroy(backend)
	}
	if C.wlr_headless_add_output(backend, C.uint(width), C.uint(height)) == nil {
		destroy()
		return nil, errors.New("wlr_headless_add_output failed")
	}
	if !bool(C.wlr_backend_start(backend)) {
		destroy()
		return nil, errors.New("wlr_backend_start failed")
	}
	return &headlessTestBackend{c: c, backend: backend, newOutput: listener}, nil
}

func requestFullscreenOnOutputForTest(c *Compositor, view *toplevelView, name string) bool {
	if c == nil || view == nil || view.toplevel == nil {
		return false
	}
	for output := range c.sceneOutputs {
		if C.GoString((*C.struct_wlr_output)(output).name) == name {
			C.sysc_fake_fullscreen_request(view.toplevel, (*C.struct_wlr_output)(output))
			return true
		}
	}
	return false
}

func requestFullscreenWithoutOutputForTest(view *toplevelView) bool {
	if view == nil || view.toplevel == nil {
		return false
	}
	C.sysc_fake_fullscreen_request(view.toplevel, nil)
	return true
}

func clearFullscreenOutputForTest(view *toplevelView) {
	if view != nil && view.toplevel != nil {
		C.sysc_fake_clear_fullscreen_output(view.toplevel)
	}
}

func (b *headlessTestBackend) destroy() {
	if b == nil || b.backend == nil {
		return
	}
	b.c.drop(b.newOutput)
	C.wlr_backend_destroy(b.backend)
	b.backend = nil
}
