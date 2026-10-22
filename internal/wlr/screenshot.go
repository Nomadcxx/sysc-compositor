package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <drm_fourcc.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <wlr/render/allocator.h>
#include <wlr/render/drm_format_set.h>
#include <wlr/render/swapchain.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/render/wlr_texture.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_output.h>
#include <wlr/render/pass.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/util/transform.h>
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

// strftime on the local time now, as Niri's make_screenshot_path does.
static size_t sysc_strftime_now(char *buf, size_t len, const char *format) {
	time_t now = time(NULL);
	struct tm tm;
	if (now == (time_t)-1 || localtime_r(&now, &tm) == NULL) {
		return 0;
	}
	return strftime(buf, len, format, &tm);
}


struct sysc_window_shot {
	struct wlr_render_pass *pass;
	int ox, oy;
	double scale;
};

static void sysc_window_shot_surface(struct wlr_surface *surface, int sx, int sy, void *data) {
	struct sysc_window_shot *shot = data;
	struct wlr_texture *texture = wlr_surface_get_texture(surface);
	if (texture == NULL) {
		return;
	}
	struct wlr_render_texture_options options = {
		.texture = texture,
		.dst_box = {
			.x = (int)((sx - shot->ox) * shot->scale),
			.y = (int)((sy - shot->oy) * shot->scale),
			.width = (int)(surface->current.width * shot->scale),
			.height = (int)(surface->current.height * shot->scale),
		},
		.transform = wlr_output_transform_invert(surface->current.transform),
		.blend_mode = WLR_RENDER_BLEND_MODE_PREMULTIPLIED,
	};
	wlr_surface_get_buffer_source_box(surface, &options.src_box);
	wlr_render_pass_add_texture(shot->pass, &options);
}

// Draws one window, its surface and subsurfaces but no popups and nothing
// that overlaps it, into a transparent buffer of its own and reads it back
// as premultiplied ARGB8888, as Niri screenshots a window alone. width and
// height are the window geometry times scale.
static bool sysc_screenshot_window(struct wlr_xdg_surface *xdg, double scale, int width, int height,
		struct wlr_allocator *alloc, struct wlr_renderer *renderer, void *data) {
	struct wlr_drm_format_set formats = {0};
	if (!wlr_drm_format_set_add(&formats, DRM_FORMAT_ARGB8888, DRM_FORMAT_MOD_INVALID)) {
		return false;
	}
	struct wlr_buffer *buffer = wlr_allocator_create_buffer(alloc, width, height,
		wlr_drm_format_set_get(&formats, DRM_FORMAT_ARGB8888));
	wlr_drm_format_set_finish(&formats);
	if (buffer == NULL) {
		return false;
	}
	bool ok = false;
	struct wlr_render_pass *pass = wlr_renderer_begin_buffer_pass(renderer, buffer, NULL);
	if (pass != NULL) {
		wlr_render_pass_add_rect(pass, &(struct wlr_render_rect_options){
			.box = { .width = width, .height = height },
			.blend_mode = WLR_RENDER_BLEND_MODE_NONE,
		});
		struct sysc_window_shot shot = {
			.pass = pass, .ox = xdg->geometry.x, .oy = xdg->geometry.y, .scale = scale,
		};
		wlr_surface_for_each_surface(xdg->surface, sysc_window_shot_surface, &shot);
		if (wlr_render_pass_submit(pass)) {
			struct wlr_texture *texture = wlr_texture_from_buffer(renderer, buffer);
			if (texture != NULL) {
				struct wlr_texture_read_pixels_options read = {
					.data = data, .format = DRM_FORMAT_ARGB8888, .stride = width * 4,
				};
				ok = wlr_texture_read_pixels(texture, &read);
				wlr_texture_destroy(texture);
			}
		}
	}
	wlr_buffer_drop(buffer);
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
	"path/filepath"
	"runtime/cgo"
	"strings"
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

// expandScreenshotPath is Niri's make_screenshot_path: the pattern through
// strftime on the local time, then a leading ~ expanded.
func expandScreenshotPath(pattern string) (string, error) {
	format := C.CString(pattern)
	defer C.free(unsafe.Pointer(format))
	var buf [2048]C.char
	n := C.sysc_strftime_now(&buf[0], C.size_t(len(buf)), format)
	if n == 0 {
		return "", errors.New("formatting the screenshot path failed")
	}
	path := C.GoStringN(&buf[0], C.int(n))
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~ in the screenshot path: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path, nil
}

// ScreenshotScreen captures the focused output, copies it to the clipboard
// and, with writeToDisk, writes it to path, or to the screenshot-path
// pattern when path is empty. It runs on the compositor thread; the PNG is
// encoded and written on a goroutine. The pointer is not drawn: it is not
// part of the scene (recorded in the M11 plan).
func (c *Compositor) ScreenshotScreen(writeToDisk bool, path string) error {
	var target unsafe.Pointer
	active := c.FocusedOutputName()
	for pointer := range c.sceneOutputs {
		if target == nil || C.GoString((*C.struct_wlr_output)(pointer).name) == active {
			target = pointer
		}
	}
	if target == nil {
		return errors.New("wlr: no output to screenshot")
	}
	img, err := c.captureOutput(target)
	if err != nil {
		return err
	}
	c.saveScreenshot(img, writeToDisk, path)
	return nil
}

// saveScreenshot is Niri's save_screenshot: encode on a goroutine, offer the
// PNG as the clipboard and write it to disk from there, then report the
// path written, or nil, through the screenshot hook on the compositor
// thread. An explicit path is written as given; a configured pattern is
// expanded and its directory created.
func (c *Compositor) saveScreenshot(img image.Image, writeToDisk bool, explicit string) {
	pattern := c.screenshotPattern
	go func() {
		data, err := encodePNG(img)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: encoding screenshot: %v\n", err)
			return
		}
		_ = c.Post(func() {
			if err := c.setPNGSelection(data); err != nil {
				fmt.Fprintf(os.Stderr, "sysc-compositor: %v\n", err)
			}
		})
		var written *string
		if writeToDisk {
			path, create := explicit, false
			if path == "" && pattern != nil {
				if expanded, err := expandScreenshotPath(*pattern); err != nil {
					fmt.Fprintf(os.Stderr, "sysc-compositor: %v\n", err)
				} else {
					path, create = expanded, true
				}
			}
			if path != "" {
				if dir := filepath.Dir(path); create && dir != "." {
					_ = os.MkdirAll(dir, 0o755)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					fmt.Fprintf(os.Stderr, "sysc-compositor: saving screenshot: %v\n", err)
				} else {
					written = &path
				}
			}
		}
		_ = c.Post(func() {
			if c.screenshotTaken != nil {
				c.screenshotTaken(written)
			}
		})
	}()
}

