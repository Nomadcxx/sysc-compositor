package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE -I${SRCDIR}/protocols

#include <stdlib.h>

#include <wlr/backend.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_fractional_scale_v1.h>
#include <wlr/types/wlr_layer_shell_v1.h>
#include <wlr/types/wlr_linux_dmabuf_v1.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_shm.h>
#include <wlr/types/wlr_subcompositor.h>
#include <wlr/types/wlr_viewporter.h>
#include <wlr/types/wlr_xdg_output_v1.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_xcursor_manager.h>

// Global versions match what Niri 26.04 advertises (wayland-info on the host,
// 2026-10-16), capped by wlroots 0.20. sysc-shell needs layer-shell 4 at
// least and binds no higher.
const uint32_t sysc_compositor_version = 6;
const uint32_t sysc_shm_version = 2;
const uint32_t sysc_xdg_version = 7;
const uint32_t sysc_layer_shell_version = 5;
const uint32_t sysc_dmabuf_version = 5;

// The scene sends each surface its output's fractional scale itself
// (types/scene/surface.c in 0.20), so the manager needs nothing else.
static bool sysc_fractional_scale_create(struct wl_display *display) {
	return wlr_fractional_scale_manager_v1_create(display, 1) != NULL;
}

// dmabuf feedback comes from the scene: it tells clients which buffers can be
// scanned out on the output they are on.
static bool sysc_dmabuf_create(struct wl_display *display,
		struct wlr_renderer *renderer, struct wlr_scene *scene) {
	struct wlr_linux_dmabuf_v1 *dmabuf = wlr_linux_dmabuf_v1_create_with_renderer(
			display, sysc_dmabuf_version, renderer);
	if (dmabuf == NULL) {
		return false;
	}
	wlr_scene_set_linux_dmabuf_v1(scene, dmabuf);
	return true;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Compositor holds the wlroots object graph for one running server. Every
// method must be called on the thread that ran LockOSThread.
type Compositor struct {
	display    *C.struct_wl_display
	backend    *C.struct_wlr_backend
	renderer   *C.struct_wlr_renderer
	alloc      *C.struct_wlr_allocator
	comp       *C.struct_wlr_compositor
	layout     *C.struct_wlr_output_layout
	scene      *C.struct_wlr_scene
	seat       *C.struct_wlr_seat
	xdgShell   *C.struct_wlr_xdg_shell
	layerShell *C.struct_wlr_layer_shell_v1
	socketName string
	listeners  map[*Listener]struct{}

	loop                   *C.struct_wl_event_loop
	cursor                 *C.struct_wlr_cursor
	xcursorManager         *C.struct_wlr_xcursor_manager
	sceneOutputs           map[unsafe.Pointer]*C.struct_wlr_scene_output
	frames                 map[Signal]*C.struct_wlr_scene_output
	mapped                 map[unsafe.Pointer]bool
	sceneTrees             map[unsafe.Pointer]*C.struct_wlr_scene_tree
	layerTrees             [layerCount]*C.struct_wlr_scene_tree
	layerSurfaces          map[unsafe.Pointer]*layerSurface // by wlr_surface
	layerOrder             []*layerSurface                  // creation order
	usable                 map[unsafe.Pointer]rect          // per wlr_output, after exclusive zones
	requested              rect                             // last host-requested output size, for tests
	popups                 map[*C.struct_wlr_xdg_popup]struct{}
	views                  map[unsafe.Pointer]*toplevelView
	pointerDevices         map[unsafe.Pointer]bool
	keyboardDevices        map[unsafe.Pointer]*C.struct_wlr_keyboard
	keyboardListeners      map[unsafe.Pointer][]*Listener
	inputDestroyListeners  map[unsafe.Pointer]*Listener
	activeKeyboard         *C.struct_wlr_keyboard
	modifiers              uint32
	focusedView            *toplevelView
	pointerGrab            *pointerGrab
	grabView               *toplevelView
	grabButton             uint32
	pressedPointerButtons  map[uint32]bool
	pointerSerialButtons   map[uint32]uint32
	consumedPointerButtons map[uint32]bool
	capabilities           int

	// injected is the table of seats fed by the compositor rather than the
	// user's hardware, after the user seat (c.seat) in creation order.
	injected []*injectedSeat

	// viewMapped, when set, runs on the compositor thread once a toplevel is
	// mapped and hit-testable. It is the hook tests use to act once a client
	// is on screen.
	viewMapped func(*toplevelView)

	// post carries work from other goroutines onto the compositor thread.
	post postQueue

	// nextViewID and touchCount give windows stable ids and a
	// most-recently-focused order for the control socket.
	nextViewID uint64
	touchCount uint64
}

// New creates the display, its nested socket, and the globals a client needs to
// map an xdg toplevel. It does not start the event loop.
func New() (*Compositor, error) {
	c := &Compositor{}

	c.display = C.wl_display_create()
	if c.display == nil {
		return nil, errors.New("wl_display_create failed")
	}

	name := C.wl_display_add_socket_auto(c.display)
	if name == nil {
		return nil, errors.New("wl_display_add_socket_auto failed: no free wayland socket")
	}
	c.socketName = C.GoString(name)

	c.loop = C.wl_display_get_event_loop(c.display)
	c.listeners = map[*Listener]struct{}{}
	c.sceneOutputs = map[unsafe.Pointer]*C.struct_wlr_scene_output{}
	c.frames = map[Signal]*C.struct_wlr_scene_output{}
	c.mapped = map[unsafe.Pointer]bool{}
	c.sceneTrees = map[unsafe.Pointer]*C.struct_wlr_scene_tree{}
	c.layerSurfaces = map[unsafe.Pointer]*layerSurface{}
	c.usable = map[unsafe.Pointer]rect{}
	c.popups = map[*C.struct_wlr_xdg_popup]struct{}{}
	c.views = map[unsafe.Pointer]*toplevelView{}
	c.pointerDevices = map[unsafe.Pointer]bool{}
	c.keyboardDevices = map[unsafe.Pointer]*C.struct_wlr_keyboard{}
	c.keyboardListeners = map[unsafe.Pointer][]*Listener{}
	c.inputDestroyListeners = map[unsafe.Pointer]*Listener{}
	c.pressedPointerButtons = map[uint32]bool{}
	c.pointerSerialButtons = map[uint32]uint32{}
	c.consumedPointerButtons = map[uint32]bool{}

	c.backend = C.wlr_backend_autocreate(c.loop, nil)
	if c.backend == nil {
		return nil, errors.New("wlr_backend_autocreate failed")
	}

	c.renderer = C.wlr_renderer_autocreate(c.backend)
	if c.renderer == nil {
		return nil, errors.New("wlr_renderer_autocreate failed")
	}

	c.alloc = C.wlr_allocator_autocreate(c.backend, c.renderer)
	if c.comp = C.wlr_compositor_create(c.display, C.sysc_compositor_version, c.renderer); c.comp == nil {
		return nil, errors.New("wlr_compositor_create failed")
	}
	if c.alloc == nil {
		return nil, errors.New("wlr_allocator_autocreate failed")
	}

	C.wlr_subcompositor_create(c.display)
	C.wlr_data_device_manager_create(c.display)
	C.wlr_viewporter_create(c.display)
	if C.wlr_shm_create_with_renderer(c.display, C.sysc_shm_version, c.renderer) == nil {
		return nil, errors.New("wlr_shm_create_with_renderer failed")
	}
	if c.layout = C.wlr_output_layout_create(c.display); c.layout == nil {
		return nil, errors.New("wlr_output_layout_create failed")
	}
	if c.xdgShell = C.wlr_xdg_shell_create(c.display, C.uint32_t(C.sysc_xdg_version)); c.xdgShell == nil {
		return nil, errors.New("wlr_xdg_shell_create failed")
	}
	if c.scene = C.wlr_scene_create(); c.scene == nil {
		return nil, errors.New("wlr_scene_create failed")
	}
	if !c.createLayerTrees() {
		return nil, errors.New("wlr_scene_tree_create failed")
	}
	// xdg-output gives clients output names and logical geometry; waybar and
	// grim refuse to start without it, and Niri advertises it.
	if C.wlr_xdg_output_manager_v1_create(c.display, c.layout) == nil {
		return nil, errors.New("wlr_xdg_output_manager_v1_create failed")
	}
	if c.layerShell = C.wlr_layer_shell_v1_create(c.display, C.sysc_layer_shell_version); c.layerShell == nil {
		return nil, errors.New("wlr_layer_shell_v1_create failed")
	}
	if !bool(C.sysc_fractional_scale_create(c.display)) {
		return nil, errors.New("wlr_fractional_scale_manager_v1_create failed")
	}
	if !bool(C.sysc_dmabuf_create(c.display, c.renderer, c.scene)) {
		return nil, errors.New("wlr_linux_dmabuf_v1_create_with_renderer failed: the renderer offers no dmabuf formats")
	}
	if c.seat = C.wlr_seat_create(c.display, cCString("seat0")); c.seat == nil {
		return nil, errors.New("wlr_seat_create failed")
	}

	if c.cursor = C.wlr_cursor_create(); c.cursor == nil {
		return nil, errors.New("wlr_cursor_create failed")
	}
	C.wlr_cursor_attach_output_layout(c.cursor, c.layout)

	// wlroots 0.20 raises these itself; the compositor must listen before the
	// backend starts, because wlr_backend_start announces outputs immediately.
	c.keep(
		AddListener(unsafe.Pointer(&c.backend.events.new_output), c.handleNewOutput),
		AddListener(unsafe.Pointer(&c.backend.events.new_input), c.handleNewInput),
		AddListener(c.newPopupSignal(), c.handleNewPopup),
		AddListener(unsafe.Pointer(&c.layerShell.events.new_surface), c.handleNewLayerSurface),
		AddListener(c.newToplevelSignal(), c.handleNewToplevel),
		AddListener(unsafe.Pointer(&c.cursor.events.motion), c.handleCursorMotion),
		AddListener(unsafe.Pointer(&c.cursor.events.motion_absolute), c.handleCursorMotionAbsolute),
		AddListener(unsafe.Pointer(&c.cursor.events.button), c.handleCursorButton),
		AddListener(unsafe.Pointer(&c.cursor.events.axis), c.handleCursorAxis),
		AddListener(unsafe.Pointer(&c.cursor.events.frame), c.handleCursorFrame),
		AddListener(unsafe.Pointer(&c.seat.events.request_set_cursor), c.handleRequestSetCursor),
	)

	if err := c.startPost(); err != nil {
		return nil, err
	}
	if !bool(C.wlr_backend_start(c.backend)) {
		return nil, errors.New("wlr_backend_start failed")
	}
	return c, nil
}

// SocketName is the nested Wayland socket this compositor serves. A smoke script
// spawns clients against it; it is not the compositor's own WAYLAND_DISPLAY.
func (c *Compositor) SocketName() string { return c.socketName }

// Display exposes the wl_display for callers that must reach wlroots directly.
func (c *Compositor) Display() unsafe.Pointer { return unsafe.Pointer(c.display) }

// Seat returns the single seat created at New.
func (c *Compositor) Seat() unsafe.Pointer { return unsafe.Pointer(c.seat) }

// Scene returns the scene-graph root clients are mapped into.
func (c *Compositor) Scene() unsafe.Pointer { return unsafe.Pointer(c.scene) }

// Run starts the event loop. It blocks until Terminate is called.
func (c *Compositor) Run() error {
	C.wl_display_run(c.display)
	return nil
}

// Terminate asks Run to return. It is safe to call from a signal goroutine
// because wl_display_terminate only writes a flag on the display.
func (c *Compositor) Terminate() { C.wl_display_terminate(c.display) }

// Destroy releases the compositor. Listeners added through AddListener are
// removed first so no token outlives its wl_signal.
func (c *Compositor) Destroy() {
	c.stopPost()
	// Before any seat goes: wlroots frees a seat's popup grab twice when the
	// seat is destroyed with the grab open (dismissGrabbedPopups).
	c.dismissGrabbedPopups(nil)
	for l := range c.listeners {
		l.Destroy()
	}
	c.listeners = nil
	c.sceneOutputs = nil
	c.frames = nil
	c.mapped = nil
	c.sceneTrees = nil
	c.layerSurfaces = nil
	c.layerOrder = nil
	c.usable = nil
	c.popups = nil
	c.views = nil
	c.pointerDevices = nil
	c.keyboardDevices = nil
	c.keyboardListeners = nil
	c.inputDestroyListeners = nil
	c.destroyInjectedSeats()
	if c.cursor != nil {
		C.wlr_cursor_destroy(c.cursor)
		c.cursor = nil
	}
	if c.xcursorManager != nil {
		C.wlr_xcursor_manager_destroy(c.xcursorManager)
		c.xcursorManager = nil
	}
	if c.display != nil {
		C.wl_display_destroy(c.display)
		c.display = nil
	}
}

func cCString(s string) *C.char { return C.CString(s) }

// keep holds listeners until they are dropped or the compositor is destroyed.
func (c *Compositor) keep(ls ...*Listener) {
	for _, l := range ls {
		c.listeners[l] = struct{}{}
	}
}

// drop destroys a listener whose object has gone and forgets it, so windows
// and devices that come and go leave nothing behind.
func (c *Compositor) drop(l *Listener) {
	l.Destroy()
	delete(c.listeners, l)
}

// liveListeners is how many listeners the compositor holds, for tests.
func (c *Compositor) liveListeners() int { return len(c.listeners) }
