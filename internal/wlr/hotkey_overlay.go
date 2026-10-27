package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5 xkbcommon
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <xkbcommon/xkbcommon.h>
#include <scenefx/types/wlr_scene.h>

// sysc_keysym_canonical_name is Niri's keysym parse then keysym_get_name: a
// case-insensitive lookup, except that XF86Screensaver and XF86ScreenSaver
// have no case mapping, so the exact spelling wins and the uppercase one is
// the fallback.
static int sysc_keysym_canonical_name(const char *name, char *out, size_t size) {
	xkb_keysym_t sym = xkb_keysym_from_name(name, XKB_KEYSYM_CASE_INSENSITIVE);
	if (sym == XKB_KEY_XF86Screensaver) {
		sym = xkb_keysym_from_name(name, XKB_KEYSYM_NO_FLAGS);
		if (sym == XKB_KEY_NoSymbol) {
			sym = XKB_KEY_XF86ScreenSaver;
		}
	}
	if (sym == XKB_KEY_NoSymbol) {
		return -1;
	}
	return xkb_keysym_get_name(sym, out, size);
}

static void *sysc_overlay_tree_create(struct wlr_scene_tree *parent, struct wlr_scene_node *below) {
	struct wlr_scene_tree *tree = wlr_scene_tree_create(parent);
	if (tree != NULL) {
		wlr_scene_node_place_below(&tree->node, below);
	}
	return tree;
}
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"os"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
)

// The hotkey overlay (Niri's hotkey_overlay.rs): a panel of the important
// binds centred on every output, over windows and layers but under the
// screenshot UI and the lock screen. It opens at startup unless
// skip-at-startup is set, and on show-hotkey-overlay. It takes no input:
// the next key press, button press or scroll still goes where it would, and
// closes it.

type hotkeyOverlay struct {
	open         bool
	binds        []config.OverlayBind
	hideNotBound bool
	tree         *C.struct_wlr_scene_tree
	panels       map[float64]*image.RGBA // by output scale; nil entries could not be drawn
}

// HotkeyOverlayOpen reports whether the hotkey overlay is up.
func (c *Compositor) HotkeyOverlayOpen() bool { return c.hotkeyOverlay.open }

// ShowHotkeyOverlay is Niri's ShowHotkeyOverlay action.
func (c *Compositor) ShowHotkeyOverlay() {
	if !c.hotkeyOverlay.open {
		c.hotkeyOverlay.open = true
		c.syncHotkeyOverlay()
	}
}

func (c *Compositor) hideHotkeyOverlay() {
	if c.hotkeyOverlay.open {
		c.hotkeyOverlay.open = false
		c.syncHotkeyOverlay()
	}
}

// configureHotkeyOverlay takes the binds the overlay lists, and drops the
// panels drawn from the old ones.
func (c *Compositor) configureHotkeyOverlay(binds []config.OverlayBind, settings config.HotkeyOverlay) {
	c.hotkeyOverlay.binds = binds
	c.hotkeyOverlay.hideNotBound = settings.HideNotBound
	c.hotkeyOverlay.panels = nil
	c.syncHotkeyOverlay()
}

// keyName is the canonical XKB name of a bind's key, as Niri prints it.
func keyName(name string) string {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var buf [64]C.char
	if C.sysc_keysym_canonical_name(cname, &buf[0], C.size_t(len(buf))) < 0 {
		return name
	}
	return C.GoString(&buf[0])
}

func (c *Compositor) hotkeyOverlayPanel(scale float64) *image.RGBA {
	o := &c.hotkeyOverlay
	if img, ok := o.panels[scale]; ok {
		return img
	}
	lines := ui.HotkeyLines(o.binds, o.hideNotBound, c.modKey, keyName)
	img, err := ui.HotkeyOverlay(lines, scale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: hotkey overlay: %v\n", err)
		img = nil
	}
	if o.panels == nil {
		o.panels = map[float64]*image.RGBA{}
	}
	o.panels[scale] = img
	return img
}

// syncHotkeyOverlay redraws the overlay for the current outputs and state.
func (c *Compositor) syncHotkeyOverlay() {
	o := &c.hotkeyOverlay
	if o.tree != nil || o.open {
		defer c.damageCaptureSources()
	}
	if o.tree != nil {
		C.wlr_scene_node_destroy(&o.tree.node)
		o.tree = nil
	}
	if !o.open || len(c.sceneOutputs) == 0 {
		return
	}
	below := &c.lockTree.node
	if c.shotUI != nil {
		below = &(*C.struct_wlr_scene_tree)(c.shotUI.tree).node
	}
	o.tree = (*C.struct_wlr_scene_tree)(C.sysc_overlay_tree_create(&c.scene.tree, below))
	if o.tree == nil {
		return
	}
	for pointer := range c.sceneOutputs {
		if !c.outputIsEnabled(pointer) {
			continue
		}
		scale := float64((*C.struct_wlr_output)(pointer).scale)
		img := c.hotkeyOverlayPanel(scale)
		if img == nil {
			continue
		}
		node := sceneImage(o.tree, img)
		if node == nil {
			continue
		}
		sceneImageIgnoreInput(node)
		box := c.outputLayoutBox(pointer)
		size := image.Pt(int(math.Round(float64(img.Rect.Dx())/scale)), int(math.Round(float64(img.Rect.Dy())/scale)))
		at := box.Min.Add(image.Pt(max(0, (box.Dx()-size.X)/2), max(0, (box.Dy()-size.Y)/2)))
		sceneImagePlace(node, image.Rectangle{Min: at, Max: at.Add(size)}, ui.HotkeyOverlayOpacity)
	}
}
