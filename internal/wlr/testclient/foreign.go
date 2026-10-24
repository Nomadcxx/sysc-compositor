package testclient

/*
#cgo pkg-config: wayland-client
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>

static const struct wl_interface sysc_ext_handle_interface, sysc_wlr_handle_interface;
static const struct wl_interface *sysc_ext_types[] = { &sysc_ext_handle_interface };
static const struct wl_message sysc_ext_requests[] = {{"stop", "", NULL}, {"destroy", "", NULL}};
static const struct wl_message sysc_ext_events[] = {{"toplevel", "n", sysc_ext_types}, {"finished", "", NULL}};
static const struct wl_interface sysc_ext_list_interface = {
	"ext_foreign_toplevel_list_v1", 1, 2, sysc_ext_requests, 2, sysc_ext_events,
};
static const struct wl_message sysc_ext_handle_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_ext_handle_events[] = {
	{"closed", "", NULL}, {"done", "", NULL}, {"title", "s", NULL},
	{"app_id", "s", NULL}, {"identifier", "s", NULL},
};
static const struct wl_interface sysc_ext_handle_interface = {
	"ext_foreign_toplevel_handle_v1", 1, 1, sysc_ext_handle_requests, 5, sysc_ext_handle_events,
};
static const struct wl_interface *sysc_wlr_types[] = {
	&sysc_wlr_handle_interface, &wl_seat_interface,
	&wl_surface_interface, NULL, NULL, NULL, NULL, &wl_output_interface,
	&sysc_wlr_handle_interface,
};
static const struct wl_message sysc_wlr_requests[] = {{"stop", "", NULL}};
static const struct wl_message sysc_wlr_events[] = {{"toplevel", "n", sysc_wlr_types}, {"finished", "", NULL}};
static const struct wl_interface sysc_wlr_manager_interface = {
	"zwlr_foreign_toplevel_manager_v1", 3, 1, sysc_wlr_requests, 2, sysc_wlr_events,
};
static const struct wl_message sysc_wlr_handle_requests[] = {
	{"set_maximized", "", NULL}, {"unset_maximized", "", NULL},
	{"set_minimized", "", NULL}, {"unset_minimized", "", NULL},
	{"activate", "o", sysc_wlr_types + 1}, {"close", "", NULL},
	{"set_rectangle", "oiiii", sysc_wlr_types + 2}, {"destroy", "", NULL},
	{"set_fullscreen", "2?o", sysc_wlr_types + 7}, {"unset_fullscreen", "2", NULL},
};
static const struct wl_message sysc_wlr_handle_events[] = {
	{"title", "s", NULL}, {"app_id", "s", NULL},
	{"output_enter", "o", sysc_wlr_types + 7}, {"output_leave", "o", sysc_wlr_types + 7},
	{"state", "a", NULL}, {"done", "", NULL}, {"closed", "", NULL},
	{"parent", "3?o", sysc_wlr_types + 8},
};
static const struct wl_interface sysc_wlr_handle_interface = {
	"zwlr_foreign_toplevel_handle_v1", 3, 10, sysc_wlr_handle_requests, 8, sysc_wlr_handle_events,
};

struct sysc_foreign_entry {
	struct wl_proxy *proxy;
	char identifier[33], title[128], app_id[128];
	uint32_t state;
	int identifiers, done, closed, outputs;
};
struct sysc_foreign_client {
	struct wl_display *display;
	struct wl_proxy *ext, *wlr;
	struct wl_seat *seat;
	struct wl_output *output;
	// ponytail: this test client accepts 16 windows; grow these arrays if a test needs more.
	struct sysc_foreign_entry ext_entries[16], wlr_entries[16];
	int ext_count, wlr_count, ext_finished;
};
static void sysc_foreign_title(void *data, struct wl_proxy *p, const char *s) {
	snprintf(((struct sysc_foreign_entry *)data)->title, 128, "%s", s);
}
static void sysc_foreign_app(void *data, struct wl_proxy *p, const char *s) {
	snprintf(((struct sysc_foreign_entry *)data)->app_id, 128, "%s", s);
}
static void sysc_foreign_identifier(void *data, struct wl_proxy *p, const char *s) {
	struct sysc_foreign_entry *e = data;
	snprintf(e->identifier, 33, "%s", s);
	e->identifiers++;
}
static void sysc_foreign_done(void *data, struct wl_proxy *p) { ((struct sysc_foreign_entry *)data)->done++; }
static void sysc_foreign_closed(void *data, struct wl_proxy *p) { ((struct sysc_foreign_entry *)data)->closed++; }
static void sysc_foreign_output_enter(void *data, struct wl_proxy *p, struct wl_output *o) {
	((struct sysc_foreign_entry *)data)->outputs++;
}
static void sysc_foreign_output_leave(void *data, struct wl_proxy *p, struct wl_output *o) {
	((struct sysc_foreign_entry *)data)->outputs--;
}
static void sysc_foreign_state(void *data, struct wl_proxy *p, struct wl_array *states) {
	struct sysc_foreign_entry *e = data;
	e->state = 0;
	uint32_t *s;
	wl_array_for_each(s, states) { if (*s < 32) e->state |= 1u << *s; }
}
static void sysc_foreign_parent(void *data, struct wl_proxy *p, struct wl_proxy *parent) {}
static void (*sysc_ext_handle_listener[])(void) = {
	(void (*)(void))sysc_foreign_closed, (void (*)(void))sysc_foreign_done,
	(void (*)(void))sysc_foreign_title, (void (*)(void))sysc_foreign_app,
	(void (*)(void))sysc_foreign_identifier,
};
static void (*sysc_wlr_handle_listener[])(void) = {
	(void (*)(void))sysc_foreign_title, (void (*)(void))sysc_foreign_app,
	(void (*)(void))sysc_foreign_output_enter, (void (*)(void))sysc_foreign_output_leave,
	(void (*)(void))sysc_foreign_state, (void (*)(void))sysc_foreign_done,
	(void (*)(void))sysc_foreign_closed, (void (*)(void))sysc_foreign_parent,
};
static void sysc_foreign_ext_toplevel(void *data, struct wl_proxy *p, struct wl_proxy *handle) {
	struct sysc_foreign_client *c = data;
	if (c->ext_count == 16) { wl_proxy_destroy(handle); return; }
	struct sysc_foreign_entry *e = &c->ext_entries[c->ext_count++];
	e->proxy = handle;
	wl_proxy_add_listener(handle, sysc_ext_handle_listener, e);
}
static void sysc_foreign_wlr_toplevel(void *data, struct wl_proxy *p, struct wl_proxy *handle) {
	struct sysc_foreign_client *c = data;
	if (c->wlr_count == 16) { wl_proxy_destroy(handle); return; }
	struct sysc_foreign_entry *e = &c->wlr_entries[c->wlr_count++];
	e->proxy = handle;
	wl_proxy_add_listener(handle, sysc_wlr_handle_listener, e);
}
static void sysc_foreign_ext_finished(void *data, struct wl_proxy *p) { ((struct sysc_foreign_client *)data)->ext_finished++; }
static void sysc_foreign_wlr_finished(void *data, struct wl_proxy *p) {}
static void (*sysc_ext_listener[])(void) = {(void (*)(void))sysc_foreign_ext_toplevel, (void (*)(void))sysc_foreign_ext_finished};
static void (*sysc_wlr_listener[])(void) = {(void (*)(void))sysc_foreign_wlr_toplevel, (void (*)(void))sysc_foreign_wlr_finished};
static void sysc_foreign_seat_capabilities(void *d, struct wl_seat *s, uint32_t c) {}
static const struct wl_seat_listener sysc_foreign_seat_listener = {.capabilities = sysc_foreign_seat_capabilities};
static void sysc_foreign_geometry(void *d, struct wl_output *o, int32_t x, int32_t y, int32_t w,
		int32_t h, int32_t sub, const char *make, const char *model, int32_t transform) {}
static void sysc_foreign_mode(void *d, struct wl_output *o, uint32_t f, int32_t w, int32_t h, int32_t r) {}
static void sysc_foreign_output_done(void *d, struct wl_output *o) {}
static void sysc_foreign_scale(void *d, struct wl_output *o, int32_t f) {}
static const struct wl_output_listener sysc_foreign_output_listener = {
	.geometry = sysc_foreign_geometry, .mode = sysc_foreign_mode,
	.done = sysc_foreign_output_done, .scale = sysc_foreign_scale,
};
static void sysc_foreign_global(void *data, struct wl_registry *r, uint32_t name, const char *iface, uint32_t version) {
	struct sysc_foreign_client *c = data;
	if (strcmp(iface, "ext_foreign_toplevel_list_v1") == 0) {
		c->ext = wl_registry_bind(r, name, &sysc_ext_list_interface, 1);
		wl_proxy_add_listener(c->ext, sysc_ext_listener, c);
	} else if (strcmp(iface, "zwlr_foreign_toplevel_manager_v1") == 0) {
		c->wlr = wl_registry_bind(r, name, &sysc_wlr_manager_interface, version < 3 ? version : 3);
		wl_proxy_add_listener(c->wlr, sysc_wlr_listener, c);
	} else if (strcmp(iface, "wl_seat") == 0 && c->seat == NULL) {
		c->seat = wl_registry_bind(r, name, &wl_seat_interface, 1);
		wl_seat_add_listener(c->seat, &sysc_foreign_seat_listener, c);
	} else if (strcmp(iface, "wl_output") == 0 && c->output == NULL) {
		c->output = wl_registry_bind(r, name, &wl_output_interface, version < 3 ? version : 3);
		wl_output_add_listener(c->output, &sysc_foreign_output_listener, c);
	}
}
static void sysc_foreign_global_remove(void *data, struct wl_registry *r, uint32_t name) {}
static const struct wl_registry_listener sysc_foreign_registry_listener = {sysc_foreign_global, sysc_foreign_global_remove};
static struct sysc_foreign_client *sysc_foreign_client_start(const char *display) {
	struct sysc_foreign_client *c = calloc(1, sizeof(*c));
	if (c == NULL) return NULL;
	c->display = wl_display_connect(display);
	if (c->display == NULL) { free(c); return NULL; }
	struct wl_registry *r = wl_display_get_registry(c->display);
	wl_registry_add_listener(r, &sysc_foreign_registry_listener, c);
	if (wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0 || c->ext == NULL || c->wlr == NULL) {
		wl_display_disconnect(c->display); free(c); return NULL;
	}
	return c;
}
static int sysc_foreign_sync(struct sysc_foreign_client *c) {
	return wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_foreign_request(struct sysc_foreign_client *c, int index, int op) {
	if (index < 0 || index >= c->wlr_count || c->wlr_entries[index].proxy == NULL) return -1;
	struct wl_proxy *p = c->wlr_entries[index].proxy;
	if (op == 4) wl_proxy_marshal_flags(p, op, NULL, wl_proxy_get_version(p), 0, c->seat);
	else if (op == 8) wl_proxy_marshal_flags(p, op, NULL, wl_proxy_get_version(p), 0, c->output);
	else wl_proxy_marshal_flags(p, op, NULL, wl_proxy_get_version(p), 0);
	return sysc_foreign_sync(c);
}
static int sysc_foreign_stop(struct sysc_foreign_client *c) {
	wl_proxy_marshal_flags(c->ext, 0, NULL, 1, 0);
	return sysc_foreign_sync(c);
}
static int sysc_foreign_destroy_handle(struct sysc_foreign_client *c, int index) {
	if (index < 0 || index >= c->ext_count || c->ext_entries[index].proxy == NULL) return -1;
	wl_proxy_marshal_flags(c->ext_entries[index].proxy, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
	c->ext_entries[index].proxy = NULL;
	return sysc_foreign_sync(c);
}
static void sysc_foreign_client_free(struct sysc_foreign_client *c) {
	wl_display_disconnect(c->display); free(c);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type ForeignWindow struct {
	Identifier, Title, AppID           string
	Identifiers, Done, Closed, Outputs int
	State                              uint32
}

type ForeignClient struct{ state *C.struct_sysc_foreign_client }

func StartForeignClient(display string) (*ForeignClient, error) {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	state := C.sysc_foreign_client_start(name)
	if state == nil {
		return nil, fmt.Errorf("foreign-toplevel globals unavailable or connection failed")
	}
	return &ForeignClient{state}, nil
}

func (c *ForeignClient) Sync() error {
	if C.sysc_foreign_sync(c.state) != 0 {
		return fmt.Errorf("foreign-toplevel roundtrip failed")
	}
	return nil
}

func (c *ForeignClient) Windows(wlr bool) []ForeignWindow {
	entries, count := c.state.ext_entries[:], int(c.state.ext_count)
	if wlr {
		entries, count = c.state.wlr_entries[:], int(c.state.wlr_count)
	}
	windows := make([]ForeignWindow, count)
	for i, e := range entries[:count] {
		windows[i] = ForeignWindow{Identifier: C.GoString(&e.identifier[0]), Title: C.GoString(&e.title[0]), AppID: C.GoString(&e.app_id[0]),
			Identifiers: int(e.identifiers), Done: int(e.done), Closed: int(e.closed), Outputs: int(e.outputs), State: uint32(e.state)}
	}
	return windows
}

func (c *ForeignClient) Request(index, opcode int) error {
	if C.sysc_foreign_request(c.state, C.int(index), C.int(opcode)) != 0 {
		return fmt.Errorf("foreign-toplevel request %d failed", opcode)
	}
	return nil
}

func (c *ForeignClient) Stop() error {
	if C.sysc_foreign_stop(c.state) != 0 || c.state.ext_finished != 1 {
		return fmt.Errorf("foreign list did not finish")
	}
	return nil
}

func (c *ForeignClient) DestroyHandle(index int) error {
	if C.sysc_foreign_destroy_handle(c.state, C.int(index)) != 0 {
		return fmt.Errorf("foreign handle destroy failed")
	}
	return nil
}

func (c *ForeignClient) Close() { C.sysc_foreign_client_free(c.state) }
