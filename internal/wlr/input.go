package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/backend.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_pointer_gestures_v1.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_switch.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/types/wlr_xcursor_manager.h>
#include <wlr/util/edges.h>
#include <linux/input-event-codes.h>

static struct wlr_xcursor_manager *sysc_xcursor_manager(const char *theme, uint32_t size) {
	return wlr_xcursor_manager_create(theme, size);
}

// The field is named `type`, which is a Go keyword, so cgo cannot expose
// struct_wlr_input_device.type directly. One C accessor is the whole fix.
static int sysc_device_type(struct wlr_input_device *device) {
	return (int)device->type;
}

// 0.20 takes the manager and a name, not a resolved xcursor, and returns at
// once when that name is already drawn (types/wlr_cursor.c:657).
static void sysc_default_image(struct wlr_cursor *cursor,
		struct wlr_xcursor_manager *manager) {
	wlr_cursor_set_xcursor(cursor, manager, "default");
}

static void sysc_named_image(struct wlr_cursor *cursor,
		struct wlr_xcursor_manager *manager, const char *name) {
	wlr_cursor_set_xcursor(cursor, manager, name);
}

static void sysc_restore_client_image(struct wlr_cursor *cursor,
		struct wlr_surface *surface, int hotspot_x, int hotspot_y) {
	wlr_cursor_set_surface(cursor, surface, hotspot_x, hotspot_y);
}

static struct wlr_surface *sysc_scene_surface_at(struct wlr_scene *scene,
		double x, double y, double *sx, double *sy) {
	struct wlr_scene_node *node = wlr_scene_node_at(&scene->tree.node, x, y, sx, sy);
	if (node == NULL || node->type != WLR_SCENE_NODE_BUFFER) {
		return NULL;
	}
	struct wlr_scene_surface *scene_surface =
		wlr_scene_surface_try_from_buffer(wlr_scene_buffer_from_node(node));
	return scene_surface == NULL ? NULL : scene_surface->surface;
}

static void sysc_scene_tree_position(struct wlr_scene_tree *tree, int *x, int *y) {
	wlr_scene_node_coords(&tree->node, x, y);
}

static void sysc_scene_tree_set_position(struct wlr_scene_tree *tree, int x, int y) {
	wlr_scene_node_set_position(&tree->node, x, y);
}

static void sysc_scene_tree_set_enabled(struct wlr_scene_tree *tree, bool enabled) {
	wlr_scene_node_set_enabled(&tree->node, enabled);
}

// Only the user seat's focused client may change the drawn image (rule
// D2.4.2); injected seats have no listener on this signal at all.
static bool sysc_set_cursor_from_seat(struct wlr_cursor *cursor, struct wlr_seat *seat,
		struct wlr_seat_pointer_request_set_cursor_event *event) {
	if (event->seat_client != seat->pointer_state.focused_client) {
		return false;
	}
	wlr_cursor_set_surface(cursor, event->surface, event->hotspot_x, event->hotspot_y);
	return true;
}

static bool sysc_seat_has_set_cursor_listener(struct wlr_seat *seat) {
	return !wl_list_empty(&seat->events.request_set_cursor.listener_list);
}

static void sysc_seat_keyboard_focus(struct wlr_seat *seat,
		struct wlr_surface *surface, struct wlr_keyboard *keyboard) {
	wlr_seat_keyboard_notify_enter(seat, surface, keyboard->keycodes,
			keyboard->num_keycodes, &keyboard->modifiers);
}
*/
import "C"

