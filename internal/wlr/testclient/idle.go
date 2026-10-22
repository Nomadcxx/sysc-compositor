package testclient

/*
#cgo pkg-config: wayland-client

#define _GNU_SOURCE
#include <poll.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// xdg-shell (just enough to map a toplevel), ext-idle-notify-v1 and
// idle-inhibit-unstable-v1, written out by hand like the other protocols here.
static const struct wl_interface sysc_idle_xdg_surface_interface;
static const struct wl_interface sysc_idle_xdg_toplevel_interface;
static const struct wl_interface sysc_idle_notification_interface;
static const struct wl_interface sysc_idle_inhibitor_interface;

static const struct wl_interface *sysc_idle_types[] = {
	&sysc_idle_xdg_surface_interface, &wl_surface_interface,
	&sysc_idle_xdg_toplevel_interface,
	&sysc_idle_notification_interface, NULL, &wl_seat_interface,
	&sysc_idle_inhibitor_interface, &wl_surface_interface,
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
static const struct wl_message sysc_idle_xdg_toplevel_requests[] = {{"destroy", "", NULL}};
static const struct wl_message sysc_idle_xdg_toplevel_events[] = {
	{"configure", "iia", NULL},
	{"close", "", NULL},
};
static const struct wl_interface sysc_idle_xdg_toplevel_interface = {
	"xdg_toplevel", 1, 1, sysc_idle_xdg_toplevel_requests, 2, sysc_idle_xdg_toplevel_events,
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

#define SYSC_IDLE_EVENTS 32

struct sysc_idle_client {
	struct wl_display *display;
	struct wl_compositor *compositor;
	struct wl_shm *shm;
	struct wl_seat *seat;
	struct wl_proxy *wm_base, *notifier, *inhibit_manager;
	struct wl_surface *surface;
	struct wl_proxy *xdg_surface, *toplevel, *notification, *inhibitor;
	struct wl_buffer *buffer;
	uint32_t configure_serial;
	bool configured;
	int events[SYSC_IDLE_EVENTS];
	int head, tail;
};

static void sysc_idle_global(void *data, struct wl_registry *registry,
		uint32_t name, const char *interface, uint32_t version) {
	struct sysc_idle_client *c = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		c->compositor = wl_registry_bind(registry, name, &wl_compositor_interface, 4);
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		c->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
	} else if (strcmp(interface, wl_seat_interface.name) == 0) {
		// The user seat is created first, so it is advertised first.
		if (c->seat == NULL) {
			c->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
		}
	} else if (strcmp(interface, "xdg_wm_base") == 0) {
		c->wm_base = wl_registry_bind(registry, name, &sysc_idle_wm_base_interface, 1);
	} else if (strcmp(interface, "ext_idle_notifier_v1") == 0) {
		c->notifier = wl_registry_bind(registry, name, &sysc_idle_notifier_interface, 1);
	} else if (strcmp(interface, "zwp_idle_inhibit_manager_v1") == 0) {
		c->inhibit_manager = wl_registry_bind(registry, name, &sysc_idle_inhibit_manager_interface, 1);
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

static struct wl_buffer *sysc_idle_buffer(struct wl_shm *shm, int width, int height) {
	int stride = width * 4, size = stride * height;
	int fd = memfd_create("sysc-idle-test", MFD_CLOEXEC);
	if (fd < 0) {
		return NULL;
	}
	if (ftruncate(fd, size) < 0) {
		close(fd);
		return NULL;
	}
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

static void sysc_idle_client_free(struct sysc_idle_client *c) {
	if (c->display != NULL) {
		wl_display_disconnect(c->display);
	}
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

// Waits up to timeout_ms for the next notification event: 1 idled,
// 2 resumed, 0 none, -1 a broken connection.
static int sysc_idle_client_wait(struct sysc_idle_client *c, int timeout_ms) {
	if (c->head != c->tail) {
		int event = c->events[c->head];
		c->head = (c->head + 1) % SYSC_IDLE_EVENTS;
		return event;
	}
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
		return ready == 0 ? 0 : -1;
	}
	if (wl_display_read_events(c->display) < 0 || wl_display_dispatch_pending(c->display) < 0) {
		return -1;
	}
	if (c->head == c->tail) {
		return 0;
	}
	int event = c->events[c->head];
	c->head = (c->head + 1) % SYSC_IDLE_EVENTS;
	return event;
}

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

// IdleClient maps a toplevel and watches an ext-idle-notify notification on
// the user seat. It is driven from one goroutine.
type IdleClient struct {
	state *C.struct_sysc_idle_client
}

// StartIdleClient connects, maps a toplevel and takes a notification with the
// given idle timeout on the first advertised seat, which is the user seat.
func StartIdleClient(display string, timeout time.Duration) (*IdleClient, error) {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	var state *C.struct_sysc_idle_client
	if code := int(C.sysc_idle_client_start(name, C.uint32_t(timeout.Milliseconds()), &state)); code != 0 {
		if state != nil {
			C.sysc_idle_client_free(state)
		}
		return nil, fmt.Errorf("Wayland idle client failed at step %d", code)
	}
	return &IdleClient{state: state}, nil
}

// Next waits up to timeout for the notification's next event and returns
// "idled", "resumed", or "" when none came.
func (c *IdleClient) Next(timeout time.Duration) (string, error) {
	switch C.sysc_idle_client_wait(c.state, C.int(timeout.Milliseconds())) {
	case 1:
		return "idled", nil
	case 2:
		return "resumed", nil
	case 0:
		return "", nil
	default:
		return "", fmt.Errorf("Wayland idle client connection broke")
	}
}

// Inhibit creates or destroys an idle inhibitor on the client's toplevel.
func (c *IdleClient) Inhibit(on bool) error {
	if code := int(C.sysc_idle_client_inhibit(c.state, C.bool(on))); code != 0 {
		return fmt.Errorf("Wayland idle inhibitor request failed at step %d", code)
	}
	return nil
}

// Close disconnects.
func (c *IdleClient) Close() {
	if c != nil && c.state != nil {
		C.sysc_idle_client_free(c.state)
		c.state = nil
	}
}
