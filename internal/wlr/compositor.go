package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE -I${SRCDIR}/protocols

#include <stdlib.h>

#include <wlr/backend.h>
#include <wlr/backend/multi.h>
#include <wlr/backend/session.h>
#include <wlr/backend/wayland.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard_group.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_pointer_gestures_v1.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_data_device.h>
#include <wlr/types/wlr_fractional_scale_v1.h>
#include <wlr/types/wlr_layer_shell_v1.h>
#include <wlr/types/wlr_idle_inhibit_v1.h>
#include <wlr/types/wlr_idle_notify_v1.h>
#include <wlr/types/wlr_keyboard_shortcuts_inhibit_v1.h>
#include <wlr/types/wlr_input_method_v2.h>
#include <wlr/types/wlr_linux_dmabuf_v1.h>
#include <wlr/types/wlr_text_input_v3.h>
#include <wlr/types/wlr_pointer_constraints_v1.h>
#include <wlr/types/wlr_relative_pointer_v1.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_shm.h>
#include <wlr/types/wlr_subcompositor.h>
#include <wlr/types/wlr_viewporter.h>
#include <wlr/types/wlr_xdg_output_v1.h>
#include <wlr/types/wlr_xdg_activation_v1.h>
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

// The automatically selected backend can wrap a nested Wayland backend in a
// multi-backend, so inspect its children before choosing the nested Mod key.
static void sysc_backend_find_wayland(struct wlr_backend *backend, void *data) {
	bool *nested = data;
	if (*nested) {
		return;
	}
	if (wlr_backend_is_wl(backend)) {
		*nested = true;
	} else if (wlr_backend_is_multi(backend)) {
		wlr_multi_for_each_backend(backend, sysc_backend_find_wayland, data);
	}
}

static bool sysc_backend_is_nested(struct wlr_backend *backend) {
	bool nested = false;
	sysc_backend_find_wayland(backend, &nested);
	return nested;
}

// The scene sends each surface its output's fractional scale itself
// (types/scene/surface.c in 0.20), so the manager needs nothing else.
static bool sysc_fractional_scale_create(struct wl_display *display) {
	return wlr_fractional_scale_manager_v1_create(display, 1) != NULL;
}