// SetScreenshotHandler sets the hook that reports each finished screenshot:
// the path written, or nil when it went to the clipboard only.
func (c *Compositor) SetScreenshotHandler(fn func(path *string)) { c.screenshotTaken = fn }

// ScreenshotWindow captures one window alone, the focused one when id is nil,
// at its output's scale, then saves it as ScreenshotScreen does.
func (c *Compositor) ScreenshotWindow(id *uint64, writeToDisk bool, path string) error {
	view := c.focusedView
	if id != nil {
		view = nil
		for _, v := range c.views {
			if v.id == *id && v.mapped {
				view = v
			}
		}
	}
	if view == nil || view.toplevel == nil {
		return errors.New("wlr: no window to screenshot")
	}
	xdg := view.toplevel.base
	scale := 1.0
	for pointer := range c.sceneOutputs {
		if C.GoString((*C.struct_wlr_output)(pointer).name) == c.outputNameForView(view) {
			scale = float64((*C.struct_wlr_output)(pointer).scale)
		}
	}
	width := int(float64(xdg.geometry.width) * scale)
	height := int(float64(xdg.geometry.height) * scale)
	if width <= 0 || height <= 0 {
		return errors.New("wlr: the window has no size")
	}
	pixels := C.malloc(C.size_t(width * height * 4))
	if pixels == nil {
		return errors.New("wlr: out of memory for a screenshot")
	}
	defer C.free(pixels)
	if !bool(C.sysc_screenshot_window(xdg, C.double(scale), C.int(width), C.int(height), c.alloc, c.renderer, pixels)) {
		return errors.New("wlr: rendering the window for a screenshot failed")
	}
	// Premultiplied B, G, R, A to Go's premultiplied RGBA.
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	src := unsafe.Slice((*byte)(pixels), width*height*4)
	for i := 0; i < width*height; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = src[i*4+2], src[i*4+1], src[i*4], src[i*4+3]
	}
	c.saveScreenshot(img, writeToDisk, path)
	return nil
}