import (
	"math"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

type pointerGrabMode uint8

const (
	grabNone pointerGrabMode = iota
	grabMove
	grabResize
)

const (
	edgeTop uint32 = 1 << iota
	edgeBottom
	edgeLeft
	edgeRight
)

type SwitchType uint8

const (
	SwitchLid SwitchType = iota
	SwitchTabletMode
)

type SwitchToggle struct {
	Type SwitchType
	On   bool
}

type pointerGrab struct {
	mode             pointerGrabMode
	cursorX, cursorY float64
	x, y             int
	width, height    int
	edges            uint32
}

type pointerSwipeState struct {
	fingers         uint32
	lastTimeMsec    uint32
	gestureTime     time.Duration
	forwarded       bool
	axis            wm.SwipeAxis
	selector        wm.SwipeAxisSelector
	output          string
	workspaceID     uint64
	actionApplied   bool
	scrollRemainder float64
}

func (s *pointerSwipeState) advanceGestureTime(timeMsec uint32) time.Duration {
	s.gestureTime += time.Duration(uint32(timeMsec-s.lastTimeMsec)) * time.Millisecond
	s.lastTimeMsec = timeMsec
	return s.gestureTime
}

type dndEdgeScrollState struct {
	drag        unsafe.Pointer
	listeners   []*Listener
	timer       *time.Timer
	tracker     wm.DNDEdgeScrollTracker
	output      string
	workspaceID uint64
	direction   int
	remainder   float64
	generation  uint64
}

type tiledWindowMoveState struct {
	view    *toplevelView
	button  uint32
	started bool
}

func (g pointerGrab) resizeOptions() layout.ResizeOptions {
	return layout.ResizeOptions{
		ResizeWidth:  g.edges&(edgeLeft|edgeRight) != 0,
		ResizeHeight: g.edges&(edgeTop|edgeBottom) != 0,
		AnchorRight:  g.edges&edgeLeft != 0,
		AnchorBottom: g.edges&edgeTop != 0,
	}
}

type toplevelView struct {
	foreign                 *foreignToplevel
	toplevel                *C.struct_wlr_xdg_toplevel
	tree                    *C.struct_wlr_scene_tree
	appearance              config.WindowRuleResult
	shadowNode              unsafe.Pointer
	focusRingNode           unsafe.Pointer
	cornerRadii             config.CornerRadius
	clipToGeometry          bool
	presentationOpacity     float64
	openAnimationStarted    bool
	openAnimationFinished   bool
	presentationVisible     bool
	workspaceID             uint64
	workspaceActive         bool
	workspaceIndex          int
	outputName              string
	resizeSnapshot          unsafe.Pointer
	resizeBaseWidth         int
	resizeBaseHeight        int
	resizeWidth             float64
	resizeHeight            float64
	resizeScaleX            float64
	resizeScaleY            float64
	resizeTargetWidth       int
	resizeTargetHeight      int
	closeSnapshot           unsafe.Pointer
	closeSnapshotTimer      *time.Timer
	closeSnapshotGeneration uint64
	listeners               []*Listener
	overviewSurfaces        map[unsafe.Pointer]*overviewSurfaceListeners
	mapped                  bool
	fullscreen              bool
	lastFullscreen          bool
	lastGeometry            rect
	rulesApplied            bool
	id                      uint64
	touched                 uint64
	focusTimestamp          time.Duration
	hasFocusTimestamp       bool
}

type pointerGeometry struct {
	x, y          int
	width, height int
}

func (g pointerGrab) geometry(cursorX, cursorY float64) pointerGeometry {
	geometry := pointerGeometry{x: g.x, y: g.y, width: g.width, height: g.height}
	dx := int(math.Round(cursorX - g.cursorX))
	dy := int(math.Round(cursorY - g.cursorY))
	if g.mode == grabMove {
		geometry.x += dx
		geometry.y += dy
		return geometry
	}
	if g.mode != grabResize {
		return geometry
	}
	if g.edges&edgeLeft != 0 {
		geometry.width -= dx
	} else if g.edges&edgeRight != 0 {
		geometry.width += dx
	}
	if g.edges&edgeTop != 0 {
		geometry.height -= dy
	} else if g.edges&edgeBottom != 0 {
		geometry.height += dy
	}
	if geometry.width < 1 {
		geometry.width = 1
	}
	if geometry.height < 1 {
		geometry.height = 1
	}
	if g.edges&edgeLeft != 0 {
		geometry.x = g.x + g.width - geometry.width
	}
	if g.edges&edgeTop != 0 {
		geometry.y = g.y + g.height - geometry.height
	}
	return geometry
}

// handleNewInput assigns a backend input device to the user seat.
//
// wlroots 0.20 does not wire devices to a seat for you: the compositor reads
// new_input and assigns by device type.
//
// The two device kinds are NOT symmetric in 0.20, and that is worth stating
// because it is the first hard evidence for the Tier 3 viability note. A
// keyboard has an explicit setter, wlr_seat_set_keyboard. A pointer has none:
// there is no wlr_seat_set_pointer, and wlr_scene exposes no routing either, so
// the cursor is attached to the layout and the compositor feeds the seat by
// calling the wlr_seat_pointer_notify_* functions itself. wlroots 0.20 has one
// interesting seat and leaves everything else to the compositor.
func (c *Compositor) handleNewInput(data unsafe.Pointer) {
	device := (*C.struct_wlr_input_device)(data)
	deviceKey := unsafe.Pointer(device)
	switch C.sysc_device_type(device) {
	case C.WLR_INPUT_DEVICE_KEYBOARD:
		kb := C.wlr_keyboard_from_input_device(device)
		if kb == nil || !c.addKeyboard(kb) {
			return
		}
		c.keyboardDevices[deviceKey] = true
	case C.WLR_INPUT_DEVICE_POINTER:
		if c.xcursorManager == nil {
			theme := C.CString(c.cursorTheme)
			c.xcursorManager = C.sysc_xcursor_manager(theme, C.uint32_t(c.cursorSize))
			C.free(unsafe.Pointer(theme))
			if c.xcursorManager != nil {
				// A cursor with no image is an invisible cursor, so the
				// theme is loaded once, here.
				if C.wlr_xcursor_manager_load(c.xcursorManager, 1.0) {
					c.showDefaultImage()
				}
			}
		}
		C.wlr_cursor_attach_input_device(c.cursor, device)
		c.attachPointerGestureListeners(deviceKey, C.wlr_pointer_from_input_device(device))
		c.pointerDevices[deviceKey] = true
	case C.WLR_INPUT_DEVICE_SWITCH:
		switchDevice := C.wlr_switch_from_input_device(device)
		listener := AddListener(unsafe.Pointer(&switchDevice.events.toggle), c.handleSwitchToggle)
		c.switchToggleListeners[deviceKey] = listener
		c.keep(listener)
	default:
		return
	}
	destroy := AddListener(unsafe.Pointer(&device.events.destroy), func(unsafe.Pointer) {
		c.handleInputDestroy(deviceKey)
	})
	c.inputDestroyListeners[deviceKey] = destroy
	c.keep(destroy)
	c.refreshCapabilities()
}

func (c *Compositor) handleInputDestroy(device unsafe.Pointer) {
	c.detachPointerGestureListeners(device)
	if listener := c.switchToggleListeners[device]; listener != nil {
		c.drop(listener)
		delete(c.switchToggleListeners, device)
	}
	if listener := c.inputDestroyListeners[device]; listener != nil {
		c.drop(listener)
		delete(c.inputDestroyListeners, device)
	}
	if c.pointerDevices[device] {
		pointer := C.wlr_pointer_from_input_device((*C.struct_wlr_input_device)(device))
		c.releasePointerButtons(pointer)
		if virtual := c.virtualPointers[pointer]; virtual != nil {
			c.drop(virtual.outputGone)
			delete(c.virtualPointers, pointer)
		}
		C.wlr_cursor_detach_input_device(c.cursor, (*C.struct_wlr_input_device)(device))
		delete(c.pointerDevices, device)
	}
	// The keyboard group lets a destroyed keyboard go by itself.
	delete(c.keyboardDevices, device)
	c.refreshCapabilities()
}

func (c *Compositor) handleSwitchToggle(data unsafe.Pointer) {
	event := (*C.struct_wlr_switch_toggle_event)(data)
	var switchType SwitchType
	switch event.switch_type {
	case C.WLR_SWITCH_TYPE_LID:
		switchType = SwitchLid
	case C.WLR_SWITCH_TYPE_TABLET_MODE:
		switchType = SwitchTabletMode
	default:
		return
	}
	if c.switchToggle != nil {
		c.switchToggle(SwitchToggle{Type: switchType, On: event.switch_state == C.WLR_SWITCH_STATE_ON})
	}
}

func (c *Compositor) attachPointerGestureListeners(device unsafe.Pointer, pointer *C.struct_wlr_pointer) {
	if pointer == nil {
		return
	}
	listeners := []*Listener{
		AddListener(unsafe.Pointer(&pointer.events.swipe_begin), func(data unsafe.Pointer) {
			c.handlePointerSwipeBegin(device, data)
		}),
		AddListener(unsafe.Pointer(&pointer.events.swipe_update), func(data unsafe.Pointer) {
			c.handlePointerSwipeUpdate(device, data)
		}),
		AddListener(unsafe.Pointer(&pointer.events.swipe_end), func(data unsafe.Pointer) {
			c.handlePointerSwipeEnd(device, data)
		}),
	}
	c.pointerGestureListeners[device] = listeners
	c.keep(listeners...)
}

func (c *Compositor) detachPointerGestureListeners(device unsafe.Pointer) {
	if state := c.pointerSwipeStates[device]; state != nil {
		if state.fingers == 4 && c.model != nil {
			c.model.EndOverviewGesture(state.gestureTime)
			c.syncOverviewScene()
			c.notifyStateChanged()
		} else if state.forwarded {
			C.wlr_pointer_gestures_v1_send_swipe_end(c.pointerGestures, c.seat,
				C.uint32_t(time.Now().UnixMilli()), true)
		}
	}
	delete(c.pointerSwipeStates, device)
	for _, listener := range c.pointerGestureListeners[device] {
		c.drop(listener)
	}
	delete(c.pointerGestureListeners, device)
}

func (c *Compositor) handlePointerSwipeBegin(device, data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_swipe_begin_event)(data)
	state := &pointerSwipeState{fingers: uint32(event.fingers), lastTimeMsec: uint32(event.time_msec)}
	c.pointerSwipeStates[device] = state
	switch state.fingers {
	case 3:
		state.output = c.outputNameAt(float64(c.cursor.x), float64(c.cursor.y))
		if state.output == "" && c.model != nil {
			state.output = c.model.Snapshot().FocusedOutput
		}
		state.workspaceID = c.workspaceUnderGesture(state.output)
	case 4:
		if c.model != nil {
			progress, _, _ := c.animationValue(animationKey{property: "overview"},
				c.model.OverviewProgress(), time.Now())
			c.cancelAnimation(animationKey{property: "overview"})
			c.model.BeginOverviewGestureAt(progress)
			c.syncOverviewScene()
			c.notifyStateChanged()
		}
	default:
		state.forwarded = true
		C.wlr_pointer_gestures_v1_send_swipe_begin(c.pointerGestures, c.seat,
			event.time_msec, event.fingers)
	}
}