// dmabuf feedback comes from the scene: it tells clients which buffers can be
// scanned out on the output they are on. A renderer with no dmabuf texture
// formats (pixman) serves wl_shm clients only, so the global is left out, as
// wlr_renderer_init_wl_display does.
static bool sysc_dmabuf_create(struct wl_display *display,
		struct wlr_renderer *renderer, struct wlr_scene *scene) {
	if (wlr_renderer_get_texture_formats(renderer, WLR_BUFFER_CAP_DMABUF) == NULL) {
		return true;
	}
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
	"image"
	"os"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// Compositor holds the wlroots object graph for one running server. Every
// method must be called on the thread that ran LockOSThread.
type Compositor struct {
	display              *C.struct_wl_display
	backend              *C.struct_wlr_backend
	session              *C.struct_wlr_session
	renderer             *C.struct_wlr_renderer
	alloc                *C.struct_wlr_allocator
	comp                 *C.struct_wlr_compositor
	layout               *C.struct_wlr_output_layout
	scene                *C.struct_wlr_scene
	xdgDecorationManager unsafe.Pointer
	kdeDecorationManager unsafe.Pointer
	windowShadowTree     *C.struct_wlr_scene_tree
	windowAnimationTree  *C.struct_wlr_scene_tree
	windowFocusTree      *C.struct_wlr_scene_tree
	overviewTree         *C.struct_wlr_scene_tree
	insertHintTree       *C.struct_wlr_scene_tree
	insertHintNode       *C.struct_wlr_scene_rect
	seat                 *C.struct_wlr_seat
	pointerGestures      *C.struct_wlr_pointer_gestures_v1
	xdgShell             *C.struct_wlr_xdg_shell
	xdgActivation        *C.struct_wlr_xdg_activation_v1
	layerShell           *C.struct_wlr_layer_shell_v1
	socketName           string
	listeners            map[*Listener]struct{}

	loop                    *C.struct_wl_event_loop
	cursor                  *C.struct_wlr_cursor
	xcursorManager          *C.struct_wlr_xcursor_manager
	sceneOutputs            map[unsafe.Pointer]*C.struct_wlr_scene_output
	mapped                  map[unsafe.Pointer]bool
	sceneTrees              map[unsafe.Pointer]*C.struct_wlr_scene_tree
	layerTrees              [layerCount]*C.struct_wlr_scene_tree
	overviewConfig          config.Overview
	animationConfig         config.Animations
	blurConfig              config.Blur
	screenshotPattern       *string
	screenshotTaken         func(path *string)
	shotUI                  *screenshotUI
	hotkeyOverlay           hotkeyOverlay
	exitDialog              exitDialog
	lastShotSelection       image.Rectangle
	animations              map[animationKey]*runningAnimation
	animationTimer          *time.Timer
	activeWorkspaces        map[string]activeWorkspace
	closingNodes            map[uint64]unsafe.Pointer
	nextAnimationID         uint64
	preferNoCSD             bool
	overviewBackdrops       []overviewBackdrop
	overviewProjections     []overviewProjection
	hotCorners              config.HotCorners
	hotCornerTracker        wm.HotCornerTracker
	dndEdgeConfig           config.DNDEdgeViewScroll
	dndScroll               *dndEdgeScrollState
	tiledMove               *tiledWindowMoveState
	layerSurfaces           map[unsafe.Pointer]*layerSurface // by wlr_surface
	backgroundEffects       map[unsafe.Pointer]*backgroundEffect
	backgroundEffectHandle  uintptr
	layerOrder              []*layerSurface         // creation order
	usable                  map[unsafe.Pointer]rect // per wlr_output, after exclusive zones
	requested               rect                    // last host-requested output size, for tests
	drawn                   int                     // frames that ran the scene commit, for tests
	popups                  map[*C.struct_wlr_xdg_popup]struct{}
	views                   map[unsafe.Pointer]*toplevelView
	pointerDevices          map[unsafe.Pointer]bool
	pointerGestureListeners map[unsafe.Pointer][]*Listener
	pointerSwipeStates      map[unsafe.Pointer]*pointerSwipeState
	switchToggleListeners   map[unsafe.Pointer]*Listener
	switchToggle            func(SwitchToggle)
	keyboardDevices         map[unsafe.Pointer]bool
	suppressedKeys          map[uint32]bool
	keyboardGroup           *C.struct_wlr_keyboard_group
	keyboard                *C.struct_wlr_keyboard // the group's, which the user seat hears
	keyboardConfig          Keyboard
	keyboardRepeatSource    *C.struct_wl_event_source
	keyboardRepeatToken     uintptr
	keyboardRepeatBinding   bind.Binding
	keyboardRepeatInterval  int
	inputDestroyListeners   map[unsafe.Pointer]*Listener
	modifiers               uint32
	focusedView             *toplevelView
	onDemandLayerFocus      *layerSurface
	pointerGrab             *pointerGrab
	grabView                *toplevelView
	grabButton              uint32
	pressedPointerButtons   map[uint32]bool
	pointerSerialButtons    map[uint32]uint32
	consumedPointerButtons  map[uint32]bool
	capabilities            int
	// drawnImage is the xcursor name the compositor last drew, or "" while
	// a client's surface is the image. wlroots keeps its own copy private.
	drawnImage                     string
	cursorSurface                  *C.struct_wlr_surface
	cursorHotspotX, cursorHotspotY int
	cursorSurfaceDestroy           *Listener
	cursorClientSet                bool
	// cursorShape is the theme image the pointer's client last asked for
	// through cursor-shape-v1, restored after a compositor grab; "" when it
	// set a surface instead.
	cursorShape string
	// focusOriginX/Y is where the user seat's focused surface starts in
	// layout coordinates, as of the last motion outside an implicit grab.
	focusOriginX, focusOriginY C.double
	focusedOutputName          string
	activeOutputName           string
	stateChanged               func()
	lastWindowState            []WindowInfo
	recentWindows              config.RecentWindows
	mruBindings                []bind.Binding
	switcher                   switcher
	focusTimer                 *time.Timer
	pendingFocus               *toplevelView
	pendingFocusStamp          time.Duration
	pendingFocusGeneration     uint64

	// injected is the table of seats fed by the compositor rather than the
	// user's hardware, after the user seat (c.seat) in creation order.
	injected []*injectedSeat

	// Injected input is gated while the session is locked or every output
	// is off (sysc-1260). injectedGated is the gate's last state, so held
	// input is released once, as it closes.
	sessionLocked bool
	injectedGated bool
	// powerOffActive is set between PowerOffMonitors and PowerOnMonitors;
	// actionPoweredOff is the outputs PowerOffMonitors turned off, the only
	// ones PowerOnMonitors turns back on.
	powerOffActive   bool
	actionPoweredOff map[unsafe.Pointer]struct{}

	// The session lock (lock.go). lock is the live locker, or nil; the
	// session can stay sealed (sessionLocked) with no locker after a crash.
	lock          *sessionLock
	lockTree      *C.struct_wlr_scene_tree
	lockBackdrops map[unsafe.Pointer]*C.struct_wlr_scene_rect
	// lockPendingOutputs are the enabled outputs that have not drawn a frame
	// since the lock was accepted; `locked` waits for them. nil once sent.
	lockPendingOutputs map[unsafe.Pointer]struct{}

	// Idle (idle.go). idleInhibited is what the notifier was last told.
	idleNotifier      *C.struct_wlr_idle_notifier_v1
	idleInhibitors    map[*C.struct_wlr_idle_inhibitor_v1]*Listener
	dbusIdleInhibited bool
	idleInhibited     bool

	shortcutsInhibitors map[*C.struct_wlr_keyboard_shortcuts_inhibitor_v1]*shortcutsInhibitor

	// Pointer constraints (pointer_constraints.go). activeConstraint is on
	// the surface with the user seat's pointer focus, or nil.
	relativePointer    *C.struct_wlr_relative_pointer_manager_v1
	pointerConstraints *C.struct_wlr_pointer_constraints_v1
	activeConstraint   *C.struct_wlr_pointer_constraint_v1

	// Text input relayed to the input method (text_input.go).
	textInputs      map[*C.struct_wlr_text_input_v3]*textInput
	activeTextInput *C.struct_wlr_text_input_v3
	inputMethod     *C.struct_wlr_input_method_v2
	inputPopups     map[*C.struct_wlr_input_popup_surface_v2]*inputPopup

	// viewMapped, when set, runs on the compositor thread once a toplevel is
	// mapped and hit-testable. It is the hook tests use to act once a client
	// is on screen.
	viewMapped func(*toplevelView)

	// post carries work from other goroutines onto the compositor thread.
	post postQueue

	model         *wm.Model
	bindings      []bind.Binding
	windowRules   []config.WindowRule
	layerRules    []config.LayerRule
	modKey        bind.Modifier
	layoutOptions LayoutOptions
	cursorTheme   string
	cursorSize    int
	spawn         func([]string) error
	spawnSh       func(string) error

	// nextViewID and touchCount give windows stable ids and a
	// most-recently-focused order for the control socket.
	nextViewID uint64
	touchCount uint64
}

// Options are the settings a compositor starts with.
type Options struct {
	Keyboard     Keyboard
	ModKey       string
	ModKeyNested string
	Layout       *LayoutOptions
	CursorTheme  string
	CursorSize   int
	Binds        []bind.Binding
	Workspaces   []wm.WorkspaceConfig
	WindowRules  []config.WindowRule
	LayerRules   []config.LayerRule
	Overview     config.Overview
	Gestures     config.Gestures
	Animations   config.Animations
	Blur         config.Blur
	PreferNoCSD  bool
	// ScreenshotPath is the screenshot-path pattern, nil for clipboard only.
	ScreenshotPath *string
	// OverlayBinds and HotkeyOverlay feed the hotkey overlay;
	// ShowHotkeyOverlayAtStartup is Niri's default unless skip-at-startup.
	OverlayBinds               []config.OverlayBind
	HotkeyOverlay              config.HotkeyOverlay
	ShowHotkeyOverlayAtStartup bool
	// RecentWindows is the recent-windows section; nil is Niri's default.
	RecentWindows *config.RecentWindows
}

type LayoutOptions struct {
	Gaps                  int
	ColumnWidthProportion float64
	FixedColumnWidth      int
	InsertHint            config.InsertHint
	FocusRing             config.FocusRing
	Shadow                config.Shadow
}

// ErrNestedTestNeedsConsent is what NewWith returns inside a test binary unless
// SYSC_NESTED_TESTS=1. A test compositor nests in the live session and maps a
// window on the owner's desktop, so building one takes the owner's say-so.
var ErrNestedTestNeedsConsent = errors.New("nested compositor tests need SYSC_NESTED_TESTS=1: they open a window on the live desktop")

// New is NewWith niri's defaults.
func New() (*Compositor, error) { return NewWith(Options{}) }

// NewWith creates the display, its nested socket, and the globals a client
// needs to map an xdg toplevel. It does not start the event loop. The backend
// starts here and announces its devices at once, so everything they need is
// set before that.
func NewWith(opts Options) (*Compositor, error) {
	if testing.Testing() && os.Getenv("SYSC_NESTED_TESTS") != "1" {
		return nil, ErrNestedTestNeedsConsent
	}
	opts = normalizeOptions(opts)
	c := &Compositor{}
	// Destroy takes a half-built compositor apart, so a failed start leaves no
	// socket, display or backend behind.
	built := false
	defer func() {
		if !built {
			c.Destroy()
		}
	}()
	c.overviewConfig = opts.Overview
	c.animationConfig = opts.Animations
	c.blurConfig = opts.Blur
	c.screenshotPattern = opts.ScreenshotPath
	c.hotkeyOverlay.open = opts.ShowHotkeyOverlayAtStartup
	c.configureHotkeyOverlay(opts.OverlayBinds, opts.HotkeyOverlay)
	c.preferNoCSD = opts.PreferNoCSD
	c.hotCorners = opts.Gestures.HotCorners
	c.dndEdgeConfig = opts.Gestures.DNDEdgeViewScroll
	c.model = wm.New(opts.Workspaces...)
	c.windowRules = slices.Clone(opts.WindowRules)
	c.layerRules = slices.Clone(opts.LayerRules)
	c.layoutOptions = *opts.Layout
	c.cursorTheme, c.cursorSize = opts.CursorTheme, opts.CursorSize
	c.keyboardConfig = opts.Keyboard

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
	c.mapped = map[unsafe.Pointer]bool{}
	c.sceneTrees = map[unsafe.Pointer]*C.struct_wlr_scene_tree{}
	c.animations = map[animationKey]*runningAnimation{}
	c.activeWorkspaces = map[string]activeWorkspace{}
	c.closingNodes = map[uint64]unsafe.Pointer{}
	c.layerSurfaces = map[unsafe.Pointer]*layerSurface{}
	c.backgroundEffects = map[unsafe.Pointer]*backgroundEffect{}
	c.usable = map[unsafe.Pointer]rect{}
	c.popups = map[*C.struct_wlr_xdg_popup]struct{}{}
	c.views = map[unsafe.Pointer]*toplevelView{}
	c.pointerDevices = map[unsafe.Pointer]bool{}
	c.pointerGestureListeners = map[unsafe.Pointer][]*Listener{}
	c.pointerSwipeStates = map[unsafe.Pointer]*pointerSwipeState{}
	c.switchToggleListeners = map[unsafe.Pointer]*Listener{}
	c.keyboardDevices = map[unsafe.Pointer]bool{}
	c.suppressedKeys = map[uint32]bool{}
	c.inputDestroyListeners = map[unsafe.Pointer]*Listener{}
	c.pressedPointerButtons = map[uint32]bool{}
	c.pointerSerialButtons = map[uint32]uint32{}
	c.consumedPointerButtons = map[uint32]bool{}

	c.backend = C.wlr_backend_autocreate(c.loop, &c.session)
	if c.backend == nil {
		return nil, errors.New("wlr_backend_autocreate failed")
	}
	var err error
	c.modKey, err = bind.ResolveModKey(opts.ModKey, opts.ModKeyNested, bool(C.sysc_backend_is_nested(c.backend)))
	if err != nil {
		return nil, err
	}
	c.bindings = append([]bind.Binding(nil), opts.Binds...)
	for i := range c.bindings {
		c.bindings[i].Chord = c.bindings[i].Chord.ResolveMod(c.modKey)
	}
	c.configureSwitcher(*opts.RecentWindows)

	c.renderer = (*C.struct_wlr_renderer)(createSceneFXRenderer(unsafe.Pointer(c.backend)))
	if c.renderer == nil {
		return nil, errors.New("fx_renderer_create failed")
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
	if c.pointerGestures = C.wlr_pointer_gestures_v1_create(c.display); c.pointerGestures == nil {
		return nil, errors.New("wlr_pointer_gestures_v1_create failed")
	}
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
	if err := c.createDecorationManager(opts.PreferNoCSD); err != nil {
		return nil, err
	}
	if c.xdgActivation = C.wlr_xdg_activation_v1_create(c.display); c.xdgActivation == nil {
		return nil, errors.New("wlr_xdg_activation_v1_create failed")
	}
	if c.scene = C.wlr_scene_create(); c.scene == nil {
		return nil, errors.New("wlr_scene_create failed")
	}
	c.applyBlurConfig()
	if !c.createLayerTrees() {
		return nil, errors.New("wlr_scene_tree_create failed")
	}
	if !c.createAppearanceTrees() {
		return nil, errors.New("window appearance scene tree create failed")
	}
	if !c.createOverviewTree() {
		return nil, errors.New("overview scene tree create failed")
	}
	if !c.createSessionLock() {
		return nil, errors.New("wlr_session_lock_manager_v1_create failed")
	}
	if !c.createIdle() {
		return nil, errors.New("idle notify or idle inhibit manager create failed")
	}
	if !c.createCursorShape() {
		return nil, errors.New("wlr_cursor_shape_manager_v1_create failed")
	}
	if !c.createShortcutsInhibit() {
		return nil, errors.New("wlr_keyboard_shortcuts_inhibit_v1_create failed")
	}
	if !c.createPointerConstraints() {
		return nil, errors.New("relative pointer or pointer constraints manager create failed")
	}
	if !c.createTextInput() {
		return nil, errors.New("text input or input method manager create failed")
	}
	if !c.createCaptureGlobals() {
		return nil, errors.New("capture or clipboard global create failed")
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
		return nil, errors.New("wlr_linux_dmabuf_v1_create_with_renderer failed")
	}
	if err := c.createBackgroundEffectProtocol(); err != nil {
		return nil, err
	}
	if c.seat = C.wlr_seat_create(c.display, cCString("seat0")); c.seat == nil {
		return nil, errors.New("wlr_seat_create failed")
	}
	c.watchSelectionRequests()
	if err := c.startKeyboardGroup(opts.Keyboard); err != nil {
		return nil, err
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
		AddListener(unsafe.Pointer(&c.xdgActivation.events.request_activate), c.handleActivationRequest),
		AddListener(unsafe.Pointer(&c.cursor.events.motion), c.handleCursorMotion),
		AddListener(unsafe.Pointer(&c.cursor.events.motion_absolute), c.handleCursorMotionAbsolute),
		AddListener(unsafe.Pointer(&c.cursor.events.button), c.handleCursorButton),
		AddListener(unsafe.Pointer(&c.cursor.events.axis), c.handleCursorAxis),
		AddListener(unsafe.Pointer(&c.cursor.events.frame), c.handleCursorFrame),
		AddListener(unsafe.Pointer(&c.seat.events.request_set_cursor), c.handleRequestSetCursor),
		AddListener(unsafe.Pointer(&c.seat.events.start_drag), c.handleStartDrag),
	)

	if err := c.startPost(); err != nil {
		return nil, err
	}
	if !bool(C.wlr_backend_start(c.backend)) {
		return nil, errors.New("wlr_backend_start failed")
	}
	built = true
	return c, nil
}

func normalizeOptions(opts Options) Options {
	defaults := config.Default()
	if opts.Overview == (config.Overview{}) {
		opts.Overview = defaults.Overview
	}
	if opts.Animations == (config.Animations{}) {
		opts.Animations = defaults.Animations
	}
	if opts.Blur == (config.Blur{}) {
		opts.Blur = defaults.Blur
	}
	if opts.Gestures.DNDEdgeViewScroll == (config.DNDEdgeViewScroll{}) {
		opts.Gestures.DNDEdgeViewScroll = defaults.Gestures.DNDEdgeViewScroll
	}
	if opts.RecentWindows == nil {
		opts.RecentWindows = &defaults.RecentWindows
	}
	if opts.Layout == nil {
		opts.Layout = &LayoutOptions{
			Gaps: defaults.Layout.Gaps, ColumnWidthProportion: 0.5,
			InsertHint: defaults.Layout.InsertHint, FocusRing: defaults.Layout.FocusRing,
			Shadow: defaults.Layout.Shadow,
		}
	}
	if opts.CursorTheme == "" {
		opts.CursorTheme = "default"
	}
	if opts.CursorSize <= 0 {
		opts.CursorSize = 24
	}
	return opts
}

// Reconfigure applies a parsed candidate on the compositor thread.
func (c *Compositor) Reconfigure(opts Options) error {
	opts = normalizeOptions(opts)
	modKey, err := bind.ResolveModKey(opts.ModKey, opts.ModKeyNested,
		bool(C.sysc_backend_is_nested(c.backend)))
	if err != nil {
		return err
	}
	decorationChanged := opts.PreferNoCSD != c.preferNoCSD
	if decorationChanged {
		if err := c.setDecorationPreference(opts.PreferNoCSD); err != nil {
			return err
		}
	}
	if opts.Keyboard != c.keyboardConfig {
		if err := c.reconfigureKeyboard(opts.Keyboard); err != nil {
			if decorationChanged {
				_ = c.setDecorationPreference(c.preferNoCSD)
			}
			return err
		}
		c.keyboardConfig = opts.Keyboard
	}

	c.stopKeyboardRepeat()
	c.modKey = modKey
	c.bindings = slices.Clone(opts.Binds)
	for i := range c.bindings {
		c.bindings[i].Chord = c.bindings[i].Chord.ResolveMod(c.modKey)
	}
	c.layoutOptions = *opts.Layout
	c.overviewConfig = opts.Overview
	c.animationConfig = opts.Animations
	c.screenshotPattern = opts.ScreenshotPath
	c.configureSwitcher(*opts.RecentWindows)
	c.configureHotkeyOverlay(opts.OverlayBinds, opts.HotkeyOverlay)
	if opts.Blur != c.blurConfig {
		c.blurConfig = opts.Blur
		c.applyBlurConfig()
	}
	c.hotCorners = opts.Gestures.HotCorners
	c.dndEdgeConfig = opts.Gestures.DNDEdgeViewScroll
	c.windowRules = slices.Clone(opts.WindowRules)
	c.layerRules = slices.Clone(opts.LayerRules)
	c.preferNoCSD = opts.PreferNoCSD
	c.reconfigureCursor(opts.CursorTheme, opts.CursorSize)
	c.hotCornerTracker = wm.HotCornerTracker{}
	if c.dndScroll != nil {
		c.stopDndEdgeScroll(c.dndScroll)
	}
	workspaces := make([]wm.WorkspaceConfig, len(opts.Workspaces))
	for i, workspace := range opts.Workspaces {
		workspaces[i] = wm.WorkspaceConfig{Name: workspace.Name, OpenOnOutput: workspace.OpenOnOutput}
	}
	c.model.SetWorkspaceConfig(workspaces)
	c.reapplyWindowRules()
	c.reapplyLayerRules()
	for output := range c.sceneOutputs {
		c.syncWorkspaceOutput(output)
	}
	c.applyWorkspaceModel()
	return nil
}

// SocketName is the nested Wayland socket this compositor serves. A smoke script
// spawns clients against it; it is not the compositor's own WAYLAND_DISPLAY.
func (c *Compositor) SocketName() string { return c.socketName }

// SetStateChanged registers a compositor-thread callback for output, layer,
// and focused-output changes. The callback may only copy state into Go-owned
// values; it must not block or perform socket I/O.
func (c *Compositor) SetStateChanged(fn func()) {
	c.stateChanged = fn
	c.refreshState()
}

// SetSpawnHandlers runs KDL spawn actions through the server's child process
// environment. The handlers run on the compositor thread.
func (c *Compositor) SetSpawnHandlers(spawn func([]string) error, spawnSh func(string) error) {
	c.spawn, c.spawnSh = spawn, spawnSh
}

// SetSwitchToggleHandler registers the compositor-thread handler for lid and
// tablet-mode input switches.
func (c *Compositor) SetSwitchToggleHandler(fn func(SwitchToggle)) {
	c.switchToggle = fn
}

// WorkspaceSnapshot copies model state to plain Go values on the compositor
// thread, for the server's immutable IPC snapshots.
func (c *Compositor) WorkspaceSnapshot() wm.Snapshot {
	if c.model == nil {
		return wm.Snapshot{}
	}
	return c.model.Snapshot()
}

func (c *Compositor) notifyStateChanged() {
	if c.stateChanged != nil {
		c.stateChanged()
	}
}

func (c *Compositor) refreshState() {
	c.focusedOutputName = c.FocusedOutputName()
	c.notifyStateChanged()
}

func (c *Compositor) notifyFocusedOutputChanged() {
	name := c.FocusedOutputName()
	if name != c.focusedOutputName {
		c.focusedOutputName = name
		c.notifyStateChanged()
	}
}

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

// Terminate asks Run to return. It is safe from any goroutine and at any
// time: the terminate is posted to the compositor thread, and once Destroy
// has run there is no loop left to stop, so the call does nothing.
func (c *Compositor) Terminate() {
	_ = c.Post(func() { C.wl_display_terminate(c.display) })
}

// Destroy releases the compositor. Listeners added through AddListener are
// removed first so no token outlives its wl_signal.
func (c *Compositor) Destroy() {
	c.stopKeyboardRepeat()
	if c.focusTimer != nil {
		c.focusTimer.Stop()
		c.focusTimer = nil
	}
	if c.dndScroll != nil && c.dndScroll.timer != nil {
		c.dndScroll.timer.Stop()
		c.dndScroll.timer = nil
	}
	if c.animationTimer != nil {
		c.animationTimer.Stop()
		c.animationTimer = nil
	}
	for _, view := range c.views {
		if view.closeSnapshotTimer != nil {
			view.closeSnapshotTimer.Stop()
			view.closeSnapshotTimer = nil
		}
		if view.closeSnapshot != nil {
			sceneFXNodeDestroy(view.closeSnapshot)
			view.closeSnapshot = nil
		}
		if view.resizeSnapshot != nil {
			sceneFXNodeDestroy(view.resizeSnapshot)
			view.resizeSnapshot = nil
		}
	}
	for id, node := range c.closingNodes {
		sceneFXNodeDestroy(node)
		delete(c.closingNodes, id)
	}
	c.destroyBackgroundEffects()
	c.animations = nil
	c.stopPost()
	// Before any seat goes: wlroots frees a seat's popup grab twice when the
	// seat is destroyed with the grab open (dismissGrabbedPopups).
	c.dismissGrabbedPopups(nil)
	for l := range c.listeners {
		l.Destroy()
	}
	c.listeners = nil
	c.sceneOutputs = nil
	c.mapped = nil
	c.sceneTrees = nil
	c.layerSurfaces = nil
	c.layerOrder = nil
	c.usable = nil
	c.popups = nil
	c.views = nil
	c.pointerDevices = nil
	c.keyboardDevices = nil
	c.suppressedKeys = nil
	c.inputDestroyListeners = nil
	c.destroyInjectedSeats()
	c.destroyKeyboardGroup()
	if c.cursor != nil {
		C.wlr_cursor_destroy(c.cursor)
		c.cursor = nil
	}
	if c.xcursorManager != nil {
		C.wlr_xcursor_manager_destroy(c.xcursorManager)
		c.xcursorManager = nil
	}
	if c.display != nil {
		C.wl_display_destroy_clients(c.display)
	}
	c.destroyBackend()
	if c.session != nil {
		C.wlr_session_destroy(c.session)
		c.session = nil
	}
	if c.display != nil {
		C.wl_display_destroy(c.display)
		c.display = nil
	}
	c.destroyBackgroundEffectProtocol()
}

func (c *Compositor) destroyBackend() {
	if c.backend != nil {
		C.wlr_backend_destroy(c.backend)
		c.backend = nil
	}
}

func cCString(s string) *C.char { return C.CString(s) }

// keep holds listeners until they are dropped or the compositor is destroyed.
func (c *Compositor) keep(ls ...*Listener) {
	for _, l := range ls {
		if l == nil {
			continue
		}
		c.listeners[l] = struct{}{}
	}
}

// drop destroys a listener whose object has gone and forgets it, so windows
// and devices that come and go leave nothing behind.
func (c *Compositor) drop(l *Listener) {
	if l == nil {
		return
	}
	l.Destroy()
	delete(c.listeners, l)
}

// liveListeners is how many listeners the compositor holds, for tests.
func (c *Compositor) liveListeners() int { return len(c.listeners) }
