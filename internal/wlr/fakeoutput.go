package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/backend/headless.h>
#include <wlr/render/color.h>
#include <wlr/types/wlr_gamma_control_v1.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_tearing_control_v1.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <scenefx/types/wlr_scene.h>

// Evaluates a transform at mid-grey; a NULL transform is the default ramp.
static bool sysc_test_gamma_eval(struct wlr_color_transform *transform, float rgb[3]) {
	if (transform == NULL) return true;
	const float input[3] = {0.5f, 0.5f, 0.5f};
	wlr_color_transform_eval(transform, rgb, input);
	return false;
}

static bool sysc_test_gamma_commit(void *data, float rgb[3], bool *reset) {
	struct wlr_output_event_commit *event = data;
	if (!(event->state->committed & WLR_OUTPUT_STATE_COLOR_TRANSFORM)) return false;
	*reset = sysc_test_gamma_eval(event->state->color_transform, rgb);
	return true;
}

static bool sysc_test_gamma_current(struct wlr_output *output, float rgb[3]) {
	return sysc_test_gamma_eval(output->WLR_PRIVATE.color_transform, rgb);
}

static bool sysc_test_commit_tore(void *data) {
	return ((struct wlr_output_event_commit *)data)->state->tearing_page_flip;
}

static struct wl_signal *sysc_test_gamma_requests(void *pointer) {
	struct wlr_scene *scene = pointer;
	return scene->gamma_control_manager_v1 == NULL ? NULL :
		&scene->gamma_control_manager_v1->events.set_gamma;
}

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

func gammaSizeForTest(output unsafe.Pointer) int {
	return int(C.wlr_output_get_gamma_size((*C.struct_wlr_output)(output)))
}

// gammaCurrentForTest reports the ramp the output holds now, as the commit
// observer does; wlroots drops a commit field that repeats it.
func gammaCurrentForTest(output unsafe.Pointer) ([3]float64, bool) {
	var rgb [3]C.float
	reset := C.sysc_test_gamma_current((*C.struct_wlr_output)(output), &rgb[0])
	return [3]float64{float64(rgb[0]), float64(rgb[1]), float64(rgb[2])}, bool(reset)
}

// tearingHintForTest is the presentation hint recorded for a view's surface.
func tearingHintForTest(c *Compositor, view *toplevelView) uint32 {
	return uint32(C.wlr_tearing_control_manager_v1_surface_hint_from_surface(
		c.tearingControl, view.toplevel.base.surface))
}

// observeTearingCommitsForTest reports whether each output commit tore.
func observeTearingCommitsForTest(output unsafe.Pointer, notify func(tore bool)) *Listener {
	out := (*C.struct_wlr_output)(output)
	return AddListener(unsafe.Pointer(&out.events.commit), func(data unsafe.Pointer) {
		notify(bool(C.sysc_test_commit_tore(data)))
	})
}

func observeGammaCommitsForTest(output unsafe.Pointer, notify func([3]float64, bool)) *Listener {
	out := (*C.struct_wlr_output)(output)
	return AddListener(unsafe.Pointer(&out.events.commit), func(data unsafe.Pointer) {
		var rgb [3]C.float
		var reset C.bool
		if bool(C.sysc_test_gamma_commit(data, &rgb[0], &reset)) {
			notify([3]float64{float64(rgb[0]), float64(rgb[1]), float64(rgb[2])}, bool(reset))
		}
	})
}

func observeGammaRequestsForTest(c *Compositor, notify func()) *Listener {
	signal := C.sysc_test_gamma_requests(unsafe.Pointer(c.scene))
	if signal == nil {
		return nil
	}
	return AddListener(unsafe.Pointer(signal), func(unsafe.Pointer) { notify() })
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