func (c *Compositor) handlePointerSwipeUpdate(device, data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_swipe_update_event)(data)
	state := c.pointerSwipeStates[device]
	if state == nil {
		return
	}
	if state.forwarded {
		C.wlr_pointer_gestures_v1_send_swipe_update(c.pointerGestures, c.seat,
			event.time_msec, event.dx, event.dy)
		return
	}
	if c.model == nil {
		return
	}
	if state.fingers == 4 {
		timestamp := state.advanceGestureTime(uint32(event.time_msec))
		if c.model.UpdateOverviewGesture(-float64(event.dy), timestamp) {
			c.syncOverviewScene()
		}
		return
	}
	axis, delta, selected := state.selector.Add(float64(event.dx), float64(event.dy))
	if !selected {
		return
	}
	newAxis := state.axis == wm.SwipeAxisNone
	state.axis = axis
	if axis == wm.SwipeAxisHorizontal {
		state.scrollRemainder += delta
		pixels := int(math.Trunc(state.scrollRemainder))
		state.scrollRemainder -= float64(pixels)
		if pixels != 0 && state.workspaceID != 0 && c.model.ScrollWorkspaceView(state.workspaceID, pixels) {
			c.applyWorkspaceModel()
		}
		return
	}
	if newAxis && !state.actionApplied {
		direction := -1
		if delta < 0 {
			direction = 1
		}
		state.actionApplied = c.model.FocusWorkspaceByDelta(state.output, direction)
		if state.actionApplied {
			c.applyWorkspaceModel()
			c.activateWorkspaceFocus()
		}
	}
}

func (c *Compositor) handlePointerSwipeEnd(device, data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_swipe_end_event)(data)
	state := c.pointerSwipeStates[device]
	delete(c.pointerSwipeStates, device)
	if state == nil {
		return
	}
	if state.forwarded {
		C.wlr_pointer_gestures_v1_send_swipe_end(c.pointerGestures, c.seat,
			event.time_msec, event.cancelled)
		return
	}
	timestamp := state.advanceGestureTime(uint32(event.time_msec))
	if state.fingers == 4 && c.model != nil {
		from, to, velocity, ok := c.model.FinishOverviewGestureAnimation(timestamp)
		if ok {
			c.animateOverviewProgress(from, to, velocity)
			c.syncOverviewScene()
			c.notifyStateChanged()
		}
	}
}

func (c *Compositor) workspaceUnderGesture(output string) uint64 {
	if c.model == nil {
		return 0
	}
	if c.model.OverviewProgress() > 0 || c.model.OverviewOpen() {
		if workspaceID, _, ok := c.model.OverviewWorkspaceUnder(output,
			float64(c.cursor.x), float64(c.cursor.y), c.model.OverviewZoom(c.overviewConfig.Zoom)); ok {
			return workspaceID
		}
		return 0
	}
	for _, workspace := range c.model.Snapshot().Workspaces {
		if workspace.Output == output && workspace.IsActive {
			return workspace.ID
		}
	}
	return 0
}

func (c *Compositor) updateHotCorner() {
	if c.model == nil {
		return
	}
	output := c.outputNameAt(float64(c.cursor.x), float64(c.cursor.y))
	if output == "" {
		c.hotCornerTracker.Update("", layout.Bounds{}, float64(c.cursor.x), float64(c.cursor.y), wm.HotCorners{})
		return
	}
	bounds, ok := c.outputLayoutBounds(output)
	if !ok {
		return
	}
	corners := wm.HotCorners{
		TopLeft: c.hotCorners.TopLeft, TopRight: c.hotCorners.TopRight,
		BottomLeft: c.hotCorners.BottomLeft, BottomRight: c.hotCorners.BottomRight,
		Off: c.hotCorners.Off,
	}
	if c.hotCornerTracker.Update(output, bounds, float64(c.cursor.x), float64(c.cursor.y), corners) {
		c.ToggleOverview()
	}
}

func (c *Compositor) activeWorkspaceID(output string) uint64 {
	if c.model == nil {
		return 0
	}
	for _, workspace := range c.model.Snapshot().Workspaces {
		if workspace.Output == output && workspace.IsActive {
			return workspace.ID
		}
	}
	return 0
}

func (c *Compositor) dndEdgeScrollTarget(output string, x, y float64) (uint64, layout.Bounds, bool) {
	outputBounds, ok := c.outputLayoutBounds(output)
	if !ok || c.model == nil {
		return 0, layout.Bounds{}, false
	}
	if c.model.OverviewProgress() > 0 {
		return c.model.OverviewWorkspaceUnder(output, x, y, c.model.OverviewZoom(c.overviewConfig.Zoom))
	}
	workspaceID := c.activeWorkspaceID(output)
	return workspaceID, outputBounds, workspaceID != 0
}

func (c *Compositor) handleStartDrag(data unsafe.Pointer) {
	drag := (*C.struct_wlr_drag)(data)
	if c.dndScroll != nil {
		c.destroyDndScrollState(c.dndScroll)
	}
	state := &dndEdgeScrollState{drag: unsafe.Pointer(drag)}
	c.dndScroll = state
	state.listeners = []*Listener{
		AddListener(unsafe.Pointer(&drag.events.motion), func(unsafe.Pointer) {
			c.updateDndEdgeScroll(state)
		}),
		AddListener(unsafe.Pointer(&drag.events.drop), func(unsafe.Pointer) {
			c.stopDndEdgeScroll(state)
		}),
		AddListener(unsafe.Pointer(&drag.events.destroy), func(unsafe.Pointer) {
			c.destroyDndScrollState(state)
		}),
	}
	c.keep(state.listeners...)
	c.updateDndEdgeScroll(state)
}

func (c *Compositor) updateDndEdgeScroll(state *dndEdgeScrollState) {
	if state == nil || c.dndScroll != state {
		return
	}
	x, y := float64(c.cursor.x), float64(c.cursor.y)
	output := c.outputNameAt(x, y)
	workspaceID, bounds, ok := c.dndEdgeScrollTarget(output, x, y)
	direction, _ := wm.DNDEdgeScrollAt(bounds, x, y,
		c.dndEdgeConfig.TriggerWidth, c.dndEdgeConfig.MaxSpeed)
	if !ok || direction == 0 || workspaceID == 0 {
		c.stopDndEdgeScroll(state)
		return
	}
	if state.output == output && state.workspaceID == workspaceID && state.direction == direction {
		return
	}
	c.stopDndEdgeScroll(state)
	state.output, state.workspaceID, state.direction = output, workspaceID, direction
	state.tracker.Update(time.Now(), bounds, x, y, c.dndEdgeConfig.TriggerWidth,
		time.Duration(c.dndEdgeConfig.DelayMS)*time.Millisecond, c.dndEdgeConfig.MaxSpeed)
	c.scheduleDndEdgeTick(state, time.Duration(c.dndEdgeConfig.DelayMS)*time.Millisecond)
}

func (c *Compositor) scheduleDndEdgeTick(state *dndEdgeScrollState, delay time.Duration) {
	state.generation++
	generation := state.generation
	state.timer = time.AfterFunc(delay, func() {
		_ = c.Post(func() { c.dndEdgeScrollTick(state, generation) })
	})
}

