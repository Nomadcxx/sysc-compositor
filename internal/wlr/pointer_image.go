package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/types/wlr_output.h>
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
		if _, err := captureAllocation(int(w), int(h), int(w)*4); err != nil {
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
// coordinates) at scale. Draw clips a partially visible pointer too.
func (c *Compositor) drawPointer(dst draw.Image, box image.Rectangle, scale float64) {
	img, hotspot, ok := c.pointerImage(scale)
	if !ok {
		return
	}
	at := c.pointerOrigin(box, scale, hotspot)
	draw.Draw(dst, img.Bounds().Add(at), img, image.Point{}, draw.Over)
}

// Output buffers precede the output transform; pointer coordinates follow it.
func (c *Compositor) drawOutputPointer(dst draw.Image, output unsafe.Pointer) {
	scale := float64((*C.struct_wlr_output)(output).scale)
	img, hotspot, ok := c.pointerImage(scale)
	if !ok {
		return
	}
	at := c.pointerOrigin(c.outputLayoutBox(output), scale, hotspot)
	drawTransformedPointer(dst, img, at, uint32((*C.struct_wlr_output)(output).transform))
}

func drawTransformedPointer(dst draw.Image, icon *image.RGBA, at image.Point, transform uint32) {
	if transform == C.WL_OUTPUT_TRANSFORM_NORMAL {
		origin := dst.Bounds().Min.Add(at)
		draw.Draw(dst, image.Rectangle{Min: origin, Max: origin.Add(icon.Bounds().Size())}, icon, icon.Bounds().Min, draw.Over)
		return
	}
	bounds := dst.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	toBuffer := func(p image.Point) image.Point {
		x, y := p.X, p.Y
		switch transform {
		case C.WL_OUTPUT_TRANSFORM_90:
			p = image.Pt(y, h-1-x)
		case C.WL_OUTPUT_TRANSFORM_180:
			p = image.Pt(w-1-x, h-1-y)
		case C.WL_OUTPUT_TRANSFORM_270:
			p = image.Pt(w-1-y, x)
		case C.WL_OUTPUT_TRANSFORM_FLIPPED:
			p = image.Pt(w-1-x, y)
		case C.WL_OUTPUT_TRANSFORM_FLIPPED_90:
			p = image.Pt(w-1-y, h-1-x)
		case C.WL_OUTPUT_TRANSFORM_FLIPPED_180:
			p = image.Pt(x, h-1-y)
		case C.WL_OUTPUT_TRANSFORM_FLIPPED_270:
			p = image.Pt(y, x)
		}
		return p.Add(bounds.Min)
	}
	first, last := toBuffer(at), toBuffer(at.Add(icon.Bounds().Size()).Sub(image.Pt(1, 1)))
	box := image.Rect(min(first.X, last.X), min(first.Y, last.Y), max(first.X, last.X)+1, max(first.Y, last.Y)+1)
	if !box.Overlaps(bounds) {
		return
	}
	rotated := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := icon.Rect.Min.Y; y < icon.Rect.Max.Y; y++ {
		for x := icon.Rect.Min.X; x < icon.Rect.Max.X; x++ {
			p := toBuffer(at.Add(image.Pt(x, y).Sub(icon.Rect.Min))).Sub(box.Min)
			rotated.SetRGBA(p.X, p.Y, icon.RGBAAt(x, y))
		}
	}
	draw.Draw(dst, box, rotated, image.Point{}, draw.Over)
}
