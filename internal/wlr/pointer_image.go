package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/types/wlr_xcursor_manager.h>
#include <wlr/xcursor.h>

// sysc_xcursor_image is the first frame of a theme image at scale: its size,
// hotspot and premultiplied ARGB pixels, which the manager owns.
static bool sysc_xcursor_image(struct wlr_xcursor_manager *manager, const char *name, float scale,
		uint32_t *width, uint32_t *height, uint32_t *hx, uint32_t *hy, uint8_t **pixels) {
	if (!wlr_xcursor_manager_load(manager, scale)) {
		return false;
	}
	struct wlr_xcursor *xcursor = wlr_xcursor_manager_get_xcursor(manager, name, scale);
	if (xcursor == NULL || xcursor->image_count == 0) {
		return false;
	}
	struct wlr_xcursor_image *image = xcursor->images[0];
	*width = image->width;
	*height = image->height;
	*hx = image->hotspot_x;
	*hy = image->hotspot_y;
	*pixels = image->buffer;
	return true;
}
*/
import "C"

import (
	"image"
	"image/draw"
	"math"
	"unsafe"
)

// pointerImage is the drawn pointer at scale, with its hotspot in the
// image's pixels; ok is false when no pointer is drawn. wlroots keeps the
// cursor's own image private, so this redraws what the compositor set: a
// theme image by name, or the client's cursor surface.
func (c *Compositor) pointerImage(scale float64) (img *image.RGBA, hotspot image.Point, ok bool) {
	if c.drawnImage != "" && c.xcursorManager != nil {
		name := C.CString(c.drawnImage)
		defer C.free(unsafe.Pointer(name))
		var w, h, hx, hy C.uint32_t
		var pixels *C.uint8_t
		if !bool(C.sysc_xcursor_image(c.xcursorManager, name, C.float(scale), &w, &h, &hx, &hy, &pixels)) {
			return nil, image.Point{}, false
		}
		src := unsafe.Slice((*byte)(unsafe.Pointer(pixels)), int(w)*int(h)*4)
		return rgbaFromARGB(src, int(w), int(h)), image.Pt(int(hx), int(hy)), true
	}
	if c.drawnImage == "" && c.cursorSurface != nil && bool(c.cursorSurface.mapped) {
		s := c.cursorSurface
		w := int(math.Round(float64(s.current.width) * scale))
		h := int(math.Round(float64(s.current.height) * scale))
		if w <= 0 || h <= 0 {
			return nil, image.Point{}, false
		}
		img, err := c.renderSurfaceTree(s, 0, 0, scale, w, h)
		if err != nil {
			return nil, image.Point{}, false
		}
		hotspot := image.Pt(int(math.Round(float64(c.cursorHotspotX)*scale)), int(math.Round(float64(c.cursorHotspotY)*scale)))
		return img, hotspot, true
	}
	return nil, image.Point{}, false
}

// pointerOrigin is where the pointer image's top-left lands in an image of
// box (layout coordinates) drawn at scale.
func (c *Compositor) pointerOrigin(box image.Rectangle, scale float64, hotspot image.Point) image.Point {
	x := math.Round((float64(c.cursor.x) - float64(box.Min.X)) * scale)
	y := math.Round((float64(c.cursor.y) - float64(box.Min.Y)) * scale)
	return image.Pt(int(x), int(y)).Sub(hotspot)
}

// drawPointer draws the pointer over dst, an image of box (layout
// coordinates) at scale, when the pointer is over it.
func (c *Compositor) drawPointer(dst draw.Image, box image.Rectangle, scale float64) {
	if !image.Pt(int(c.cursor.x), int(c.cursor.y)).In(box) {
		return
	}
	img, hotspot, ok := c.pointerImage(scale)
	if !ok {
		return
	}
	at := c.pointerOrigin(box, scale, hotspot)
	draw.Draw(dst, img.Bounds().Add(at), img, image.Point{}, draw.Over)
}
