package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_xdg_activation_v1.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_scene.h>

// The scene graph does the surface tracking, the buffer plumbing and the
// subsurface tree. wlroots 0.20 no longer takes the layout here; the scene owns
// output placement. An xdg surface's tree origin is its window-geometry origin,
// and a popup's tree sits at its geometry inside its parent's tree.
static struct wlr_scene_tree *sysc_scene_xdg_create(struct wlr_scene_tree *parent,
		struct wlr_xdg_surface *xdg_surface) {
	return wlr_scene_xdg_surface_create(parent, xdg_surface);
}

static struct wlr_scene_tree *sysc_tree_parent(struct wlr_scene_tree *tree) {
	return tree->node.parent;
}

static void sysc_tree_coords(struct wlr_scene_tree *tree, int *x, int *y) {
	wlr_scene_node_coords(&tree->node, x, y);
}

// role-specific pointers sit in an anonymous union cgo cannot name.
static struct wlr_xdg_popup *sysc_xdg_popup(struct wlr_xdg_surface *xdg) {
	return xdg->role == WLR_XDG_SURFACE_ROLE_POPUP ? xdg->popup : NULL;
}

static struct wlr_xdg_surface *sysc_popup_parent_xdg(struct wlr_xdg_popup *popup) {
	return popup->parent == NULL ? NULL : wlr_xdg_surface_try_from_wlr_surface(popup->parent);
}

// sysc_popup_root is the surface a popup chain hangs from: a toplevel or a
// layer surface. Its surface coordinates are the ones
// wlr_xdg_popup_unconstrain_from_box expects.
static struct wlr_surface *sysc_popup_root(struct wlr_xdg_popup *popup) {
	struct wlr_surface *surface = popup->parent;
	struct wlr_xdg_surface *xdg;
	while (surface != NULL && (xdg = wlr_xdg_surface_try_from_wlr_surface(surface)) != NULL &&
			xdg->role == WLR_XDG_SURFACE_ROLE_POPUP && xdg->popup != NULL) {
		surface = xdg->popup->parent;
	}
	return surface;
}

// sysc_window_surface is the surface a subsurface or popup belongs to: the
// root of its subsurface tree, then up popup parents, which are not
// subsurfaces, until a toplevel or layer surface.
static struct wlr_surface *sysc_window_surface(struct wlr_surface *surface) {
	surface = wlr_surface_get_root_surface(surface);
	struct wlr_xdg_surface *xdg;
	while ((xdg = wlr_xdg_surface_try_from_wlr_surface(surface)) != NULL &&
			xdg->role == WLR_XDG_SURFACE_ROLE_POPUP && xdg->popup != NULL &&
			xdg->popup->parent != NULL) {
		surface = wlr_surface_get_root_surface(xdg->popup->parent);
	}
	return surface;
}

// sysc_output_box is the box of the output at (x, y), or of the output nearest
// to it when the point is on none.
static bool sysc_output_box(struct wlr_output_layout *layout, double x, double y,
		struct wlr_box *box) {
	double cx = x, cy = y;
	wlr_output_layout_closest_point(layout, NULL, x, y, &cx, &cy);
	struct wlr_output *output = wlr_output_layout_output_at(layout, cx, cy);
	if (output == NULL) {
		return false;
	}
	wlr_output_layout_get_box(layout, output, box);
	return true;
}

static void sysc_output_layout_box(struct wlr_output_layout *layout,
		struct wlr_output *output, struct wlr_box *box) {
	wlr_output_layout_get_box(layout, output, box);
}

static void sysc_layout_box(struct wlr_output_layout *layout, struct wlr_box *box) {
	wlr_output_layout_get_box(layout, NULL, box);
}
*/
import "C"

import "unsafe"

// ensureSceneTree gives an xdg toplevel its tree in the windows layer. Popups
// get theirs from handleNewPopup, under their parent.
func (c *Compositor) ensureSceneTree(xdgSurface *C.struct_wlr_xdg_surface) *C.struct_wlr_scene_tree {
	if tree := c.sceneTrees[unsafe.Pointer(xdgSurface)]; tree != nil {
		return tree
	}
	return c.trackXDGTree(xdgSurface, C.sysc_scene_xdg_create(c.layerTrees[layerWindows], xdgSurface))
}

