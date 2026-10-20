package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <sys/types.h>
#include <time.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_xdg_shell.h>

static pid_t sysc_surface_pid(struct wlr_surface *surface) {
	pid_t pid = 0;
	wl_client_get_credentials(wl_resource_get_client(surface->resource), &pid, NULL, NULL);
	return pid;
}

static uint64_t sysc_monotonic_time_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (uint64_t)ts.tv_sec * 1000000000ULL + (uint64_t)ts.tv_nsec;
}
*/
import "C"

import (
	"cmp"
	"reflect"
	"slices"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// WindowInfo is an immutable snapshot of one mapped toplevel. ID is stable for
// the window's lifetime and never reused within a run.
type WindowInfo struct {
	ID                uint64
	AppID, Title      *string
	PID               int
	X, Y              int // layout coordinates of the window geometry
	Width, Height     int
	Output            string
	IsFocused         bool
	FocusTimestamp    time.Duration
	HasFocusTimestamp bool
}

// Windows lists toplevels, most recently focused first by either seat, then
// newest first. Call it on the compositor thread.
func (c *Compositor) Windows() []WindowInfo {
	views := make([]*toplevelView, 0, len(c.views))
	for _, v := range c.views {
		if v.toplevel != nil && v.tree != nil && v.mapped && bool(v.toplevel.base.surface.mapped) {
			views = append(views, v)
		}
	}
	slices.SortFunc(views, func(a, b *toplevelView) int {
		return cmp.Or(cmp.Compare(b.touched, a.touched), cmp.Compare(b.id, a.id))
	})
	out := make([]WindowInfo, 0, len(views))
	for _, v := range views {
		x, y := v.treePosition()
		g := v.toplevel.base.geometry
		out = append(out, WindowInfo{
			ID: v.id, AppID: cStringPointer(v.toplevel.app_id), Title: cStringPointer(v.toplevel.title),
			PID: int(C.sysc_surface_pid(v.toplevel.base.surface)),
			X:   x, Y: y, Width: int(g.width), Height: int(g.height),
			Output: c.outputNameForView(v), IsFocused: c.keyboardFocusedView() == v,
			FocusTimestamp: v.focusTimestamp, HasFocusTimestamp: v.hasFocusTimestamp,
		})
	}
	return out
}

func cStringPointer(value *C.char) *string {
	if value == nil {
		return nil
	}
	text := C.GoString(value)
	return &text
}

func (c *Compositor) notifyWindowStateChanged() {
	windows := c.Windows()
	if reflect.DeepEqual(windows, c.lastWindowState) {
		return
	}
	c.lastWindowState = slices.Clone(windows)
	c.notifyStateChanged()
}

func (c *Compositor) mapWindowWithRules(view *toplevelView, clientFullscreen bool) {
	if c.model == nil || view == nil || view.toplevel == nil {
		return
	}
	result := config.ResolveWindowRules(c.windowRules,
		cString(view.toplevel.app_id), cString(view.toplevel.title))
	output := c.model.Snapshot().FocusedOutput
	if output == "" {
		output = c.FocusedOutputName()
	}
	requestedOutput := ""
	if toplevelOutput := view.toplevel.requested.fullscreen_output; clientFullscreen && toplevelOutput != nil {
		name := C.GoString(toplevelOutput.name)
		if c.hasWorkspaceOutput(name) {
			requestedOutput = name
		}
	}
	mapped := false
	if result.OpenOnWorkspace != nil {
		if workspaceID, ok := c.model.WorkspaceIDByName(*result.OpenOnWorkspace); ok {
			mapped = c.model.MapWindowToWorkspace(view.id, workspaceID)
		}
	}
	if !mapped {
		if result.OpenOnOutput != nil {
			output = *result.OpenOnOutput
		} else if requestedOutput != "" {
			output = requestedOutput
		}
		c.model.MapWindow(view.id, output)
	}
	if clientFullscreen {
		c.model.SetFullscreen(view.id, true)
	}
	if result.OpenFloating != nil && *result.OpenFloating {
		c.model.SetWindowFloating(view.id, true, layout.Bounds{})
	}
	if result.OpenFullscreen != nil && *result.OpenFullscreen != clientFullscreen {
		c.SetFullscreen(view.id, *result.OpenFullscreen)
	}
	if result.DefaultColumnWidth != nil || result.DefaultWindowHeight != nil {
		for _, window := range c.model.Snapshot().Windows {
			if window.ID != view.id {
				continue
			}
			bounds := c.workspaceBounds(window.Output)
			width, height := 0, 0
			resize := layout.ResizeOptions{}
			geometry := view.geometry()
			if result.DefaultColumnWidth != nil {
				if result.DefaultColumnWidth.WindowChooses {
					width = geometry.w
				} else {
					width = result.DefaultColumnWidth.Resolve(bounds.Width)
				}
				resize.ResizeWidth = true
			}
			if result.DefaultWindowHeight != nil {
				if result.DefaultWindowHeight.WindowChooses {
					height = geometry.h
				} else {
					height = result.DefaultWindowHeight.Resolve(bounds.Height)
				}
				resize.ResizeHeight = true
			}
			c.model.ResizeWindow(view.id, window.Tile.X, window.Tile.Y, width, height, resize)
			break
		}
	}
}

func (c *Compositor) keyboardFocusedView() *toplevelView {
	if c.seat == nil {
		return nil
	}
	return c.viewForSurface(c.seat.keyboard_state.focused_surface)
}

func (c *Compositor) recordWindowFocus(view *toplevelView) {
	stamp := time.Duration(C.sysc_monotonic_time_ns())
	if c.pendingFocus != nil {
		c.cancelPendingWindowFocus(c.pendingFocus)
	}
	if !view.hasFocusTimestamp {
		view.focusTimestamp = stamp
		view.hasFocusTimestamp = true
		return
	}
	if c.focusTimer != nil {
		c.focusTimer.Stop()
	}
	c.pendingFocus = view
	c.pendingFocusStamp = stamp
	c.pendingFocusGeneration++
	generation := c.pendingFocusGeneration
	c.focusTimer = time.AfterFunc(750*time.Millisecond, func() {
		_ = c.Post(func() {
			c.commitPendingWindowFocus(view, generation)
		})
	})
}

func (c *Compositor) commitPendingWindowFocus(view *toplevelView, generation uint64) {
	if c.pendingFocus != view || c.pendingFocusGeneration != generation {
		return
	}
	c.pendingFocus = nil
	c.focusTimer = nil
	if !view.mapped {
		return
	}
	view.focusTimestamp = c.pendingFocusStamp
	view.hasFocusTimestamp = true
	c.notifyWindowStateChanged()
}

func (c *Compositor) cancelPendingWindowFocus(view *toplevelView) {
	if c.pendingFocus != view {
		return
	}
	if c.focusTimer != nil {
		c.focusTimer.Stop()
	}
	c.focusTimer = nil
	c.pendingFocus = nil
	c.pendingFocusGeneration++
}

// FocusWindow focuses the mapped toplevel with id. Unknown or unmapped IDs are
// harmless no-ops, as in Niri's window actions.
func (c *Compositor) FocusWindow(id uint64) bool {
	for _, view := range c.views {
		if view.id == id && view.mapped && bool(view.toplevel.base.surface.mapped) {
			c.focusView(view)
			return true
		}
	}
	return false
}

// CloseWindow sends the xdg-toplevel close request for id, or for the Niri
// layout-focused window when id is nil. A layer surface may own keyboard focus.
func (c *Compositor) CloseWindow(id *uint64) bool {
	var target *toplevelView
	if id == nil {
		target = c.focusedView
	} else {
		for _, view := range c.views {
			if view.id == *id {
				target = view
				break
			}
		}
	}
	if target == nil || !target.mapped || !bool(target.toplevel.base.surface.mapped) {
		return false
	}
	C.wlr_xdg_toplevel_send_close(target.toplevel)
	return true
}

// FocusWorkspaceID focuses an IPC workspace by its stable model ID.
func (c *Compositor) FocusWorkspaceID(id uint64) bool {
	if c.model == nil || !c.model.FocusWorkspaceID(id) {
		return false
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

// SetWindowFloating changes the mapped window's layer. The current scene
// rectangle seeds floating geometry when moving out of the tiled layout.
func (c *Compositor) SetWindowFloating(id uint64, floating bool) bool {
	if c.model == nil {
		return false
	}
	for _, view := range c.views {
		if view.id != id || !view.mapped || !bool(view.toplevel.base.surface.mapped) {
			continue
		}
		var bounds layout.Bounds
		if floating {
			x, y := view.treePosition()
			geometry := view.geometry()
			bounds = layout.Bounds{X: x, Y: y, Width: geometry.w, Height: geometry.h}
		}
		if !c.model.SetWindowFloating(id, floating, bounds) {
			return false
		}
		c.applyWorkspaceModel()
		c.activateWorkspaceFocus()
		return true
	}
	return false
}

// ToggleWindowFloating changes the active workspace window's layer.
func (c *Compositor) ToggleWindowFloating() bool {
	if c.model == nil {
		return false
	}
	id, ok := c.model.ActiveWindow()
	if !ok {
		return false
	}
	for _, window := range c.model.Snapshot().Windows {
		if window.ID == id {
			return c.SetWindowFloating(id, !window.Tile.Floating)
		}
	}
	return false
}

func (c *Compositor) FocusFloating() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusFloating)
}

func (c *Compositor) FocusTiling() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusTiling)
}

