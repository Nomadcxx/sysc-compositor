// Package testclient provides the small Wayland client used by compositor
// lifecycle tests. It is imported only from _test.go files.
package testclient

/*
#cgo pkg-config: wayland-client

#include <fcntl.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// The generated xdg client bindings are intentionally avoided in this small
// test client. These are the protocol messages it uses at version 1.
static const struct wl_interface sysc_xdg_surface_interface;
static const struct wl_interface sysc_xdg_toplevel_interface;

static const struct wl_interface *sysc_xdg_wm_base_surface_types[] = {
	&sysc_xdg_surface_interface, &wl_surface_interface,
};
static const struct wl_message sysc_xdg_wm_base_requests[] = {
	{"destroy", "", NULL},
	{"create_positioner", "n", NULL},
	{"get_xdg_surface", "no", sysc_xdg_wm_base_surface_types},
	{"pong", "u", NULL},
};
static const struct wl_message sysc_xdg_wm_base_events[] = {
	{"ping", "u", NULL},
};
static const struct wl_interface sysc_xdg_wm_base_interface = {
	"xdg_wm_base", 1, 4, sysc_xdg_wm_base_requests, 1, sysc_xdg_wm_base_events,
};

static const struct wl_interface *sysc_xdg_surface_toplevel_types[] = {
	&sysc_xdg_toplevel_interface,
};
static const struct wl_message sysc_xdg_surface_requests[] = {
	{"destroy", "", NULL},
	{"get_toplevel", "n", sysc_xdg_surface_toplevel_types},
	{"get_popup", "", NULL},
	{"set_window_geometry", "", NULL},
	{"ack_configure", "u", NULL},
};
static const struct wl_message sysc_xdg_surface_events[] = {
	{"configure", "u", NULL},
};
static const struct wl_interface sysc_xdg_surface_interface = {
	"xdg_surface", 1, 5, sysc_xdg_surface_requests, 1, sysc_xdg_surface_events,
};

static const struct wl_message sysc_xdg_toplevel_requests[] = {
	{"destroy", "", NULL},
	{"set_parent", "", NULL},
	{"set_title", "s", NULL},
	{"set_app_id", "s", NULL},
	{"show_window_menu", "", NULL},
	{"move", "", NULL},
	{"resize", "", NULL},
	{"set_max_size", "", NULL},
	{"set_min_size", "", NULL},
	{"set_maximized", "", NULL},
	{"unset_maximized", "", NULL},
	{"set_fullscreen", "", NULL},
	{"unset_fullscreen", "", NULL},
	{"set_minimized", "", NULL},
};
static const struct wl_message sysc_xdg_toplevel_events[] = {
	{"configure", "iia", NULL},
	{"close", "", NULL},
};
static const struct wl_interface sysc_xdg_toplevel_interface = {
	"xdg_toplevel", 1, 14, sysc_xdg_toplevel_requests, 2, sysc_xdg_toplevel_events,
};

static const struct wl_interface sysc_xdg_toplevel_decoration_interface;
static const struct wl_interface *sysc_xdg_decoration_manager_get_toplevel_types[] = {
	&sysc_xdg_toplevel_decoration_interface, &sysc_xdg_toplevel_interface,
};
static const struct wl_message sysc_xdg_decoration_manager_requests[] = {
	{"destroy", "", NULL},
	{"get_toplevel_decoration", "no", sysc_xdg_decoration_manager_get_toplevel_types},
};
static const struct wl_interface sysc_xdg_decoration_manager_interface = {
	"zxdg_decoration_manager_v1", 1, 2, sysc_xdg_decoration_manager_requests, 0, NULL,
};
static const struct wl_message sysc_xdg_toplevel_decoration_requests[] = {
	{"destroy", "", NULL},
	{"set_mode", "u", NULL},
	{"unset_mode", "", NULL},
};
static const struct wl_message sysc_xdg_toplevel_decoration_events[] = {
	{"configure", "u", NULL},
};
static const struct wl_interface sysc_xdg_toplevel_decoration_interface = {
	"zxdg_toplevel_decoration_v1", 1, 3, sysc_xdg_toplevel_decoration_requests,
	1, sysc_xdg_toplevel_decoration_events,
};

struct sysc_client_globals {
	struct wl_compositor *compositor;
	struct wl_subcompositor *subcompositor;
	struct wl_shm *shm;
	struct wl_proxy *xdg_wm_base;
	struct wl_proxy *xdg_decoration_manager;
};

static void sysc_client_global(void *data, struct wl_registry *registry,
		uint32_t name, const char *interface, uint32_t version) {
	struct sysc_client_globals *globals = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		globals->compositor = wl_registry_bind(registry, name,
				&wl_compositor_interface, version < 4 ? version : 4);
	} else if (strcmp(interface, wl_subcompositor_interface.name) == 0) {
		globals->subcompositor = wl_registry_bind(registry, name,
				&wl_subcompositor_interface, 1);
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		globals->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
	} else if (strcmp(interface, "xdg_wm_base") == 0) {
		globals->xdg_wm_base = wl_registry_bind(registry, name,
				&sysc_xdg_wm_base_interface, 1);
	} else if (strcmp(interface, "zxdg_decoration_manager_v1") == 0) {
		globals->xdg_decoration_manager = wl_registry_bind(registry, name,
				&sysc_xdg_decoration_manager_interface, 1);
	}
}

static void sysc_client_global_remove(void *data, struct wl_registry *registry,
		uint32_t name) {
}

static const struct wl_registry_listener sysc_client_registry_listener = {
	.global = sysc_client_global,
	.global_remove = sysc_client_global_remove,
};

static void sysc_xdg_wm_base_ping(void *data, struct wl_proxy *proxy, uint32_t serial) {
	wl_proxy_marshal_flags(proxy, 3, NULL, wl_proxy_get_version(proxy), 0, serial);
}

struct sysc_xdg_wm_base_listener {
	void (*ping)(void *, struct wl_proxy *, uint32_t);
};
static const struct sysc_xdg_wm_base_listener sysc_xdg_wm_base_listener = {
	.ping = sysc_xdg_wm_base_ping,
};

static void sysc_xdg_surface_configure(void *data, struct wl_proxy *proxy, uint32_t serial) {
	wl_proxy_marshal_flags(proxy, 4, NULL, wl_proxy_get_version(proxy), 0, serial);
}

struct sysc_xdg_surface_listener {
	void (*configure)(void *, struct wl_proxy *, uint32_t);
};
static const struct sysc_xdg_surface_listener sysc_xdg_surface_listener = {
	.configure = sysc_xdg_surface_configure,
};

static void sysc_xdg_toplevel_configure(void *data, struct wl_proxy *proxy,
		int32_t width, int32_t height, struct wl_array *states) {
}

static void sysc_xdg_toplevel_close(void *data, struct wl_proxy *proxy) {
}

struct sysc_xdg_toplevel_listener {
	void (*configure)(void *, struct wl_proxy *, int32_t, int32_t, struct wl_array *);
	void (*close)(void *, struct wl_proxy *);
};
static const struct sysc_xdg_toplevel_listener sysc_xdg_toplevel_listener = {
	.configure = sysc_xdg_toplevel_configure,
	.close = sysc_xdg_toplevel_close,
};

struct sysc_decoration_state {
	uint32_t mode;
	bool configured;
};

static void sysc_xdg_decoration_configure(void *data, struct wl_proxy *proxy,
		uint32_t mode) {
	struct sysc_decoration_state *state = data;
	state->mode = mode;
	state->configured = true;
}

struct sysc_xdg_decoration_listener {
	void (*configure)(void *, struct wl_proxy *, uint32_t);
};
static const struct sysc_xdg_decoration_listener sysc_xdg_decoration_listener = {
	.configure = sysc_xdg_decoration_configure,
};

static struct wl_buffer *sysc_client_buffer(struct wl_shm *shm, int width, int height) {
	char path[] = "/tmp/sysc-wlr-projection-XXXXXX";
	int fd = mkstemp(path);
	if (fd < 0) {
		return NULL;
	}
	unlink(path);
	int stride = width * 4;
	int size = stride * height;
	if (ftruncate(fd, size) < 0) {
		close(fd);
		return NULL;
	}
	struct wl_shm_pool *pool = wl_shm_create_pool(shm, fd, size);
	close(fd);
	if (pool == NULL) {
		return NULL;
	}
	struct wl_buffer *buffer = wl_shm_pool_create_buffer(pool, 0, width, height,
			stride, WL_SHM_FORMAT_ARGB8888);
	wl_shm_pool_destroy(pool);
	return buffer;
}

static int sysc_client_subsurface_sequence(const char *display_name) {
	int result = 1;
	struct wl_display *display = wl_display_connect(display_name);
	if (display == NULL) {
		return 2;
	}
	struct sysc_client_globals globals = {0};
	struct wl_registry *registry = wl_display_get_registry(display);
	struct wl_surface *root = NULL, *child = NULL;
	struct wl_subsurface *subsurface = NULL;
	struct wl_buffer *root_buffer = NULL, *child_buffer = NULL, *updated_child_buffer = NULL;
	struct wl_proxy *xdg_surface = NULL, *xdg_toplevel = NULL;
	if (registry == NULL || wl_registry_add_listener(registry,
			&sysc_client_registry_listener, &globals) < 0 ||
		wl_display_roundtrip(display) < 0 || globals.compositor == NULL ||
		globals.subcompositor == NULL || globals.shm == NULL || globals.xdg_wm_base == NULL) {
		result = 3;
		goto done;
	}
	if (wl_proxy_add_listener(globals.xdg_wm_base,
			(void (**)(void))&sysc_xdg_wm_base_listener, &globals) < 0) {
		result = 4;
		goto done;
	}

	root = wl_compositor_create_surface(globals.compositor);
	if (root == NULL) {
		result = 5;
		goto done;
	}
	xdg_surface = wl_proxy_marshal_flags(globals.xdg_wm_base, 2,
			&sysc_xdg_surface_interface, wl_proxy_get_version(globals.xdg_wm_base),
			0, NULL, root);
	if (xdg_surface == NULL || wl_proxy_add_listener(xdg_surface,
			(void (**)(void))&sysc_xdg_surface_listener, NULL) < 0) {
		result = 6;
		goto done;
	}
	xdg_toplevel = wl_proxy_marshal_flags(xdg_surface, 1,
			&sysc_xdg_toplevel_interface, wl_proxy_get_version(xdg_surface), 0, NULL);
	if (xdg_toplevel == NULL || wl_proxy_add_listener(xdg_toplevel,
			(void (**)(void))&sysc_xdg_toplevel_listener, NULL) < 0) {
		result = 7;
		goto done;
	}
	wl_proxy_marshal_flags(xdg_toplevel, 2, NULL, wl_proxy_get_version(xdg_toplevel), 0,
			"sysc overview projection test");
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 8;
		goto done;
	}
	root_buffer = sysc_client_buffer(globals.shm, 120, 80);
	if (root_buffer == NULL) {
		result = 9;
		goto done;
	}
	wl_surface_attach(root, root_buffer, 0, 0);
	wl_surface_damage(root, 0, 0, 120, 80);
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 10;
		goto done;
	}

	child = wl_compositor_create_surface(globals.compositor);
	subsurface = wl_subcompositor_get_subsurface(globals.subcompositor, child, root);
	child_buffer = sysc_client_buffer(globals.shm, 32, 24);
	if (child == NULL || subsurface == NULL || child_buffer == NULL) {
		result = 11;
		goto done;
	}
	wl_subsurface_set_position(subsurface, 20, 18);
	wl_surface_attach(child, child_buffer, 0, 0);
	wl_surface_damage(child, 0, 0, 32, 24);
	wl_surface_commit(child);
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 12;
		goto done;
	}

	updated_child_buffer = sysc_client_buffer(globals.shm, 48, 30);
	if (updated_child_buffer == NULL) {
		result = 13;
		goto done;
	}
	wl_surface_attach(child, updated_child_buffer, 0, 0);
	wl_surface_damage(child, 0, 0, 48, 30);
	wl_surface_commit(child);
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 14;
		goto done;
	}

	wl_subsurface_destroy(subsurface);
	subsurface = NULL;
	if (wl_display_roundtrip(display) < 0) {
		result = 15;
		goto done;
	}
	wl_surface_destroy(child);
	child = NULL;
	if (wl_display_roundtrip(display) < 0) {
		result = 16;
		goto done;
	}

	result = 0;
done:
	if (subsurface != NULL) {
		wl_subsurface_destroy(subsurface);
	}
	if (child != NULL) {
		wl_surface_destroy(child);
	}
	if (xdg_toplevel != NULL) {
		wl_proxy_marshal_flags(xdg_toplevel, 0, NULL, wl_proxy_get_version(xdg_toplevel),
				WL_MARSHAL_FLAG_DESTROY);
	}
	if (xdg_surface != NULL) {
		wl_proxy_marshal_flags(xdg_surface, 0, NULL, wl_proxy_get_version(xdg_surface),
				WL_MARSHAL_FLAG_DESTROY);
	}
	if (root != NULL) {
		wl_surface_destroy(root);
	}
	if (root_buffer != NULL) {
		wl_buffer_destroy(root_buffer);
	}
	if (child_buffer != NULL) {
		wl_buffer_destroy(child_buffer);
	}
	if (updated_child_buffer != NULL) {
		wl_buffer_destroy(updated_child_buffer);
	}
	if (display != NULL) {
		(void)wl_display_roundtrip(display);
	}
	if (globals.xdg_wm_base != NULL) {
		wl_proxy_marshal_flags(globals.xdg_wm_base, 0, NULL,
				wl_proxy_get_version(globals.xdg_wm_base), WL_MARSHAL_FLAG_DESTROY);
	}
	if (registry != NULL) {
		wl_registry_destroy(registry);
	}
	if (globals.compositor != NULL) {
		wl_compositor_destroy(globals.compositor);
	}
	if (globals.subcompositor != NULL) {
		wl_subcompositor_destroy(globals.subcompositor);
	}
	if (globals.shm != NULL) {
		wl_shm_destroy(globals.shm);
	}
	wl_display_disconnect(display);
	return result;
}

static int sysc_client_decoration_mode_sequence(const char *display_name) {
	int result = 1;
	struct wl_display *display = wl_display_connect(display_name);
	if (display == NULL) {
		return 2;
	}
	struct sysc_client_globals globals = {0};
	struct wl_registry *registry = wl_display_get_registry(display);
	struct wl_surface *root = NULL;
	struct wl_proxy *xdg_surface = NULL, *xdg_toplevel = NULL, *decoration = NULL;
	struct sysc_decoration_state state = {0};
	if (registry == NULL || wl_registry_add_listener(registry,
			&sysc_client_registry_listener, &globals) < 0 ||
		wl_display_roundtrip(display) < 0 || globals.compositor == NULL ||
		globals.xdg_wm_base == NULL || globals.xdg_decoration_manager == NULL) {
		result = 3;
		goto done;
	}
	if (wl_proxy_add_listener(globals.xdg_wm_base,
			(void (**)(void))&sysc_xdg_wm_base_listener, &globals) < 0) {
		result = 4;
		goto done;
	}
	root = wl_compositor_create_surface(globals.compositor);
	if (root == NULL) {
		result = 5;
		goto done;
	}
	xdg_surface = wl_proxy_marshal_flags(globals.xdg_wm_base, 2,
			&sysc_xdg_surface_interface, wl_proxy_get_version(globals.xdg_wm_base),
			0, NULL, root);
	if (xdg_surface == NULL || wl_proxy_add_listener(xdg_surface,
			(void (**)(void))&sysc_xdg_surface_listener, NULL) < 0) {
		result = 6;
		goto done;
	}
	xdg_toplevel = wl_proxy_marshal_flags(xdg_surface, 1,
			&sysc_xdg_toplevel_interface, wl_proxy_get_version(xdg_surface), 0, NULL);
	if (xdg_toplevel == NULL || wl_proxy_add_listener(xdg_toplevel,
			(void (**)(void))&sysc_xdg_toplevel_listener, NULL) < 0) {
		result = 7;
		goto done;
	}
	decoration = wl_proxy_marshal_flags(globals.xdg_decoration_manager, 1,
			&sysc_xdg_toplevel_decoration_interface,
			wl_proxy_get_version(globals.xdg_decoration_manager), 0, NULL, xdg_toplevel);
	if (decoration == NULL || wl_proxy_add_listener(decoration,
			(void (**)(void))&sysc_xdg_decoration_listener, &state) < 0) {
		result = 8;
		goto done;
	}
	wl_proxy_marshal_flags(xdg_toplevel, 2, NULL, wl_proxy_get_version(xdg_toplevel), 0,
			"sysc xdg decoration test");
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 9;
		goto done;
	}
	if (!state.configured || state.mode != 2) {
		result = 10;
		goto done;
	}
	wl_proxy_marshal_flags(decoration, 1, NULL, wl_proxy_get_version(decoration), 0, 1u);
	wl_surface_commit(root);
	if (wl_display_roundtrip(display) < 0) {
		result = 11;
		goto done;
	}
	if (!state.configured || state.mode != 1) {
		result = 12;
		goto done;
	}
	result = 0;
done:
	if (decoration != NULL) {
		wl_proxy_marshal_flags(decoration, 0, NULL, wl_proxy_get_version(decoration),
				WL_MARSHAL_FLAG_DESTROY);
	}
	if (xdg_toplevel != NULL) {
		wl_proxy_marshal_flags(xdg_toplevel, 0, NULL, wl_proxy_get_version(xdg_toplevel),
				WL_MARSHAL_FLAG_DESTROY);
	}
	if (xdg_surface != NULL) {
		wl_proxy_marshal_flags(xdg_surface, 0, NULL, wl_proxy_get_version(xdg_surface),
				WL_MARSHAL_FLAG_DESTROY);
	}
	if (root != NULL) {
		wl_surface_destroy(root);
	}
	if (display != NULL) {
		(void)wl_display_roundtrip(display);
	}
	if (globals.xdg_decoration_manager != NULL) {
		wl_proxy_marshal_flags(globals.xdg_decoration_manager, 0, NULL,
				wl_proxy_get_version(globals.xdg_decoration_manager), WL_MARSHAL_FLAG_DESTROY);
	}
	if (globals.xdg_wm_base != NULL) {
		wl_proxy_marshal_flags(globals.xdg_wm_base, 0, NULL,
				wl_proxy_get_version(globals.xdg_wm_base), WL_MARSHAL_FLAG_DESTROY);
	}
	if (registry != NULL) {
		wl_registry_destroy(registry);
	}
	if (globals.compositor != NULL) {
		wl_compositor_destroy(globals.compositor);
	}
	if (display != NULL) {
		wl_display_disconnect(display);
	}
	return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// RunSubsurfaceSequence creates an xdg toplevel with a subsurface, commits a
// new child buffer, then destroys the child while leaving the root mapped.
func RunSubsurfaceSequence(display string) error {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	if code := int(C.sysc_client_subsurface_sequence(name)); code != 0 {
		return fmt.Errorf("Wayland subsurface client failed at step %d", code)
	}
	return nil
}

// RunDecorationModeSequence checks the initial server-side preference and a
// later explicit client-side request over a real xdg-decoration connection.
func RunDecorationModeSequence(display string) error {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	if code := int(C.sysc_client_decoration_mode_sequence(name)); code != 0 {
		return fmt.Errorf("Wayland xdg-decoration client failed at step %d", code)
	}
	return nil
}
