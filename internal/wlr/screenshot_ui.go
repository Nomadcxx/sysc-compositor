package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_output_layout.h>
#include <scenefx/types/wlr_scene.h>

// SceneFX forks struct wlr_scene, so these take void pointers.
static void *sysc_shot_tree_create(void *parent, void *below) {
	struct wlr_scene_tree *tree = wlr_scene_tree_create(parent);
	if (tree != NULL && below != NULL) {
		wlr_scene_node_place_below(&tree->node, below);
	}
	return tree;
}

static bool sysc_shot_frame(void *tree, struct wlr_buffer *buffer, int x, int y, int w, int h) {
	struct wlr_scene_buffer *node = wlr_scene_buffer_create(tree, buffer);
	if (node == NULL) {
		return false;
	}
	wlr_scene_buffer_set_dest_size(node, w, h);
	wlr_scene_node_set_position(&node->node, x, y);
	return true;
}

static void sysc_shot_tree_destroy(void *tree) {
	wlr_scene_node_destroy(&((struct wlr_scene_tree *)tree)->node);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"os"
	"unsafe"
)

// The interactive screenshot UI (Niri's screenshot_ui.rs), text-free part:
// every output's current frame is frozen and dimmed outside a selection
// that the pointer drags out. Space or Return captures the selection from
// the frozen frame; Escape cancels. While open it takes all user input. The
// selection is kept for the next opening, as in Niri. The hint text waits
// for the owner's text-rendering decision (M11 plan).

type screenshotUI struct {
	tree      unsafe.Pointer
	overlay   unsafe.Pointer // dim and border rects, rebuilt as the selection moves
	frames    map[unsafe.Pointer]*frozenFrame
	selection image.Rectangle // layout coordinates
	output    unsafe.Pointer  // the output the selection is on
	anchor    image.Point
	dragging  bool
	path      string
}

// evdevButtonLeft is BTN_LEFT.
const evdevButtonLeft = 0x110

type frozenFrame struct {
	buffer *C.struct_wlr_buffer
	box    image.Rectangle // the output's layout box
	scale  float64
}

// ScreenshotUIOpen reports whether the screenshot UI is up.
func (c *Compositor) ScreenshotUIOpen() bool { return c.shotUI != nil }

// Screenshot opens the interactive screenshot UI. path, when set, is where
// the confirmed screenshot is written, as given.
func (c *Compositor) Screenshot(path string) error {
	if c.shotUI != nil {
		return nil
	}
	if c.sessionLocked {
		return errors.New("wlr: no screenshot UI while the session is locked")
	}
	tree := C.sysc_shot_tree_create(unsafe.Pointer(&c.scene.tree), unsafe.Pointer(&c.lockTree.node))
	if tree == nil {
		return errors.New("wlr: creating the screenshot UI failed")
	}
	ui := &screenshotUI{tree: tree, frames: map[unsafe.Pointer]*frozenFrame{}, path: path}
	for pointer := range c.sceneOutputs {
		if !c.outputIsEnabled(pointer) {
			continue
		}
		buffer, err := c.renderOutput(pointer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: screenshot UI: %v\n", err)
			continue
		}
		box := c.outputLayoutBox(pointer)
		if !bool(C.sysc_shot_frame(tree, buffer, C.int(box.Min.X), C.int(box.Min.Y), C.int(box.Dx()), C.int(box.Dy()))) {
			C.wlr_buffer_unlock(buffer)
			continue
		}
		ui.frames[pointer] = &frozenFrame{buffer: buffer, box: box, scale: float64((*C.struct_wlr_output)(pointer).scale)}
	}
	if len(ui.frames) == 0 {
		C.sysc_shot_tree_destroy(tree)
		return errors.New("wlr: no output to screenshot")
	}
	c.shotUI = ui
	c.cancelTiledWindowMove()
	c.clearPointerGrab()
	ui.selection, ui.output = c.initialShotSelection()
	c.drawShotSelection()
	return nil
}

// initialShotSelection is the last selection when it still fits an output,
// else the middle half of the output under the pointer.
func (c *Compositor) initialShotSelection() (image.Rectangle, unsafe.Pointer) {
	ui := c.shotUI
	for pointer, frame := range ui.frames {
		if !c.lastShotSelection.Empty() && c.lastShotSelection.In(frame.box) {
			return c.lastShotSelection, pointer
		}
	}
	cursor := image.Pt(int(c.cursor.x), int(c.cursor.y))
	var output unsafe.Pointer
	for pointer, frame := range ui.frames {
		if output == nil || cursor.In(frame.box) {
			output = pointer
		}
	}
	box := ui.frames[output].box
	return image.Rect(box.Min.X+box.Dx()/4, box.Min.Y+box.Dy()/4,
		box.Max.X-box.Dx()/4, box.Max.Y-box.Dy()/4), output
}