func (c *Compositor) SwitchFocusFloatingAndTiling() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).SwitchFocusFloatingAndTiling)
}

func (c *Compositor) FocusWindowInColumn(index uint8) bool {
	return c.applyWorkspaceFocusAction(func(model *wm.Model) bool {
		return model.FocusWindowInColumn(index)
	})
}

func (c *Compositor) FocusWindowDown() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusWindowDown)
}

func (c *Compositor) FocusWindowUp() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusWindowUp)
}

func (c *Compositor) MoveWindowDown() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveWindowDown)
}

func (c *Compositor) MoveWindowUp() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveWindowUp)
}

func (c *Compositor) ConsumeWindowIntoColumn() bool {
	return c.applyWorkspaceAction((*wm.Model).ConsumeWindowIntoColumn)
}

func (c *Compositor) ExpelWindowFromColumn() bool {
	return c.applyWorkspaceAction((*wm.Model).ExpelWindowFromColumn)
}

func (c *Compositor) ConsumeOrExpelWindowRight(id *uint64) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool { return model.ConsumeOrExpelWindowRight(id) })
}

func (c *Compositor) ConsumeOrExpelWindowLeft(id *uint64) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool { return model.ConsumeOrExpelWindowLeft(id) })
}

func (c *Compositor) ToggleColumnTabbedDisplay() bool {
	return c.applyWorkspaceAction((*wm.Model).ToggleColumnTabbedDisplay)
}

