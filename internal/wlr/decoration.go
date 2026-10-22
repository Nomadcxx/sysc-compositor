package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_xdg_decoration_v1.h>
#include <wlr/types/wlr_server_decoration.h>
#include <wlr/util/edges.h>

static void sysc_xdg_toplevel_set_tiled_edges(struct wlr_xdg_toplevel *toplevel,
		uint32_t tiled_edges) {
	if (toplevel->base->initialized) {
		wlr_xdg_toplevel_set_tiled(toplevel, tiled_edges);
	} else {
		toplevel->scheduled.tiled = tiled_edges;
	}
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
	sysc_xdg_toplevel_set_tiled_edges(decoration->toplevel,
		WLR_EDGE_TOP | WLR_EDGE_RIGHT | WLR_EDGE_BOTTOM | WLR_EDGE_LEFT);
}

static void sysc_xdg_decoration_apply_request(
		struct wlr_xdg_toplevel_decoration_v1 *decoration) {
	enum wlr_xdg_toplevel_decoration_v1_mode mode = decoration->requested_mode;
	if (mode == WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_NONE) {
		mode = WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE;
	}
	if (decoration->toplevel->base->initialized) {
		wlr_xdg_toplevel_decoration_v1_set_mode(decoration, mode);
	} else {
		decoration->scheduled_mode = mode;
	}
	uint32_t tiled_edges = mode == WLR_XDG_TOPLEVEL_DECORATION_V1_MODE_SERVER_SIDE ?
		WLR_EDGE_TOP | WLR_EDGE_RIGHT | WLR_EDGE_BOTTOM | WLR_EDGE_LEFT : WLR_EDGE_NONE;
	sysc_xdg_toplevel_set_tiled_edges(decoration->toplevel, tiled_edges);
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func (c *Compositor) createDecorationManager() error {
	if !c.preferNoCSD {
		return nil
	}
	kdeManager := C.wlr_server_decoration_manager_create(c.display)
	if kdeManager == nil {
		return errors.New("wlr_server_decoration_manager_create failed")
	}
	C.wlr_server_decoration_manager_set_default_mode(kdeManager,
		C.WLR_SERVER_DECORATION_MANAGER_MODE_SERVER)
	c.kdeDecorationManager = unsafe.Pointer(kdeManager)

	manager := C.wlr_xdg_decoration_manager_v1_create(c.display)
	if manager == nil {
		return errors.New("wlr_xdg_decoration_manager_v1_create failed")
	}
	c.xdgDecorationManager = unsafe.Pointer(manager)
	newDecoration := AddListener(unsafe.Pointer(&manager.events.new_toplevel_decoration), c.handleNewDecoration)
	c.keep(newDecoration)
	return nil
}

func (c *Compositor) handleNewDecoration(data unsafe.Pointer) {
	decoration := (*C.struct_wlr_xdg_toplevel_decoration_v1)(data)
	if c.preferNoCSD {
		C.sysc_xdg_decoration_set_initial_preference(decoration)
	}
	var requestMode, destroy *Listener
	requestMode = AddListener(unsafe.Pointer(&decoration.events.request_mode), func(unsafe.Pointer) {
		C.sysc_xdg_decoration_apply_request(decoration)
	})
	destroy = AddListener(unsafe.Pointer(&decoration.events.destroy), func(unsafe.Pointer) {
		c.drop(requestMode)
		c.drop(destroy)
	})
	c.keep(requestMode, destroy)
}