func (c *Compositor) dndEdgeScrollTick(state *dndEdgeScrollState, generation uint64) {
	if state == nil || c.dndScroll != state || state.generation != generation || state.direction == 0 {
		return
	}
	state.timer = nil
	x, y := float64(c.cursor.x), float64(c.cursor.y)
	output := c.outputNameAt(x, y)
	workspaceID, bounds, ok := c.dndEdgeScrollTarget(output, x, y)
	direction, _ := wm.DNDEdgeScrollAt(bounds, x, y,
		c.dndEdgeConfig.TriggerWidth, c.dndEdgeConfig.MaxSpeed)
	if !ok || direction != state.direction || output != state.output || workspaceID != state.workspaceID {
		c.updateDndEdgeScroll(state)
		return
	}
	delta := state.tracker.Update(time.Now(), bounds, x, y, c.dndEdgeConfig.TriggerWidth,
		time.Duration(c.dndEdgeConfig.DelayMS)*time.Millisecond, c.dndEdgeConfig.MaxSpeed)
	state.remainder += delta
	pixels := int(math.Trunc(state.remainder))
	state.remainder -= float64(pixels)
	if pixels != 0 && c.model.ScrollWorkspaceView(state.workspaceID, pixels) {
		c.applyWorkspaceModel()
	}
	c.scheduleDndEdgeTick(state, 20*time.Millisecond)
}

func (c *Compositor) stopDndEdgeScroll(state *dndEdgeScrollState) {
	if state == nil {
		return
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.generation++
	state.tracker.Reset()
	state.output, state.workspaceID, state.direction, state.remainder = "", 0, 0, 0
}

func (c *Compositor) destroyDndScrollState(state *dndEdgeScrollState) {
	if state == nil {
		return
	}
	c.stopDndEdgeScroll(state)
	for _, listener := range state.listeners {
		c.drop(listener)
	}
	state.listeners = nil
	if c.dndScroll == state {
		c.dndScroll = nil
	}
}

func (c *Compositor) handleCursorMotion(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_motion_event)(data)
	if !c.pointerInputAllowed(event.pointer) {
		return
	}
	device := (*C.struct_wlr_input_device)(unsafe.Pointer(event.pointer))
	c.pointerMotion(device, event.time_msec, float64(event.delta_x), float64(event.delta_y),
		float64(event.unaccel_dx), float64(event.unaccel_dy))
}

func (c *Compositor) handleCursorMotionAbsolute(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_motion_absolute_event)(data)
	if !c.pointerInputAllowed(event.pointer) {
		return
	}
	device := (*C.struct_wlr_input_device)(unsafe.Pointer(event.pointer))
	dx, dy := c.absoluteDelta(device, event.x, event.y)
	c.pointerMotion(device, event.time_msec, dx, dy, dx, dy)
}

func (c *Compositor) processCursorMotion(time C.uint32_t) {
	if c.switcher.isOpen() {
		C.wlr_seat_pointer_notify_clear_focus(c.seat)
		c.showDefaultImage()
		return
	}
	c.updateHotCorner()
	if c.model != nil && (c.model.OverviewProgress() > 0 || c.model.OverviewOpen()) {
		C.wlr_seat_pointer_notify_clear_focus(c.seat)
		c.showDefaultImage()
		return
	}
	if c.pointerGrab != nil && c.grabView != nil && c.grabView.tree != nil {
		if c.tiledMove != nil && c.tiledMove.view == c.grabView && c.model != nil {
			_, started := c.model.UpdateWindowDrag(float64(c.cursor.x), float64(c.cursor.y),
				c.outputNameAt(float64(c.cursor.x), float64(c.cursor.y)))
			if started {
				c.tiledMove.started = true
				geometry := c.pointerGrab.geometry(float64(c.cursor.x), float64(c.cursor.y))
				C.sysc_scene_tree_set_position(c.grabView.tree, C.int(geometry.x), C.int(geometry.y))
			}
			c.syncInsertHint()
			return
		}
		geometry := c.pointerGrab.geometry(float64(c.cursor.x), float64(c.cursor.y))
		if c.pointerGrab.mode == grabResize {
			if c.model == nil || !c.model.ResizeWindow(c.grabView.id,
				geometry.x, geometry.y, geometry.width, geometry.height,
				c.pointerGrab.resizeOptions()) {
				C.sysc_scene_tree_set_position(c.grabView.tree,
					C.int(geometry.x), C.int(geometry.y))
				C.wlr_xdg_toplevel_set_size(c.grabView.toplevel,
					C.int(geometry.width), C.int(geometry.height))
			} else {
				c.applyWorkspaceModel()
			}
		} else {
			C.sysc_scene_tree_set_position(c.grabView.tree,
				C.int(geometry.x), C.int(geometry.y))
		}
		c.notifyWindowStateChanged()
		return
	}
	// Implicit grab: while any button is held, the pointer stays with the
	// surface it was on, or with nothing, and motion is reported against that
	// surface's origin. This is Smithay's click grab, which Niri uses.
	if len(c.pressedPointerButtons) > 0 {
		if c.seat.pointer_state.focused_surface != nil {
			C.wlr_seat_pointer_notify_motion(c.seat, time,
				c.cursor.x-c.focusOriginX, c.cursor.y-c.focusOriginY)
		}
		return
	}
	var sx, sy C.double
	surface := C.sysc_scene_surface_at(c.scene, c.cursor.x, c.cursor.y, &sx, &sy)
	if surface == nil {
		C.wlr_seat_pointer_notify_clear_focus(c.seat)
		c.showDefaultImage()
		return
	}
	if !bool(C.wlr_seat_pointer_surface_has_focus(c.seat, surface)) {
		C.wlr_seat_pointer_notify_enter(c.seat, surface, sx, sy)
	}
	c.focusOriginX, c.focusOriginY = c.cursor.x-sx, c.cursor.y-sy
	C.wlr_seat_pointer_notify_motion(c.seat, time, sx, sy)
}

// refocusPointer gives pointer focus to the surface under the pointer once
// no button or compositor grab holds it elsewhere.
func (c *Compositor) refocusPointer(time C.uint32_t) {
	if len(c.pressedPointerButtons) > 0 || c.pointerGrab != nil {
		return
	}
	c.processCursorMotion(time)
	C.wlr_seat_pointer_notify_frame(c.seat)
	c.updatePointerConstraint()
}

// showDefaultImage draws the theme's default image, as over empty space.
func (c *Compositor) showDefaultImage() {
	if c.xcursorManager == nil {
		return
	}
	C.sysc_default_image(c.cursor, c.xcursorManager)
	c.drawnImage = "default"
}

func (c *Compositor) reconfigureCursor(theme string, size int) {
	if theme == "" {
		theme = "default"
	}
	if size <= 0 {
		size = 24
	}
	if c.cursorTheme == theme && c.cursorSize == size {
		return
	}
	c.cursorTheme, c.cursorSize = theme, size
	if c.xcursorManager != nil {
		C.wlr_xcursor_manager_destroy(c.xcursorManager)
		c.xcursorManager = nil
	}
	if len(c.pointerDevices) == 0 {
		return
	}
	name := C.CString(theme)
	c.xcursorManager = C.sysc_xcursor_manager(name, C.uint32_t(size))
	C.free(unsafe.Pointer(name))
	if c.xcursorManager != nil && C.wlr_xcursor_manager_load(c.xcursorManager, 1.0) {
		c.restoreCursorImage()
	}
}

func (c *Compositor) showCursorImage(name string) {
	if c.xcursorManager == nil {
		return
	}
	image := C.CString(name)
	C.sysc_named_image(c.cursor, c.xcursorManager, image)
	C.free(unsafe.Pointer(image))
	c.drawnImage = name
}

