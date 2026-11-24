package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <string.h>
#include <drm_fourcc.h>
#include <wlr/interfaces/wlr_buffer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/util/region.h>
#include <wlr/util/transform.h>
#include <scenefx/types/wlr_scene.h>

// A read-only buffer over pixels the compositor drew itself (the ui
// package's panels). It owns a copy, so Go memory never reaches wlroots.
struct sysc_image_buffer {
	struct wlr_buffer base;
	void *data;
	size_t stride;
	uint32_t format;
};

static void sysc_image_buffer_destroy(struct wlr_buffer *wlr_buffer) {
	struct sysc_image_buffer *buffer = wl_container_of(wlr_buffer, buffer, base);
	wlr_buffer_finish(wlr_buffer);
	free(buffer->data);
	free(buffer);
}

static bool sysc_image_buffer_begin(struct wlr_buffer *wlr_buffer, uint32_t flags,
		void **data, uint32_t *format, size_t *stride) {
	if (flags & WLR_BUFFER_DATA_PTR_ACCESS_WRITE) {
		return false;
	}
	struct sysc_image_buffer *buffer = wl_container_of(wlr_buffer, buffer, base);
	*data = buffer->data;
	// Go's image.RGBA is premultiplied R, G, B, A bytes: little-endian ABGR.
	*format = buffer->format;
	*stride = buffer->stride;
	return true;
}

static void sysc_image_buffer_end(struct wlr_buffer *wlr_buffer) {}

static const struct wlr_buffer_impl sysc_image_buffer_impl = {
	.destroy = sysc_image_buffer_destroy,
	.begin_data_ptr_access = sysc_image_buffer_begin,
	.end_data_ptr_access = sysc_image_buffer_end,
};

// sysc_scene_image takes ownership of data and puts it in the scene under
// parent. The scene node holds the only buffer lock.
static struct wlr_buffer *sysc_image_buffer_create(void *data, int width, int height, size_t stride) {
	struct sysc_image_buffer *buffer = calloc(1, sizeof(*buffer));
	if (buffer == NULL) {
		free(data);
		return NULL;
	}
	wlr_buffer_init(&buffer->base, &sysc_image_buffer_impl, width, height);
	buffer->data = data;
	buffer->stride = stride;
	buffer->format = DRM_FORMAT_ABGR8888;
	return &buffer->base;
}

static struct wlr_buffer *sysc_image_buffer_copy(struct wlr_buffer *source) {
	void *data;
	uint32_t format;
	size_t stride;
	if (!wlr_buffer_begin_data_ptr_access(source,WLR_BUFFER_DATA_PTR_ACCESS_READ,&data,&format,&stride)) return NULL;
	struct wlr_buffer *copy = NULL;
	if (source->height > 0 && stride > 0 && stride <= (64*1024*1024)/(size_t)source->height) {
		void *pixels = malloc(stride*source->height);
		if (pixels != NULL) {
			memcpy(pixels,data,stride*source->height);
			copy = sysc_image_buffer_create(pixels,source->width,source->height,stride);
			if (copy != NULL) ((struct sysc_image_buffer *)copy)->format = format;
		}
	}
	wlr_buffer_end_data_ptr_access(source);
	return copy;
}

