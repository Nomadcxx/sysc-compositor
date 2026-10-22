package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <drm_fourcc.h>
#include <wlr/interfaces/wlr_buffer.h>
#include <scenefx/types/wlr_scene.h>

// A read-only buffer over pixels the compositor drew itself (the ui
// package's panels). It owns a copy, so Go memory never reaches wlroots.
struct sysc_image_buffer {
	struct wlr_buffer base;
	void *data;
	size_t stride;
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
	*format = DRM_FORMAT_ABGR8888;
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
static struct wlr_scene_buffer *sysc_scene_image(struct wlr_scene_tree *parent,
		void *data, int width, int height, size_t stride) {
	struct sysc_image_buffer *buffer = calloc(1, sizeof(*buffer));
	if (buffer == NULL) {
		free(data);
		return NULL;
	}
	wlr_buffer_init(&buffer->base, &sysc_image_buffer_impl, width, height);
	buffer->data = data;
	buffer->stride = stride;
	struct wlr_scene_buffer *node = wlr_scene_buffer_create(parent, &buffer->base);
	wlr_buffer_drop(&buffer->base);
	return node;
}

static void sysc_scene_image_place(struct wlr_scene_buffer *node, int x, int y,
		int width, int height, float opacity) {
	wlr_scene_buffer_set_dest_size(node, width, height);
	wlr_scene_buffer_set_opacity(node, opacity);
	wlr_scene_node_set_position(&node->node, x, y);
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

// sceneImage puts a copy of img under parent as a buffer node at its
// physical size; sceneImagePlace sets where and how big it is drawn.
func sceneImage(parent *C.struct_wlr_scene_tree, img *image.RGBA) *C.struct_wlr_scene_buffer {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return nil
	}
	stride := 4 * w
	data := C.malloc(C.size_t(stride * h))
	if data == nil {
		return nil
	}
	dst := unsafe.Slice((*byte)(data), stride*h)
	for y := range h {
		copy(dst[y*stride:(y+1)*stride], img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):])
	}
	return C.sysc_scene_image(parent, data, C.int(w), C.int(h), C.size_t(stride))
}

func sceneImagePlace(node *C.struct_wlr_scene_buffer, box image.Rectangle, opacity float64) {
	C.sysc_scene_image_place(node, C.int(box.Min.X), C.int(box.Min.Y), C.int(box.Dx()), C.int(box.Dy()), C.float(opacity))
}

// sceneRectSetColor recolours a rect node made by sceneFXCreateRect.
func sceneRectSetColor(node unsafe.Pointer, color [4]float32) {
	C.sysc_scene_rect_color((*C.struct_wlr_scene_node)(node), C.float(color[0]), C.float(color[1]),
		C.float(color[2]), C.float(color[3]))
}