func (c *Compositor) restoreCursorImage() {
	if c.cursorClientSet {
		C.sysc_restore_client_image(c.cursor, c.cursorSurface,
			C.int(c.cursorHotspotX), C.int(c.cursorHotspotY))
		c.drawnImage = ""
		return
	}
	if c.cursorShape != "" {
		c.showCursorImage(c.cursorShape)
		return
	}
	c.showDefaultImage()
}

func (c *Compositor) handleCursorButton(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_button_event)(data)
	if !c.pointerInputAllowed(event.pointer) {
		return
	}
	c.deliverPointerButton(event)
}

func (c *Compositor) deliverPointerButton(event *C.struct_wlr_pointer_button_event) {
	button := uint32(event.button)
	pressed := event.state == C.WL_POINTER_BUTTON_STATE_PRESSED
	owners := c.pointerButtons[event.pointer]
	if pressed {
		if owners[button] {
			return
		}
		if c.pointerButtons == nil {
			c.pointerButtons = make(map[*C.struct_wlr_pointer]map[uint32]bool)
		}
		if owners == nil {
			owners = make(map[uint32]bool)
			c.pointerButtons[event.pointer] = owners
		}
		owners[button] = true
	} else {
		if !owners[button] {
			return
		}
		delete(owners, button)
		if len(owners) == 0 {
			delete(c.pointerButtons, event.pointer)
		}
	}
	for pointer, held := range c.pointerButtons {
		if pointer != event.pointer && held[button] {
			return
		}
	}

	if pressed {
		c.pressedPointerButtons[button] = true
	} else {
		delete(c.pressedPointerButtons, button)
		for serial, pressedButton := range c.pointerSerialButtons {
			if pressedButton == button {
				delete(c.pointerSerialButtons, serial)
			}
		}
	}
	if pressed && c.hotkeyOverlay.open {
		defer c.hideHotkeyOverlay()
	}
	if pressed && c.exitDialog.open {
		c.hideExitDialog()
		c.consumedPointerButtons[button] = true
		C.wlr_seat_pointer_notify_frame(c.seat)
		return
	}
	if c.shotUIButton(button, pressed) {
		return
	}
	if c.switcherButton(button, pressed) {
		return
	}

	if !pressed {
		if c.consumedPointerButtons[button] {
			delete(c.consumedPointerButtons, button)
			if c.tiledMove != nil && c.tiledMove.button == button {
				c.finishTiledWindowMove(true)
			} else if c.grabButton == button {
				c.clearPointerGrab()
			}
			C.wlr_seat_pointer_notify_frame(c.seat)
			c.refocusPointer(event.time_msec)
			return
		}
		if c.pointerGrab != nil && (c.grabButton == 0 || c.grabButton == button) {
			c.clearPointerGrab()
		}
	}
	if pressed && c.model != nil && (c.model.OverviewProgress() > 0 || c.model.OverviewOpen()) {
		c.consumedPointerButtons[button] = true
		if button == uint32(C.BTN_LEFT) && !c.overviewSelectAt(float64(c.cursor.x), float64(c.cursor.y)) {
			c.CloseOverview()
		}
		C.wlr_seat_pointer_notify_frame(c.seat)
		return
	}

	if pressed && c.modifierActive(c.modKey) {
		if view := c.viewAtCursor(); view != nil {
			mode := grabNone
			var edges uint32
			switch button {
			case uint32(C.BTN_LEFT):
				mode = grabMove
			case uint32(C.BTN_RIGHT):
				mode = grabResize
				edges = c.resizeEdgesAt(view)
			}
			if mode != grabNone && (mode != grabResize || edges != 0) {
				if mode == grabMove && c.model != nil && !c.windowIsFloating(view.id) &&
					c.model.BeginWindowDrag(view.id, float64(c.cursor.x), float64(c.cursor.y)) {
					c.tiledMove = &tiledWindowMoveState{view: view, button: button}
				}
				if !c.startPointerGrab(view, mode, edges, button) {
					c.cancelTiledWindowMove()
					return
				}
				c.focusView(view)
				c.consumedPointerButtons[button] = true
				if mode == grabMove {
					c.showCursorImage("grabbing")
				} else {
					c.showCursorImage(resizeCursorName(edges))
				}
				return
			}
		}
	}

	if pressed {
		c.focusSurfaceAtCursor()
	}
	serial := C.wlr_seat_pointer_notify_button(c.seat,
		event.time_msec, event.button, event.state)
	if pressed && serial != 0 {
		c.pointerSerialButtons[uint32(serial)] = button
	}
	C.wlr_seat_pointer_notify_frame(c.seat)
	if !pressed {
		c.refocusPointer(event.time_msec)
	}
}

func (c *Compositor) handleCursorAxis(data unsafe.Pointer) {
	event := (*C.struct_wlr_pointer_axis_event)(data)
	if !c.pointerInputAllowed(event.pointer) {
		return
	}
	c.hideExitDialog()
	c.hideHotkeyOverlay()
	C.wlr_seat_pointer_notify_axis(c.seat, event.time_msec,
		event.orientation, event.delta, event.delta_discrete,
		event.source, event.relative_direction)
}

func (c *Compositor) handleCursorFrame(unsafe.Pointer) {
	C.wlr_seat_pointer_notify_frame(c.seat)
}

// surfaceAt is the client surface at layout coordinates, with the
// surface-local position, or nil over empty space.
func (c *Compositor) surfaceAt(x, y float64) (*C.struct_wlr_surface, C.double, C.double) {
	var sx, sy C.double
	surface := C.sysc_scene_surface_at(c.scene, C.double(x), C.double(y), &sx, &sy)
	return surface, sx, sy
}

func (c *Compositor) viewAtCursor() *toplevelView {
	surface, _, _ := c.surfaceAt(float64(c.cursor.x), float64(c.cursor.y))
	return c.viewForSurface(surface)
}

func (c *Compositor) windowIsFloating(id uint64) bool {
	if c.model == nil {
		return false
	}
	for _, window := range c.model.Snapshot().Windows {
		if window.ID == id {
			return window.Tile.Floating
		}
	}
	return false
}

func (c *Compositor) finishTiledWindowMove(commit bool) {
	move := c.tiledMove
	if move == nil {
		return
	}
	committed := false
	if commit && move.started && c.model != nil {
		committed = c.model.CommitWindowDrag()
	} else if c.model != nil {
		c.model.CancelWindowDrag()
	}
	c.tiledMove = nil
	c.syncInsertHint()
	c.clearPointerGrab()
	c.applyWorkspaceModel()
	if committed {
		c.activateWorkspaceFocus()
	}
}

func (c *Compositor) cancelTiledWindowMove() {
	if c.tiledMove == nil {
		return
	}
	c.finishTiledWindowMove(false)
}

func (c *Compositor) focusView(view *toplevelView) {
	if view == nil || view.toplevel == nil {
		return
	}
	if c.model != nil && c.model.FocusWindow(view.id) {
		c.applyWorkspaceModel()
	}
	c.setActiveOutputName(c.outputNameForView(view))
	changed := c.focusedView != view
	if changed {
		if previous := c.focusedView; previous != nil && previous.toplevel != nil {
			C.wlr_xdg_toplevel_set_activated(previous.toplevel, false)
		}
		c.focusedView = view
		c.touch(view)
	}
	c.onDemandLayerFocus = nil
	c.updateKeyboardFocus()
	if changed {
		// The layer surface may keep keyboard focus, but Niri's retained
		// layout focus still changes the workspace's active window.
		c.notifyWindowStateChanged()
	}
	c.syncFocusRings()
}