// sysc_image_buffer_patch copies what a surface's pending commit damaged from
// its buffer into backup, a copy of the previous buffer. Wayland damage covers
// every pixel that changed, so the result matches a full copy. False means
// the caller copies the whole buffer: a new size or format, a viewport (whose
// damage conversion is lossy) or a buffer without CPU access.
static bool sysc_image_buffer_patch(struct wlr_buffer *backup, void *surface_ptr) {
	struct wlr_surface *surface = surface_ptr;
	struct wlr_surface_state *pending = &surface->pending;
	struct wlr_buffer *source = pending->buffer;
	if (backup->impl != &sysc_image_buffer_impl || source == NULL ||
			pending->viewport.has_src || pending->viewport.has_dst ||
			backup->width != source->width || backup->height != source->height) {
		return false;
	}
	struct sysc_image_buffer *copy = wl_container_of(backup, copy, base);
	void *data;
	uint32_t format;
	size_t stride;
	if (!wlr_buffer_begin_data_ptr_access(source, WLR_BUFFER_DATA_PTR_ACCESS_READ, &data, &format, &stride)) {
		return false;
	}
	bool ok = format == copy->format && stride == copy->stride &&
		(format == DRM_FORMAT_ARGB8888 || format == DRM_FORMAT_XRGB8888 ||
		 format == DRM_FORMAT_ABGR8888 || format == DRM_FORMAT_XBGR8888);
	if (ok) {
		// wlroots folds surface damage in only when the state applies.
		pixman_region32_t damage;
		pixman_region32_init(&damage);
		pixman_region32_copy(&damage, &pending->surface_damage);
		wlr_region_scale(&damage, &damage, pending->scale);
		int width = source->width, height = source->height;
		if (pending->transform & WL_OUTPUT_TRANSFORM_90) {
			width = source->height;
			height = source->width;
		}
		wlr_region_transform(&damage, &damage, wlr_output_transform_invert(pending->transform), width, height);
		pixman_region32_union(&damage, &damage, &pending->buffer_damage);
		pixman_region32_intersect_rect(&damage, &damage, 0, 0, source->width, source->height);
		int count;
		const pixman_box32_t *boxes = pixman_region32_rectangles(&damage, &count);
		for (int i = 0; i < count; i++) {
			size_t offset = (size_t)boxes[i].x1 * 4, bytes = (size_t)(boxes[i].x2 - boxes[i].x1) * 4;
			for (int y = boxes[i].y1; y < boxes[i].y2; y++) {
				memcpy((char *)copy->data + y * stride + offset, (const char *)data + y * stride + offset, bytes);
			}
		}
		pixman_region32_fini(&damage);
	}
	wlr_buffer_end_data_ptr_access(source);
	return ok;
}

// sysc_test_image_patch patches a zeroed backup from an all-0xff source
// through a surface whose pending state holds the given buffer and surface
// damage. It returns how many backup pixels changed, or -1 if it declined.
static int sysc_test_image_patch(int width, int height, int source_width, int scale,
		int bx, int by, int bw, int bh, int sx, int sy, int sw, int sh) {
	void *zero = calloc((size_t)width * height, 4);
	void *full = malloc((size_t)source_width * height * 4);
	if (zero == NULL || full == NULL) {
		free(zero);
		free(full);
		return -2;
	}
	memset(full, 0xff, (size_t)source_width * height * 4);
	struct wlr_buffer *backup = sysc_image_buffer_create(zero, width, height, (size_t)width * 4);
	struct wlr_buffer *source = sysc_image_buffer_create(full, source_width, height, (size_t)source_width * 4);
	struct wlr_surface surface = {0};
	pixman_region32_init_rect(&surface.pending.buffer_damage, bx, by, bw, bh);
	pixman_region32_init_rect(&surface.pending.surface_damage, sx, sy, sw, sh);
	surface.pending.scale = scale;
	surface.pending.transform = WL_OUTPUT_TRANSFORM_NORMAL;
	surface.pending.buffer = source;
	int changed = -1;
	if (sysc_image_buffer_patch(backup, &surface)) {
		changed = 0;
		const uint32_t *pixels = ((struct sysc_image_buffer *)backup)->data;
		for (int i = 0; i < width * height; i++) {
			changed += pixels[i] != 0;
		}
	}
	pixman_region32_fini(&surface.pending.buffer_damage);
	pixman_region32_fini(&surface.pending.surface_damage);
	wlr_buffer_drop(backup);
	wlr_buffer_drop(source);
	return changed;
}

