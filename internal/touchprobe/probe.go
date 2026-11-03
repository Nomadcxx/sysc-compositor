// Package touchprobe provides a small Wayland client that asks the seat for
// touch and reports every event it receives.
//
// It is the evidence route for the touch gate: wayland-info and wlr-randr
// hang against the compositor's socket and cannot say whether a finger
// reaches a client, and the seat's capabilities are not exposed anywhere
// else. A client that binds wl_seat.get_touch and prints what it gets can.
//
// The package is deliberately free of the compositor: it is a client, and it
// links only against libwayland-client.
package touchprobe

/*
#cgo pkg-config: wayland-client

#define _GNU_SOURCE
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <wayland-client.h>
#include <wayland-client-protocol.h>

// xdg-shell, just enough to map a toplevel, written out by hand. The core
// protocol already carries wl_seat and wl_touch, so nothing else is needed.
static const struct wl_interface sysc_probe_wm_base_interface;
static const struct wl_interface sysc_probe_xdg_surface_interface;
static const struct wl_interface sysc_probe_xdg_toplevel_interface;

static const struct wl_interface *sysc_probe_wm_base_types[] = {NULL, &wl_surface_interface};
static const struct wl_interface *sysc_probe_xdg_surface_types[] = {NULL};

static const struct wl_message sysc_probe_wm_base_requests[] = {
	{"destroy", "", NULL},
	{"create_positioner", "n", NULL},
	{"get_xdg_surface", "no", sysc_probe_wm_base_types},
	{"pong", "u", NULL},
};
static const struct wl_message sysc_probe_wm_base_events[] = {{"ping", "u", NULL}};
static const struct wl_interface sysc_probe_wm_base_interface = {
	"xdg_wm_base", 1, 4, sysc_probe_wm_base_requests, 1, sysc_probe_wm_base_events,
};
static const struct wl_message sysc_probe_xdg_surface_requests[] = {
	{"destroy", "", NULL},
	{"get_toplevel", "n", sysc_probe_xdg_surface_types},
	{"get_popup", "", NULL},
	{"set_window_geometry", "iiii", NULL},
	{"ack_configure", "u", NULL},
};
static const struct wl_message sysc_probe_xdg_surface_events[] = {{"configure", "u", NULL}};
static const struct wl_interface sysc_probe_xdg_surface_interface = {
	"xdg_surface", 1, 5, sysc_probe_xdg_surface_requests, 1, sysc_probe_xdg_surface_events,
};
static const struct wl_message sysc_probe_xdg_toplevel_requests[] = {
	{"destroy", "", NULL},
	{"set_parent", "?o", NULL},
	{"set_title", "s", NULL},
	{"set_app_id", "s", NULL},
};
static const struct wl_message sysc_probe_xdg_toplevel_events[] = {
	{"configure", "iia", NULL},
	{"close", "", NULL},
};
static const struct wl_interface sysc_probe_xdg_toplevel_interface = {
	"xdg_toplevel", 1, 4, sysc_probe_xdg_toplevel_requests, 2, sysc_probe_xdg_toplevel_events,
};

enum {
	SYSC_PROBE_DOWN = 1,
	SYSC_PROBE_UP,
	SYSC_PROBE_MOTION,
	SYSC_PROBE_FRAME,
	SYSC_PROBE_CANCEL,
	SYSC_PROBE_CAPS,
	SYSC_PROBE_SEAT_NAME,
	SYSC_PROBE_TOUCH_BOUND,
	SYSC_PROBE_MAPPED,
	SYSC_PROBE_ERROR,
};

#define SYSC_PROBE_EVENTS 512

struct sysc_probe_event {
	int kind;
	int32_t id;
	double x;
	double y;
	int on_surface;
};

struct sysc_probe {
	struct wl_display *display;
	struct wl_registry *registry;
	struct wl_compositor *compositor;
	struct wl_shm *shm;
	struct wl_seat *seat;
	struct wl_touch *touch;
	struct wl_surface *surface;
	struct wl_buffer *buffer;
	struct wl_proxy *wm_base;
	struct wl_proxy *xdg_surface;
	struct wl_proxy *toplevel;

	uint32_t caps;
	int have_caps;
	int touch_bound;
	int configured;
	uint32_t configure_serial;
	int mapped;

	char seat_name[64];

	struct sysc_probe_event events[SYSC_PROBE_EVENTS];
	int head;
	int tail;
	int count;

	char error[256];
};

static void sysc_probe_push(struct sysc_probe *p, int kind, int32_t id, double x, double y,
		int on_surface) {
	if (p->count == SYSC_PROBE_EVENTS) {
		// The ring is full: the oldest event goes.
		p->head = (p->head + 1) % SYSC_PROBE_EVENTS;
		p->count--;
	}
	struct sysc_probe_event *e = &p->events[p->tail];
	e->kind = kind;
	e->id = id;
	e->x = x;
	e->y = y;
	e->on_surface = on_surface;
	p->tail = (p->tail + 1) % SYSC_PROBE_EVENTS;
	p->count++;
}

// The touch events, one line each.
static void sysc_probe_touch_down(void *data, struct wl_touch *touch, uint32_t serial,
		uint32_t time, struct wl_surface *surface, int32_t id, wl_fixed_t x, wl_fixed_t y) {
	struct sysc_probe *p = data;
	sysc_probe_push(p, SYSC_PROBE_DOWN, id, wl_fixed_to_double(x), wl_fixed_to_double(y),
			surface == p->surface);
}
static void sysc_probe_touch_up(void *data, struct wl_touch *touch, uint32_t serial,
		uint32_t time, int32_t id) {
	struct sysc_probe *p = data;
	sysc_probe_push(p, SYSC_PROBE_UP, id, 0, 0, 0);
}
static void sysc_probe_touch_motion(void *data, struct wl_touch *touch, uint32_t time,
		int32_t id, wl_fixed_t x, wl_fixed_t y) {
	struct sysc_probe *p = data;
	sysc_probe_push(p, SYSC_PROBE_MOTION, id, wl_fixed_to_double(x), wl_fixed_to_double(y), 0);
}
static void sysc_probe_touch_frame(void *data, struct wl_touch *touch) {
	struct sysc_probe *p = data;
	sysc_probe_push(p, SYSC_PROBE_FRAME, 0, 0, 0, 0);
}
static void sysc_probe_touch_cancel(void *data, struct wl_touch *touch) {
	struct sysc_probe *p = data;
	sysc_probe_push(p, SYSC_PROBE_CANCEL, 0, 0, 0, 0);
}
static const struct wl_touch_listener sysc_probe_touch_listener = {
	.down = sysc_probe_touch_down,
	.up = sysc_probe_touch_up,
	.motion = sysc_probe_touch_motion,
	.frame = sysc_probe_touch_frame,
	.cancel = sysc_probe_touch_cancel,
};

// The seat's capabilities decide whether touch is offered at all: asking for
// it without the capability is a protocol error, so the probe waits for the
// capability and asks then.
static void sysc_probe_seat_capabilities(void *data, struct wl_seat *seat, uint32_t caps) {
	struct sysc_probe *p = data;
	p->caps = caps;
	p->have_caps = 1;
	sysc_probe_push(p, SYSC_PROBE_CAPS, 0, 0, 0, 0);
	if ((caps & WL_SEAT_CAPABILITY_TOUCH) && p->touch == NULL) {
		p->touch = wl_seat_get_touch(seat);
		if (p->touch != NULL) {
			wl_touch_add_listener(p->touch, &sysc_probe_touch_listener, p);
			p->touch_bound = 1;
			sysc_probe_push(p, SYSC_PROBE_TOUCH_BOUND, 0, 0, 0, 0);
		}
	}
}
static void sysc_probe_seat_name(void *data, struct wl_seat *seat, const char *name) {
	struct sysc_probe *p = data;
	if (name != NULL) {
		snprintf(p->seat_name, sizeof(p->seat_name), "%s", name);
	}
	sysc_probe_push(p, SYSC_PROBE_SEAT_NAME, 0, 0, 0, 0);
}
static const struct wl_seat_listener sysc_probe_seat_listener = {
	.capabilities = sysc_probe_seat_capabilities,
	.name = sysc_probe_seat_name,
};

static void sysc_probe_wm_base_ping(void *data, struct wl_proxy *proxy, uint32_t serial) {
	struct sysc_probe *p = data;
	wl_proxy_marshal_flags(proxy, 3, NULL, 1, 0, serial);
}
struct sysc_probe_wm_base_listener {
	void (*ping)(void *, struct wl_proxy *, uint32_t);
};
static const struct sysc_probe_wm_base_listener sysc_probe_wm_base_listener = {
	.ping = sysc_probe_wm_base_ping,
};

static void sysc_probe_surface_configure(void *data, struct wl_proxy *proxy, uint32_t serial) {
	struct sysc_probe *p = data;
	p->configure_serial = serial;
	p->configured = 1;
}
struct sysc_probe_xdg_surface_listener {
	void (*configure)(void *, struct wl_proxy *, uint32_t);
};
static const struct sysc_probe_xdg_surface_listener sysc_probe_xdg_surface_listener = {
	.configure = sysc_probe_surface_configure,
};

static void sysc_probe_toplevel_configure(void *data, struct wl_proxy *proxy, int32_t width,
		int32_t height, struct wl_array *states) {
}
static void sysc_probe_toplevel_close(void *data, struct wl_proxy *proxy) {
}
struct sysc_probe_xdg_toplevel_listener {
	void (*configure)(void *, struct wl_proxy *, int32_t, int32_t, struct wl_array *);
	void (*close)(void *, struct wl_proxy *);
};
static const struct sysc_probe_xdg_toplevel_listener sysc_probe_xdg_toplevel_listener = {
	.configure = sysc_probe_toplevel_configure,
	.close = sysc_probe_toplevel_close,
};

static void sysc_probe_registry_global(void *data, struct wl_registry *registry, uint32_t name,
		const char *interface, uint32_t version) {
	struct sysc_probe *p = data;
	if (strcmp(interface, wl_compositor_interface.name) == 0) {
		if (p->compositor == NULL) {
			p->compositor = wl_registry_bind(registry, name, &wl_compositor_interface, 4);
		}
	} else if (strcmp(interface, wl_shm_interface.name) == 0) {
		if (p->shm == NULL) {
			p->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
		}
	} else if (strcmp(interface, wl_seat_interface.name) == 0) {
		if (p->seat == NULL) {
			p->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
			if (p->seat != NULL) {
				wl_seat_add_listener(p->seat, &sysc_probe_seat_listener, p);
			}
		}
	} else if (strcmp(interface, sysc_probe_wm_base_interface.name) == 0) {
		if (p->wm_base == NULL) {
			p->wm_base = wl_registry_bind(registry, name, &sysc_probe_wm_base_interface, 1);
			if (p->wm_base != NULL) {
				wl_proxy_add_listener(p->wm_base,
						(void (**)(void))&sysc_probe_wm_base_listener, p);
			}
		}
	}
}
static void sysc_probe_registry_remove(void *data, struct wl_registry *registry, uint32_t name) {
}
static const struct wl_registry_listener sysc_probe_registry_listener = {
	.global = sysc_probe_registry_global,
	.global_remove = sysc_probe_registry_remove,
};

static struct wl_buffer *sysc_probe_buffer(struct wl_shm *shm, int width, int height) {
	int stride = width * 4, size = stride * height;
	int fd = memfd_create("sysc-touch-probe", MFD_CLOEXEC);
	if (fd < 0) {
		return NULL;
	}
	if (ftruncate(fd, size) < 0) {
		close(fd);
		return NULL;
	}
	uint32_t *pixels = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
	if (pixels == MAP_FAILED) {
		close(fd);
		return NULL;
	}
	for (int i = 0; i < width * height; i++) {
		pixels[i] = 0xff224466;
	}
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

// Connects, binds what the probe needs and maps a toplevel. Returns 0, or
// the step that failed.
static int sysc_probe_start(const char *display_name, const char *title, int with_window,
		struct sysc_probe **out) {
	struct sysc_probe *p = calloc(1, sizeof(*p));
	if (p == NULL) {
		return 1;
	}
	*out = p;
	if ((p->display = wl_display_connect(display_name)) == NULL) {
		snprintf(p->error, sizeof(p->error), "cannot connect to the display");
		return 2;
	}
	p->registry = wl_display_get_registry(p->display);
	if (p->registry == NULL) {
		snprintf(p->error, sizeof(p->error), "no registry");
		return 3;
	}
	wl_registry_add_listener(p->registry, &sysc_probe_registry_listener, p);
	if (wl_display_roundtrip(p->display) < 0) {
		snprintf(p->error, sizeof(p->error), "the registry roundtrip failed");
		return 4;
	}
	if (p->compositor == NULL || p->shm == NULL || p->seat == NULL || p->wm_base == NULL) {
		snprintf(p->error, sizeof(p->error),
				"the compositor offers no compositor, shm, seat or xdg_wm_base");
		return 5;
	}
	if (!with_window) {
		return 0;
	}
	p->surface = wl_compositor_create_surface(p->compositor);
	p->xdg_surface = wl_proxy_marshal_flags(p->wm_base, 2, &sysc_probe_xdg_surface_interface,
			1, 0, NULL, p->surface);
	if (p->surface == NULL || p->xdg_surface == NULL) {
		snprintf(p->error, sizeof(p->error), "no surface");
		return 6;
	}
	wl_proxy_add_listener(p->xdg_surface, (void (**)(void))&sysc_probe_xdg_surface_listener, p);
	p->toplevel = wl_proxy_marshal_flags(p->xdg_surface, 1, &sysc_probe_xdg_toplevel_interface,
			1, 0, NULL);
	if (p->toplevel == NULL) {
		snprintf(p->error, sizeof(p->error), "no toplevel");
		return 7;
	}
	wl_proxy_add_listener(p->toplevel, (void (**)(void))&sysc_probe_xdg_toplevel_listener, p);
	wl_proxy_marshal_flags(p->toplevel, 2, NULL, 1, 0, title);
	wl_proxy_marshal_flags(p->toplevel, 3, NULL, 1, 0, "sysc-compositor-touch-probe");
	wl_surface_commit(p->surface);
	while (!p->configured) {
		if (wl_display_dispatch(p->display) < 0) {
			snprintf(p->error, sizeof(p->error), "the toplevel was never configured");
			return 8;
		}
	}
	p->buffer = sysc_probe_buffer(p->shm, 200, 150);
	if (p->buffer == NULL) {
		snprintf(p->error, sizeof(p->error), "no buffer");
		return 9;
	}
	wl_proxy_marshal_flags(p->xdg_surface, 4, NULL, 1, 0, p->configure_serial);
	wl_surface_attach(p->surface, p->buffer, 0, 0);
	wl_surface_damage(p->surface, 0, 0, 200, 150);
	wl_surface_commit(p->surface);
	if (wl_display_roundtrip(p->display) < 0) {
		snprintf(p->error, sizeof(p->error), "the window commit failed");
		return 10;
	}
	p->mapped = 1;
	sysc_probe_push(p, SYSC_PROBE_MAPPED, 0, 0, 0, 0);
	return 0;
}

// Reads and dispatches whatever arrives within timeout_ms. Returns 0, or -1
// on a broken connection.
static int sysc_probe_dispatch(struct sysc_probe *p, int timeout_ms) {
	while (wl_display_prepare_read(p->display) != 0) {
		if (wl_display_dispatch_pending(p->display) < 0) {
			return -1;
		}
	}
	if (wl_display_flush(p->display) < 0) {
		wl_display_cancel_read(p->display);
		return -1;
	}
	struct pollfd pfd = { .fd = wl_display_get_fd(p->display), .events = POLLIN };
	int ready = poll(&pfd, 1, timeout_ms);
	if (ready <= 0) {
		wl_display_cancel_read(p->display);
		// Go preempts threads with SIGURG, which interrupts poll.
		return (ready == 0 || errno == EINTR) ? 0 : -1;
	}
	if (wl_display_read_events(p->display) < 0 || wl_display_dispatch_pending(p->display) < 0) {
		return -1;
	}
	return 0;
}

// Takes the pending events. The caller reads them with sysc_probe_event_at
// and then releases them.
static int sysc_probe_take(struct sysc_probe *p) {
	return p->count;
}
static struct sysc_probe_event *sysc_probe_event_at(struct sysc_probe *p, int index) {
	return &p->events[(p->head + index) % SYSC_PROBE_EVENTS];
}
static void sysc_probe_release(struct sysc_probe *p) {
	p->head = p->tail;
	p->count = 0;
}
static int sysc_probe_event_kind(struct sysc_probe_event *e) { return e->kind; }
static int32_t sysc_probe_event_id(struct sysc_probe_event *e) { return e->id; }
static double sysc_probe_event_x(struct sysc_probe_event *e) { return e->x; }
static double sysc_probe_event_y(struct sysc_probe_event *e) { return e->y; }
static int sysc_probe_event_on_surface(struct sysc_probe_event *e) { return e->on_surface; }

static uint32_t sysc_probe_caps(struct sysc_probe *p) { return p->caps; }
static int sysc_probe_touch_bound(struct sysc_probe *p) { return p->touch_bound; }
static int sysc_probe_mapped(struct sysc_probe *p) { return p->mapped; }
static const char *sysc_probe_seat_label(struct sysc_probe *p) { return p->seat_name; }
static const char *sysc_probe_error(struct sysc_probe *p) { return p->error; }

static void sysc_probe_free(struct sysc_probe *p) {
	if (p == NULL) {
		return;
	}
	if (p->display != NULL) {
		wl_display_disconnect(p->display);
	}
	free(p);
}
*/
import "C"

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// The event kinds the client reports.
const (
	eventDown      = 1
	eventUp        = 2
	eventMotion    = 3
	eventFrame     = 4
	eventCancel    = 5
	eventCaps      = 6
	eventSeatName  = 7
	eventTouchBind = 8
	eventMapped    = 9
)