func (c *Compositor) focusSurfaceAtCursor() {
	c.setActiveOutputName(c.outputNameAt(float64(c.cursor.x), float64(c.cursor.y)))
	surface, _, _ := c.surfaceAt(float64(c.cursor.x), float64(c.cursor.y))
	if view := c.viewForSurface(surface); view != nil {
		c.focusView(view)
		return
	}
	c.focusLayerSurface(c.layerSurfaceForSurface(surface))
}

func (c *Compositor) focusLayerSurface(layer *layerSurface) {
	if layer != nil && bool(layer.surface.surface.mapped) &&
		snapshotKeyboardMode(int(layer.surface.current.keyboard_interactive)) == LayerKeyboardOnDemandMode {
		c.onDemandLayerFocus = layer
		c.setActiveOutputName(layer.outputName)
	} else {
		c.onDemandLayerFocus = nil
	}
	c.updateKeyboardFocus()
}

func (c *Compositor) updateKeyboardFocus() {
	previousFocusedView := c.keyboardFocusedView()
	if c.exitDialog.open {
		// The dialog holds the keyboard, as Niri's ExitConfirmDialog focus.
		C.wlr_seat_keyboard_notify_clear_focus(c.seat)
		c.keyboardFocusChanged(previousFocusedView)
		return
	}
	if c.sessionLocked {
		// Sealed: only a lock surface may hold user keyboard focus.
		if surface := c.lockKeyboardSurface(); surface == nil {
			C.wlr_seat_keyboard_notify_clear_focus(c.seat)
		} else if c.seat.keyboard_state.focused_surface != surface {
			c.enterKeyboardSurface(surface, c.keyboard)
		}
		c.keyboardFocusChanged(previousFocusedView)
		return
	}
	if c.switcher.isOpen() {
		// The window switcher holds the keyboard, as Niri's Mru focus.
		C.wlr_seat_keyboard_notify_clear_focus(c.seat)
		c.keyboardFocusChanged(previousFocusedView)
		return
	}
	if layer := c.onDemandLayerFocus; layer != nil &&
		(!bool(layer.surface.surface.mapped) || snapshotKeyboardMode(int(layer.surface.current.keyboard_interactive)) != LayerKeyboardOnDemandMode) {
		c.onDemandLayerFocus = nil
	}

	activeOutput := c.FocusedOutputName()
	focusedView := c.focusedView
	if focusedView != nil && focusedView.toplevel != nil {
		if activeOutput == "" || c.outputNameForView(focusedView) != activeOutput {
			focusedView = nil
		}
	} else {
		focusedView = nil
	}

	candidates := make([]layerFocusCandidate, len(c.layerOrder))
	for i, layer := range c.layerOrder {
		candidates[i] = layerFocusCandidate{
			index: i, layer: layer.layer, output: layer.outputName,
			keyboard: snapshotKeyboardMode(int(layer.surface.current.keyboard_interactive)),
			mapped:   bool(layer.surface.surface.mapped), onDemandFocused: c.onDemandLayerFocus == layer,
		}
	}
	focusedLayerIndex := chooseLayerKeyboardFocus(candidates, activeOutput, focusedView != nil)
	var focusedLayer *layerSurface
	if focusedLayerIndex >= 0 && focusedLayerIndex < len(c.layerOrder) {
		focusedLayer = c.layerOrder[focusedLayerIndex]
	}

	var surface *C.struct_wlr_surface
	if focusedLayer != nil {
		surface = focusedLayer.surface.surface
	} else if focusedView != nil {
		surface = focusedView.toplevel.base.surface
	}
	if c.focusedView != nil && c.focusedView.toplevel != nil {
		C.wlr_xdg_toplevel_set_activated(c.focusedView.toplevel, focusedView == c.focusedView && focusedLayer == nil)
	}
	if surface == nil {
		C.wlr_seat_keyboard_notify_clear_focus(c.seat)
		c.keyboardFocusChanged(previousFocusedView)
		return
	}
	surface = C.wlr_surface_get_root_surface(surface)
	if c.seat.keyboard_state.focused_surface != surface {
		c.enterKeyboardSurface(surface, c.currentKeyboard())
	}
	c.keyboardFocusChanged(previousFocusedView)
}

func (c *Compositor) enterKeyboardSurface(surface *C.struct_wlr_surface, keyboard *C.struct_wlr_keyboard) {
	c.prepareKeyboardFocus()
	c.activateKeyboard(keyboard)
	C.sysc_seat_keyboard_focus(c.seat, surface, c.seatKeyboard)
}

func (c *Compositor) keyboardFocusChanged(previous *toplevelView) {
	c.updateShortcutsInhibitors()
	c.updateTextInputFocus()
	current := c.keyboardFocusedView()
	if current == previous {
		return
	}
	if current != nil {
		c.recordWindowFocus(current)
	}
	c.notifyWindowStateChanged()
}

func (c *Compositor) startPointerGrab(view *toplevelView, mode pointerGrabMode,
	edges uint32, button uint32) bool {
	if view == nil || view.toplevel == nil || view.tree == nil {
		return false
	}
	var x, y C.int
	C.sysc_scene_tree_position(view.tree, &x, &y)
	width := int(view.toplevel.base.geometry.width)
	height := int(view.toplevel.base.geometry.height)
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	c.pointerGrab = &pointerGrab{
		mode: mode, cursorX: float64(c.cursor.x), cursorY: float64(c.cursor.y),
		x: int(x), y: int(y), width: width, height: height, edges: edges,
	}
	c.grabView = view
	c.grabButton = button
	// Niri starts its grabs with Focus::Clear: the window under the pointer
	// gets a leave rather than an enter that stops moving.
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	return true
}

func (c *Compositor) clearPointerGrab() {
	wasGrabbed := c.pointerGrab != nil
	if c.pointerGrab != nil && c.pointerGrab.mode == grabMove && c.grabView != nil &&
		c.grabView.tree != nil && c.model != nil {
		x, y := c.grabView.treePosition()
		geometry := c.grabView.geometry()
		c.model.SetFloatingGeometry(c.grabView.id, layout.Bounds{
			X: x, Y: y, Width: geometry.w, Height: geometry.h,
		})
	}
	c.pointerGrab = nil
	c.grabView = nil
	c.grabButton = 0
	if wasGrabbed {
		c.restoreCursorImage()
	}
}

func (c *Compositor) resizeEdgesAt(view *toplevelView) uint32 {
	if view == nil || c.model == nil {
		return 0
	}
	x, y := view.treePosition()
	geometry := view.geometry()
	fx := (float64(c.cursor.x) - float64(x)) / float64(max(geometry.w, 1))
	fy := (float64(c.cursor.y) - float64(y)) / float64(max(geometry.h, 1))
	var edges uint32
	if fx < 1.0/3.0 {
		edges |= edgeLeft
	} else if fx > 2.0/3.0 {
		edges |= edgeRight
	}
	if fy < 1.0/3.0 {
		edges |= edgeTop
	} else if fy > 2.0/3.0 {
		edges |= edgeBottom
	}
	return edges
}