// drawShotSelection dims every frozen frame outside the selection and draws
// a border around it.
func (c *Compositor) drawShotSelection() {
	ui := c.shotUI
	if ui.overlay != nil {
		C.sysc_shot_tree_destroy(ui.overlay)
	}
	ui.overlay = C.sysc_shot_tree_create(ui.tree, nil)
	if ui.overlay == nil {
		return
	}
	dim := [4]float32{0, 0, 0, 0.5}
	rect := func(r image.Rectangle, color [4]float32) {
		if r.Empty() {
			return
		}
		if node := sceneFXCreateRect(ui.overlay, r.Dx(), r.Dy(), color); node != nil {
			sceneFXNodeSetPosition(node, r.Min.X, r.Min.Y)
		}
	}
	sel := ui.selection
	for _, frame := range ui.frames {
		b := frame.box
		if !sel.Overlaps(b) {
			rect(b, dim)
			continue
		}
		in := sel.Intersect(b)
		rect(image.Rect(b.Min.X, b.Min.Y, b.Max.X, in.Min.Y), dim)
		rect(image.Rect(b.Min.X, in.Max.Y, b.Max.X, b.Max.Y), dim)
		rect(image.Rect(b.Min.X, in.Min.Y, in.Min.X, in.Max.Y), dim)
		rect(image.Rect(in.Max.X, in.Min.Y, b.Max.X, in.Max.Y), dim)
	}
	white := [4]float32{1, 1, 1, 1}
	const border = 2
	rect(image.Rect(sel.Min.X-border, sel.Min.Y-border, sel.Max.X+border, sel.Min.Y), white)
	rect(image.Rect(sel.Min.X-border, sel.Max.Y, sel.Max.X+border, sel.Max.Y+border), white)
	rect(image.Rect(sel.Min.X-border, sel.Min.Y, sel.Min.X, sel.Max.Y), white)
	rect(image.Rect(sel.Max.X, sel.Min.Y, sel.Max.X+border, sel.Max.Y), white)
}

// outputLayoutBox is an output's box in layout coordinates.
func (c *Compositor) outputLayoutBox(pointer unsafe.Pointer) image.Rectangle {
	var box C.struct_wlr_box
	C.wlr_output_layout_get_box(c.layout, (*C.struct_wlr_output)(pointer), &box)
	return image.Rect(int(box.x), int(box.y), int(box.x+box.width), int(box.y+box.height))
}

// closeScreenshotUI drops the frozen frames and keeps the selection.
func (c *Compositor) closeScreenshotUI() {
	ui := c.shotUI
	if ui == nil {
		return
	}
	c.shotUI = nil
	c.lastShotSelection = ui.selection
	for _, frame := range ui.frames {
		C.wlr_buffer_unlock(frame.buffer)
	}
	C.sysc_shot_tree_destroy(ui.tree)
	c.refocusPointer(nowMsec())
}

// confirmScreenshot captures the selection from its frozen frame and saves it
// as the other screenshots are saved.
func (c *Compositor) confirmScreenshot(writeToDisk bool) {
	ui := c.shotUI
	frame := ui.frames[ui.output]
	sel := ui.selection.Intersect(frame.box).Sub(frame.box.Min)
	box := image.Rect(int(float64(sel.Min.X)*frame.scale), int(float64(sel.Min.Y)*frame.scale),
		int(float64(sel.Max.X)*frame.scale), int(float64(sel.Max.Y)*frame.scale))
	img, err := c.readBuffer(frame.buffer, box)
	path := ui.path
	c.closeScreenshotUI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: screenshot: %v\n", err)
		return
	}
	c.saveScreenshot(img, writeToDisk, path)
}

// shotUIKey takes a user key press while the UI is open; every key is the
// UI's. It reports whether the UI took the key.
func (c *Compositor) shotUIKey(keycode uint32) bool {
	if c.shotUI == nil {
		return false
	}
	switch {
	case c.keysymIs(keycode, "Escape"):
		c.closeScreenshotUI()
	case c.keysymIs(keycode, "space"), c.keysymIs(keycode, "Return"):
		if !c.shotUI.selection.Empty() {
			c.confirmScreenshot(true)
		}
	}
	return true
}

// shotUIButton takes a user pointer button while the UI is open: a left press
// starts a new selection at the pointer and the release ends it.
func (c *Compositor) shotUIButton(button uint32, pressed bool) bool {
	ui := c.shotUI
	if ui == nil {
		return false
	}
	if button != evdevButtonLeft {
		return true
	}
	if !pressed {
		ui.dragging = false
		return true
	}
	cursor := image.Pt(int(c.cursor.x), int(c.cursor.y))
	for pointer, frame := range ui.frames {
		if cursor.In(frame.box) {
			ui.output, ui.anchor, ui.dragging = pointer, cursor, true
			ui.selection = image.Rectangle{Min: cursor, Max: cursor}
			c.drawShotSelection()
		}
	}
	return true
}

// shotUIMotion grows a dragged selection to the pointer, within the output
// the drag started on.
func (c *Compositor) shotUIMotion() bool {
	ui := c.shotUI
	if ui == nil {
		return false
	}
	if ui.dragging {
		box := ui.frames[ui.output].box
		cursor := image.Pt(int(c.cursor.x), int(c.cursor.y))
		ui.selection = image.Rectangle{Min: ui.anchor, Max: cursor}.Canon().Intersect(box)
		c.drawShotSelection()
	}
	return true
}