func (c *Compositor) SetColumnDisplay(display layout.ColumnDisplay) bool {
	return c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.SetColumnDisplay(display)
	})
}

func (c *Compositor) FocusWorkspaceName(name string) bool {
	return c.applyWorkspaceFocusAction(func(model *wm.Model) bool {
		return model.FocusWorkspaceName(name)
	})
}

func (c *Compositor) SetWorkspaceName(id uint64, name string) bool {
	if c.model == nil || !c.model.SetWorkspaceName(id, name) {
		return false
	}
	c.applyWorkspaceModel()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) UnsetWorkspaceName(id uint64) bool {
	if c.model == nil || !c.model.UnsetWorkspaceName(id) {
		return false
	}
	c.applyWorkspaceModel()
	c.notifyStateChanged()
	return true
}

func (c *Compositor) FocusColumnLeft() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusColumnLeft)
}

func (c *Compositor) FocusColumnRight() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusColumnRight)
}

func (c *Compositor) FocusColumnFirst() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusColumnFirst)
}

func (c *Compositor) FocusColumnLast() bool {
	return c.applyWorkspaceFocusAction((*wm.Model).FocusColumnLast)
}

func (c *Compositor) MoveColumnLeft() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveColumnLeft)
}

func (c *Compositor) MoveColumnRight() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveColumnRight)
}

func (c *Compositor) MoveColumnToFirst() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveColumnToFirst)
}

func (c *Compositor) MoveColumnToLast() bool {
	return c.applyWorkspaceAction((*wm.Model).MoveColumnToLast)
}

func (c *Compositor) MoveWindowToWorkspace(id *uint64, workspaceID uint64, follow bool) bool {
	windowID, ok := c.windowActionID(id)
	if !ok || c.model == nil || !c.model.MoveWindowToWorkspace(windowID, workspaceID, follow) {
		return false
	}
	return c.applyWorkspaceMove(true, follow)
}

func (c *Compositor) MoveColumnToWorkspace(workspaceID uint64, follow bool) bool {
	if c.model == nil || !c.model.MoveFocusedColumnToWorkspace(workspaceID, follow) {
		return false
	}
	return c.applyWorkspaceMove(true, follow)
}

func (c *Compositor) MoveWindowToWorkspaceDown(follow bool) bool {
	return c.model != nil && c.applyWorkspaceMove(c.model.MoveWindowToWorkspaceDown(follow), follow)
}

