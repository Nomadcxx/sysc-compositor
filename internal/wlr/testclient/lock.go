package testclient

/*
#cgo pkg-config: wayland-client

#define _GNU_SOURCE
#include <errno.h>
#include <poll.h>
#include <stdbool.h>
#include <time.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// ext-session-lock-v1, written out by hand like the other protocols here.
static const struct wl_interface sysc_session_lock_interface;
static const struct wl_interface sysc_session_lock_surface_interface;

static const struct wl_interface *sysc_session_lock_types[] = {
	&sysc_session_lock_interface,
	&sysc_session_lock_surface_interface, &wl_surface_interface, &wl_output_interface,
};
static const struct wl_message sysc_session_lock_manager_requests[] = {
	{"destroy", "", NULL},
	{"lock", "n", sysc_session_lock_types},
};
static const struct wl_interface sysc_session_lock_manager_interface = {
	"ext_session_lock_manager_v1", 1, 2, sysc_session_lock_manager_requests, 0, NULL,
};
static const struct wl_message sysc_session_lock_requests[] = {
	{"destroy", "", NULL},
	{"get_lock_surface", "noo", sysc_session_lock_types + 1},
	{"unlock_and_destroy", "", NULL},
};
static const struct wl_message sysc_session_lock_events[] = {
	{"locked", "", NULL},
	{"finished", "", NULL},
};
static const struct wl_interface sysc_session_lock_interface = {
	"ext_session_lock_v1", 1, 3, sysc_session_lock_requests, 2, sysc_session_lock_events,
};
static const struct wl_message sysc_session_lock_surface_requests[] = {
	{"destroy", "", NULL},
	{"ack_configure", "u", NULL},
};
static const struct wl_message sysc_session_lock_surface_events[] = {
	{"configure", "uuu", NULL},
};
static const struct wl_interface sysc_session_lock_surface_interface = {
	"ext_session_lock_surface_v1", 1, 2, sysc_session_lock_surface_requests,
	1, sysc_session_lock_surface_events,
};

#define SYSC_LOCK_MAX_OUTPUTS 8

struct sysc_lock_surface {
	struct sysc_lock_client *client;
	struct wl_surface *surface;
	struct wl_proxy *lock_surface;
	struct wl_buffer *buffer;
	uint32_t serial, width, height;
	bool configured;
};

struct sysc_lock_client {
	struct wl_display *display;
	struct wl_registry *registry;
	struct wl_compositor *compositor;
	struct wl_shm *shm;
	struct wl_proxy *manager;
	struct wl_proxy *lock;
	struct wl_output *outputs[SYSC_LOCK_MAX_OUTPUTS];
	int num_outputs;
	struct sysc_lock_surface surfaces[SYSC_LOCK_MAX_OUTPUTS];
	bool locked, finished;
};

static void sysc_lock_global(void *data, struct wl_registry *registry,
		uint32_t name, const char *interface, uint32_t version) {
	struct sysc_lock_client *client = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		client->compositor = wl_registry_bind(registry, name, &wl_compositor_interface, 4);
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		client->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
	} else if (strcmp(interface, wl_output_interface.name) == 0) {
		if (client->num_outputs < SYSC_LOCK_MAX_OUTPUTS) {
			client->outputs[client->num_outputs++] =
				wl_registry_bind(registry, name, &wl_output_interface, 1);
		}
	} else if (strcmp(interface, sysc_session_lock_manager_interface.name) == 0) {
		client->manager = wl_registry_bind(registry, name, &sysc_session_lock_manager_interface, 1);
	}
}

static void sysc_lock_global_remove(void *data, struct wl_registry *registry, uint32_t name) {
}

static const struct wl_registry_listener sysc_lock_registry_listener = {
	.global = sysc_lock_global,
	.global_remove = sysc_lock_global_remove,
};

static void sysc_lock_locked(void *data, struct wl_proxy *proxy) {
	((struct sysc_lock_client *)data)->locked = true;
}

static void sysc_lock_finished(void *data, struct wl_proxy *proxy) {
	((struct sysc_lock_client *)data)->finished = true;
}

struct sysc_lock_listener {
	void (*locked)(void *, struct wl_proxy *);
	void (*finished)(void *, struct wl_proxy *);
};
static const struct sysc_lock_listener sysc_lock_listener = {
	.locked = sysc_lock_locked,
	.finished = sysc_lock_finished,
};

static void sysc_lock_surface_configure(void *data, struct wl_proxy *proxy,
		uint32_t serial, uint32_t width, uint32_t height) {
	struct sysc_lock_surface *s = data;
	s->serial = serial;
	s->width = width;
	s->height = height;
	s->configured = true;
}

struct sysc_lock_surface_listener {
	void (*configure)(void *, struct wl_proxy *, uint32_t, uint32_t, uint32_t);
};
static const struct sysc_lock_surface_listener sysc_lock_surface_listener = {
	.configure = sysc_lock_surface_configure,
};

static struct wl_buffer *sysc_lock_buffer(struct wl_shm *shm, uint32_t width, uint32_t height) {
	int stride = (int)width * 4;
	int size = stride * (int)height;
	int fd = memfd_create("sysc-lock-test", MFD_CLOEXEC);
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
	struct wl_buffer *buffer = wl_shm_pool_create_buffer(pool, 0, (int)width, (int)height,
			stride, WL_SHM_FORMAT_XRGB8888);
	wl_shm_pool_destroy(pool);
	return buffer;
}

static int64_t sysc_lock_now_ms(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

// Dispatches until the deadline: 0 when something was dispatched, 1 when the
// deadline passed, -1 on a broken connection. A compositor that never
// answers fails the test instead of hanging it.
static int sysc_lock_dispatch(struct wl_display *display, int64_t deadline) {
	while (wl_display_prepare_read(display) != 0) {
		if (wl_display_dispatch_pending(display) < 0) {
			return -1;
		}
	}
	wl_display_flush(display);
	int64_t left = deadline - sysc_lock_now_ms();
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
	if (wl_display_read_events(display) < 0 || wl_display_dispatch_pending(display) < 0) {
		return -1;
	}
	return 0;
}

static void sysc_lock_client_free(struct sysc_lock_client *client) {
	if (client->display != NULL) {
		wl_display_disconnect(client->display);
	}
	free(client);
}

// Connects, asks for a lock and waits for `locked` or `finished`. With
// map_surfaces, it then gives every output a lock surface and maps it at the
// configured size. Returns 0, or the step that failed.
static int sysc_lock_client_start(const char *display_name, bool map_surfaces,
		struct sysc_lock_client **out) {
	struct sysc_lock_client *client = calloc(1, sizeof(*client));
	if (client == NULL) {
		return 1;
	}
	*out = client;
	client->display = wl_display_connect(display_name);
	if (client->display == NULL) {
		return 2;
	}
	client->registry = wl_display_get_registry(client->display);
	if (client->registry == NULL ||
			wl_registry_add_listener(client->registry, &sysc_lock_registry_listener, client) < 0 ||
			wl_display_roundtrip(client->display) < 0) {
		return 3;
	}
	if (client->compositor == NULL || client->shm == NULL || client->manager == NULL ||
			client->num_outputs == 0) {
		return 4;
	}
	client->lock = wl_proxy_marshal_flags(client->manager, 1, &sysc_session_lock_interface,
			1, 0, NULL);
	if (client->lock == NULL ||
			wl_proxy_add_listener(client->lock, (void (**)(void))&sysc_lock_listener, client) < 0) {
		return 5;
	}
	if (map_surfaces) {
		for (int i = 0; i < client->num_outputs; i++) {
			struct sysc_lock_surface *s = &client->surfaces[i];
			s->client = client;
			s->surface = wl_compositor_create_surface(client->compositor);
			if (s->surface == NULL) {
				return 6;
			}
			s->lock_surface = wl_proxy_marshal_flags(client->lock, 1,
					&sysc_session_lock_surface_interface, 1, 0, NULL, s->surface,
					client->outputs[i]);
			if (s->lock_surface == NULL || wl_proxy_add_listener(s->lock_surface,
					(void (**)(void))&sysc_lock_surface_listener, s) < 0) {
				return 7;
			}
		}
	}
	int64_t deadline = sysc_lock_now_ms() + 3000;
	while (!client->locked && !client->finished) {
		int r = sysc_lock_dispatch(client->display, deadline);
		if (r < 0) {
			return 8;
		}
		if (r > 0) {
			return 12; // no `locked` or `finished` within 3 s
		}
	}
	if (client->finished || !map_surfaces) {
		return 0;
	}
	for (int i = 0; i < client->num_outputs; i++) {
		struct sysc_lock_surface *s = &client->surfaces[i];
		while (!s->configured) {
			int r = sysc_lock_dispatch(client->display, deadline);
			if (r < 0) {
				return 9;
			}
			if (r > 0) {
				return 13; // no configure within the deadline
			}
		}
		s->buffer = sysc_lock_buffer(client->shm, s->width, s->height);
		if (s->buffer == NULL) {
			return 10;
		}
		wl_proxy_marshal_flags(s->lock_surface, 1, NULL, 1, 0, s->serial);
		wl_surface_attach(s->surface, s->buffer, 0, 0);
		wl_surface_damage(s->surface, 0, 0, (int)s->width, (int)s->height);
		wl_surface_commit(s->surface);
	}
	if (wl_display_roundtrip(client->display) < 0) {
		return 11;
	}
	return 0;
}

static int sysc_lock_client_unlock(struct sysc_lock_client *client) {
	for (int i = 0; i < client->num_outputs; i++) {
		struct sysc_lock_surface *s = &client->surfaces[i];
		if (s->lock_surface != NULL) {
			wl_proxy_marshal_flags(s->lock_surface, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
			s->lock_surface = NULL;
		}
	}
	wl_proxy_marshal_flags(client->lock, 2, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
	client->lock = NULL;
	return wl_display_roundtrip(client->display) < 0 ? 1 : 0;
}

static bool sysc_lock_client_finished(struct sysc_lock_client *client) { return client->finished; }
static int sysc_lock_client_outputs(struct sysc_lock_client *client) { return client->num_outputs; }
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// ErrLockFinished is what StartLockClient returns when the compositor refused
// the lock with `finished`.
var ErrLockFinished = errors.New("session lock refused with finished")

// LockClient is a minimal ext-session-lock-v1 locker. It holds the lock until
// Unlock, or until Close drops the connection the way a crashed locker would.
type LockClient struct {
	state *C.struct_sysc_lock_client
}

// StartLockClient locks the session and returns once the compositor has sent
// `locked`. With mapSurfaces it then maps one lock surface per output at the
// size the compositor configured. The connection is not dispatched after
// that, so the compositor's later events wait unread.
func StartLockClient(display string, mapSurfaces bool) (*LockClient, error) {
	name := C.CString(display)
	defer C.free(unsafe.Pointer(name))
	var state *C.struct_sysc_lock_client
	if code := int(C.sysc_lock_client_start(name, C.bool(mapSurfaces), &state)); code != 0 {
		if state != nil {
			C.sysc_lock_client_free(state)
		}
		return nil, fmt.Errorf("Wayland session-lock client failed at step %d", code)
	}
	if bool(C.sysc_lock_client_finished(state)) {
		C.sysc_lock_client_free(state)
		return nil, ErrLockFinished
	}
	return &LockClient{state: state}, nil
}

// Outputs is how many wl_output globals the locker saw.
func (l *LockClient) Outputs() int { return int(C.sysc_lock_client_outputs(l.state)) }

// Unlock destroys the lock surfaces and sends unlock_and_destroy.
func (l *LockClient) Unlock() error {
	if l == nil || l.state == nil {
		return errors.New("session-lock client is closed")
	}
	if code := int(C.sysc_lock_client_unlock(l.state)); code != 0 {
		return fmt.Errorf("Wayland session-lock unlock failed at step %d", code)
	}
	return nil
}

// Close disconnects. Without Unlock first, that is a locker crash.
func (l *LockClient) Close() {
	if l != nil && l.state != nil {
		C.sysc_lock_client_free(l.state)
		l.state = nil
	}
}