// trackXDGTree records an xdg surface's tree until the surface is destroyed.
func (c *Compositor) trackXDGTree(xdgSurface *C.struct_wlr_xdg_surface, tree *C.struct_wlr_scene_tree) *C.struct_wlr_scene_tree {
	if tree == nil {
		return nil
	}
	key := unsafe.Pointer(xdgSurface)
	c.mapped[key] = true
	c.sceneTrees[key] = tree
	var destroy *Listener
	destroy = AddListener(unsafe.Pointer(&xdgSurface.events.destroy), func(unsafe.Pointer) {
		delete(c.mapped, key)
		delete(c.sceneTrees, key)
		c.drop(destroy)
	})
	c.keep(destroy)
	return tree
}

// handleNewPopup waits for the popup's initial commit, then draws it under its
// parent's tree and fits it inside the output. wlroots sends a popup no
// configure until the compositor schedules one, so without this a menu never
// maps at all.
func (c *Compositor) handleNewPopup(data unsafe.Pointer) {
	popup := (*C.struct_wlr_xdg_popup)(data)
	c.popups[popup] = struct{}{}
	var commit, reposition, damage, destroy *Listener
	damage = AddListener(unsafe.Pointer(&popup.base.surface.events.commit), func(unsafe.Pointer) {
		c.damageCaptureSources()
	})
	commit = AddListener(unsafe.Pointer(&popup.base.surface.events.commit), func(unsafe.Pointer) {
		if !bool(popup.base.initial_commit) {
			return
		}
		c.drop(commit)
		commit = nil
		parent := c.surfaceTree(popup.parent)
		if parent == nil {
			// Its parent is nothing this compositor draws.
			C.wlr_xdg_popup_destroy(popup)
			return
		}
		tree := c.trackXDGTree(popup.base, C.sysc_scene_xdg_create(parent, popup.base))
		if view := c.viewForSurface(popup.parent); tree != nil && view != nil && view.clipToGeometry {
			sceneFXSetSubsurfaceClip(unsafe.Pointer(&tree.node), false, 0, 0, 0, 0)
		}
		c.unconstrainPopup(popup)
	})
	reposition = AddListener(unsafe.Pointer(&popup.events.reposition), func(unsafe.Pointer) {
		c.unconstrainPopup(popup)
	})
	destroy = AddListener(unsafe.Pointer(&popup.events.destroy), func(unsafe.Pointer) {
		delete(c.popups, popup)
		for _, l := range []*Listener{commit, reposition, damage, destroy} {
			if l != nil {
				c.drop(l)
			}
		}
	})
	c.keep(commit, reposition, damage, destroy)
}

func (c *Compositor) clearPopupClips(view *toplevelView) {
	for popup := range c.popups {
		if c.viewForSurface(popup.parent) != view {
			continue
		}
		if tree := c.sceneTrees[unsafe.Pointer(popup.base)]; tree != nil {
			sceneFXSetSubsurfaceClip(unsafe.Pointer(&tree.node), false, 0, 0, 0, 0)
		}
	}
}

// dismissGrabbedPopups destroys every popup holding a grab on seat, or on any
// seat when seat is nil, so the seat can then be destroyed.
//
// wlroots 0.20 (through master as of 2026-10-16) frees a popup grab twice
// when its seat is destroyed with the grab open: the seat's destroy handler
// calls destroy_xdg_popup_grab, which destroys each popup, whose reset
// destroys the now-empty grab again (types/xdg_shell/wlr_xdg_popup.c). Popups
// destroyed first take the ordinary path, which ends the grab once.
func (c *Compositor) dismissGrabbedPopups(seat *C.struct_wlr_seat) {
	for {
		var victim *C.struct_wlr_xdg_popup
		for p := range c.popups {
			if p.seat != nil && (seat == nil || p.seat == seat) {
				victim = p
				break
			}
		}
		if victim == nil {
			return
		}
		C.wlr_xdg_popup_destroy(victim)
		// The destroy listener forgets it; this guards against a popup
		// whose listener is already gone looping forever.
		delete(c.popups, victim)
	}
}

// unconstrainPopup fits a popup inside the output its root window is on, then
// schedules the configure that carries the result.
func (c *Compositor) unconstrainPopup(popup *C.struct_wlr_xdg_popup) {
	defer C.wlr_xdg_surface_schedule_configure(popup.base)
	box, _, _, ok := c.popupConstraint(popup)
	if !ok {
		return
	}
	cbox := C.struct_wlr_box{x: C.int(box.x), y: C.int(box.y), width: C.int(box.w), height: C.int(box.h)}
	C.wlr_xdg_popup_unconstrain_from_box(popup, &cbox)
}

