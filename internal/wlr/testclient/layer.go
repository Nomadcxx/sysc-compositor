package testclient

/*
#cgo pkg-config: wayland-client

#define _GNU_SOURCE
#include <errno.h>
#include <poll.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// wlr-layer-shell-unstable-v1 and the xdg-shell popup requests, written out
// by hand like the other protocols here.
static const struct wl_interface sysc_layer_surface_interface;
static const struct wl_interface sysc_layer_xdg_surface_interface;
static const struct wl_interface sysc_layer_positioner_interface;
static const struct wl_interface sysc_layer_popup_interface;

static const struct wl_interface *sysc_layer_types[] = {
	&sysc_layer_surface_interface, &wl_surface_interface, &wl_output_interface, NULL, NULL,
	&sysc_layer_popup_interface,
	&sysc_layer_positioner_interface,
	&sysc_layer_xdg_surface_interface, &wl_surface_interface,
	&sysc_layer_popup_interface, &sysc_layer_xdg_surface_interface, &sysc_layer_positioner_interface,
	&wl_seat_interface, NULL,
};
static const struct wl_message sysc_layer_shell_requests[] = {
	{"get_layer_surface", "no?ous", sysc_layer_types},
	{"destroy", "3", NULL},
};
static const struct wl_interface sysc_layer_shell_interface = {
	"zwlr_layer_shell_v1", 4, 2, sysc_layer_shell_requests, 0, NULL,
};
static const struct wl_message sysc_layer_surface_requests[] = {
	{"set_size", "uu", NULL},
	{"set_anchor", "u", NULL},
	{"set_exclusive_zone", "i", NULL},
	{"set_margin", "iiii", NULL},
	{"set_keyboard_interactivity", "u", NULL},
	{"get_popup", "o", sysc_layer_types + 5},
	{"ack_configure", "u", NULL},
	{"destroy", "", NULL},
};
static const struct wl_message sysc_layer_surface_events[] = {
	{"configure", "uuu", NULL},
	{"closed", "", NULL},
};
static const struct wl_interface sysc_layer_surface_interface = {
	"zwlr_layer_surface_v1", 4, 8, sysc_layer_surface_requests, 2, sysc_layer_surface_events,
};
static const struct wl_message sysc_layer_wm_base_requests[] = {
	{"destroy", "", NULL},
	{"create_positioner", "n", sysc_layer_types + 6},
	{"get_xdg_surface", "no", sysc_layer_types + 7},
	{"pong", "u", NULL},
};
static const struct wl_message sysc_layer_wm_base_events[] = {{"ping", "u", NULL}};
static const struct wl_interface sysc_layer_wm_base_interface = {
	"xdg_wm_base", 2, 4, sysc_layer_wm_base_requests, 1, sysc_layer_wm_base_events,
};
static const struct wl_message sysc_layer_positioner_requests[] = {
	{"destroy", "", NULL},
	{"set_size", "ii", NULL},
	{"set_anchor_rect", "iiii", NULL},
	{"set_anchor", "u", NULL},
	{"set_gravity", "u", NULL},
	{"set_constraint_adjustment", "u", NULL},
	{"set_offset", "ii", NULL},
};
static const struct wl_interface sysc_layer_positioner_interface = {
	"xdg_positioner", 2, 7, sysc_layer_positioner_requests, 0, NULL,
};
static const struct wl_message sysc_layer_xdg_surface_requests[] = {
	{"destroy", "", NULL},
	{"get_toplevel", "n", sysc_layer_types + 3},
	{"get_popup", "n?oo", sysc_layer_types + 9},
	{"set_window_geometry", "iiii", NULL},
	{"ack_configure", "u", NULL},
};
static const struct wl_message sysc_layer_xdg_surface_events[] = {{"configure", "u", NULL}};
static const struct wl_interface sysc_layer_xdg_surface_interface = {
	"xdg_surface", 2, 5, sysc_layer_xdg_surface_requests, 1, sysc_layer_xdg_surface_events,
};
static const struct wl_message sysc_layer_popup_requests[] = {
	{"destroy", "", NULL},
	{"grab", "ou", sysc_layer_types + 12},
};
static const struct wl_message sysc_layer_popup_events[] = {
	{"configure", "iiii", NULL},
	{"popup_done", "", NULL},
};
static const struct wl_interface sysc_layer_popup_interface = {
	"xdg_popup", 2, 2, sysc_layer_popup_requests, 2, sysc_layer_popup_events,
};

struct sysc_layer_client {
	struct wl_display *display;
	struct wl_compositor *compositor;
	struct wl_shm *shm;
	struct wl_seat *seat;
	struct wl_pointer *pointer;
	struct wl_proxy *shell, *wm_base, *layer, *xdg_surface, *popup;
	struct wl_surface *bar, *popup_surface;
	struct wl_buffer *bar_buffer, *popup_buffer;
	uint32_t layer_serial, layer_width, popup_serial;
	bool layer_configured, popup_configured, on_popup;
	int popup_enters, popup_axes, popup_buttons;
};

static void sysc_layer_global(void *data, struct wl_registry *registry,
		uint32_t name, const char *interface, uint32_t version) {
	struct sysc_layer_client *c = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		c->compositor = wl_registry_bind(registry, name, &wl_compositor_interface, 4);
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		c->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
	} else if (strcmp(interface, wl_seat_interface.name) == 0 && c->seat == NULL) {
		// Version 1, so the pointer needs no frame or axis_source handlers:
		// libwayland calls a NULL handler and crashes.
		c->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
	} else if (strcmp(interface, sysc_layer_shell_interface.name) == 0) {
		c->shell = wl_registry_bind(registry, name, &sysc_layer_shell_interface, 4);
	} else if (strcmp(interface, sysc_layer_wm_base_interface.name) == 0) {
		c->wm_base = wl_registry_bind(registry, name, &sysc_layer_wm_base_interface, 2);
	}
}
static void sysc_layer_global_remove(void *data, struct wl_registry *registry, uint32_t name) {}
static const struct wl_registry_listener sysc_layer_registry_listener = {
	.global = sysc_layer_global, .global_remove = sysc_layer_global_remove,
};

static void sysc_layer_ping(void *data, struct wl_proxy *wm_base, uint32_t serial) {
	wl_proxy_marshal_flags(wm_base, 3, NULL, wl_proxy_get_version(wm_base), 0, serial);
}
static const struct { void (*ping)(void *, struct wl_proxy *, uint32_t); }
	sysc_layer_wm_base_listener = { sysc_layer_ping };

static void sysc_layer_configure(void *data, struct wl_proxy *layer,
		uint32_t serial, uint32_t width, uint32_t height) {
	struct sysc_layer_client *c = data;
	c->layer_serial = serial;
	c->layer_width = width;
	c->layer_configured = true;
}
static void sysc_layer_closed(void *data, struct wl_proxy *layer) {}
static const struct {
	void (*configure)(void *, struct wl_proxy *, uint32_t, uint32_t, uint32_t);
	void (*closed)(void *, struct wl_proxy *);
} sysc_layer_surface_listener = { sysc_layer_configure, sysc_layer_closed };

static void sysc_layer_popup_surface_configure(void *data, struct wl_proxy *xdg, uint32_t serial) {
	struct sysc_layer_client *c = data;
	c->popup_serial = serial;
	c->popup_configured = true;
}
static const struct { void (*configure)(void *, struct wl_proxy *, uint32_t); }
	sysc_layer_xdg_surface_listener = { sysc_layer_popup_surface_configure };

static void sysc_layer_popup_configure(void *data, struct wl_proxy *popup,
		int32_t x, int32_t y, int32_t width, int32_t height) {}
static void sysc_layer_popup_done(void *data, struct wl_proxy *popup) {}
static const struct {
	void (*configure)(void *, struct wl_proxy *, int32_t, int32_t, int32_t, int32_t);
	void (*popup_done)(void *, struct wl_proxy *);
} sysc_layer_popup_listener = { sysc_layer_popup_configure, sysc_layer_popup_done };

static void sysc_layer_pointer_enter(void *data, struct wl_pointer *pointer, uint32_t serial,
		struct wl_surface *surface, wl_fixed_t x, wl_fixed_t y) {
	struct sysc_layer_client *c = data;
	c->on_popup = surface == c->popup_surface;
	if (c->on_popup) c->popup_enters++;
}
static void sysc_layer_pointer_leave(void *data, struct wl_pointer *pointer, uint32_t serial,
		struct wl_surface *surface) {
	((struct sysc_layer_client *)data)->on_popup = false;
}
static void sysc_layer_pointer_motion(void *data, struct wl_pointer *pointer, uint32_t time,
		wl_fixed_t x, wl_fixed_t y) {}
static void sysc_layer_pointer_button(void *data, struct wl_pointer *pointer, uint32_t serial,
		uint32_t time, uint32_t button, uint32_t state) {
	struct sysc_layer_client *c = data;
	if (c->on_popup) c->popup_buttons++;
}
static void sysc_layer_pointer_axis(void *data, struct wl_pointer *pointer, uint32_t time,
		uint32_t axis, wl_fixed_t value) {
	struct sysc_layer_client *c = data;
	if (c->on_popup) c->popup_axes++;
}
static const struct wl_pointer_listener sysc_layer_pointer_listener = {
	.enter = sysc_layer_pointer_enter,
	.leave = sysc_layer_pointer_leave,
	.motion = sysc_layer_pointer_motion,
	.button = sysc_layer_pointer_button,
	.axis = sysc_layer_pointer_axis,
};

static void sysc_layer_seat_capabilities(void *data, struct wl_seat *seat, uint32_t caps) {
	struct sysc_layer_client *c = data;
	if ((caps & WL_SEAT_CAPABILITY_POINTER) && c->pointer == NULL) {
		c->pointer = wl_seat_get_pointer(seat);
		wl_pointer_add_listener(c->pointer, &sysc_layer_pointer_listener, c);
	}
}
static void sysc_layer_seat_name(void *data, struct wl_seat *seat, const char *name) {}
static const struct wl_seat_listener sysc_layer_seat_listener = {
	.capabilities = sysc_layer_seat_capabilities, .name = sysc_layer_seat_name,
};

static struct wl_buffer *sysc_layer_buffer(struct wl_shm *shm, int width, int height, uint32_t color) {
	int stride = width * 4, size = stride * height;
	int fd = memfd_create("sysc-layer-test", MFD_CLOEXEC);
	if (fd < 0) return NULL;
	if (ftruncate(fd, size) < 0) { close(fd); return NULL; }
	uint32_t *pixels = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
	if (pixels == MAP_FAILED) { close(fd); return NULL; }
	for (int i = 0; i < size / 4; i++) pixels[i] = color;
	munmap(pixels, size);
	struct wl_shm_pool *pool = wl_shm_create_pool(shm, fd, size);
	close(fd);
	if (pool == NULL) return NULL;
	struct wl_buffer *buffer = wl_shm_pool_create_buffer(pool, 0, width, height, stride,
			WL_SHM_FORMAT_XRGB8888);
	wl_shm_pool_destroy(pool);
	return buffer;
}

static int64_t sysc_layer_now_ms(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

// Dispatches until the deadline: 0 when something was dispatched, 1 when the
// deadline passed, -1 on a broken connection.
static int sysc_layer_dispatch(struct wl_display *display, int64_t deadline) {
	while (wl_display_prepare_read(display) != 0) {
		if (wl_display_dispatch_pending(display) < 0) return -1;
	}
	wl_display_flush(display);
	int64_t left = deadline - sysc_layer_now_ms();
	if (left <= 0) {
		wl_display_cancel_read(display);
		return 1;
	}
	struct pollfd pfd = { .fd = wl_display_get_fd(display), .events = POLLIN };
	int ready = poll(&pfd, 1, (int)left);
	if (ready <= 0) {
		wl_display_cancel_read(display);
		// Go preempts threads with SIGURG, which interrupts poll.
		return ready < 0 && errno != EINTR ? -1 : 0;
	}
	if (wl_display_read_events(display) < 0 || wl_display_dispatch_pending(display) < 0) return -1;
	return 0;
}

static int sysc_layer_wait(struct sysc_layer_client *c, bool *flag, int64_t deadline) {
	while (!*flag) {
		if (sysc_layer_dispatch(c->display, deadline) != 0) return -1;
	}
	return 0;
}

static void sysc_layer_client_free(struct sysc_layer_client *c) {
	if (c->display != NULL) wl_display_disconnect(c->display);
	free(c);
}

// Maps a 30 px on-demand bar on the top layer, then a 120x80 xdg popup that
// hangs from the bar's bottom-left corner. Returns 0, or the step that failed.
static int sysc_layer_client_start(const char *display_name, struct sysc_layer_client **out) {
	struct sysc_layer_client *c = calloc(1, sizeof(*c));
	if (c == NULL) return 1;
	*out = c;
	if ((c->display = wl_display_connect(display_name)) == NULL) return 2;
	struct wl_registry *registry = wl_display_get_registry(c->display);
	if (registry == NULL || wl_registry_add_listener(registry, &sysc_layer_registry_listener, c) < 0 ||
			wl_display_roundtrip(c->display) < 0) {
		return 3;
	}
	if (c->compositor == NULL || c->shm == NULL || c->seat == NULL || c->shell == NULL ||
			c->wm_base == NULL) {
		return 4;
	}
	wl_proxy_add_listener(c->wm_base, (void (**)(void))&sysc_layer_wm_base_listener, c);
	wl_seat_add_listener(c->seat, &sysc_layer_seat_listener, c);
	if (wl_display_roundtrip(c->display) < 0 || c->pointer == NULL) return 5;

	c->bar = wl_compositor_create_surface(c->compositor);
	c->layer = wl_proxy_marshal_flags(c->shell, 0, &sysc_layer_surface_interface, 4, 0,
			NULL, c->bar, NULL, 2, "sysc-test-bar"); // layer top
	if (c->bar == NULL || c->layer == NULL) return 6;
	wl_proxy_add_listener(c->layer, (void (**)(void))&sysc_layer_surface_listener, c);
	wl_proxy_marshal_flags(c->layer, 0, NULL, 4, 0, 0, 30);           // set_size
	wl_proxy_marshal_flags(c->layer, 1, NULL, 4, 0, 1 | 4 | 8);       // anchor top, left, right
	wl_proxy_marshal_flags(c->layer, 4, NULL, 4, 0, 2);               // on_demand keyboard
	wl_surface_commit(c->bar);
	int64_t deadline = sysc_layer_now_ms() + 3000;
	if (sysc_layer_wait(c, &c->layer_configured, deadline) < 0) return 7;
	if ((c->bar_buffer = sysc_layer_buffer(c->shm, (int)c->layer_width, 30, 0x303030)) == NULL) return 8;
	wl_proxy_marshal_flags(c->layer, 6, NULL, 4, 0, c->layer_serial);  // ack_configure
	wl_surface_attach(c->bar, c->bar_buffer, 0, 0);
	wl_surface_damage(c->bar, 0, 0, (int)c->layer_width, 30);
	wl_surface_commit(c->bar);

	c->popup_surface = wl_compositor_create_surface(c->compositor);
	c->xdg_surface = wl_proxy_marshal_flags(c->wm_base, 2, &sysc_layer_xdg_surface_interface, 2, 0,
			NULL, c->popup_surface);
	struct wl_proxy *positioner = wl_proxy_marshal_flags(c->wm_base, 1,
			&sysc_layer_positioner_interface, 2, 0, NULL);
	if (c->popup_surface == NULL || c->xdg_surface == NULL || positioner == NULL) return 9;
	wl_proxy_add_listener(c->xdg_surface, (void (**)(void))&sysc_layer_xdg_surface_listener, c);
	wl_proxy_marshal_flags(positioner, 1, NULL, 2, 0, 120, 80);        // set_size
	wl_proxy_marshal_flags(positioner, 2, NULL, 2, 0, 0, 0, 40, 30);   // set_anchor_rect
	wl_proxy_marshal_flags(positioner, 3, NULL, 2, 0, 6);              // anchor bottom_left
	wl_proxy_marshal_flags(positioner, 4, NULL, 2, 0, 8);              // gravity bottom_right
	c->popup = wl_proxy_marshal_flags(c->xdg_surface, 2, &sysc_layer_popup_interface, 2, 0,
			NULL, NULL, positioner);
	wl_proxy_marshal_flags(positioner, 0, NULL, 2, WL_MARSHAL_FLAG_DESTROY);
	if (c->popup == NULL) return 10;
	wl_proxy_add_listener(c->popup, (void (**)(void))&sysc_layer_popup_listener, c);
	wl_proxy_marshal_flags(c->layer, 5, NULL, 4, 0, c->popup);         // get_popup
	wl_surface_commit(c->popup_surface);
	if (sysc_layer_wait(c, &c->popup_configured, deadline) < 0) return 11;
	if ((c->popup_buffer = sysc_layer_buffer(c->shm, 120, 80, 0x6060a0)) == NULL) return 12;
	wl_proxy_marshal_flags(c->xdg_surface, 4, NULL, 2, 0, c->popup_serial); // ack_configure
	wl_surface_attach(c->popup_surface, c->popup_buffer, 0, 0);
	wl_surface_damage(c->popup_surface, 0, 0, 120, 80);
	wl_surface_commit(c->popup_surface);
	return wl_display_roundtrip(c->display) < 0 ? 13 : 0;
}

// Dispatches until the pointer is on the popup: 0, 1 on timeout, -1 on a
// broken connection.
static int sysc_layer_wait_popup_enter(struct sysc_layer_client *c, int timeout_ms) {
	int64_t deadline = sysc_layer_now_ms() + timeout_ms;
	while (!c->on_popup) {
		int r = sysc_layer_dispatch(c->display, deadline);
		if (r != 0) return r;
	}
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

// LayerPopupClient maps a top-layer bar with an xdg popup under it, the way a
// bar's menu opens, and counts the pointer events the popup gets. Like the
// other clients here it is dispatched only from its own calls.
type LayerPopupClient struct {
	state *C.struct_sysc_layer_client
}

// PopupInput is the pointer events the popup surface received.
type PopupInput struct {
	Enters, Axes, Buttons int
}

// StartLayerPopupClient maps a 30 px top-layer bar with on-demand keyboard
// interactivity across the top of the default output, then a 120x80 popup at
// the bar's bottom-left corner: layout (0..120, 30..110) on an output at the
// origin. Add the seat's pointer first: the client binds wl_pointer only when
// the seat already has one.
func StartLayerPopupClient(display string) (*LayerPopupClient, error) {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	var state *C.struct_sysc_layer_client
	if code := int(C.sysc_layer_client_start(name, &state)); code != 0 {
		if state != nil {
			C.sysc_layer_client_free(state)
		}
		return nil, fmt.Errorf("Wayland layer popup client failed at step %d", code)
	}
	return &LayerPopupClient{state: state}, nil
}

// WaitPopupEnter dispatches until the pointer is on the popup, or the timeout
// passes.
func (c *LayerPopupClient) WaitPopupEnter(timeout time.Duration) bool {
	return C.sysc_layer_wait_popup_enter(c.state, C.int(timeout.Milliseconds())) == 0
}

// Sync reads every event the compositor has sent.
func (c *LayerPopupClient) Sync() error {
	if C.wl_display_roundtrip(c.state.display) < 0 {
		return errors.New("layer popup client roundtrip failed")
	}
	return nil
}

func (c *LayerPopupClient) PopupInput() PopupInput {
	return PopupInput{
		Enters:  int(c.state.popup_enters),
		Axes:    int(c.state.popup_axes),
		Buttons: int(c.state.popup_buttons),
	}
}

func (c *LayerPopupClient) Close() {
	if c != nil && c.state != nil {
		C.sysc_layer_client_free(c.state)
		c.state = nil
	}
}
