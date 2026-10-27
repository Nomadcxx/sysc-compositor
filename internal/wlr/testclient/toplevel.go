package testclient

/*
#cgo pkg-config: wayland-client

#define _GNU_SOURCE
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// xdg-shell (just enough to map a toplevel), ext-idle-notify-v1,
// idle-inhibit-unstable-v1 and cursor-shape-v1, written out by hand like the
// other protocols here.
static const struct wl_interface sysc_idle_xdg_surface_interface;
static const struct wl_interface sysc_cursor_shape_device_interface;
static const struct wl_interface sysc_effect_surface_interface;
static const struct wl_interface sysc_shortcuts_inhibitor_interface;
static const struct wl_interface sysc_relative_pointer_interface;
static const struct wl_interface sysc_locked_pointer_interface;
static const struct wl_interface sysc_text_input_interface;
static const struct wl_interface sysc_input_method_interface;
static const struct wl_interface sysc_input_popup_interface;
static const struct wl_interface sysc_im_grab_interface;
static const struct wl_interface sysc_virtual_keyboard_interface, sysc_virtual_pointer_interface;
static const struct wl_interface sysc_idle_xdg_toplevel_interface;
static const struct wl_interface sysc_idle_notification_interface;
static const struct wl_interface sysc_idle_inhibitor_interface;

static const struct wl_interface *sysc_idle_types[] = {
	&sysc_idle_xdg_surface_interface, &wl_surface_interface,
	&sysc_idle_xdg_toplevel_interface,
	&sysc_idle_notification_interface, NULL, &wl_seat_interface,
	&sysc_idle_inhibitor_interface, &wl_surface_interface,
	&sysc_cursor_shape_device_interface, &wl_pointer_interface,
	&sysc_effect_surface_interface, &wl_surface_interface,
	&wl_region_interface,
	&sysc_shortcuts_inhibitor_interface, &wl_surface_interface, &wl_seat_interface,
	&sysc_relative_pointer_interface, &wl_pointer_interface,
	&sysc_locked_pointer_interface, &wl_surface_interface, &wl_pointer_interface,
	&wl_region_interface, NULL,
	&sysc_text_input_interface, &wl_seat_interface,
	&wl_seat_interface, &sysc_input_method_interface,
	&sysc_input_popup_interface, &wl_surface_interface,
	&wl_surface_interface,
};
static const struct wl_message sysc_idle_wm_base_requests[] = {
	{"destroy", "", NULL},
	{"create_positioner", "n", NULL},
	{"get_xdg_surface", "no", sysc_idle_types},
	{"pong", "u", NULL},
};
static const struct wl_message sysc_idle_wm_base_events[] = {{"ping", "u", NULL}};
static const struct wl_interface sysc_idle_wm_base_interface = {
	"xdg_wm_base", 1, 4, sysc_idle_wm_base_requests, 1, sysc_idle_wm_base_events,
};
static const struct wl_message sysc_idle_xdg_surface_requests[] = {
	{"destroy", "", NULL},
	{"get_toplevel", "n", sysc_idle_types + 2},
	{"get_popup", "", NULL},
	{"set_window_geometry", "iiii", NULL},
	{"ack_configure", "u", NULL},
};
static const struct wl_message sysc_idle_xdg_surface_events[] = {{"configure", "u", NULL}};
static const struct wl_interface sysc_idle_xdg_surface_interface = {
	"xdg_surface", 1, 5, sysc_idle_xdg_surface_requests, 1, sysc_idle_xdg_surface_events,
};
static const struct wl_message sysc_idle_xdg_toplevel_requests[] = {
	{"destroy", "", NULL}, {"set_parent", "?o", NULL},
	{"set_title", "s", NULL}, {"set_app_id", "s", NULL},
};
static const struct wl_message sysc_idle_xdg_toplevel_events[] = {
	{"configure", "iia", NULL},
	{"close", "", NULL},
};
static const struct wl_interface sysc_idle_xdg_toplevel_interface = {
	"xdg_toplevel", 1, 4, sysc_idle_xdg_toplevel_requests, 2, sysc_idle_xdg_toplevel_events,
};
static const struct wl_message sysc_idle_notifier_requests[] = {
	{"destroy", "", NULL},
	{"get_idle_notification", "nuo", sysc_idle_types + 3},
};
static const struct wl_interface sysc_idle_notifier_interface = {
	"ext_idle_notifier_v1", 1, 2, sysc_idle_notifier_requests, 0, NULL,
};
static const struct wl_message sysc_idle_notification_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_idle_notification_events[] = {
	{"idled", "", NULL},
	{"resumed", "", NULL},
};
static const struct wl_interface sysc_idle_notification_interface = {
	"ext_idle_notification_v1", 1, 1, sysc_idle_notification_requests,
	2, sysc_idle_notification_events,
};
static const struct wl_message sysc_idle_inhibit_manager_requests[] = {
	{"destroy", "", NULL},
	{"create_inhibitor", "no", sysc_idle_types + 6},
};
static const struct wl_interface sysc_idle_inhibit_manager_interface = {
	"zwp_idle_inhibit_manager_v1", 1, 2, sysc_idle_inhibit_manager_requests, 0, NULL,
};
static const struct wl_message sysc_idle_inhibitor_requests[] = {{"destroy", "", NULL}};
static const struct wl_interface sysc_idle_inhibitor_interface = {
	"zwp_idle_inhibitor_v1", 1, 1, sysc_idle_inhibitor_requests, 0, NULL,
};

static const struct wl_message sysc_cursor_shape_manager_requests[] = {
	{"destroy", "", NULL},
	{"get_pointer", "no", sysc_idle_types + 8},
};
static const struct wl_interface sysc_cursor_shape_manager_interface = {
	"wp_cursor_shape_manager_v1", 1, 2, sysc_cursor_shape_manager_requests, 0, NULL,
};
static const struct wl_message sysc_cursor_shape_device_requests[] = {
	{"destroy", "", NULL},
	{"set_shape", "uu", NULL},
};
static const struct wl_interface sysc_cursor_shape_device_interface = {
	"wp_cursor_shape_device_v1", 1, 2, sysc_cursor_shape_device_requests, 0, NULL,
};

static const struct wl_message sysc_effect_manager_requests[] = {
	{"destroy", "", NULL},
	{"get_background_effect", "no", sysc_idle_types + 10},
};
static const struct wl_message sysc_effect_manager_events[] = {{"capabilities", "u", NULL}};
static const struct wl_interface sysc_effect_manager_interface = {
	"ext_background_effect_manager_v1", 1, 2, sysc_effect_manager_requests,
	1, sysc_effect_manager_events,
};
static const struct wl_message sysc_effect_surface_requests[] = {
	{"destroy", "", NULL},
	{"set_blur_region", "?o", sysc_idle_types + 12},
};
static const struct wl_interface sysc_effect_surface_interface = {
	"ext_background_effect_surface_v1", 1, 2, sysc_effect_surface_requests, 0, NULL,
};

static const struct wl_message sysc_shortcuts_inhibit_manager_requests[] = {
	{"destroy", "", NULL},
	{"inhibit_shortcuts", "noo", sysc_idle_types + 13},
};
static const struct wl_interface sysc_shortcuts_inhibit_manager_interface = {
	"zwp_keyboard_shortcuts_inhibit_manager_v1", 1, 2,
	sysc_shortcuts_inhibit_manager_requests, 0, NULL,
};
static const struct wl_message sysc_shortcuts_inhibitor_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_shortcuts_inhibitor_events[] = {
	{"active", "", NULL},
	{"inactive", "", NULL},
};
static const struct wl_interface sysc_shortcuts_inhibitor_interface = {
	"zwp_keyboard_shortcuts_inhibitor_v1", 1, 1, sysc_shortcuts_inhibitor_requests,
	2, sysc_shortcuts_inhibitor_events,
};

static const struct wl_message sysc_relative_pointer_manager_requests[] = {
	{"destroy", "", NULL},
	{"get_relative_pointer", "no", sysc_idle_types + 16},
};
static const struct wl_interface sysc_relative_pointer_manager_interface = {
	"zwp_relative_pointer_manager_v1", 1, 2, sysc_relative_pointer_manager_requests, 0, NULL,
};
static const struct wl_message sysc_relative_pointer_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_relative_pointer_events[] = {
	{"relative_motion", "uuffff", NULL},
};
static const struct wl_interface sysc_relative_pointer_interface = {
	"zwp_relative_pointer_v1", 1, 1, sysc_relative_pointer_requests, 1, sysc_relative_pointer_events,
};
static const struct wl_message sysc_pointer_constraints_requests[] = {
	{"destroy", "", NULL},
	{"lock_pointer", "noo?ou", sysc_idle_types + 18},
	{"confine_pointer", "noo?ou", NULL},
};
static const struct wl_interface sysc_pointer_constraints_interface = {
	"zwp_pointer_constraints_v1", 1, 3, sysc_pointer_constraints_requests, 0, NULL,
};
static const struct wl_message sysc_locked_pointer_requests[] = {
	{"destroy", "", NULL},
	{"set_cursor_position_hint", "ff", NULL},
	{"set_region", "?o", NULL},
};
static const struct wl_message sysc_locked_pointer_events[] = {
	{"locked", "", NULL},
	{"unlocked", "", NULL},
};
static const struct wl_interface sysc_locked_pointer_interface = {
	"zwp_locked_pointer_v1", 1, 3, sysc_locked_pointer_requests, 2, sysc_locked_pointer_events,
};

// text-input-unstable-v3 (version 1) and input-method-unstable-v2.
static const struct wl_message sysc_text_input_manager_requests[] = {
	{"destroy", "", NULL},
	{"get_text_input", "no", sysc_idle_types + 23},
};
static const struct wl_interface sysc_text_input_manager_interface = {
	"zwp_text_input_manager_v3", 1, 2, sysc_text_input_manager_requests, 0, NULL,
};
static const struct wl_message sysc_text_input_requests[] = {
	{"destroy", "", NULL},
	{"enable", "", NULL},
	{"disable", "", NULL},
	{"set_surrounding_text", "sii", NULL},
	{"set_text_change_cause", "u", NULL},
	{"set_content_type", "uu", NULL},
	{"set_cursor_rectangle", "iiii", NULL},
	{"commit", "", NULL},
};
static const struct wl_message sysc_text_input_events[] = {
	{"enter", "o", sysc_idle_types + 29},
	{"leave", "o", sysc_idle_types + 29},
	{"preedit_string", "?sii", NULL},
	{"commit_string", "?s", NULL},
	{"delete_surrounding_text", "uu", NULL},
	{"done", "u", NULL},
};
static const struct wl_interface sysc_text_input_interface = {
	"zwp_text_input_v3", 1, 8, sysc_text_input_requests, 6, sysc_text_input_events,
};
static const struct wl_message sysc_input_method_manager_requests[] = {
	{"get_input_method", "on", sysc_idle_types + 25},
	{"destroy", "", NULL},
};
static const struct wl_interface sysc_input_method_manager_interface = {
	"zwp_input_method_manager_v2", 1, 2, sysc_input_method_manager_requests, 0, NULL,
};
static const struct wl_message sysc_input_method_requests[] = {
	{"commit_string", "s", NULL},
	{"set_preedit_string", "sii", NULL},
	{"delete_surrounding_text", "uu", NULL},
	{"commit", "u", NULL},
	{"get_input_popup_surface", "no", sysc_idle_types + 27},
	{"grab_keyboard", "n", NULL},
	{"destroy", "", NULL},
};

static const struct wl_interface *sysc_virtual_keyboard_types[] = {&wl_seat_interface, &sysc_virtual_keyboard_interface};
static const struct wl_message sysc_virtual_keyboard_manager_requests[] = {{"create_virtual_keyboard", "on", sysc_virtual_keyboard_types}};
static const struct wl_interface sysc_virtual_keyboard_manager_interface = {
	"zwp_virtual_keyboard_manager_v1", 1, 1, sysc_virtual_keyboard_manager_requests, 0, NULL,
};
static const struct wl_message sysc_virtual_keyboard_requests[] = {
	{"keymap", "uhu", NULL}, {"key", "uuu", NULL}, {"modifiers", "uuuu", NULL}, {"destroy", "", NULL},
};
static const struct wl_interface sysc_virtual_keyboard_interface = {
	"zwp_virtual_keyboard_v1", 1, 4, sysc_virtual_keyboard_requests, 0, NULL,
};
static const struct wl_interface *sysc_virtual_pointer_types[] = {
	&wl_seat_interface, &sysc_virtual_pointer_interface,
	&wl_seat_interface, &wl_output_interface, &sysc_virtual_pointer_interface,
};
static const struct wl_message sysc_virtual_pointer_manager_requests[] = {
	{"create_virtual_pointer", "?on", sysc_virtual_pointer_types}, {"destroy", "", NULL},
	{"create_virtual_pointer_with_output", "2?o?on", sysc_virtual_pointer_types + 2},
};
static const struct wl_interface sysc_virtual_pointer_manager_interface = {
	"zwlr_virtual_pointer_manager_v1", 2, 3, sysc_virtual_pointer_manager_requests, 0, NULL,
};
static const struct wl_message sysc_virtual_pointer_requests[] = {
	{"motion", "uff", NULL}, {"motion_absolute", "uuuuu", NULL}, {"button", "uuu", NULL},
	{"axis", "uuf", NULL}, {"frame", "", NULL}, {"axis_source", "u", NULL},
	{"axis_stop", "uu", NULL}, {"axis_discrete", "uufi", NULL}, {"destroy", "", NULL},
};
static const struct wl_interface sysc_virtual_pointer_interface = {
	"zwlr_virtual_pointer_v1", 2, 9, sysc_virtual_pointer_requests, 0, NULL,
};
static const struct wl_message sysc_im_grab_requests[] = {{"release", "", NULL}};
static const struct wl_message sysc_im_grab_events[] = {
	{"keymap", "uhu", NULL}, {"key", "uuuu", NULL}, {"modifiers", "uuuuu", NULL}, {"repeat_info", "ii", NULL},
};
static const struct wl_interface sysc_im_grab_interface = {
	"zwp_input_method_keyboard_grab_v2", 1, 1, sysc_im_grab_requests, 4, sysc_im_grab_events,
};
static const struct wl_message sysc_input_method_events[] = {
	{"activate", "", NULL},
	{"deactivate", "", NULL},
	{"surrounding_text", "suu", NULL},
	{"text_change_cause", "u", NULL},
	{"content_type", "uu", NULL},
	{"done", "", NULL},
	{"unavailable", "", NULL},
};
static const struct wl_interface sysc_input_method_interface = {
	"zwp_input_method_v2", 1, 7, sysc_input_method_requests, 7, sysc_input_method_events,
};
static const struct wl_message sysc_input_popup_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_input_popup_events[] = {
	{"text_input_rectangle", "iiii", NULL},
};
static const struct wl_interface sysc_input_popup_interface = {
	"zwp_input_popup_surface_v2", 1, 1, sysc_input_popup_requests, 1, sysc_input_popup_events,
};

#define SYSC_IDLE_EVENTS 32

struct sysc_idle_client {
	bool close_requested;
	struct wl_display *display;
	struct wl_compositor *compositor;
	struct wl_subcompositor *subcompositor;
	struct wl_surface *child_surface;
	struct wl_subsurface *child;
	struct wl_buffer *child_buffer;
	struct wl_shm *shm;
	struct wl_seat *seat;
	struct wl_seat *other_seat;
	struct wl_output *output, *other_output;
	struct wl_keyboard *keyboard, *additional_keyboard;
	int keymap_fd;
	uint32_t keymap_size;
	int key_presses, key_releases, keyboard_modifiers;
	uint32_t enter_keys[32];
	size_t enter_key_count;
	int keyboard_enters;
	uint32_t last_key;
	struct wl_proxy *virtual_keyboard_manager, *virtual_keyboard;
	struct wl_proxy *virtual_pointer_manager, *virtual_pointer, *im_grab;
	int im_keys, im_modifiers, im_keymaps, im_rate, im_delay;
	uint32_t im_last_key, im_last_state;
	int pointer_buttons, pointer_axes;
	struct wl_proxy *wm_base, *notifier, *inhibit_manager, *cursor_shape_manager;
	struct wl_proxy *effect_manager, *effect;
	struct wl_proxy *shortcuts_manager, *shortcuts_inhibitor;
	struct wl_proxy *relative_manager, *relative_pointer, *constraints, *locked_pointer;
	bool pointer_locked;
	struct wl_proxy *text_input_manager, *input_method_manager, *text_input, *input_method;
	struct wl_proxy *input_popup;
	struct wl_surface *popup_surface;
	struct wl_buffer *popup_buffer;
	bool text_entered, im_active;
	uint32_t im_done;
	char im_surrounding[64];
	char ti_committed[64];
	uint32_t ti_done;
	double relative_dx, relative_dy;
	struct wl_pointer *pointer;
	struct wl_proxy *cursor_shape_device;
	uint32_t enter_serial;
	bool entered;
	struct wl_surface *surface;
	struct wl_proxy *xdg_surface, *toplevel, *notification, *inhibitor;
	struct wl_buffer *buffer;
	uint32_t configure_serial;
	bool configured;
	int events[SYSC_IDLE_EVENTS];
	int head, tail;
};

static void sysc_input_output_geometry(void *data, struct wl_output *output, int32_t x, int32_t y,
		int32_t pw, int32_t ph, int32_t subpixel, const char *make, const char *model, int32_t transform) {}
static void sysc_input_output_mode(void *data, struct wl_output *output, uint32_t flags,
		int32_t width, int32_t height, int32_t refresh) {}
static const struct wl_output_listener sysc_input_output_listener = {
	.geometry = sysc_input_output_geometry, .mode = sysc_input_output_mode,
};

static void sysc_idle_global(void *data, struct wl_registry *registry,
		uint32_t name, const char *interface, uint32_t version) {
	struct sysc_idle_client *c = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		c->compositor = wl_registry_bind(registry, name, &wl_compositor_interface, 4);
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		c->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
	} else if (strcmp(interface, wl_output_interface.name) == 0 && c->other_output == NULL) {
		struct wl_output *output = wl_registry_bind(registry, name, &wl_output_interface, 1);
		wl_output_add_listener(output, &sysc_input_output_listener, c);
		if (c->output == NULL) c->output = output; else c->other_output = output;
	} else if (strcmp(interface, wl_seat_interface.name) == 0) {
		// The user seat is created first, so it is advertised first.
		if (c->seat == NULL) {
			c->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
		} else if (c->other_seat == NULL) {
			c->other_seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
		}
	} else if (strcmp(interface, "zwp_virtual_keyboard_manager_v1") == 0) {
		c->virtual_keyboard_manager = wl_registry_bind(registry, name, &sysc_virtual_keyboard_manager_interface, 1);
	} else if (strcmp(interface, "zwlr_virtual_pointer_manager_v1") == 0) {
		c->virtual_pointer_manager = wl_registry_bind(registry, name, &sysc_virtual_pointer_manager_interface, 2);
	} else if (strcmp(interface, "xdg_wm_base") == 0) {
		c->wm_base = wl_registry_bind(registry, name, &sysc_idle_wm_base_interface, 1);
	} else if (strcmp(interface, "ext_idle_notifier_v1") == 0) {
		c->notifier = wl_registry_bind(registry, name, &sysc_idle_notifier_interface, 1);
	} else if (strcmp(interface, "zwp_idle_inhibit_manager_v1") == 0) {
		c->inhibit_manager = wl_registry_bind(registry, name, &sysc_idle_inhibit_manager_interface, 1);
	} else if (strcmp(interface, "zwp_text_input_manager_v3") == 0) {
		c->text_input_manager = wl_registry_bind(registry, name, &sysc_text_input_manager_interface, 1);
	} else if (strcmp(interface, "zwp_input_method_manager_v2") == 0) {
		c->input_method_manager = wl_registry_bind(registry, name,
				&sysc_input_method_manager_interface, 1);
	} else if (strcmp(interface, "zwp_relative_pointer_manager_v1") == 0) {
		c->relative_manager = wl_registry_bind(registry, name,
				&sysc_relative_pointer_manager_interface, 1);
	} else if (strcmp(interface, "zwp_pointer_constraints_v1") == 0) {
		c->constraints = wl_registry_bind(registry, name, &sysc_pointer_constraints_interface, 1);
	} else if (strcmp(interface, "wl_subcompositor") == 0) {
		c->subcompositor = wl_registry_bind(registry, name, &wl_subcompositor_interface, 1);
	} else if (strcmp(interface, "zwp_keyboard_shortcuts_inhibit_manager_v1") == 0) {
		c->shortcuts_manager = wl_registry_bind(registry, name,
				&sysc_shortcuts_inhibit_manager_interface, 1);
	} else if (strcmp(interface, "ext_background_effect_manager_v1") == 0) {
		c->effect_manager = wl_registry_bind(registry, name, &sysc_effect_manager_interface, 1);
	} else if (strcmp(interface, "wp_cursor_shape_manager_v1") == 0) {
		c->cursor_shape_manager = wl_registry_bind(registry, name, &sysc_cursor_shape_manager_interface, 1);
	}
}

static void sysc_idle_global_remove(void *data, struct wl_registry *registry, uint32_t name) {
}

static const struct wl_registry_listener sysc_idle_registry_listener = {
	.global = sysc_idle_global,
	.global_remove = sysc_idle_global_remove,
};

static void sysc_idle_ping(void *data, struct wl_proxy *proxy, uint32_t serial) {
	wl_proxy_marshal_flags(proxy, 3, NULL, wl_proxy_get_version(proxy), 0, serial);
}
static const struct { void (*ping)(void *, struct wl_proxy *, uint32_t); }
	sysc_idle_wm_base_listener = { sysc_idle_ping };

static void sysc_idle_surface_configure(void *data, struct wl_proxy *proxy, uint32_t serial) {
	struct sysc_idle_client *c = data;
	c->configure_serial = serial;
	c->configured = true;
}
static const struct { void (*configure)(void *, struct wl_proxy *, uint32_t); }
	sysc_idle_xdg_surface_listener = { sysc_idle_surface_configure };

static void sysc_idle_toplevel_configure(void *data, struct wl_proxy *proxy,
		int32_t width, int32_t height, struct wl_array *states) {
}
static void sysc_idle_toplevel_close(void *data, struct wl_proxy *proxy) {
	((struct sysc_idle_client *)data)->close_requested = true;
}
static const struct {
	void (*configure)(void *, struct wl_proxy *, int32_t, int32_t, struct wl_array *);
	void (*close)(void *, struct wl_proxy *);
} sysc_idle_toplevel_listener = { sysc_idle_toplevel_configure, sysc_idle_toplevel_close };

static void sysc_idle_push(struct sysc_idle_client *c, int event) {
	int next = (c->tail + 1) % SYSC_IDLE_EVENTS;
	if (next != c->head) {
		c->events[c->tail] = event;
		c->tail = next;
	}
}
static void sysc_idle_idled(void *data, struct wl_proxy *proxy) { sysc_idle_push(data, 1); }
static void sysc_idle_resumed(void *data, struct wl_proxy *proxy) { sysc_idle_push(data, 2); }
static const struct {
	void (*idled)(void *, struct wl_proxy *);
	void (*resumed)(void *, struct wl_proxy *);
} sysc_idle_notification_listener = { sysc_idle_idled, sysc_idle_resumed };

static void sysc_pointer_enter(void *data, struct wl_pointer *pointer, uint32_t serial,
		struct wl_surface *surface, wl_fixed_t x, wl_fixed_t y) {
	struct sysc_idle_client *c = data;
	c->enter_serial = serial;
	c->entered = true;
}
static void sysc_pointer_leave(void *data, struct wl_pointer *pointer, uint32_t serial,
		struct wl_surface *surface) {
	((struct sysc_idle_client *)data)->entered = false;
}
static void sysc_pointer_motion(void *data, struct wl_pointer *pointer, uint32_t time,
		wl_fixed_t x, wl_fixed_t y) {
}
static void sysc_pointer_button(void *data, struct wl_pointer *pointer, uint32_t serial,
		uint32_t time, uint32_t button, uint32_t state) {
	((struct sysc_idle_client *)data)->pointer_buttons++;
}
static void sysc_pointer_axis(void *data, struct wl_pointer *pointer, uint32_t time,
		uint32_t axis, wl_fixed_t value) {
	((struct sysc_idle_client *)data)->pointer_axes++;
}
static const struct wl_pointer_listener sysc_pointer_listener = {
	.enter = sysc_pointer_enter,
	.leave = sysc_pointer_leave,
	.motion = sysc_pointer_motion,
	.button = sysc_pointer_button,
	.axis = sysc_pointer_axis,
};

static void sysc_keyboard_keymap(void *data, struct wl_keyboard *keyboard, uint32_t format, int fd, uint32_t size) {
	struct sysc_idle_client *c = data;
	if (c->keymap_fd >= 0) close(c->keymap_fd);
	c->keymap_fd = dup(fd);
	c->keymap_size = size;
	close(fd);
}
static void sysc_keyboard_enter(void *data, struct wl_keyboard *k, uint32_t serial, struct wl_surface *s, struct wl_array *keys) {
	struct sysc_idle_client *c = data;
	c->keyboard_enters++;
	c->enter_key_count = keys->size / sizeof(uint32_t);
	if (c->enter_key_count > 32) c->enter_key_count = 32;
	memcpy(c->enter_keys, keys->data, c->enter_key_count * sizeof(uint32_t));
}
static void sysc_keyboard_leave(void *d, struct wl_keyboard *k, uint32_t serial, struct wl_surface *s) {}
static void sysc_keyboard_key(void *data, struct wl_keyboard *keyboard, uint32_t serial, uint32_t time, uint32_t key, uint32_t state) {
	struct sysc_idle_client *c = data;
	c->last_key = key;
	if (state) c->key_presses++; else c->key_releases++;
}
static void sysc_keyboard_modifiers(void *data, struct wl_keyboard *keyboard, uint32_t serial,
		uint32_t depressed, uint32_t latched, uint32_t locked, uint32_t group) {
	((struct sysc_idle_client *)data)->keyboard_modifiers++;
}
static const struct wl_keyboard_listener sysc_keyboard_listener = {
	.keymap = sysc_keyboard_keymap, .enter = sysc_keyboard_enter, .leave = sysc_keyboard_leave,
	.key = sysc_keyboard_key, .modifiers = sysc_keyboard_modifiers,
};

static int sysc_client_bind_additional_keyboard(struct sysc_idle_client *client) {
	client->additional_keyboard = wl_seat_get_keyboard(client->seat);
	if (client->additional_keyboard == NULL) return -1;
	wl_keyboard_add_listener(client->additional_keyboard, &sysc_keyboard_listener, client);
	return wl_display_roundtrip(client->display);
}

static void sysc_seat_capabilities(void *data, struct wl_seat *seat, uint32_t caps) {
	struct sysc_idle_client *c = data;
	if ((caps & WL_SEAT_CAPABILITY_KEYBOARD) && c->keyboard == NULL) {
		c->keyboard = wl_seat_get_keyboard(seat);
		if (c->keyboard != NULL) wl_keyboard_add_listener(c->keyboard, &sysc_keyboard_listener, c);
	}
	if ((caps & WL_SEAT_CAPABILITY_POINTER) && c->pointer == NULL) {
		c->pointer = wl_seat_get_pointer(seat);
		if (c->pointer != NULL) {
			wl_pointer_add_listener(c->pointer, &sysc_pointer_listener, c);
		}
	}
}
static void sysc_seat_name(void *data, struct wl_seat *seat, const char *name) {
}
static const struct wl_seat_listener sysc_seat_listener = {
	.capabilities = sysc_seat_capabilities,
	.name = sysc_seat_name,
};

static struct wl_buffer *sysc_idle_color_buffer(struct wl_shm *shm, int width, int height, uint32_t color) {
	int stride = width * 4, size = stride * height;
	int fd = memfd_create("sysc-idle-test", MFD_CLOEXEC);
	if (fd < 0) {
		return NULL;
	}
	if (ftruncate(fd, size) < 0) {
		close(fd);
		return NULL;
	}
	uint32_t *pixels = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
	if (pixels == MAP_FAILED) { close(fd); return NULL; }
	for (int i = 0; i < width * height; i++) pixels[i] = color;
	munmap(pixels, size);
	struct wl_shm_pool *pool = wl_shm_create_pool(shm, fd, size);
	close(fd);
	if (pool == NULL) {
		return NULL;
	}
	struct wl_buffer *buffer = wl_shm_pool_create_buffer(pool, 0, width, height, stride,
			WL_SHM_FORMAT_XRGB8888);
	wl_shm_pool_destroy(pool);
	return buffer;
}

static int sysc_idle_paint_child(struct sysc_idle_client *c, uint32_t color, int x, int y, int width, int height) {
 if (!c->subcompositor) return -1;
 if (!c->child) {
  c->child_surface = wl_compositor_create_surface(c->compositor);
  c->child = wl_subcompositor_get_subsurface(c->subcompositor, c->child_surface, c->surface);
  wl_subsurface_set_desync(c->child);
 }
 struct wl_buffer *buffer = sysc_idle_color_buffer(c->shm, width, height, color);
 if (!buffer) return -1;
 if (c->child_buffer) wl_buffer_destroy(c->child_buffer);
 c->child_buffer = buffer;
 wl_subsurface_set_position(c->child, x, y);
 wl_surface_attach(c->child_surface, buffer, 0, 0);
 wl_surface_damage(c->child_surface, 0, 0, width, height);
 wl_surface_commit(c->child_surface);
 wl_surface_commit(c->surface);
 return wl_display_roundtrip(c->display);
}

static struct wl_buffer *sysc_idle_buffer(struct wl_shm *shm, int width, int height) {
	return sysc_idle_color_buffer(shm, width, height, 0);
}

static int sysc_idle_paint(struct sysc_idle_client *c, uint32_t color, int width, int height) {
	struct wl_buffer *next = sysc_idle_color_buffer(c->shm, width, height, color);
	if (next == NULL) return -1;
	wl_buffer_destroy(c->buffer); c->buffer = next;
	wl_surface_attach(c->surface, next, 0, 0);
	wl_surface_damage(c->surface, 0, 0, width, height);
	wl_surface_commit(c->surface);
	return wl_display_roundtrip(c->display);
}

// Describes why the connection failed, for test errors.
static void sysc_client_error(struct sysc_idle_client *c, char *out, int size) {
	const struct wl_interface *interface = NULL;
	uint32_t id = 0;
	int err = wl_display_get_error(c->display);
	if (err == EPROTO) {
		uint32_t code = wl_display_get_protocol_error(c->display, &interface, &id);
		snprintf(out, size, "protocol error %u on %s#%u", code,
			interface != NULL ? interface->name : "?", id);
	} else {
		snprintf(out, size, "%s", strerror(err));
	}
}

static void sysc_idle_client_free(struct sysc_idle_client *c) {
	if (c->display != NULL) {
		wl_display_disconnect(c->display);
	}
	if (c->keymap_fd >= 0) close(c->keymap_fd);
	free(c);
}

// Maps a toplevel and takes an idle notification on the user seat. Returns 0,
// or the step that failed.
static int sysc_idle_client_start(const char *display_name, uint32_t timeout_ms,
		struct sysc_idle_client **out) {
	struct sysc_idle_client *c = calloc(1, sizeof(*c));
	if (c == NULL) {
		return 1;
	}
	*out = c;
	c->keymap_fd = -1;
	if ((c->display = wl_display_connect(display_name)) == NULL) {
		return 2;
	}
	struct wl_registry *registry = wl_display_get_registry(c->display);
	if (registry == NULL || wl_registry_add_listener(registry, &sysc_idle_registry_listener, c) < 0 ||
			wl_display_roundtrip(c->display) < 0) {
		return 3;
	}
	if (c->compositor == NULL || c->shm == NULL || c->seat == NULL || c->wm_base == NULL ||
			c->notifier == NULL || c->inhibit_manager == NULL) {
		return 4;
	}
	wl_proxy_add_listener(c->wm_base, (void (**)(void))&sysc_idle_wm_base_listener, c);
	// The pointer is taken once the seat has one: asking earlier is a
	// missing_capability error on a seat with no pointer device yet.
	wl_seat_add_listener(c->seat, &sysc_seat_listener, c);
	c->surface = wl_compositor_create_surface(c->compositor);
	c->xdg_surface = wl_proxy_marshal_flags(c->wm_base, 2, &sysc_idle_xdg_surface_interface,
			1, 0, NULL, c->surface);
	if (c->surface == NULL || c->xdg_surface == NULL) {
		return 5;
	}
	wl_proxy_add_listener(c->xdg_surface, (void (**)(void))&sysc_idle_xdg_surface_listener, c);
	c->toplevel = wl_proxy_marshal_flags(c->xdg_surface, 1, &sysc_idle_xdg_toplevel_interface,
			1, 0, NULL);
	if (c->toplevel == NULL) {
		return 6;
	}
	wl_proxy_add_listener(c->toplevel, (void (**)(void))&sysc_idle_toplevel_listener, c);
	wl_surface_commit(c->surface);
	while (!c->configured) {
		if (wl_display_dispatch(c->display) < 0) {
			return 7;
		}
	}
	if ((c->buffer = sysc_idle_buffer(c->shm, 200, 150)) == NULL) {
		return 8;
	}
	wl_proxy_marshal_flags(c->xdg_surface, 4, NULL, 1, 0, c->configure_serial);
	wl_surface_attach(c->surface, c->buffer, 0, 0);
	wl_surface_damage(c->surface, 0, 0, 200, 150);
	wl_surface_commit(c->surface);
	c->notification = wl_proxy_marshal_flags(c->notifier, 1, &sysc_idle_notification_interface,
			1, 0, NULL, timeout_ms, c->seat);
	if (c->notification == NULL || wl_proxy_add_listener(c->notification,
			(void (**)(void))&sysc_idle_notification_listener, c) < 0) {
		return 9;
	}
	return wl_display_roundtrip(c->display) < 0 ? 10 : 0;
}

// Reads and dispatches whatever arrives within timeout_ms. Returns 0, or -1
// on a broken connection.
static int sysc_idle_client_dispatch(struct sysc_idle_client *c, int timeout_ms) {
	while (wl_display_prepare_read(c->display) != 0) {
		if (wl_display_dispatch_pending(c->display) < 0) {
			return -1;
		}
	}
	if (wl_display_flush(c->display) < 0) {
		wl_display_cancel_read(c->display);
		return -1;
	}
	struct pollfd pfd = { .fd = wl_display_get_fd(c->display), .events = POLLIN };
	int ready = poll(&pfd, 1, timeout_ms);
	if (ready <= 0) {
		wl_display_cancel_read(c->display);
		// Go preempts threads with SIGURG, which interrupts poll.
		return ready == 0 || errno == EINTR ? 0 : -1;
	}
	if (wl_display_read_events(c->display) < 0 || wl_display_dispatch_pending(c->display) < 0) {
		return -1;
	}
	return 0;
}

// The oldest notification event not yet taken: 1 idled, 2 resumed, 0 none.
static int sysc_idle_client_pop(struct sysc_idle_client *c) {
	if (c->head == c->tail) {
		return 0;
	}
	int event = c->events[c->head];
	c->head = (c->head + 1) % SYSC_IDLE_EVENTS;
	return event;
}

// Sets the toplevel's blur region (rects as x, y, width, height quads) and
// commits the surface.
static int sysc_client_set_blur_region(struct sysc_idle_client *c, const int32_t *rects, int count) {
	if (c->effect_manager == NULL) {
		return 1;
	}
	if (c->effect == NULL) {
		c->effect = wl_proxy_marshal_flags(c->effect_manager, 1, &sysc_effect_surface_interface,
				1, 0, NULL, c->surface);
		if (c->effect == NULL) {
			return 2;
		}
	}
	struct wl_region *region = wl_compositor_create_region(c->compositor);
	if (region == NULL) {
		return 3;
	}
	for (int i = 0; i < count; i++) {
		wl_region_add(region, rects[4*i], rects[4*i+1], rects[4*i+2], rects[4*i+3]);
	}
	wl_proxy_marshal_flags(c->effect, 1, NULL, 1, 0, region);
	wl_region_destroy(region);
	wl_surface_commit(c->surface);
	return wl_display_roundtrip(c->display) < 0 ? 4 : 0;
}

static void sysc_relative_motion(void *data, struct wl_proxy *proxy, uint32_t hi, uint32_t lo,
		wl_fixed_t dx, wl_fixed_t dy, wl_fixed_t udx, wl_fixed_t udy) {
	struct sysc_idle_client *c = data;
	c->relative_dx += wl_fixed_to_double(dx);
	c->relative_dy += wl_fixed_to_double(dy);
}
static const struct {
	void (*relative_motion)(void *, struct wl_proxy *, uint32_t, uint32_t,
		wl_fixed_t, wl_fixed_t, wl_fixed_t, wl_fixed_t);
} sysc_relative_pointer_listener = { sysc_relative_motion };

static void sysc_pointer_locked(void *data, struct wl_proxy *proxy) {
	((struct sysc_idle_client *)data)->pointer_locked = true;
}
static void sysc_pointer_unlocked(void *data, struct wl_proxy *proxy) {
	((struct sysc_idle_client *)data)->pointer_locked = false;
}
static const struct {
	void (*locked)(void *, struct wl_proxy *);
	void (*unlocked)(void *, struct wl_proxy *);
} sysc_locked_pointer_listener = { sysc_pointer_locked, sysc_pointer_unlocked };

// Takes relative motion and asks for a persistent pointer lock on the toplevel.
static int sysc_client_lock_pointer(struct sysc_idle_client *c) {
	if (c->relative_manager == NULL || c->constraints == NULL || c->pointer == NULL) {
		return 1;
	}
	c->relative_pointer = wl_proxy_marshal_flags(c->relative_manager, 1,
			&sysc_relative_pointer_interface, 1, 0, NULL, c->pointer);
	c->locked_pointer = wl_proxy_marshal_flags(c->constraints, 1,
			&sysc_locked_pointer_interface, 1, 0, NULL, c->surface, c->pointer, NULL,
			(uint32_t)2);
	if (c->relative_pointer == NULL || c->locked_pointer == NULL) {
		return 2;
	}
	wl_proxy_add_listener(c->relative_pointer, (void (**)(void))&sysc_relative_pointer_listener, c);
	wl_proxy_add_listener(c->locked_pointer, (void (**)(void))&sysc_locked_pointer_listener, c);
	return wl_display_roundtrip(c->display) < 0 ? 3 : 0;
}

// Destroys the pointer lock.
static int sysc_client_unlock_pointer(struct sysc_idle_client *c) {
	if (c->locked_pointer != NULL) {
		wl_proxy_marshal_flags(c->locked_pointer, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->locked_pointer = NULL;
	}
	return wl_display_roundtrip(c->display) < 0 ? 1 : 0;
}

static int sysc_client_set_identity(struct sysc_idle_client *c, const char *title, const char *app_id) {
	wl_proxy_marshal_flags(c->toplevel, 2, NULL, 1, 0, title);
	wl_proxy_marshal_flags(c->toplevel, 3, NULL, 1, 0, app_id);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static bool sysc_client_pointer_locked(struct sysc_idle_client *c) { return c->pointer_locked; }
static double sysc_client_relative_dx(struct sysc_idle_client *c) { return c->relative_dx; }
static double sysc_client_relative_dy(struct sysc_idle_client *c) { return c->relative_dy; }

static void sysc_ti_enter(void *data, struct wl_proxy *p, struct wl_surface *s) {
	((struct sysc_idle_client *)data)->text_entered = true;
}
static void sysc_ti_leave(void *data, struct wl_proxy *p, struct wl_surface *s) {
	((struct sysc_idle_client *)data)->text_entered = false;
}
static void sysc_ti_preedit(void *data, struct wl_proxy *p, const char *text, int32_t b, int32_t e) {
}
static void sysc_ti_commit_string(void *data, struct wl_proxy *p, const char *text) {
	struct sysc_idle_client *c = data;
	snprintf(c->ti_committed, sizeof(c->ti_committed), "%s", text != NULL ? text : "");
}
static void sysc_ti_delete(void *data, struct wl_proxy *p, uint32_t before, uint32_t after) {
}
static void sysc_ti_done(void *data, struct wl_proxy *p, uint32_t serial) {
	((struct sysc_idle_client *)data)->ti_done++;
}
static const struct {
	void (*enter)(void *, struct wl_proxy *, struct wl_surface *);
	void (*leave)(void *, struct wl_proxy *, struct wl_surface *);
	void (*preedit_string)(void *, struct wl_proxy *, const char *, int32_t, int32_t);
	void (*commit_string)(void *, struct wl_proxy *, const char *);
	void (*delete_surrounding_text)(void *, struct wl_proxy *, uint32_t, uint32_t);
	void (*done)(void *, struct wl_proxy *, uint32_t);
} sysc_text_input_listener = {
	sysc_ti_enter, sysc_ti_leave, sysc_ti_preedit, sysc_ti_commit_string, sysc_ti_delete, sysc_ti_done,
};

static void sysc_im_activate(void *data, struct wl_proxy *p) {
	((struct sysc_idle_client *)data)->im_active = true;
}
static void sysc_im_deactivate(void *data, struct wl_proxy *p) {
	((struct sysc_idle_client *)data)->im_active = false;
}
static void sysc_im_surrounding(void *data, struct wl_proxy *p, const char *text, uint32_t cursor,
		uint32_t anchor) {
	struct sysc_idle_client *c = data;
	snprintf(c->im_surrounding, sizeof(c->im_surrounding), "%s", text);
}
static void sysc_im_cause(void *data, struct wl_proxy *p, uint32_t cause) {
}
static void sysc_im_content_type(void *data, struct wl_proxy *p, uint32_t hint, uint32_t purpose) {
}
static void sysc_im_done(void *data, struct wl_proxy *p) {
	((struct sysc_idle_client *)data)->im_done++;
}
static void sysc_im_unavailable(void *data, struct wl_proxy *p) {
}
static const struct {
	void (*activate)(void *, struct wl_proxy *);
	void (*deactivate)(void *, struct wl_proxy *);
	void (*surrounding_text)(void *, struct wl_proxy *, const char *, uint32_t, uint32_t);
	void (*text_change_cause)(void *, struct wl_proxy *, uint32_t);
	void (*content_type)(void *, struct wl_proxy *, uint32_t, uint32_t);
	void (*done)(void *, struct wl_proxy *);
	void (*unavailable)(void *, struct wl_proxy *);
} sysc_input_method_listener = {
	sysc_im_activate, sysc_im_deactivate, sysc_im_surrounding, sysc_im_cause,
	sysc_im_content_type, sysc_im_done, sysc_im_unavailable,
};

// Takes a text input on the toplevel's seat and the seat's input method.
static int sysc_client_start_text_input(struct sysc_idle_client *c) {
	if (c->text_input_manager == NULL || c->input_method_manager == NULL) {
		return 1;
	}
	c->text_input = wl_proxy_marshal_flags(c->text_input_manager, 1, &sysc_text_input_interface,
			1, 0, NULL, c->seat);
	c->input_method = wl_proxy_marshal_flags(c->input_method_manager, 0,
			&sysc_input_method_interface, 1, 0, c->seat, NULL);
	if (c->text_input == NULL || c->input_method == NULL) {
		return 2;
	}
	wl_proxy_add_listener(c->text_input, (void (**)(void))&sysc_text_input_listener, c);
	wl_proxy_add_listener(c->input_method, (void (**)(void))&sysc_input_method_listener, c);
	return wl_display_roundtrip(c->display) < 0 ? 3 : 0;
}

// Enables the text input with surrounding text and a cursor rectangle.
static int sysc_client_enable_text_input(struct sysc_idle_client *c, const char *text,
		int32_t x, int32_t y, int32_t w, int32_t h) {
	int32_t length = (int32_t)strlen(text);
	wl_proxy_marshal_flags(c->text_input, 1, NULL, 1, 0);
	wl_proxy_marshal_flags(c->text_input, 3, NULL, 1, 0, text, length, length);
	wl_proxy_marshal_flags(c->text_input, 6, NULL, 1, 0, x, y, w, h);
	wl_proxy_marshal_flags(c->text_input, 7, NULL, 1, 0);
	return wl_display_roundtrip(c->display) < 0 ? 1 : 0;
}

// The input method commits a string, as an IME does on a finished word.
static int sysc_client_im_commit(struct sysc_idle_client *c, const char *text) {
	wl_proxy_marshal_flags(c->input_method, 0, NULL, 1, 0, text);
	wl_proxy_marshal_flags(c->input_method, 3, NULL, 1, 0, c->im_done);
	return wl_display_roundtrip(c->display) < 0 ? 1 : 0;
}

static void sysc_im_grab_keymap(void *data, struct wl_proxy *p, uint32_t format, int fd, uint32_t size) {
	((struct sysc_idle_client *)data)->im_keymaps++;
	close(fd);
}
static void sysc_im_grab_key(void *data, struct wl_proxy *p, uint32_t serial, uint32_t time, uint32_t key, uint32_t state) {
	struct sysc_idle_client *c = data;
	c->im_keys++;
	c->im_last_key = key;
	c->im_last_state = state;
}
static void sysc_im_grab_modifiers(void *data, struct wl_proxy *p, uint32_t serial, uint32_t depressed,
		uint32_t latched, uint32_t locked, uint32_t group) {
	((struct sysc_idle_client *)data)->im_modifiers++;
}
static void sysc_im_grab_repeat(void *data, struct wl_proxy *p, int32_t rate, int32_t delay) {
	struct sysc_idle_client *c = data;
	c->im_rate = rate;
	c->im_delay = delay;
}
static void (*sysc_im_grab_listener[])(void) = {
	(void (*)(void))sysc_im_grab_keymap, (void (*)(void))sysc_im_grab_key,
	(void (*)(void))sysc_im_grab_modifiers, (void (*)(void))sysc_im_grab_repeat,
};
static int sysc_client_grab_keyboard(struct sysc_idle_client *c) {
	c->im_grab = wl_proxy_marshal_flags(c->input_method, 5, &sysc_im_grab_interface, 1, 0, NULL);
	if (c->im_grab == NULL) return -1;
	wl_proxy_add_listener(c->im_grab, sysc_im_grab_listener, c);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_release_keyboard(struct sysc_idle_client *c) {
	if (c->im_grab != NULL) {
		wl_proxy_marshal_flags(c->im_grab, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->im_grab = NULL;
	}
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_keyboard(struct sysc_idle_client *c, bool other) {
	if (c->virtual_keyboard_manager == NULL || (other && c->other_seat == NULL)) return -1;
	c->virtual_keyboard = wl_proxy_marshal_flags(c->virtual_keyboard_manager, 0,
		&sysc_virtual_keyboard_interface, 1, 0, other ? c->other_seat : c->seat, NULL);
	if (c->virtual_keyboard == NULL || wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0) return -1;
	if (c->keymap_fd < 0) return -1;
	wl_proxy_marshal_flags(c->virtual_keyboard, 0, NULL, 1, 0,
		WL_KEYBOARD_KEYMAP_FORMAT_XKB_V1, c->keymap_fd, c->keymap_size);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_key(struct sysc_idle_client *c, uint32_t key, bool press) {
	if (c->virtual_keyboard == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_keyboard, 1, NULL, 1, 0, 1u, key, (uint32_t)press);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_modifiers(struct sysc_idle_client *c, uint32_t depressed) {
	if (c->virtual_keyboard == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_keyboard, 2, NULL, 1, 0, depressed, 0u, 0u, 0u);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_keyboard_destroy(struct sysc_idle_client *c) {
	if (c->virtual_keyboard != NULL) {
		wl_proxy_marshal_flags(c->virtual_keyboard, 3, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->virtual_keyboard = NULL;
	}
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_pointer(struct sysc_idle_client *c, int seat) {
	if (c->virtual_pointer_manager == NULL || (seat > 0 && c->other_seat == NULL)) return -1;
	c->virtual_pointer = wl_proxy_marshal_flags(c->virtual_pointer_manager, 0,
		&sysc_virtual_pointer_interface, 2, 0, seat < 0 ? NULL : seat > 0 ? c->other_seat : c->seat, NULL);
	return c->virtual_pointer == NULL || wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_pointer_output(struct sysc_idle_client *c, bool other) {
	struct wl_output *output = other ? c->other_output : c->output;
	if (c->virtual_pointer_manager == NULL || output == NULL) return -1;
	c->virtual_pointer = wl_proxy_marshal_flags(c->virtual_pointer_manager, 2,
		&sysc_virtual_pointer_interface, 2, 0, NULL, output, NULL);
	return c->virtual_pointer == NULL || wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_motion(struct sysc_idle_client *c, double dx, double dy) {
	if (c->virtual_pointer == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_pointer, 0, NULL, 2, 0, 1u, wl_fixed_from_double(dx), wl_fixed_from_double(dy));
	wl_proxy_marshal_flags(c->virtual_pointer, 4, NULL, 2, 0);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_absolute(struct sysc_idle_client *c, uint32_t x, uint32_t y, uint32_t width, uint32_t height) {
	if (c->virtual_pointer == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_pointer, 1, NULL, 2, 0, 1u, x, y, width, height);
	wl_proxy_marshal_flags(c->virtual_pointer, 4, NULL, 2, 0);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_button(struct sysc_idle_client *c, uint32_t button, bool press) {
	if (c->virtual_pointer == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_pointer, 2, NULL, 2, 0, 1u, button, (uint32_t)press);
	wl_proxy_marshal_flags(c->virtual_pointer, 4, NULL, 2, 0);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_axis(struct sysc_idle_client *c, double value) {
	if (c->virtual_pointer == NULL) return -1;
	wl_proxy_marshal_flags(c->virtual_pointer, 5, NULL, 2, 0, 0u);
	wl_proxy_marshal_flags(c->virtual_pointer, 7, NULL, 2, 0, 1u, 0u, wl_fixed_from_double(value), 1);
	wl_proxy_marshal_flags(c->virtual_pointer, 4, NULL, 2, 0);
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}
static int sysc_client_virtual_pointer_destroy(struct sysc_idle_client *c) {
	if (c->virtual_pointer != NULL) {
		wl_proxy_marshal_flags(c->virtual_pointer, 8, NULL, 2, WL_MARSHAL_FLAG_DESTROY);
		c->virtual_pointer = NULL;
	}
	return wl_display_roundtrip(c->display) < 0 ? -1 : 0;
}

// The input method maps a w x h popup surface.
static int sysc_client_im_popup(struct sysc_idle_client *c, int w, int h) {
	c->popup_surface = wl_compositor_create_surface(c->compositor);
	c->input_popup = wl_proxy_marshal_flags(c->input_method, 4, &sysc_input_popup_interface,
			1, 0, NULL, c->popup_surface);
	c->popup_buffer = sysc_idle_buffer(c->shm, w, h);
	if (c->popup_surface == NULL || c->input_popup == NULL || c->popup_buffer == NULL) {
		return 1;
	}
	wl_surface_attach(c->popup_surface, c->popup_buffer, 0, 0);
	wl_surface_damage(c->popup_surface, 0, 0, w, h);
	wl_surface_commit(c->popup_surface);
	return wl_display_roundtrip(c->display) < 0 ? 2 : 0;
}

static bool sysc_client_text_entered(struct sysc_idle_client *c) { return c->text_entered; }
static bool sysc_client_im_active(struct sysc_idle_client *c) { return c->im_active; }
static const char *sysc_client_im_surrounding(struct sysc_idle_client *c) { return c->im_surrounding; }
static const char *sysc_client_ti_committed(struct sysc_idle_client *c) { return c->ti_committed; }

// Asks to inhibit compositor shortcuts on the toplevel for the user seat.
static int sysc_client_inhibit_shortcuts(struct sysc_idle_client *c) {
	if (c->shortcuts_manager == NULL) {
		return 1;
	}
	c->shortcuts_inhibitor = wl_proxy_marshal_flags(c->shortcuts_manager, 1,
			&sysc_shortcuts_inhibitor_interface, 1, 0, NULL, c->surface, c->seat);
	if (c->shortcuts_inhibitor == NULL) {
		return 2;
	}
	return wl_display_roundtrip(c->display) < 0 ? 3 : 0;
}

// Sets a cursor shape against the latest pointer enter serial.
static int sysc_client_set_cursor_shape(struct sysc_idle_client *c, uint32_t shape) {
	if (c->cursor_shape_manager == NULL || c->pointer == NULL) {
		return 1;
	}
	if (c->cursor_shape_device == NULL) {
		c->cursor_shape_device = wl_proxy_marshal_flags(c->cursor_shape_manager, 1,
				&sysc_cursor_shape_device_interface, 1, 0, NULL, c->pointer);
		if (c->cursor_shape_device == NULL) {
			return 2;
		}
	}
	wl_proxy_marshal_flags(c->cursor_shape_device, 1, NULL, 1, 0, c->enter_serial, shape);
	return wl_display_roundtrip(c->display) < 0 ? 3 : 0;
}

static bool sysc_client_pointer_entered(struct sysc_idle_client *c) { return c->entered; }

static int sysc_idle_client_inhibit(struct sysc_idle_client *c, bool on) {
	if (on && c->inhibitor == NULL) {
		c->inhibitor = wl_proxy_marshal_flags(c->inhibit_manager, 1,
				&sysc_idle_inhibitor_interface, 1, 0, NULL, c->surface);
		if (c->inhibitor == NULL) {
			return 1;
		}
	} else if (!on && c->inhibitor != NULL) {
		wl_proxy_marshal_flags(c->inhibitor, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->inhibitor = NULL;
	}
	return wl_display_flush(c->display) < 0 ? 2 : 0;
}
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

// ToplevelClient maps a toplevel on the user seat, the first advertised one.
// It watches an ext-idle-notify notification, can hold an idle inhibitor on
// its surface, and can set a cursor-shape-v1 shape. It is driven from one
// goroutine.
type ToplevelClient struct {
	state *C.struct_sysc_idle_client
}

// StartToplevelClient connects, maps a toplevel and takes an idle
// notification with the given timeout.
func StartToplevelClient(display string, timeout time.Duration) (*ToplevelClient, error) {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	var state *C.struct_sysc_idle_client
	if code := int(C.sysc_idle_client_start(name, C.uint32_t(timeout.Milliseconds()), &state)); code != 0 {
		detail := ""
		if state != nil {
			if state.display != nil {
				var buf [256]C.char
				C.sysc_client_error(state, &buf[0], C.int(len(buf)))
				detail = ": " + C.GoString(&buf[0])
			}
			C.sysc_idle_client_free(state)
		}
		return nil, fmt.Errorf("Wayland toplevel client failed at step %d%s", code, detail)
	}
	return &ToplevelClient{state: state}, nil
}

// CommitBuffer attaches and commits the client's existing toplevel buffer,
// including after a test forces the server-side surface to unmap.
func (c *ToplevelClient) CommitBuffer() error {
	C.wl_surface_attach(c.state.surface, c.state.buffer, 0, 0)
	C.wl_surface_damage(c.state.surface, 0, 0, 200, 150)
	C.wl_surface_commit(c.state.surface)
	if C.wl_display_roundtrip(c.state.display) < 0 {
		return fmt.Errorf("Wayland toplevel buffer commit failed: %s", c.failure())
	}
	return nil
}

// PaintSubsurface commits a colored child surface and checkpoints the surface tree.
func (c *ToplevelClient) PaintSubsurface(color uint32, x, y, width, height int) error {
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return fmt.Errorf("invalid test child dimensions")
	}
	if C.sysc_idle_paint_child(c.state, C.uint32_t(color), C.int(x), C.int(y), C.int(width), C.int(height)) < 0 {
		return fmt.Errorf("paint child failed: %s", c.failure())
	}
	return nil
}

// Paint replaces the client buffer with a solid XRGB image and checkpoints its commit.
func (c *ToplevelClient) Paint(color uint32, width, height int) error {
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return fmt.Errorf("invalid test image dimensions")
	}
	if C.sysc_idle_paint(c.state, C.uint32_t(color), C.int(width), C.int(height)) < 0 {
		return fmt.Errorf("paint failed: %s", c.failure())
	}
	return nil
}

func (c *ToplevelClient) SetIdentity(title, appID string) error {
	t, a := C.CString(title), C.CString(appID)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(a))
	if C.sysc_client_set_identity(c.state, t, a) != 0 {
		return fmt.Errorf("set toplevel identity failed")
	}
	return nil
}

func (c *ToplevelClient) CloseRequested() bool { return bool(c.state.close_requested) }

type InputEvents struct {
	Presses, Releases, Modifiers, PointerButtons, PointerAxes     int
	GrabKeys, GrabModifiers, GrabKeymaps, RepeatRate, RepeatDelay int
	LastKey, GrabLastKey, GrabLastState                           uint32
	KeyboardEnters                                                int
	EnterKeys                                                     []uint32
}

func (c *ToplevelClient) SyncInput() error {
	if C.wl_display_roundtrip(c.state.display) < 0 {
		return fmt.Errorf("input roundtrip failed: %s", c.failure())
	}
	return nil
}

// BindAdditionalKeyboard observes native enter state without changing focus.
func (c *ToplevelClient) BindAdditionalKeyboard() error {
	if C.sysc_client_bind_additional_keyboard(c.state) < 0 {
		return fmt.Errorf("additional keyboard binding failed: %s", c.failure())
	}
	return nil
}

func (c *ToplevelClient) InputEvents() InputEvents {
	keys := make([]uint32, int(c.state.enter_key_count))
	for i := range keys {
		keys[i] = uint32(c.state.enter_keys[i])
	}
	return InputEvents{Presses: int(c.state.key_presses), Releases: int(c.state.key_releases), Modifiers: int(c.state.keyboard_modifiers),
		KeyboardEnters: int(c.state.keyboard_enters), EnterKeys: keys,
		PointerButtons: int(c.state.pointer_buttons), PointerAxes: int(c.state.pointer_axes), GrabKeys: int(c.state.im_keys),
		GrabModifiers: int(c.state.im_modifiers), GrabKeymaps: int(c.state.im_keymaps), RepeatRate: int(c.state.im_rate), RepeatDelay: int(c.state.im_delay),
		LastKey: uint32(c.state.last_key), GrabLastKey: uint32(c.state.im_last_key), GrabLastState: uint32(c.state.im_last_state)}
}

func (c *ToplevelClient) GrabInputMethodKeyboard() error {
	if C.sysc_client_grab_keyboard(c.state) != 0 {
		return fmt.Errorf("input method keyboard grab failed: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) ReleaseInputMethodKeyboard() error {
	if C.sysc_client_release_keyboard(c.state) != 0 {
		return fmt.Errorf("input method keyboard release failed")
	}
	return nil
}
func (c *ToplevelClient) CreateVirtualKeyboard(otherSeat bool) error {
	if C.sysc_client_virtual_keyboard(c.state, C.bool(otherSeat)) != 0 {
		return fmt.Errorf("virtual keyboard unavailable or rejected: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) VirtualKey(key uint32, pressed bool) error {
	if C.sysc_client_virtual_key(c.state, C.uint32_t(key), C.bool(pressed)) != 0 {
		return fmt.Errorf("virtual key failed: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) VirtualModifiers(depressed uint32) error {
	if C.sysc_client_virtual_modifiers(c.state, C.uint32_t(depressed)) != 0 {
		return fmt.Errorf("virtual modifiers failed")
	}
	return nil
}
func (c *ToplevelClient) DestroyVirtualKeyboard() error {
	if C.sysc_client_virtual_keyboard_destroy(c.state) != 0 {
		return fmt.Errorf("virtual keyboard destruction failed")
	}
	return nil
}
func (c *ToplevelClient) CreateVirtualPointer(otherSeat bool) error {
	seat := C.int(0)
	if otherSeat {
		seat = 1
	}
	if C.sysc_client_virtual_pointer(c.state, seat) != 0 {
		return fmt.Errorf("virtual pointer unavailable or rejected: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) CreateDefaultVirtualPointer() error {
	if C.sysc_client_virtual_pointer(c.state, -1) != 0 {
		return fmt.Errorf("default virtual pointer unavailable or rejected: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) CreateVirtualPointerForOutput(otherOutput bool) error {
	if C.sysc_client_virtual_pointer_output(c.state, C.bool(otherOutput)) != 0 {
		return fmt.Errorf("output virtual pointer unavailable or rejected: %s", c.failure())
	}
	return nil
}
func (c *ToplevelClient) VirtualMotion(dx, dy float64) error {
	if C.sysc_client_virtual_motion(c.state, C.double(dx), C.double(dy)) != 0 {
		return fmt.Errorf("virtual motion failed")
	}
	return nil
}
func (c *ToplevelClient) VirtualAbsolute(x, y, width, height uint32) error {
	if C.sysc_client_virtual_absolute(c.state, C.uint32_t(x), C.uint32_t(y), C.uint32_t(width), C.uint32_t(height)) != 0 {
		return fmt.Errorf("virtual absolute motion failed")
	}
	return nil
}
func (c *ToplevelClient) VirtualButton(button uint32, pressed bool) error {
	if C.sysc_client_virtual_button(c.state, C.uint32_t(button), C.bool(pressed)) != 0 {
		return fmt.Errorf("virtual button failed")
	}
	return nil
}
func (c *ToplevelClient) VirtualAxis(value float64) error {
	if C.sysc_client_virtual_axis(c.state, C.double(value)) != 0 {
		return fmt.Errorf("virtual axis failed")
	}
	return nil
}
func (c *ToplevelClient) DestroyVirtualPointer() error {
	if C.sysc_client_virtual_pointer_destroy(c.state) != 0 {
		return fmt.Errorf("virtual pointer destruction failed")
	}
	return nil
}

// Next waits up to timeout for the notification's next event and returns
// "idled", "resumed", or "" when none came.
func (c *ToplevelClient) Next(timeout time.Duration) (string, error) {
	for deadline := time.Now().Add(timeout); ; {
		switch C.sysc_idle_client_pop(c.state) {
		case 1:
			return "idled", nil
		case 2:
			return "resumed", nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return "", nil
		}
		if C.sysc_idle_client_dispatch(c.state, C.int(left.Milliseconds())) < 0 {
			return "", fmt.Errorf("Wayland toplevel client connection broke: %s", c.failure())
		}
	}
}

// Inhibit creates or destroys an idle inhibitor on the client's toplevel.
func (c *ToplevelClient) Inhibit(on bool) error {
	if code := int(C.sysc_idle_client_inhibit(c.state, C.bool(on))); code != 0 {
		return fmt.Errorf("Wayland idle inhibitor request failed at step %d", code)
	}
	return nil
}

// WaitPointerEnter dispatches until the user seat's pointer is over the
// toplevel, or the timeout passes. Notification events seen meanwhile are
// kept for Next.
func (c *ToplevelClient) WaitPointerEnter(timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); !bool(C.sysc_client_pointer_entered(c.state)); {
		left := time.Until(deadline)
		if left <= 0 {
			return false
		}
		if C.sysc_idle_client_dispatch(c.state, C.int(left.Milliseconds())) < 0 {
			return false
		}
	}
	return true
}

// SetCursorShape asks for a wp_cursor_shape_device_v1 shape (1 default,
// 9 text, ...) with the latest enter serial.
func (c *ToplevelClient) SetCursorShape(shape uint32) error {
	if code := int(C.sysc_client_set_cursor_shape(c.state, C.uint32_t(shape))); code != 0 {
		return fmt.Errorf("Wayland cursor-shape request failed at step %d", code)
	}
	return nil
}

// SetBlurRegion sets the toplevel's ext-background-effect blur region, as
// x, y, width, height quads in surface coordinates, and commits.
func (c *ToplevelClient) SetBlurRegion(rects ...[4]int32) error {
	flat := make([]C.int32_t, 0, 4*len(rects)+1)
	for _, r := range rects {
		flat = append(flat, C.int32_t(r[0]), C.int32_t(r[1]), C.int32_t(r[2]), C.int32_t(r[3]))
	}
	flat = append(flat, 0)
	if code := int(C.sysc_client_set_blur_region(c.state, &flat[0], C.int(len(rects)))); code != 0 {
		return fmt.Errorf("Wayland blur region request failed at step %d", code)
	}
	return nil
}

// InhibitShortcuts asks the compositor to pass its shortcuts to the toplevel
// while it has keyboard focus.
func (c *ToplevelClient) InhibitShortcuts() error {
	if code := int(C.sysc_client_inhibit_shortcuts(c.state)); code != 0 {
		return fmt.Errorf("Wayland shortcuts inhibit request failed at step %d", code)
	}
	return nil
}

// LockPointer takes relative motion and asks for a persistent pointer lock
// on the toplevel.
func (c *ToplevelClient) LockPointer() error {
	if code := int(C.sysc_client_lock_pointer(c.state)); code != 0 {
		return fmt.Errorf("Wayland pointer lock request failed at step %d", code)
	}
	return nil
}

// UnlockPointer destroys the pointer lock.
func (c *ToplevelClient) UnlockPointer() error {
	if code := int(C.sysc_client_unlock_pointer(c.state)); code != 0 {
		return fmt.Errorf("Wayland pointer unlock failed at step %d", code)
	}
	return nil
}

// WaitPointerLocked dispatches until the lock reports `locked`, or the
// timeout passes.
func (c *ToplevelClient) WaitPointerLocked(timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); !bool(C.sysc_client_pointer_locked(c.state)); {
		left := time.Until(deadline)
		if left <= 0 || C.sysc_idle_client_dispatch(c.state, C.int(left.Milliseconds())) < 0 {
			return false
		}
	}
	return true
}

// RelativeMotion dispatches for up to wait and returns the relative motion
// summed since the lock was taken.
func (c *ToplevelClient) RelativeMotion(wait time.Duration) (float64, float64) {
	for deadline := time.Now().Add(wait); ; {
		left := time.Until(deadline)
		if left <= 0 || C.sysc_idle_client_dispatch(c.state, C.int(left.Milliseconds())) < 0 {
			break
		}
	}
	return float64(C.sysc_client_relative_dx(c.state)), float64(C.sysc_client_relative_dy(c.state))
}

// StartTextInput takes a text input and the user seat's input method, both
// on this connection: the test plays the application and the IME at once.
func (c *ToplevelClient) StartTextInput() error {
	if code := int(C.sysc_client_start_text_input(c.state)); code != 0 {
		return fmt.Errorf("Wayland text input start failed at step %d", code)
	}
	return nil
}

// EnableTextInput enables the text input with surrounding text and a cursor
// rectangle in surface coordinates, and commits.
func (c *ToplevelClient) EnableTextInput(text string, x, y, w, h int32) error {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	if C.sysc_client_enable_text_input(c.state, ctext, C.int32_t(x), C.int32_t(y), C.int32_t(w), C.int32_t(h)) != 0 {
		return fmt.Errorf("Wayland text input enable failed")
	}
	return nil
}

// InputMethodCommit has the input method commit a string.
func (c *ToplevelClient) InputMethodCommit(text string) error {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	if C.sysc_client_im_commit(c.state, ctext) != 0 {
		return fmt.Errorf("Wayland input method commit failed")
	}
	return nil
}

// InputMethodPopup has the input method map a popup of the given size.
func (c *ToplevelClient) InputMethodPopup(w, h int) error {
	if code := int(C.sysc_client_im_popup(c.state, C.int(w), C.int(h))); code != 0 {
		return fmt.Errorf("Wayland input popup failed at step %d", code)
	}
	return nil
}

// TextInputState dispatches for up to wait, then reports whether the text
// input has entered the toplevel, whether the input method is active, the
// surrounding text the input method last saw, and the string the text input
// last had committed.
func (c *ToplevelClient) TextInputState(wait time.Duration) (entered, active bool, surrounding, committed string) {
	for deadline := time.Now().Add(wait); ; {
		left := time.Until(deadline)
		if left <= 0 || C.sysc_idle_client_dispatch(c.state, C.int(left.Milliseconds())) < 0 {
			break
		}
	}
	return bool(C.sysc_client_text_entered(c.state)), bool(C.sysc_client_im_active(c.state)),
		C.GoString(C.sysc_client_im_surrounding(c.state)), C.GoString(C.sysc_client_ti_committed(c.state))
}

// failure describes why the connection broke.
func (c *ToplevelClient) failure() string {
	var buf [256]C.char
	C.sysc_client_error(c.state, &buf[0], C.int(len(buf)))
	return C.GoString(&buf[0])
}

// Close disconnects.
func (c *ToplevelClient) Close() {
	if c != nil && c.state != nil {
		C.sysc_idle_client_free(c.state)
		c.state = nil
	}
}