func resizeCursorName(edges uint32) string {
	switch edges & (edgeLeft | edgeRight | edgeTop | edgeBottom) {
	case edgeLeft:
		return "w-resize"
	case edgeRight:
		return "e-resize"
	case edgeTop:
		return "n-resize"
	case edgeBottom:
		return "s-resize"
	case edgeLeft | edgeTop:
		return "nw-resize"
	case edgeRight | edgeTop:
		return "ne-resize"
	case edgeLeft | edgeBottom:
		return "sw-resize"
	default:
		return "se-resize"
	}
}

func (c *Compositor) handleToplevelMove(view *toplevelView, data unsafe.Pointer) {
	event := (*C.struct_wlr_xdg_toplevel_move_event)(data)
	button, pressed := c.pointerSerialButtons[uint32(event.serial)]
	if event.toplevel != view.toplevel || !bool(C.wlr_seat_validate_pointer_grab_serial(
		c.seat, view.toplevel.base.surface, event.serial)) || !pressed ||
		!c.pressedPointerButtons[button] {
		return
	}
	c.startPointerGrab(view, grabMove, 0, button)
}

func (c *Compositor) handleToplevelResize(view *toplevelView, data unsafe.Pointer) {
	event := (*C.struct_wlr_xdg_toplevel_resize_event)(data)
	button, pressed := c.pointerSerialButtons[uint32(event.serial)]
	if event.toplevel != view.toplevel || !bool(C.wlr_seat_validate_pointer_grab_serial(
		c.seat, view.toplevel.base.surface, event.serial)) || !pressed ||
		!c.pressedPointerButtons[button] {
		return
	}
	c.startPointerGrab(view, grabResize, uint32(event.edges), button)
}

func (c *Compositor) handleNewToplevel(data unsafe.Pointer) {
	toplevel := (*C.struct_wlr_xdg_toplevel)(data)
	tree := c.ensureSceneTree(toplevel.base)
	if tree == nil {
		return
	}
	c.nextViewID++
	view := &toplevelView{toplevel: toplevel, tree: tree, id: c.nextViewID, presentationOpacity: 1}
	c.views[unsafe.Pointer(toplevel)] = view
	add := func(signal Signal, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		view.listeners = append(view.listeners, listener)
		c.keep(listener)
	}
	var initialCommit *Listener
	initialCommit = AddListener(unsafe.Pointer(&toplevel.base.surface.events.commit), func(unsafe.Pointer) {
		if !toplevel.base.initialized || !toplevel.base.initial_commit {
			return
		}
		c.drop(initialCommit)
		if c.preferNoCSD && !bool(toplevel.requested.fullscreen) {
			C.wlr_xdg_toplevel_set_tiled(toplevel,
				C.uint32_t(C.WLR_EDGE_TOP|C.WLR_EDGE_RIGHT|C.WLR_EDGE_BOTTOM|C.WLR_EDGE_LEFT))
		}
		// kitty --start-as=fullscreen asks before its first commit.
		if bool(toplevel.requested.fullscreen) {
			c.applyFullscreen(view)
			return
		}
		C.wlr_xdg_toplevel_set_size(toplevel, 0, 0)
	})
	view.listeners = append(view.listeners, initialCommit)
	c.keep(initialCommit)
	add(unsafe.Pointer(&toplevel.events.request_move), func(data unsafe.Pointer) {
		c.handleToplevelMove(view, data)
	})
	add(unsafe.Pointer(&toplevel.events.request_resize), func(data unsafe.Pointer) {
		c.handleToplevelResize(view, data)
	})
	// A request before the initial commit is answered by that commit's
	// configure; one sent before then is a protocol error.
	add(unsafe.Pointer(&toplevel.events.request_fullscreen), func(unsafe.Pointer) {
		if bool(toplevel.base.initialized) {
			c.applyFullscreen(view)
		}
	})
	add(unsafe.Pointer(&toplevel.events.set_title), func(unsafe.Pointer) {
		c.notifyWindowStateChanged()
	})
	add(unsafe.Pointer(&toplevel.events.set_app_id), func(unsafe.Pointer) {
		c.notifyWindowStateChanged()
	})
	// wlroots clears a seat's pointer focus when the surface is destroyed,
	// never on unmap, and a destroyed toplevel unmaps first.
	add(unsafe.Pointer(&toplevel.base.surface.events.unmap), func(unsafe.Pointer) {
		if c.tiledMove != nil && c.tiledMove.view == view {
			c.cancelTiledWindowMove()
		}
		if view.closeSnapshot == nil && view.presentationVisible {
			if snapshot, count := sceneFXCloneView(unsafe.Pointer(c.windowAnimationTree), unsafe.Pointer(view.tree)); count > 0 {
				view.closeSnapshot = snapshot
			}
		}
		view.mapped = false
		c.closeForeignToplevel(view)
		view.presentationVisible = false
		view.openAnimationStarted = false
		view.openAnimationFinished = false
		view.presentationOpacity = 1
		if view.closeSnapshotTimer != nil {
			view.closeSnapshotTimer.Stop()
			view.closeSnapshotTimer = nil
		}
		view.closeSnapshotGeneration++
		c.cancelViewAnimations(view)
		c.cancelResizeAnimation(view)
		c.setViewAppearanceEnabled(view, false)
		c.cancelPendingWindowFocus(view)
		if c.model != nil {
			c.model.RemoveWindow(view.id)
			c.applyWorkspaceModel()
		}
		c.switcherWindowGone(view.id)
		c.releaseView(view)
		if view.closeSnapshot != nil {
			snapshot := view.closeSnapshot
			view.closeSnapshot = nil
			owner := c.nextAnimationOwner()
			c.closingNodes[owner] = snapshot
			c.startCloseSnapshot(owner, snapshot)
		}
		c.syncOverviewScene()
		c.notifyWindowStateChanged()
	})
	add(unsafe.Pointer(&toplevel.events.destroy), func(unsafe.Pointer) {
		c.handleToplevelDestroy(view)
	})
	// The map signal fires inside the commit, before the scene attaches the
	// buffer. This callback runs after the scene's commit listener, when the
	// mapped view can be hit-tested.
	add(unsafe.Pointer(&toplevel.base.surface.events.commit), func(unsafe.Pointer) {
		c.syncOverviewSurfaceListeners(view)
		if !bool(toplevel.base.surface.mapped) {
			c.syncOverviewScene()
			return
		}
		geometry := view.geometry()
		geometryChanged := geometry != view.lastGeometry
		view.lastGeometry = geometry
		fullscreen := bool(toplevel.current.fullscreen)
		if fullscreen != view.lastFullscreen {
			view.lastFullscreen = fullscreen
			if !fullscreen && c.model != nil {
				c.applyWorkspaceModel()
			}
		}
		newlyMapped := !view.mapped
		view.mapped = true
		if newlyMapped {
			if c.model != nil {
				clientFullscreen := bool(toplevel.current.fullscreen) || bool(toplevel.requested.fullscreen)
				if !view.rulesApplied {
					c.mapWindowWithRules(view, clientFullscreen)
					view.rulesApplied = true
				} else {
					output := c.model.Snapshot().FocusedOutput
					if output == "" {
						output = c.FocusedOutputName()
					}
					c.mapWindowAfterUnmap(view, output, clientFullscreen)
				}
			}
			// Niri activates newly mapped windows (ActivateWindow::Smart); its
			// dialog and activation-token heuristics come with M5.
			c.focusView(view)
		}
		if c.syncWorkspaceWindowSizeHints(view) {
			c.applyWorkspaceModel()
		} else if geometryChanged && c.model != nil {
			c.applyWorkspaceModel()
		}
		c.applyViewAppearance(view)
		c.notifyWindowStateChanged()
		c.syncOverviewScene()
		c.syncSwitcher()
		if newlyMapped && c.viewMapped != nil {
			c.viewMapped(view)
		}
	})
}

