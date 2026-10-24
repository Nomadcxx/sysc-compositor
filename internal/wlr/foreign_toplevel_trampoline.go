package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdint.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_foreign_toplevel_management_v1.h>

// ext-foreign-toplevel-list-v1 version 1 metadata. The wlroots constructor
// cannot accept our decimal mapping ID, so only this small endpoint is local.
static const struct wl_interface sysc_foreign_handle_interface;
static const struct wl_interface *sysc_foreign_types[] = { &sysc_foreign_handle_interface };
static const struct wl_message sysc_foreign_list_requests[] = {{"stop", "", NULL}, {"destroy", "", NULL}};
static const struct wl_message sysc_foreign_list_events[] = {{"toplevel", "n", sysc_foreign_types}, {"finished", "", NULL}};
static const struct wl_interface sysc_foreign_list_interface = {
	"ext_foreign_toplevel_list_v1", 1, 2, sysc_foreign_list_requests, 2, sysc_foreign_list_events,
};
static const struct wl_message sysc_foreign_handle_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_foreign_handle_events[] = {
	{"closed", "", NULL}, {"done", "", NULL}, {"title", "s", NULL},
	{"app_id", "s", NULL}, {"identifier", "s", NULL},
};
static const struct wl_interface sysc_foreign_handle_interface = {
	"ext_foreign_toplevel_handle_v1", 1, 1, sysc_foreign_handle_requests, 5, sysc_foreign_handle_events,
};

extern void sysc_foreign_list_event(uintptr_t, struct wl_resource *, int);
extern void sysc_foreign_resource_destroyed(uintptr_t, struct wl_resource *);
static void sysc_foreign_destroy_resource(struct wl_client *client, struct wl_resource *resource) {
	wl_resource_destroy(resource);
}
static void sysc_foreign_resource_gone(struct wl_resource *resource) {
	sysc_foreign_resource_destroyed((uintptr_t)wl_resource_get_user_data(resource), resource);
}
static void sysc_foreign_stop(struct wl_client *client, struct wl_resource *resource) {
	sysc_foreign_list_event((uintptr_t)wl_resource_get_user_data(resource), resource, 1);
}
static const struct {
	void (*stop)(struct wl_client *, struct wl_resource *);
	void (*destroy)(struct wl_client *, struct wl_resource *);
} sysc_foreign_list_impl = { sysc_foreign_stop, sysc_foreign_destroy_resource };
static const struct {
	void (*destroy)(struct wl_client *, struct wl_resource *);
} sysc_foreign_handle_impl = { sysc_foreign_destroy_resource };
static void sysc_foreign_bind(struct wl_client *client, void *data, uint32_t version, uint32_t id) {
	struct wl_resource *resource = wl_resource_create(client, &sysc_foreign_list_interface, version, id);
	if (resource == NULL) { wl_client_post_no_memory(client); return; }
	wl_resource_set_implementation(resource, &sysc_foreign_list_impl, data, sysc_foreign_resource_gone);
	sysc_foreign_list_event((uintptr_t)data, resource, 0);
}
struct wl_global *sysc_foreign_list_create(struct wl_display *display, uintptr_t handle) {
	return wl_global_create(display, &sysc_foreign_list_interface, 1, (void *)handle, sysc_foreign_bind);
}
struct wl_resource *sysc_foreign_handle_create(struct wl_resource *manager, uintptr_t handle) {
	struct wl_client *client = wl_resource_get_client(manager);
	struct wl_resource *resource = wl_resource_create(client, &sysc_foreign_handle_interface, 1, 0);
	if (resource == NULL) { wl_client_post_no_memory(client); return NULL; }
	wl_resource_set_implementation(resource, &sysc_foreign_handle_impl, (void *)handle, sysc_foreign_resource_gone);
	wl_resource_post_event(manager, 0, resource);
	return resource;
}
void sysc_foreign_list_finished(struct wl_resource *resource) { wl_resource_post_event(resource, 1); }
void sysc_foreign_handle_closed(struct wl_resource *resource) { wl_resource_post_event(resource, 0); }
void sysc_foreign_handle_done(struct wl_resource *resource) { wl_resource_post_event(resource, 1); }
void sysc_foreign_handle_title(struct wl_resource *resource, const char *title) { wl_resource_post_event(resource, 2, title); }
void sysc_foreign_handle_app_id(struct wl_resource *resource, const char *app_id) { wl_resource_post_event(resource, 3, app_id); }
void sysc_foreign_handle_identifier(struct wl_resource *resource, const char *identifier) { wl_resource_post_event(resource, 4, identifier); }

void sysc_foreign_set_output(struct wlr_foreign_toplevel_handle_v1 *handle, struct wlr_output *output) {
	bool entered = false;
	struct wlr_foreign_toplevel_handle_v1_output *entry, *tmp;
	wl_list_for_each_safe(entry, tmp, &handle->outputs, link) {
		if (entry->output == output) entered = true;
		else wlr_foreign_toplevel_handle_v1_output_leave(handle, entry->output);
	}
	if (output != NULL && !entered) wlr_foreign_toplevel_handle_v1_output_enter(handle, output);
}

void sysc_foreign_manager_no_memory(struct wlr_foreign_toplevel_manager_v1 *manager) {
	struct wl_resource *resource;
	wl_resource_for_each(resource, &manager->resources) wl_resource_post_no_memory(resource);
}
*/
import "C"
