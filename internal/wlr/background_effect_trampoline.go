package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 wayland-server pixman-1
#cgo CFLAGS: -DWLR_USE_UNSTABLE -I${SRCDIR}/protocols

#include <stdint.h>
#include <wayland-server-core.h>
#include <wayland-server-protocol.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_scene.h>
#include <scenefx/types/wlr_scene.h>
#include "ext-background-effect-v1-protocol.h"

// Wayland interface metadata generated from ext-background-effect-v1.xml.
extern const struct wl_interface ext_background_effect_surface_v1_interface;
extern const struct wl_interface wl_region_interface;
extern const struct wl_interface wl_surface_interface;

static const struct wl_interface *ext_background_effect_v1_types[] = {
	NULL,
	&ext_background_effect_surface_v1_interface,
	&wl_surface_interface,
	&wl_region_interface,
};

static const struct wl_message ext_background_effect_manager_v1_requests[] = {
	{ "destroy", "", ext_background_effect_v1_types + 0 },
	{ "get_background_effect", "no", ext_background_effect_v1_types + 1 },
};

static const struct wl_message ext_background_effect_manager_v1_events[] = {
	{ "capabilities", "u", ext_background_effect_v1_types + 0 },
};

const struct wl_interface ext_background_effect_manager_v1_interface = {
	"ext_background_effect_manager_v1", 1,
	2, ext_background_effect_manager_v1_requests,
	1, ext_background_effect_manager_v1_events,
};

static const struct wl_message ext_background_effect_surface_v1_requests[] = {
	{ "destroy", "", ext_background_effect_v1_types + 0 },
	{ "set_blur_region", "?o", ext_background_effect_v1_types + 3 },
};

const struct wl_interface ext_background_effect_surface_v1_interface = {
	"ext_background_effect_surface_v1", 1,
	2, ext_background_effect_surface_v1_requests,
	0, NULL,
};

extern void sysc_background_effect_get(uintptr_t, struct wl_resource *, uint32_t, struct wl_resource *);
extern void sysc_background_effect_set_region(uintptr_t, struct wl_resource *, struct wl_resource *);
extern void sysc_background_effect_resource_destroyed(uintptr_t);

static void sysc_bg_manager_destroy(struct wl_client *client, struct wl_resource *resource) {
	wl_resource_destroy(resource);
}

static void sysc_bg_manager_get(struct wl_client *client, struct wl_resource *resource,
		uint32_t id, struct wl_resource *surface) {
	uintptr_t handle = (uintptr_t)wl_resource_get_user_data(resource);
	sysc_background_effect_get(handle, resource, id, surface);
}

static const struct ext_background_effect_manager_v1_interface sysc_bg_manager_impl = {
	.destroy = sysc_bg_manager_destroy,
	.get_background_effect = sysc_bg_manager_get,
};

static void sysc_bg_surface_destroy(struct wl_client *client, struct wl_resource *resource) {
	wl_resource_destroy(resource);
}

static void sysc_bg_surface_set_region(struct wl_client *client, struct wl_resource *resource,
		struct wl_resource *region) {
	uintptr_t handle = (uintptr_t)wl_resource_get_user_data(resource);
	sysc_background_effect_set_region(handle, resource, region);
}

static const struct ext_background_effect_surface_v1_interface sysc_bg_surface_impl = {
	.destroy = sysc_bg_surface_destroy,
	.set_blur_region = sysc_bg_surface_set_region,
};

static void sysc_bg_surface_resource_destroyed(struct wl_resource *resource) {
	uintptr_t handle = (uintptr_t)wl_resource_get_user_data(resource);
	if (handle != 0) {
		sysc_background_effect_resource_destroyed(handle);
	}
}

static void sysc_bg_bind(struct wl_client *client, void *data, uint32_t version, uint32_t id) {
	struct wl_resource *resource = wl_resource_create(client,
			&ext_background_effect_manager_v1_interface, version, id);
	if (resource == NULL) {
		wl_client_post_no_memory(client);
		return;
	}
	wl_resource_set_implementation(resource, &sysc_bg_manager_impl, data, NULL);
	ext_background_effect_manager_v1_send_capabilities(resource,
			EXT_BACKGROUND_EFFECT_MANAGER_V1_CAPABILITY_BLUR);
}

struct wl_global *sysc_bg_global_create(struct wl_display *display, uintptr_t handle) {
	return wl_global_create(display, &ext_background_effect_manager_v1_interface, 1,
			(void *)handle, sysc_bg_bind);
}

struct wl_resource *sysc_bg_surface_create(struct wl_client *client, uint32_t version,
		uint32_t id, uintptr_t handle) {
	struct wl_resource *resource = wl_resource_create(client,
			&ext_background_effect_surface_v1_interface, version, id);
	if (resource == NULL) {
		wl_client_post_no_memory(client);
		return NULL;
	}
	wl_resource_set_implementation(resource, &sysc_bg_surface_impl, (void *)handle,
			sysc_bg_surface_resource_destroyed);
	return resource;
}

void sysc_bg_post_duplicate(struct wl_resource *resource) {
	wl_resource_post_error(resource,
			EXT_BACKGROUND_EFFECT_MANAGER_V1_ERROR_BACKGROUND_EFFECT_EXISTS,
			"surface already has a background effect object");
}

void sysc_bg_post_surface_destroyed(struct wl_resource *resource) {
	wl_resource_post_error(resource,
			EXT_BACKGROUND_EFFECT_SURFACE_V1_ERROR_SURFACE_DESTROYED,
			"associated wl_surface was destroyed");
}

const pixman_region32_t *sysc_bg_region(struct wl_resource *resource) {
	return resource == NULL ? NULL : wlr_region_from_resource(resource);
}

int sysc_bg_region_count(const pixman_region32_t *region) {
	return region == NULL ? 0 : pixman_region32_n_rects(region);
}

bool sysc_bg_region_rect(const pixman_region32_t *region, int index,
		int *x, int *y, int *width, int *height) {
	if (region == NULL) {
		return false;
	}
	int count = 0;
	pixman_box32_t *rects = pixman_region32_rectangles((pixman_region32_t *)region, &count);
	if (index < 0 || index >= count) {
		return false;
	}
	*x = rects[index].x1;
	*y = rects[index].y1;
	*width = rects[index].x2 - rects[index].x1;
	*height = rects[index].y2 - rects[index].y1;
	return true;
}

struct sysc_bg_surface_node {
	struct wlr_surface *surface;
	struct wlr_scene_node *node;
	struct wlr_scene_tree *parent;
};

static void sysc_bg_find_surface_node(struct wlr_scene_buffer *buffer,
		int x, int y, void *data) {
	struct sysc_bg_surface_node *found = data;
	if (found->node != NULL) {
		return;
	}
	struct wlr_scene_surface *scene_surface = wlr_scene_surface_try_from_buffer(buffer);
	if (scene_surface != NULL && scene_surface->surface == found->surface) {
		found->node = &buffer->node;
		found->parent = buffer->node.parent;
	}
}

bool sysc_bg_find_scene_surface(struct wlr_scene *scene, struct wlr_surface *surface,
		struct wlr_scene_node **node, struct wlr_scene_tree **parent, int *x, int *y) {
	struct sysc_bg_surface_node found = { .surface = surface };
	wlr_scene_node_for_each_buffer(&scene->tree.node, sysc_bg_find_surface_node, &found);
	if (found.node == NULL || found.parent == NULL) {
		return false;
	}
	*node = found.node;
	*parent = found.parent;
	*x = found.node->x;
	*y = found.node->y;
	return true;
}
*/
import "C"
