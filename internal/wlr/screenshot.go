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
#include <wlr/render/interface.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/util/transform.h>
#include <scenefx/types/wlr_scene.h>

// Renders an output's scene into a swapchain of its own, so the output's
// buffers and damage tracking are untouched, and returns the buffer locked;
// the caller unlocks it. A fresh buffer has no age, so the scene draws all of
// it. (Only wlr_scene_output_commit skips an output that needs no frame.)
// Building a state consumes a pending gamma change, and this state is never
// committed, so the change is held back for the output's next real frame.
static struct wlr_buffer *sysc_render_output(void *pointer, struct wlr_output *output) {
	struct wlr_scene_output *scene_output = pointer;
	struct wlr_swapchain *swapchain = NULL;
	if (!wlr_output_configure_primary_swapchain(output, NULL, &swapchain)) {
		return NULL;
	}

	struct wlr_scene_output_state_options options = { .swapchain = swapchain };
	struct wlr_output_state state;
	wlr_output_state_init(&state);
	bool gamma_changed = scene_output->WLR_PRIVATE.gamma_lut_changed;
	scene_output->WLR_PRIVATE.gamma_lut_changed = false;
	struct wlr_buffer *buffer = NULL;
	if (wlr_scene_output_build_state(scene_output, &state, &options) &&
			(state.committed & WLR_OUTPUT_STATE_BUFFER) && state.buffer != NULL) {
		buffer = wlr_buffer_lock(state.buffer);
	}
	scene_output->WLR_PRIVATE.gamma_lut_changed = gamma_changed;
	wlr_output_state_finish(&state);
	wlr_swapchain_destroy(swapchain);
	return buffer;
}

