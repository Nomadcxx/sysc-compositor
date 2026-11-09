package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <scenefx/types/wlr_scene.h>
static void sysc_shot_mask_source(struct wlr_scene_buffer *node, double x, double y, double width, double height) {
 struct wlr_fbox box={.x=x,.y=y,.width=width,.height=height};
 wlr_scene_buffer_set_source_box(node,&box);
 wlr_scene_buffer_set_filter_mode(node,WLR_SCALE_FILTER_NEAREST);
}
*/
import "C"

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"unsafe"
)

// Scene rects have integer logical coordinates. Use them for the interior and
// rasterize only subpixel edge strips, so a one-physical-pixel hole stays exact
// without allocating a mask the size of the whole output on every pointer move.
func drawShotPhysicalRect(tree unsafe.Pointer, frame *frozenFrame, physical image.Rectangle, shade color.RGBA) {
	physical = physical.Intersect(frame.pixelBounds())
	if physical.Empty() {
		return
	}
	scale := frame.scale
	outer := image.Rect(int(math.Floor(float64(physical.Min.X)/scale)), int(math.Floor(float64(physical.Min.Y)/scale)), int(math.Ceil(float64(physical.Max.X)/scale)), int(math.Ceil(float64(physical.Max.Y)/scale)))
	inner := image.Rectangle{Min: image.Pt(int(math.Ceil(float64(physical.Min.X)/scale)), int(math.Ceil(float64(physical.Min.Y)/scale))), Max: image.Pt(int(math.Floor(float64(physical.Max.X)/scale)), int(math.Floor(float64(physical.Max.Y)/scale)))}
	strip := func(logical image.Rectangle) {
		if logical.Empty() {
			return
		}
		origin := image.Pt(int(math.Floor(float64(logical.Min.X)*scale)), int(math.Floor(float64(logical.Min.Y)*scale)))
		end := image.Pt(int(math.Ceil(float64(logical.Max.X)*scale)), int(math.Ceil(float64(logical.Max.Y)*scale)))
		size := end.Sub(origin)
		if _, err := captureAllocation(size.X, size.Y, size.X*4); err != nil {
			return
		}
		img := image.NewRGBA(image.Rectangle{Max: size})
		draw.Draw(img, physical.Sub(origin), image.NewUniform(shade), image.Point{}, draw.Src)
		node := sceneImage((*C.struct_wlr_scene_tree)(tree), img)
		if node == nil {
			return
		}
		sceneImagePlace(node, logical.Add(frame.box.Min), 1)
		// Source coordinates compensate for the logical origin's fractional
		// physical offset. Nearest sampling keeps native pixel boundaries crisp.
		C.sysc_shot_mask_source(node, C.double(float64(logical.Min.X)*scale-float64(origin.X)), C.double(float64(logical.Min.Y)*scale-float64(origin.Y)), C.double(float64(logical.Dx())*scale), C.double(float64(logical.Dy())*scale))
		sceneImageIgnoreInput(node)
	}
	if inner.Empty() {
		strip(outer)
		return
	}
	if node := sceneFXCreateRect(tree, inner.Dx(), inner.Dy(), [4]float32{float32(shade.R) / 255, float32(shade.G) / 255, float32(shade.B) / 255, float32(shade.A) / 255}); node != nil {
		sceneFXNodeSetPosition(node, frame.box.Min.X+inner.Min.X, frame.box.Min.Y+inner.Min.Y)
	}
	strip(image.Rect(outer.Min.X, outer.Min.Y, outer.Max.X, inner.Min.Y))
	strip(image.Rect(outer.Min.X, inner.Max.Y, outer.Max.X, outer.Max.Y))
	strip(image.Rect(outer.Min.X, inner.Min.Y, inner.Min.X, inner.Max.Y))
	strip(image.Rect(inner.Max.X, inner.Min.Y, outer.Max.X, inner.Max.Y))
}
