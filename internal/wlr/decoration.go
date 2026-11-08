package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_xdg_decoration_v1.h>
#include <wlr/types/wlr_server_decoration.h>
#include <wlr/util/edges.h>

// wlr_xdg_toplevel_set_tiled schedules a configure every time, so an
// unchanged state is left alone.
static void sysc_xdg_toplevel_set_tiled_edges(struct wlr_xdg_toplevel *toplevel,
		uint32_t tiled_edges) {
	if (!toplevel->base->initialized) {
		toplevel->scheduled.tiled = tiled_edges;
	} else if (toplevel->scheduled.tiled != tiled_edges) {
		wlr_xdg_toplevel_set_tiled(toplevel, tiled_edges);
	}
}

static bool sysc_kde_decoration_is_server(struct wlr_server_decoration_manager *manager,
		struct wlr_surface *surface) {
	struct wlr_server_decoration *decoration;
	wl_list_for_each(decoration, &manager->decorations, link) {
		if (decoration->surface == surface) {
			return decoration->mode == WLR_SERVER_DECORATION_MANAGER_MODE_SERVER;
		}
	}
	return false;
}

static void sysc_xdg_decoration_set_initial_preference(
		struct wlr_xdg_toplevel_decoration_v1 *decoration) {
	enum wlr_xdg_toplevel_decoration_v1_mode mode =
		WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE;
	if (decoration->toplevel->base->initialized) {
		wlr_xdg_toplevel_decoration_v1_set_mode(decoration, mode);
	} else {
		decoration->scheduled_mode = mode;
	}
}

static void sysc_xdg_decoration_apply_request(
		struct wlr_xdg_toplevel_decoration_v1 *decoration, bool prefer_no_csd) {
	enum wlr_xdg_toplevel_decoration_v1_mode mode = decoration->requested_mode;
	if (mode == WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_NONE) {
		mode = prefer_no_csd ? WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE :
			WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_CLIENT_SIDE;
	}
	if (decoration->toplevel->base->initialized) {
		wlr_xdg_toplevel_decoration_v1_set_mode(decoration, mode);
	} else {
		decoration->scheduled_mode = mode;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func (c *Compositor) createDecorationManager(prefer bool) error {
	if !prefer {
		return nil
	}
	if c.kdeDecorationManager == nil {
		kdeManager := C.wlr_server_decoration_manager_create(c.display)
		if kdeManager == nil {
			return errors.New("wlr_server_decoration_manager_create failed")
		}
		c.kdeDecorationManager = unsafe.Pointer(kdeManager)
	}
	if c.xdgDecorationManager == nil {
		manager := C.wlr_xdg_decoration_manager_v1_create(c.display)
		if manager == nil {
			return errors.New("wlr_xdg_decoration_manager_v1_create failed")
		}
		c.xdgDecorationManager = unsafe.Pointer(manager)
		newDecoration := AddListener(unsafe.Pointer(&manager.events.new_toplevel_decoration), c.handleNewDecoration)
		c.keep(newDecoration)
	}
	C.wlr_server_decoration_manager_set_default_mode(
		(*C.struct_wlr_server_decoration_manager)(c.kdeDecorationManager),
		C.WLR_SERVER_DECORATION_MANAGER_MODE_SERVER)
	return nil
}

func (c *Compositor) setDecorationPreference(prefer bool) error {
	if prefer && c.xdgDecorationManager == nil {
		if err := c.createDecorationManager(prefer); err != nil {
			return err
		}
	}
	if c.kdeDecorationManager != nil {
		mode := C.uint32_t(C.WLR_SERVER_DECORATION_MANAGER_MODE_CLIENT)
		if prefer {
			mode = C.uint32_t(C.WLR_SERVER_DECORATION_MANAGER_MODE_SERVER)
		}
		C.wlr_server_decoration_manager_set_default_mode(
			(*C.struct_wlr_server_decoration_manager)(c.kdeDecorationManager), mode)
	}
	return nil
}

func (c *Compositor) handleNewDecoration(data unsafe.Pointer) {
	decoration := (*C.struct_wlr_xdg_toplevel_decoration_v1)(data)
	if c.preferNoCSD {
		C.sysc_xdg_decoration_set_initial_preference(decoration)
	}
	if c.toplevelDecorations == nil {
		c.toplevelDecorations = make(map[*C.struct_wlr_xdg_toplevel]*C.struct_wlr_xdg_toplevel_decoration_v1)
	}
	c.toplevelDecorations[decoration.toplevel] = decoration
	syncTiled := func() {
		if view := c.views[unsafe.Pointer(decoration.toplevel)]; view != nil {
			c.syncTiledState(view)
		}
	}
	syncTiled()
	var requestMode, destroy *Listener
	requestMode = AddListener(unsafe.Pointer(&decoration.events.request_mode), func(unsafe.Pointer) {
		C.sysc_xdg_decoration_apply_request(decoration, C.bool(c.preferNoCSD))
		syncTiled()
	})
	destroy = AddListener(unsafe.Pointer(&decoration.events.destroy), func(unsafe.Pointer) {
		delete(c.toplevelDecorations, decoration.toplevel)
		c.drop(requestMode)
		c.drop(destroy)
	})
	c.keep(requestMode, destroy)
}

// serverSideDecorated reports whether the window's current decoration mode is
// server-side, Niri's has_ssd.
func (c *Compositor) serverSideDecorated(view *toplevelView) bool {
	decoration := c.toplevelDecorations[view.toplevel]
	return decoration != nil && decoration.current.mode == C.WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE
}

// syncTiledState tells a toplevel whether it is tiled, Niri's
// update_tiled_state. A tiled client drops its rounded corners and shadows,
// even one that ignores the decoration protocols. A tiled-state rule decides
// when it is set. Otherwise a window that negotiated an xdg-decoration mode
// is tiled when that mode is server-side, and any other window follows
// prefer-no-csd or its KDE server decoration.
func (c *Compositor) syncTiledState(view *toplevelView) {
	if view == nil || view.toplevel == nil {
		return
	}
	tiled := c.preferNoCSD
	decoration := c.toplevelDecorations[view.toplevel]
	switch {
	case view.appearance.TiledState != nil:
		tiled = *view.appearance.TiledState
	case decoration != nil && decoration.scheduled_mode != C.WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_NONE:
		tiled = decoration.scheduled_mode == C.WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE
	case c.kdeDecorationManager != nil && bool(C.sysc_kde_decoration_is_server(
		(*C.struct_wlr_server_decoration_manager)(c.kdeDecorationManager), view.toplevel.base.surface)):
		tiled = true
	}
	edges := C.uint32_t(C.WLR_EDGE_NONE)
	if tiled {
		edges = C.WLR_EDGE_TOP | C.WLR_EDGE_RIGHT | C.WLR_EDGE_BOTTOM | C.WLR_EDGE_LEFT
	}
	C.sysc_xdg_toplevel_set_tiled_edges(view.toplevel, edges)
}