// popupConstraint is the output a popup must stay inside, in the surface
// coordinates of the window its chain hangs from (what
// wlr_xdg_popup_unconstrain_from_box takes), and that window's surface origin
// in layout coordinates.
func (c *Compositor) popupConstraint(popup *C.struct_wlr_xdg_popup) (box rect, ox, oy int, ok bool) {
	ox, oy, ok = c.surfaceOrigin(C.sysc_popup_root(popup))
	if !ok {
		return rect{}, 0, 0, false
	}
	var out C.struct_wlr_box
	if !bool(C.sysc_output_box(c.layout, C.double(ox), C.double(oy), &out)) {
		return rect{}, 0, 0, false
	}
	return rect{int(out.x) - ox, int(out.y) - oy, int(out.width), int(out.height)}, ox, oy, true
}

// surfaceTree is the scene tree drawn for a toplevel, popup or layer surface.
func (c *Compositor) surfaceTree(surface *C.struct_wlr_surface) *C.struct_wlr_scene_tree {
	if surface == nil {
		return nil
	}
	if xdg := C.wlr_xdg_surface_try_from_wlr_surface(surface); xdg != nil {
		return c.sceneTrees[unsafe.Pointer(xdg)]
	}
	return c.layerSurfaceTree(surface)
}

// surfaceOrigin is where a toplevel's or layer surface's own surface
// coordinates start, in layout coordinates.
func (c *Compositor) surfaceOrigin(surface *C.struct_wlr_surface) (int, int, bool) {
	tree := c.surfaceTree(surface)
	if tree == nil {
		return 0, 0, false
	}
	var x, y C.int
	C.sysc_tree_coords(tree, &x, &y)
	if xdg := C.wlr_xdg_surface_try_from_wlr_surface(surface); xdg != nil {
		x -= xdg.geometry.x
		y -= xdg.geometry.y
	}
	return int(x), int(y), true
}

// viewForSurface is the toplevel view a surface, subsurface or popup belongs
// to, or nil.
func (c *Compositor) viewForSurface(surface *C.struct_wlr_surface) *toplevelView {
	if surface == nil {
		return nil
	}
	toplevel := C.wlr_xdg_toplevel_try_from_wlr_surface(C.sysc_window_surface(surface))
	if toplevel == nil {
		return nil
	}
	return c.views[unsafe.Pointer(toplevel)]
}

// applyFullscreen answers a toplevel's fullscreen request. Fullscreen covers
// the requested output when it is connected, otherwise the assigned or default
// output. Leaving it hands the size back to the client and restores saved
// floating placement when needed.
func (c *Compositor) applyFullscreen(view *toplevelView) {
	toplevel := view.toplevel
	fullscreen := bool(toplevel.requested.fullscreen)
	focusedOutputBefore := ""
	if view.mapped && c.model != nil {
		focusedOutputBefore = c.model.Snapshot().FocusedOutput
		c.model.SetFullscreen(view.id, fullscreen)
	}
	if !fullscreen {
		C.wlr_xdg_toplevel_set_fullscreen(toplevel, false)
		C.wlr_xdg_toplevel_set_size(toplevel, 0, 0)
		return
	}
	requestedOutput := toplevel.requested.fullscreen_output
	if requestedOutput != nil {
		if _, connected := c.sceneOutputs[unsafe.Pointer(requestedOutput)]; !connected {
			requestedOutput = nil
		}
	}
	// Use the assigned/default output when the request omitted or named a
	// disconnected output, but only an explicit connected request may move it.
	output := requestedOutput
	if output == nil {
		if view.mapped {
			name := c.outputNameForView(view)
			for candidate := range c.sceneOutputs {
				if C.GoString((*C.struct_wlr_output)(candidate).name) == name {
					output = (*C.struct_wlr_output)(candidate)
					break
				}
			}
		} else {
			output = c.defaultOutput()
		}
	}
	if output == nil {
		C.wlr_xdg_toplevel_set_fullscreen(toplevel, true)
		return
	}
	if view.mapped && c.model != nil {
		if requestedOutput != nil {
			requestedName := C.GoString(requestedOutput.name)
			if requestedName != c.outputNameForView(view) {
				c.model.MoveWindowToOutput(view.id, requestedName, true)
			}
		}
	}
	C.wlr_xdg_toplevel_set_fullscreen(toplevel, true)
	var box C.struct_wlr_box
	C.sysc_output_layout_box(c.layout, output, &box)
	C.wlr_xdg_toplevel_set_size(toplevel, box.width, box.height)
	c.placeView(view, int(box.x), int(box.y))
	if view.mapped && c.model != nil {
		c.applyWorkspaceModel()
		// Moving the focused window can move model focus to the requested
		// output. Mirror that output without changing keyboard focus, which
		// may still belong to an on-demand layer surface.
		if focused := c.model.Snapshot().FocusedOutput; focused != focusedOutputBefore {
			c.setActiveOutputName(focused)
		}
	}
}