// Options is what a probe run asks for.
type Options struct {
	// Display is the Wayland display name. An empty name uses
	// $WAYLAND_DISPLAY.
	Display string
	// Title is the toplevel title. The probe's window is what a finger
	// lands on, so it is mapped unless Window is false.
	Title string
	// Window maps a toplevel, so there is a surface for a finger to land
	// on.
	Window bool
	// Timeout is how long the probe waits for a finger. Zero waits until
	// the probe is stopped.
	Timeout time.Duration
	// Out receives a line per event as it arrives.
	Out io.Writer
}

// Result is what the probe saw.
type Result struct {
	Capabilities uint32
	SeatName     string
	TouchBound   bool
	Mapped       bool
	Downs        int
	Ups          int
	Motions      int
	Frames       int
	Cancels      int
	OnSurface    bool
	LastID       int32
	LastX        float64
	LastY        float64
}

// HasTouchCapability says whether the seat advertised touch.
func (r Result) HasTouchCapability() bool { return r.Capabilities&capabilityTouch != 0 }

// Summary is the one line that says what the probe saw.
func (r Result) Summary() string {
	return fmt.Sprintf("touch-probe: summary downs=%d ups=%d motions=%d frames=%d cancels=%d on_surface=%d",
		r.Downs, r.Ups, r.Motions, r.Frames, r.Cancels, boolToInt(r.OnSurface))
}