func (c *Compositor) MoveWindowToWorkspaceUp(follow bool) bool {
	return c.model != nil && c.applyWorkspaceMove(c.model.MoveWindowToWorkspaceUp(follow), follow)
}

func (c *Compositor) MoveColumnToWorkspaceDown(follow bool) bool {
	return c.model != nil && c.applyWorkspaceMove(c.model.MoveColumnToWorkspaceDown(follow), follow)
}

func (c *Compositor) MoveColumnToWorkspaceUp(follow bool) bool {
	return c.model != nil && c.applyWorkspaceMove(c.model.MoveColumnToWorkspaceUp(follow), follow)
}

func (c *Compositor) applyWorkspaceMove(changed, follow bool) bool {
	if !changed {
		return false
	}
	c.applyWorkspaceModel()
	if follow {
		c.activateWorkspaceFocus()
	} else {
		c.notifyStateChanged()
	}
	return true
}

func (c *Compositor) MoveWindowToMonitor(id *uint64, output string) bool {
	windowID, ok := c.windowActionID(id)
	return ok && c.moveWindowToMonitor(windowID, output, "")
}

func (c *Compositor) MoveWindowToMonitorDirection(id *uint64, direction string) bool {
	windowID, ok := c.windowActionID(id)
	return ok && c.moveWindowToMonitor(windowID, "", direction)
}

func (c *Compositor) MoveColumnToMonitor(output string) bool {
	return c.moveColumnToMonitor(output, "")
}

func (c *Compositor) MoveColumnToMonitorDirection(direction string) bool {
	return c.moveColumnToMonitor("", direction)
}

func (c *Compositor) ToggleWindowFloatingFor(id *uint64) bool {
	window, ok := c.windowActionSnapshot(id)
	return ok && c.SetWindowFloating(window.ID, !window.Tile.Floating)
}

func (c *Compositor) SetWindowFloatingFor(id *uint64, floating bool) bool {
	windowID, ok := c.windowActionID(id)
	return ok && c.SetWindowFloating(windowID, floating)
}

func (c *Compositor) MoveFloatingWindow(id *uint64, x, y layout.PositionChange) bool {
	windowID, ok := c.windowActionID(id)
	return ok && c.applyWorkspaceAction(func(model *wm.Model) bool {
		return model.MoveFloatingWindow(windowID, x, y)
	})
}

func (c *Compositor) windowActionID(id *uint64) (uint64, bool) {
	if id != nil {
		return *id, true
	}
	if c.model == nil {
		return 0, false
	}
	return c.model.ActiveWindow()
}

func (c *Compositor) windowActionSnapshot(id *uint64) (wm.WindowSnapshot, bool) {
	windowID, ok := c.windowActionID(id)
	if !ok || c.model == nil {
		return wm.WindowSnapshot{}, false
	}
	for _, window := range c.model.Snapshot().Windows {
		if window.ID == windowID {
			return window, true
		}
	}
	return wm.WindowSnapshot{}, false
}

func (c *Compositor) moveWindowToMonitor(id uint64, output, direction string) bool {
	if c.model == nil {
		return false
	}
	changed := false
	if direction == "" {
		changed = c.model.MoveWindowToOutput(id, output, true)
	} else {
		changed = c.model.MoveWindowToMonitorDirection(id, direction, true)
	}
	return c.afterMonitorMove(changed)
}

func (c *Compositor) moveColumnToMonitor(output, direction string) bool {
	if c.model == nil {
		return false
	}
	changed := false
	if direction == "" {
		changed = c.model.MoveFocusedColumnToOutput(output, true)
	} else {
		changed = c.model.MoveFocusedColumnToMonitorDirection(direction, true)
	}
	return c.afterMonitorMove(changed)
}

func (c *Compositor) afterMonitorMove(changed bool) bool {
	if !changed {
		return false
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

func (c *Compositor) applyWorkspaceFocusAction(action func(*wm.Model) bool) bool {
	if c.model == nil || !action(c.model) {
		return false
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

func (c *Compositor) applyWorkspaceAction(action func(*wm.Model) bool) bool {
	if c.model == nil || !action(c.model) {
		return false
	}
	c.applyWorkspaceModel()
	c.notifyWindowStateChanged()
	return true
}

// touch records that a seat focused the view, for most-recently-focused order.
func (c *Compositor) touch(v *toplevelView) {
	if v != nil {
		c.touchCount++
		v.touched = c.touchCount
	}
}
