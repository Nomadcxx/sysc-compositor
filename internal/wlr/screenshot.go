package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <drm_fourcc.h>
#include <stdlib.h>
#include <string.h>
#include <wlr/render/allocator.h>
#include <wlr/render/drm_format_set.h>
#include <wlr/render/swapchain.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/render/wlr_texture.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_seat.h>
#include <scenefx/types/wlr_scene.h>

// Renders an output's scene into a swapchain of its own, so the output's
// buffers and damage tracking are untouched, and reads it back as
// ARGB8888 (B, G, R, A bytes) with a stride of width * 4. A fresh buffer has
// no age, so the scene draws all of it.
static bool sysc_screenshot_output(void *scene_output, struct wlr_output *output,
		struct wlr_allocator *alloc, struct wlr_renderer *renderer, void *data) {
	struct wlr_drm_format_set formats = {0};
	if (!wlr_drm_format_set_add(&formats, DRM_FORMAT_XRGB8888, DRM_FORMAT_MOD_INVALID)) {
		return false;
	}
	struct wlr_swapchain *swapchain = wlr_swapchain_create(alloc, output->width, output->height,
		wlr_drm_format_set_get(&formats, DRM_FORMAT_XRGB8888));
	wlr_drm_format_set_finish(&formats);
	if (swapchain == NULL) {
		return false;
	}
	// build_state renders whatever the buffer's age leaves undrawn; a fresh
	// buffer has none, so all of it. (Only wlr_scene_output_commit skips an
	// output that needs no frame.)
	struct wlr_scene_output_state_options options = { .swapchain = swapchain };
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	bool ok = false;
	if (wlr_scene_output_build_state(scene_output, &state, &options) &&
			(state.committed & WLR_OUTPUT_STATE_BUFFER) && state.buffer != NULL) {
		struct wlr_texture *texture = wlr_texture_from_buffer(renderer, state.buffer);
		if (texture != NULL) {
			struct wlr_texture_read_pixels_options read = {
				.data = data, .format = DRM_FORMAT_ARGB8888, .stride = output->width * 4,
			};
			ok = wlr_texture_read_pixels(texture, &read);
			wlr_texture_destroy(texture);
		}
	}
	wlr_output_state_finish(&state);
	wlr_swapchain_destroy(swapchain);
	return ok;
}

extern void sysc_png_source_send(uintptr_t handle, int fd);
extern void sysc_png_source_destroyed(uintptr_t handle);

// A selection the compositor owns: it offers image/png and hands the request
// to Go, which writes the bytes and closes the descriptor.
struct sysc_png_source {
	struct wlr_data_source base;
	uintptr_t handle;
};

static void sysc_png_send(struct wlr_data_source *source, const char *mime_type, int32_t fd) {
	sysc_png_source_send(((struct sysc_png_source *)source)->handle, fd);
}

static void sysc_png_destroy(struct wlr_data_source *source) {
	sysc_png_source_destroyed(((struct sysc_png_source *)source)->handle);
	free(source);
}

static const struct wlr_data_source_impl sysc_png_source_impl = {
	.send = sysc_png_send,
	.destroy = sysc_png_destroy,
};

static bool sysc_set_png_selection(struct wlr_seat *seat, struct wl_display *display,
		uintptr_t handle) {
	struct sysc_png_source *source = calloc(1, sizeof(*source));
	if (source == NULL) {
		return false;
	}
	wlr_data_source_init(&source->base, &sysc_png_source_impl);
	source->handle = handle;
	char **mime = wl_array_add(&source->base.mime_types, sizeof(*mime));
	if (mime == NULL || (*mime = strdup("image/png")) == NULL) {
		wlr_data_source_destroy(&source->base);
		return false;
	}
	wlr_seat_set_selection(seat, &source->base, wl_display_next_serial(display));
	return true;
}
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime/cgo"
	"unsafe"
)

// captureOutput renders one output as it would next be shown and returns it
// as an image. It runs on the compositor thread.
func (c *Compositor) captureOutput(pointer unsafe.Pointer) (*image.NRGBA, error) {
	so, ok := c.sceneOutputs[pointer]
	if !ok {
		return nil, errors.New("wlr: no such output")
	}
	output := (*C.struct_wlr_output)(pointer)
	width, height := int(output.width), int(output.height)
	if width <= 0 || height <= 0 {
		return nil, errors.New("wlr: the output has no size")
	}
	pixels := C.malloc(C.size_t(width * height * 4))
	if pixels == nil {
		return nil, errors.New("wlr: out of memory for a screenshot")
	}
	defer C.free(pixels)
	if !bool(C.sysc_screenshot_output(unsafe.Pointer(so), output, c.alloc, c.renderer, pixels)) {
		return nil, fmt.Errorf("wlr: rendering output %q for a screenshot failed", C.GoString(output.name))
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(pixels), width*height*4)
	for i := 0; i < width*height; i++ {
		// ARGB8888 is B, G, R, A in memory. XRGB content has no alpha.
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = src[i*4+2], src[i*4+1], src[i*4], 0xff
	}
	return img, nil
}

// encodePNG encodes off the compositor thread's critical path; callers run it
// on a goroutine.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pngSource is the data behind a compositor-owned clipboard selection.
type pngSource struct{ data []byte }

// setPNGSelection offers data as the user seat's clipboard, as image/png.
// It runs on the compositor thread.
func (c *Compositor) setPNGSelection(data []byte) error {
	handle := cgo.NewHandle(&pngSource{data: data})
	if !bool(C.sysc_set_png_selection(c.seat, c.display, C.uintptr_t(handle))) {
		handle.Delete()
		return errors.New("wlr: setting the screenshot selection failed")
	}
	return nil
}

// sendPNG writes a selection's bytes to a client's descriptor and closes it,
// on a goroutine so a slow reader never stalls the compositor.
func sendPNG(handle uintptr, fd int) {
	data := cgo.Handle(handle).Value().(*pngSource).data
	file := os.NewFile(uintptr(fd), "selection")
	go func() {
		defer file.Close()
		_, _ = file.Write(data)
	}()
}

// outputPixelSize is an output's buffer size in pixels.
func outputPixelSize(pointer unsafe.Pointer) (int, int) {
	output := (*C.struct_wlr_output)(pointer)
	return int(output.width), int(output.height)
}