static void sysc_scene_image_place(struct wlr_scene_buffer *node, int x, int y,
		int width, int height, float opacity) {
	wlr_scene_buffer_set_dest_size(node, width, height);
	wlr_scene_buffer_set_opacity(node, opacity);
	wlr_scene_node_set_position(&node->node, x, y);
}

static bool sysc_scene_image_no_input(struct wlr_scene_buffer *buffer, double *sx, double *sy) {
	return false;
}

// sysc_scene_image_ignore_input makes the node invisible to hit-testing, so
// the pointer reaches what is under it.
static void sysc_scene_image_ignore_input(struct wlr_scene_buffer *node) {
	node->point_accepts_input = sysc_scene_image_no_input;
}

static void sysc_scene_rect_color(struct wlr_scene_node *node, float r, float g, float b, float a) {
	float color[4] = {r, g, b, a};
	wlr_scene_rect_set_color(wlr_scene_rect_from_node(node), color);
}
*/
import "C"

import (
	"image"
	"unsafe"
)

// copyBufferPixels retains SHM contents independently of wl_buffer.release.
// ponytail: copies the full buffer (64 MiB ceiling); use damage-region copies
// if SHM-heavy workloads make the extra memcpy measurable.
func copyBufferPixels(source *C.struct_wlr_buffer) *C.struct_wlr_buffer {
	return C.sysc_image_buffer_copy(source)
}

func patchBufferPixelsForTest(width, height, sourceWidth, scale int, buffer, surface rect) int {
	return int(C.sysc_test_image_patch(C.int(width), C.int(height), C.int(sourceWidth), C.int(scale),
		C.int(buffer.x), C.int(buffer.y), C.int(buffer.w), C.int(buffer.h),
		C.int(surface.x), C.int(surface.y), C.int(surface.w), C.int(surface.h)))
}

// patchBufferPixels brings backup, a copy of surface's previous buffer, up to
// its pending commit by copying only the damage. False means copy it all.
func patchBufferPixels(backup *C.struct_wlr_buffer, surface unsafe.Pointer) bool {
	return bool(C.sysc_image_buffer_patch(backup, surface))
}

// sceneImage puts a copy of img under parent as a buffer node at its
// physical size; sceneImagePlace sets where and how big it is drawn.
func sceneImage(parent *C.struct_wlr_scene_tree, img *image.RGBA) *C.struct_wlr_scene_buffer {
	buffer := imageBuffer(img)
	if buffer == nil {
		return nil
	}
	node := C.wlr_scene_buffer_create(parent, buffer)
	C.wlr_buffer_drop(buffer)
	return node
}

// imageBuffer copies premultiplied pixels into a producer-owned native buffer.
// The caller drops the producer reference after attaching or copying it.
func imageBuffer(img *image.RGBA) *C.struct_wlr_buffer {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	size, err := captureAllocation(w, h, w*4)
	if err != nil {
		return nil
	}
	stride := 4 * w
	data := C.malloc(C.size_t(size))
	if data == nil {
		return nil
	}
	dst := unsafe.Slice((*byte)(data), stride*h)
	for y := range h {
		copy(dst[y*stride:(y+1)*stride], img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):])
	}
	return C.sysc_image_buffer_create(data, C.int(w), C.int(h), C.size_t(stride))
}

func sceneImagePlace(node *C.struct_wlr_scene_buffer, box image.Rectangle, opacity float64) {
	C.sysc_scene_image_place(node, C.int(box.Min.X), C.int(box.Min.Y), C.int(box.Dx()), C.int(box.Dy()), C.float(opacity))
}

func sceneImageIgnoreInput(node *C.struct_wlr_scene_buffer) {
	C.sysc_scene_image_ignore_input(node)
}

// sceneRectSetColor recolours a rect node made by sceneFXCreateRect.
func sceneRectSetColor(node unsafe.Pointer, color [4]float32) {
	C.sysc_scene_rect_color((*C.struct_wlr_scene_node)(node), C.float(color[0]), C.float(color[1]),
		C.float(color[2]), C.float(color[3]))
}