// Reads a box of a buffer back as ARGB8888 (B, G, R, A bytes), w * 4 a row.
static bool sysc_read_buffer(struct wlr_renderer *renderer, struct wlr_buffer *buffer,
		int x, int y, int w, int h, void *data) {
	struct wlr_texture *texture = wlr_texture_from_buffer(renderer, buffer);
	if (texture == NULL) {
		return false;
	}
	struct wlr_texture_read_pixels_options read = {
		.data = data, .format = DRM_FORMAT_ARGB8888, .stride = (uint32_t)w * 4,
		.src_box = { .x = x, .y = y, .width = w, .height = h },
	};
	bool ok = wlr_texture_read_pixels(texture, &read);
	wlr_texture_destroy(texture);
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

// Draws a surface and its subsurfaces, from (ox, oy) in surface coordinates,
// into a transparent buffer of its own and reads it back as premultiplied
// ARGB8888. A window screenshot draws no popups and nothing that overlaps
// it, as Niri screenshots a window alone; the pointer's surface is drawn
// the same way. width and height are the drawn size times scale.
static bool sysc_render_surface_tree(struct wlr_surface *surface, int ox, int oy, double scale,
		int width, int height, struct wlr_allocator *alloc, struct wlr_renderer *renderer, void *data) {
	const struct wlr_drm_format_set *formats = renderer->WLR_PRIVATE.impl->get_render_formats(renderer);
	const struct wlr_drm_format *format = wlr_drm_format_set_get(formats, DRM_FORMAT_ARGB8888);
	if (format == NULL) {
		return false;
	}
	struct wlr_buffer *buffer = wlr_allocator_create_buffer(alloc, width, height, format);

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
		struct sysc_window_shot shot = { .pass = pass, .ox = ox, .oy = oy, .scale = scale };
		wlr_surface_for_each_surface(surface, sysc_window_shot_surface, &shot);
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
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime/cgo"
	"strings"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// captureOutput renders one output as it would next be shown and returns it
// as an image. It runs on the compositor thread.
func (c *Compositor) captureOutput(pointer unsafe.Pointer) (*image.NRGBA, error) {
	return c.captureOutputForTarget(pointer, config.CaptureScreen)
}

func (c *Compositor) captureOutputForTarget(pointer unsafe.Pointer, target config.CaptureTarget) (*image.NRGBA, error) {
	buffer, err := c.renderOutputForTarget(pointer, target)
	if err != nil {
		return nil, err
	}
	defer C.wlr_buffer_unlock(buffer)
	w, h := outputPixelSize(pointer)
	return c.readBuffer(buffer, image.Rect(0, 0, w, h))
}

// renderOutput renders an output's scene into a buffer of its own, returned
// locked.
func (c *Compositor) renderOutput(pointer unsafe.Pointer) (*C.struct_wlr_buffer, error) {
	return c.renderOutputForTarget(pointer, config.CaptureScreen)
}

func (c *Compositor) renderOutputForTarget(pointer unsafe.Pointer, target config.CaptureTarget) (*C.struct_wlr_buffer, error) {
	so, ok := c.sceneOutputs[pointer]
	if !ok {
		return nil, errors.New("wlr: no such output")
	}
	output := (*C.struct_wlr_output)(pointer)
	if _, err := captureAllocation(int(output.width), int(output.height), int(output.width)*4); err != nil {
		return nil, err
	}
	restore, err := c.beginCapturePrivacy(target)
	if err != nil {
		return nil, err
	}
	defer restore()
	buffer := C.sysc_render_output(unsafe.Pointer(so), output)
	if buffer == nil {
		return nil, fmt.Errorf("wlr: rendering output %q for a screenshot failed", C.GoString(output.name))
	}
	return buffer, nil
}

// readBuffer reads box of an XRGB buffer back as an opaque image.
func (c *Compositor) readBuffer(buffer *C.struct_wlr_buffer, box image.Rectangle) (*image.NRGBA, error) {
	w, h := box.Dx(), box.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("wlr: an empty screenshot")
	}
	size, err := captureAllocation(w, h, w*4)
	if err != nil {
		return nil, err
	}
	if box.Min.X < 0 || box.Min.Y < 0 || box.Max.X > int(buffer.width) || box.Max.Y > int(buffer.height) {
		return nil, errors.New("wlr: capture region exceeds buffer")
	}
	pixels := C.malloc(C.size_t(size))
	if pixels == nil {
		return nil, errors.New("wlr: out of memory for a screenshot")
	}
	defer C.free(pixels)
	if !bool(C.sysc_read_buffer(c.renderer, buffer, C.int(box.Min.X), C.int(box.Min.Y), C.int(w), C.int(h), pixels)) {
		return nil, errors.New("wlr: reading the screenshot back failed")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	src := unsafe.Slice((*byte)(pixels), w*h*4)
	for i := 0; i < w*h; i++ {
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
// encoded and written on a goroutine. With showPointer the pointer is drawn
// in where it is.
func (c *Compositor) ScreenshotScreen(writeToDisk, showPointer bool, path string) error {
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
	if showPointer {
		c.drawOutputPointer(img, target)
	}
	c.saveScreenshot(img, writeToDisk, path)
	return nil
}

// ScreenshotScreenResult is the GNOME portal screenshot operation. Niri 26.04
// supports one output and ignores the requested filename. Rendering and PNG
// encoding are shared with screenshots; clipboard and ScreenshotCaptured stay
// untouched. Each request owns its result and any temporary fallback file.
func (c *Compositor) ScreenshotScreenResult(ctx context.Context, showPointer bool, done func(string, error)) error {
	if done == nil {
		return errors.New("wlr: a portal screenshot needs a result handler")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(c.sceneOutputs) != 1 {
		return errors.New("wlr: portal screenshot requires exactly one output")
	}
	var target unsafe.Pointer
	for output := range c.sceneOutputs {
		target = output
	}
	img, err := c.captureOutput(target)
	if err != nil {
		return err
	}
	if showPointer {
		c.drawOutputPointer(img, target)
	}
	path := ""
	if c.screenshotPattern != nil {
		path, _ = expandScreenshotPath(*c.screenshotPattern)
	}
	go func() {
		filename, err := func() (filename string, err error) {
			if err = ctx.Err(); err != nil {
				return "", err
			}
			data, err := encodePNG(img)
			if err != nil {
				return "", err
			}
			if err = ctx.Err(); err != nil {
				return "", err
			}
			var file *os.File
			temporary := path == ""
			if temporary {
				// Niri uses a shared screenshot.png fallback. A private file prevents
				// symlink following and concurrent requests overwriting one another.
				file, err = os.CreateTemp("", "sysc-screenshot-*.png")
			} else {
				file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			}
			if err != nil {
				return "", err
			}
			defer func() {
				err = errors.Join(err, file.Close())
				if err != nil && temporary {
					_ = os.Remove(file.Name())
				}
			}()
			if err = ctx.Err(); err == nil {
				_, err = file.Write(data)
			}
			if err == nil {
				err = ctx.Err()
			}
			return file.Name(), err
		}()
		if err != nil {
			done("", err)
			return
		}
		done(filename, nil)
	}()
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
func (c *Compositor) ScreenshotWindow(id *uint64, writeToDisk, showPointer bool, path string) error {
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
	scale := c.windowCaptureScale(view)
	img, err := c.renderWindowCapture(view, config.CaptureScreen)
	if err != nil {
		return err
	}
	if showPointer {
		// The pointer goes in where it is over the window, as in Niri.
		x, y := view.treePosition()
		c.drawPointer(img, image.Rect(x, y, x+int(xdg.geometry.width), y+int(xdg.geometry.height)), scale)
	}
	c.saveScreenshot(img, writeToDisk, path)
	return nil
}

// renderSurfaceTree draws a surface tree from (ox, oy) at scale into a
// width by height image.
func (c *Compositor) renderSurfaceTree(surface *C.struct_wlr_surface, ox, oy int, scale float64,
	width, height int) (*image.RGBA, error) {
	size, err := captureAllocation(width, height, width*4)
	if err != nil {
		return nil, err
	}
	pixels := C.malloc(C.size_t(size))
	if pixels == nil {
		return nil, errors.New("wlr: out of memory for a screenshot")
	}
	defer C.free(pixels)
	if !bool(C.sysc_render_surface_tree(surface, C.int(ox), C.int(oy), C.double(scale), C.int(width), C.int(height),
		c.alloc, c.renderer, pixels)) {
		return nil, errors.New("wlr: rendering a surface for a screenshot failed")
	}
	return rgbaFromARGB(unsafe.Slice((*byte)(pixels), width*height*4), width, height), nil
}

// rgbaFromARGB converts premultiplied B, G, R, A bytes to Go's premultiplied
// RGBA.
func rgbaFromARGB(src []byte, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = src[i*4+2], src[i*4+1], src[i*4], src[i*4+3]
	}
	return img
}