// The seat's capability bits.
const (
	capabilityPointer  = 1
	capabilityKeyboard = 2
	capabilityTouch    = 4
)

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// CapabilityNames spells the seat's capability bits out, in the order the
// protocol numbers them.
func CapabilityNames(caps uint32) string {
	names := []string{}
	if caps&capabilityPointer != 0 {
		names = append(names, "pointer")
	}
	if caps&capabilityKeyboard != 0 {
		names = append(names, "keyboard")
	}
	if caps&capabilityTouch != 0 {
		names = append(names, "touch")
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "|")
}

// Probe is a running probe. Its client thread owns the Wayland connection;
// everything else is read under a lock.
type Probe struct {
	state   *C.struct_sysc_probe
	out     io.Writer
	timeout time.Duration

	mu     sync.Mutex
	events []string
	result Result

	arrived     chan struct{}
	arrivedOnce sync.Once
	stopOnce    sync.Once
	stop        chan struct{}
	done        chan struct{}
	errMu       sync.Mutex
	err         error
}

// Start connects to the display and starts the probe. The connection is
// opened and the window mapped before Start returns, so a caller that gets
// no error knows the probe is on the seat.
func Start(opts Options) (*Probe, error) {
	title := opts.Title
	if title == "" {
		title = "sysc touch probe"
	}
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	var cDisplay *C.char
	if opts.Display != "" {
		cDisplay = C.CString(opts.Display)
		defer C.free(unsafe.Pointer(cDisplay))
	}
	window := C.int(0)
	if opts.Window {
		window = 1
	}
	var state *C.struct_sysc_probe
	if code := int(C.sysc_probe_start(cDisplay, cTitle, window, &state)); code != 0 {
		detail := ""
		if state != nil {
			detail = C.GoString(C.sysc_probe_error(state))
			C.sysc_probe_free(state)
		}
		return nil, fmt.Errorf("touch probe failed at step %d: %s", code, detail)
	}
	p := &Probe{
		state:   state,
		out:     opts.Out,
		timeout: opts.Timeout,
		arrived: make(chan struct{}),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go p.loop()
	return p, nil
}

// Run starts a probe, waits for it to finish and reports what it saw.
func Run(opts Options) (Result, error) {
	probe, err := Start(opts)
	if err != nil {
		return Result{}, err
	}
	defer probe.Close()
	if err := probe.Wait(); err != nil {
		return probe.Result(), err
	}
	return probe.Result(), nil
}

// Stop ends the probe. The connection is closed when Close is called.
func (p *Probe) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })
}