// handleRequestSetCursor lets the user seat's focused client set the drawn
// image. It is registered on the user seat only.
func (c *Compositor) handleRequestSetCursor(data unsafe.Pointer) {
	event := (*C.struct_wlr_seat_pointer_request_set_cursor_event)(data)
	if C.sysc_set_cursor_from_seat(c.cursor, c.seat, event) {
		if c.cursorSurfaceDestroy != nil {
			c.drop(c.cursorSurfaceDestroy)
			c.cursorSurfaceDestroy = nil
		}
		c.cursorSurface, c.cursorClientSet = event.surface, true
		c.cursorShape = ""
		c.cursorHotspotX, c.cursorHotspotY = int(event.hotspot_x), int(event.hotspot_y)
		if event.surface != nil {
			surface := event.surface
			var listener *Listener
			listener = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
				if c.cursorSurface == surface {
					c.cursorSurface = nil
					c.cursorSurfaceDestroy = nil
					c.cursorClientSet = false
					if c.pointerGrab == nil {
						c.showDefaultImage()
					}
				}
				c.drop(listener)
			})
			c.cursorSurfaceDestroy = listener
			c.keep(listener)
		}
		c.drawnImage = ""
	}
}

// setCursorListeners reports, for the user seat and an injected seat, whether
// anything listens for a client's request to change the drawn image.
func (c *Compositor) setCursorListeners(injected string) (user, other bool) {
	user = bool(C.sysc_seat_has_set_cursor_listener(c.seat))
	if s := c.injectedSeat(injected); s != nil {
		other = bool(C.sysc_seat_has_set_cursor_listener(s.seat))
	}
	return user, other
}

// userFocus is the user seat's keyboard focus and focused view, for tests.
func (c *Compositor) userFocus() (unsafe.Pointer, *toplevelView) {
	return unsafe.Pointer(c.seat.keyboard_state.focused_surface), c.focusedView
}

// injectedFocus is an injected seat's keyboard focus, for tests.
func (c *Compositor) injectedFocus(name string) unsafe.Pointer {
	if s := c.injectedSeat(name); s != nil {
		return unsafe.Pointer(s.seat.keyboard_state.focused_surface)
	}
	return nil
}

// injectedPointerFocus is an injected seat's pointer focus, for tests.
func (c *Compositor) injectedPointerFocus(name string) unsafe.Pointer {
	if s := c.injectedSeat(name); s != nil {
		return unsafe.Pointer(s.seat.pointer_state.focused_surface)
	}
	return nil
}

// unmapView unmaps a view's surface the way a client's null-buffer commit
// does, for tests: no installed client unmaps without destroying its role.
func (c *Compositor) unmapView(view *toplevelView) {
	C.wlr_surface_unmap(view.toplevel.base.surface)
}

// treePosition is a view's scene-tree origin in layout coordinates.
func (v *toplevelView) treePosition() (int, int) {
	var x, y C.int
	C.sysc_scene_tree_position(v.tree, &x, &y)
	return int(x), int(y)
}

// placeView moves a view's window-geometry origin to (x, y) in layout
// coordinates, for tests.
func (c *Compositor) placeView(view *toplevelView, x, y int) {
	C.sysc_scene_tree_set_position(view.tree, C.int(x), C.int(y))
	c.syncViewDecorations(view)
}

// geometry is a view's window geometry: its offset inside the surface (the
// client's shadow, for example) and its size.
func (view *toplevelView) geometry() rect {
	g := view.toplevel.base.geometry
	return rect{int(g.x), int(g.y), int(g.width), int(g.height)}
}

func (view *toplevelView) sceneNode() unsafe.Pointer {
	if view == nil || view.tree == nil {
		return nil
	}
	return unsafe.Pointer(&view.tree.node)
}

func (view *toplevelView) surfacePointer() unsafe.Pointer {
	if view == nil || view.toplevel == nil {
		return nil
	}
	return unsafe.Pointer(view.toplevel.base.surface)
}

// viewCenter is a mapped view's centre in layout coordinates.
func (c *Compositor) viewCenter(view *toplevelView) (float64, float64) {
	var x, y C.int
	C.sysc_scene_tree_position(view.tree, &x, &y)
	g := view.toplevel.base.geometry
	return float64(x) + float64(g.width)/2, float64(y) + float64(g.height)/2
}

// releaseView drops any compositor grab on view and every seat's keyboard and
// pointer focus inside it. Which window is focused next is layout policy (M5).
func (c *Compositor) releaseView(view *toplevelView) {
	if c.grabView == view {
		c.clearPointerGrab()
	}
	wasFocused := c.focusedView == view
	if wasFocused {
		c.focusedView = nil
	}
	seats := []*C.struct_wlr_seat{c.seat}
	for _, s := range c.injected {
		seats = append(seats, s.seat)
	}
	for _, seat := range seats {
		if c.viewForSurface(seat.keyboard_state.focused_surface) == view {
			C.wlr_seat_keyboard_notify_clear_focus(seat)
		}
		if c.viewForSurface(seat.pointer_state.focused_surface) == view {
			C.wlr_seat_pointer_notify_clear_focus(seat)
		}
	}
	if wasFocused && c.model != nil {
		c.activateWorkspaceFocus()
	} else {
		c.updateKeyboardFocus()
	}
	c.syncFocusRings()
}

func (c *Compositor) handleToplevelDestroy(view *toplevelView) {
	if view == nil {
		return
	}
	c.closeForeignToplevel(view)
	c.cancelViewAnimations(view)
	c.cancelResizeAnimation(view)
	if view.closeSnapshotTimer != nil {
		view.closeSnapshotTimer.Stop()
		view.closeSnapshotTimer = nil
	}
	if view.closeSnapshot != nil {
		sceneFXNodeDestroy(view.closeSnapshot)
		view.closeSnapshot = nil
	}
	if c.tiledMove != nil && c.tiledMove.view == view {
		c.cancelTiledWindowMove()
	}
	if c.model != nil {
		c.model.DestroyWindow(view.id)
	}
	view.mapped = false
	c.dropOverviewSurfaceListeners(view, nil)
	c.syncOverviewScene()
	c.releaseView(view)
	c.cancelPendingWindowFocus(view)
	c.destroyViewAppearance(view)
	delete(c.views, unsafe.Pointer(view.toplevel))
	for _, listener := range view.listeners {
		c.drop(listener)
	}
	view.listeners = nil
	c.notifyWindowStateChanged()
}

// refreshCapabilities advertises exactly what is attached. Advertising a
// capability no device backs is how a seat ends up promising input it cannot
// deliver.
func (c *Compositor) refreshCapabilities() {
	var caps C.uint32_t
	if len(c.pointerDevices) > 0 {
		caps |= C.WL_SEAT_CAPABILITY_POINTER
	}
	if len(c.keyboardDevices)+len(c.virtualKeyboards) > 0 {
		caps |= C.WL_SEAT_CAPABILITY_KEYBOARD
	}
	C.wlr_seat_set_capabilities(c.seat, caps)
	c.capabilities = int(caps)
}

// SeatCapabilities reports the capability bitmask the seat advertises. A
// zero-valued seat is a valid M0 state, not an error: a nested session with no
// forwarded input devices still has to serve clients.
func (c *Compositor) SeatCapabilities() int { return c.capabilities }