func (c *Compositor) handleActivationRequest(data unsafe.Pointer) {
	event := (*C.struct_wlr_xdg_activation_v1_request_activate_event)(data)
	if event.token == nil || event.surface == nil {
		return
	}
	view := c.viewForSurface(event.surface)
	c.applyActivationRequest(view, event.token.seat != nil)
}

func (c *Compositor) applyActivationRequest(view *toplevelView, hasSerial bool) bool {
	if c.model == nil || view == nil || !view.mapped || hasSerial {
		return false
	}
	return c.SetWindowUrgent(view.id, true)
}

// fullscreenState is a view's acknowledged fullscreen state and window
// geometry in layout coordinates, for tests.
func (c *Compositor) fullscreenState(view *toplevelView) (bool, rect) {
	x, y := view.treePosition()
	g := view.geometry()
	return bool(view.toplevel.current.fullscreen), rect{x, y, g.w, g.h}
}

// Mapped reports whether an xdg_surface reached the scene graph. It exists so a
// test can assert a client mapped without reading the pixels back.
func (c *Compositor) Mapped() int { return len(c.mapped) }

// newSurfaceSignal is the xdg_wm_base new_surface signal, used by the handle
// ownership test to attach a listener to a real signal.
func (c *Compositor) newSurfaceSignal() Signal {
	return unsafe.Pointer(&(*C.struct_wlr_xdg_shell)(c.xdgShell).events.new_surface)
}

func (c *Compositor) newPopupSignal() Signal {
	return unsafe.Pointer(&(*C.struct_wlr_xdg_shell)(c.xdgShell).events.new_popup)
}

func (c *Compositor) newToplevelSignal() Signal {
	return unsafe.Pointer(&(*C.struct_wlr_xdg_shell)(c.xdgShell).events.new_toplevel)
}

// rect is a box in layout coordinates.
type rect struct{ x, y, w, h int }

func (r rect) contains(o rect) bool {
	return o.x >= r.x && o.y >= r.y && o.x+o.w <= r.x+r.w && o.y+o.h <= r.y+r.h
}

// popupPlacement is where one mapped xdg popup sits, for tests.
type popupPlacement struct {
	underParent   bool // its scene tree is a child of its parent surface's tree
	box           rect // its window geometry in layout coordinates
	constraint    rect // the box it was unconstrained against, root-surface relative
	rootX, rootY  int  // the root window's surface origin in layout coordinates
	hasConstraint bool
}

// popupPlacements reports every mapped xdg popup that has a scene tree.
func (c *Compositor) popupPlacements() []popupPlacement {
	var out []popupPlacement
	for key, tree := range c.sceneTrees {
		xdg := (*C.struct_wlr_xdg_surface)(key)
		popup := C.sysc_xdg_popup(xdg)
		if popup == nil || !bool(xdg.surface.mapped) {
			continue
		}
		parent := C.sysc_popup_parent_xdg(popup)
		var x, y C.int
		C.sysc_tree_coords(tree, &x, &y)
		g := xdg.geometry
		constraint, rx, ry, ok := c.popupConstraint(popup)
		out = append(out, popupPlacement{
			underParent:   parent != nil && C.sysc_tree_parent(tree) == c.sceneTrees[unsafe.Pointer(parent)],
			box:           rect{int(x), int(y), int(g.width), int(g.height)},
			constraint:    constraint,
			rootX:         rx,
			rootY:         ry,
			hasConstraint: ok,
		})
	}
	return out
}

// layoutBox is the box every output covers, for tests.
func (c *Compositor) layoutBox() rect {
	var b C.struct_wlr_box
	C.sysc_layout_box(c.layout, &b)
	return rect{int(b.x), int(b.y), int(b.width), int(b.height)}
}