// Arrived is closed when the first finger goes down.
func (p *Probe) Arrived() <-chan struct{} { return p.arrived }

// Wait waits for the probe to finish, and reports why it stopped.
func (p *Probe) Wait() error {
	<-p.done
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}

// Close stops the probe and closes the connection.
func (p *Probe) Close() {
	p.Stop()
	<-p.done
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if p.state != nil {
		C.sysc_probe_free(p.state)
		p.state = nil
	}
}

// Result is what the probe has seen so far.
func (p *Probe) Result() Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// Events are the lines the probe has written, in order.
func (p *Probe) Events() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

func (p *Probe) setError(err error) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if p.err == nil {
		p.err = err
	}
}

// loop reads the connection until the probe is stopped or the timeout runs
// out.
func (p *Probe) loop() {
	defer close(p.done)
	var deadline time.Time
	if p.timeout > 0 {
		deadline = time.Now().Add(p.timeout)
	}
	for {
		p.drain()
		select {
		case <-p.stop:
			p.drain()
			return
		default:
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			p.drain()
			return
		}
		if C.sysc_probe_dispatch(p.state, 50) < 0 {
			p.setError(fmt.Errorf("the wayland connection failed"))
			p.drain()
			return
		}
	}
}

// drain takes the events the client received and turns them into lines.
func (p *Probe) drain() {
	count := int(C.sysc_probe_take(p.state))
	if count <= 0 {
		return
	}
	p.mu.Lock()
	result := p.result
	lines := []string{}
	for i := 0; i < count; i++ {
		event := C.sysc_probe_event_at(p.state, C.int(i))
		kind := int(C.sysc_probe_event_kind(event))
		id := int32(C.sysc_probe_event_id(event))
		x := float64(C.sysc_probe_event_x(event))
		y := float64(C.sysc_probe_event_y(event))
		onSurface := C.sysc_probe_event_on_surface(event) != 0
		switch kind {
		case eventDown:
			result.Downs++
			result.OnSurface = onSurface
			result.LastID, result.LastX, result.LastY = id, x, y
			lines = append(lines, fmt.Sprintf("touch-probe: down id=%d x=%.1f y=%.1f surface=%s",
				id, x, y, surfaceName(onSurface)))
		case eventUp:
			result.Ups++
			result.LastID = id
			lines = append(lines, fmt.Sprintf("touch-probe: up id=%d", id))
		case eventMotion:
			result.Motions++
			result.LastID, result.LastX, result.LastY = id, x, y
			lines = append(lines, fmt.Sprintf("touch-probe: motion id=%d x=%.1f y=%.1f", id, x, y))
		case eventFrame:
			result.Frames++
			lines = append(lines, "touch-probe: frame")
		case eventCancel:
			result.Cancels++
			lines = append(lines, "touch-probe: cancel")
		case eventCaps:
			result.Capabilities = uint32(C.sysc_probe_caps(p.state))
			lines = append(lines, "touch-probe: seat capabilities="+CapabilityNames(result.Capabilities))
		case eventSeatName:
			result.SeatName = C.GoString(C.sysc_probe_seat_label(p.state))
			lines = append(lines, "touch-probe: seat name="+result.SeatName)
		case eventTouchBind:
			result.TouchBound = true
			lines = append(lines, "touch-probe: touch bound")
		case eventMapped:
			result.Mapped = true
			lines = append(lines, "touch-probe: window mapped")
		}
	}
	p.events = append(p.events, lines...)
	p.result = result
	p.mu.Unlock()
	C.sysc_probe_release(p.state)
	if p.out != nil {
		for _, line := range lines {
			fmt.Fprintln(p.out, line)
		}
	}
	if result.Downs > 0 {
		p.arrivedOnce.Do(func() { close(p.arrived) })
	}
}

// surfaceName says whether the event landed on the probe's own window.
func surfaceName(onSurface bool) string {
	if onSurface {
		return "probe"
	}
	return "other"
}

// DisplayName is the display the probe would use with no display given.
func DisplayName() string { return os.Getenv("WAYLAND_DISPLAY") }
