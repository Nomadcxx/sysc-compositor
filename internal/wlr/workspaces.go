package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_xdg_shell.h>

static void sysc_workspace_output_layout_box(struct wlr_output_layout *layout,
		struct wlr_output *output, struct wlr_box *box) {
	wlr_output_layout_get_box(layout, output, box);
}

static void sysc_workspace_set_position(struct wlr_scene_tree *tree, int x, int y) {
	wlr_scene_node_set_position(&tree->node, x, y);
}

static void sysc_workspace_set_enabled(struct wlr_scene_tree *tree, bool enabled) {
	wlr_scene_node_set_enabled(&tree->node, enabled);
}

static void sysc_workspace_set_size(struct wlr_xdg_toplevel *toplevel, int width, int height) {
	wlr_xdg_toplevel_set_size(toplevel, width, height);
}
*/
import "C"

import (
	"math"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func (c *Compositor) hasWorkspaceOutput(name string) bool {
	for _, workspace := range c.model.Snapshot().Workspaces {
		if workspace.Output == name {
			return true
		}
	}
	return false
}

func (c *Compositor) syncWorkspaceOutput(output unsafe.Pointer) {
	if c.model == nil || output == nil {
		return
	}
	name := C.GoString((*C.struct_wlr_output)(output).name)
	usable := c.usable[output]
	if usable.w <= 0 || usable.h <= 0 {
		var full C.struct_wlr_box
		C.sysc_workspace_output_layout_box(c.layout, (*C.struct_wlr_output)(output), &full)
		usable = rect{int(full.x), int(full.y), int(full.width), int(full.height)}
	}
	bounds := layout.Bounds{X: usable.x, Y: usable.y, Width: usable.w, Height: usable.h}
	width := c.defaultColumnWidth(bounds.Width)
	added := false
	if !c.hasWorkspaceOutput(name) {
		added = c.model.AddOutput(name, bounds, width, c.layoutOptions.Gaps)
	} else {
		c.model.SetOutputGeometry(name, bounds, width, c.layoutOptions.Gaps)
	}
	c.applyWorkspaceModel()
	if added && c.model.Snapshot().FocusedOutput == name {
		c.activateWorkspaceFocus()
	}
}

func (c *Compositor) removeWorkspaceOutput(name string) {
	if c.model == nil {
		return
	}
	c.model.RemoveOutput(name)
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
}

// SetFullscreen changes the server-side xdg fullscreen state while retaining
// the workspace placement for restoration after the client acknowledges exit.
func (c *Compositor) SetFullscreen(id uint64, fullscreen bool) bool {
	if c.model == nil {
		return false
	}
	var view *toplevelView
	for _, candidate := range c.views {
		if candidate.id == id && candidate.mapped && bool(candidate.toplevel.base.surface.mapped) {
			view = candidate
			break
		}
	}
	if view == nil || !c.model.SetFullscreen(id, fullscreen) {
		return false
	}
	C.wlr_xdg_toplevel_set_fullscreen(view.toplevel, C.bool(fullscreen))
	if fullscreen {
		for output := range c.sceneOutputs {
			if C.GoString((*C.struct_wlr_output)(output).name) != c.outputNameForView(view) {
				continue
			}
			var bounds C.struct_wlr_box
			C.sysc_workspace_output_layout_box(c.layout, (*C.struct_wlr_output)(output), &bounds)
			C.wlr_xdg_toplevel_set_size(view.toplevel, bounds.width, bounds.height)
			C.sysc_workspace_set_position(view.tree, bounds.x, bounds.y)
			break
		}
	} else {
		C.wlr_xdg_toplevel_set_size(view.toplevel, 0, 0)
	}
	c.applyWorkspaceModel()
	c.notifyWindowStateChanged()
	return true
}

// ToggleFullscreen toggles the active workspace window's client fullscreen.
func (c *Compositor) ToggleFullscreen() bool {
	window, ok := c.activeWindowSnapshot()
	return ok && c.SetFullscreen(window.ID, !window.Fullscreen)
}

func (c *Compositor) ToggleFullscreenWindow(id *uint64) bool {
	window, ok := c.windowActionSnapshot(id)
	return ok && c.SetFullscreen(window.ID, !window.Fullscreen)
}

// SetWindowedFullscreen changes the client's xdg fullscreen state while keeping
// its ordinary workspace placement and size.
func (c *Compositor) SetWindowedFullscreen(id uint64, fullscreen bool) bool {
	if c.model == nil {
		return false
	}
	var view *toplevelView
	for _, candidate := range c.views {
		if candidate.id == id && candidate.mapped && bool(candidate.toplevel.base.surface.mapped) {
			view = candidate
			break
		}
	}
	if view == nil || !c.model.SetWindowedFullscreen(id, fullscreen) {
		return false
	}
	C.wlr_xdg_toplevel_set_fullscreen(view.toplevel, C.bool(fullscreen))
	c.applyWorkspaceModel()
	c.notifyWindowStateChanged()
	return true
}

func (c *Compositor) ToggleWindowedFullscreen() bool {
	window, ok := c.activeWindowSnapshot()
	return ok && c.SetWindowedFullscreen(window.ID, !window.WindowedFullscreen)
}

func (c *Compositor) ToggleWindowedFullscreenWindow(id *uint64) bool {
	window, ok := c.windowActionSnapshot(id)
	return ok && c.SetWindowedFullscreen(window.ID, !window.WindowedFullscreen)
}

func (c *Compositor) MoveWorkspaceToOutput(id uint64, output string) bool {
	if c.model == nil || !c.model.MoveWorkspaceToOutput(id, output) {
		return false
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

func (c *Compositor) MoveWorkspaceToMonitorDirection(direction string) bool {
	if c.model == nil || !c.model.MoveFocusedWorkspaceToMonitorDirection(direction) {
		return false
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	return true
}

func (c *Compositor) FocusMonitor(output string) bool {
	if c.model == nil || !c.model.FocusOutput(output) {
		return false
	}
	c.activateWorkspaceFocus()
	c.warpCursorToOutput(output)
	return true
}

func (c *Compositor) FocusMonitorDirection(direction string) bool {
	if c.model == nil || !c.model.FocusOutputDirection(direction) {
		return false
	}
	output := c.model.Snapshot().FocusedOutput
	c.activateWorkspaceFocus()
	c.warpCursorToOutput(output)
	return true
}

func (c *Compositor) warpCursorToOutput(name string) {
	if c.cursor == nil {
		return
	}
	for pointer := range c.sceneOutputs {
		output := (*C.struct_wlr_output)(pointer)
		if C.GoString(output.name) != name {
			continue
		}
		var bounds C.struct_wlr_box
		C.sysc_workspace_output_layout_box(c.layout, output, &bounds)
		C.wlr_cursor_warp_closest(c.cursor, nil,
			C.double(bounds.x)+C.double(bounds.width)/2,
			C.double(bounds.y)+C.double(bounds.height)/2)
		return
	}
}

func (c *Compositor) SetWindowUrgent(id uint64, urgent bool) bool {
	if c.model == nil || !c.model.SetWindowUrgent(id, urgent) {
		return false
	}
	c.notifyStateChanged()
	return true
}

func (c *Compositor) ToggleWindowUrgent(id uint64) bool {
	if c.model == nil || !c.model.ToggleWindowUrgent(id) {
		return false
	}
	c.notifyStateChanged()
	return true
}

func (c *Compositor) workspaceBounds(name string) layout.Bounds {
	for output := range c.sceneOutputs {
		if C.GoString((*C.struct_wlr_output)(output).name) != name {
			continue
		}
		area := c.usable[output]
		if area.w <= 0 || area.h <= 0 {
			var full C.struct_wlr_box
			C.sysc_workspace_output_layout_box(c.layout, (*C.struct_wlr_output)(output), &full)
			area = rect{int(full.x), int(full.y), int(full.width), int(full.height)}
		}
		return layout.Bounds{X: area.x, Y: area.y, Width: area.w, Height: area.h}
	}
	return layout.Bounds{}
}

func (c *Compositor) outputLayoutBounds(name string) (layout.Bounds, bool) {
	for output := range c.sceneOutputs {
		if C.GoString((*C.struct_wlr_output)(output).name) != name {
			continue
		}
		var box C.struct_wlr_box
		C.sysc_workspace_output_layout_box(c.layout, (*C.struct_wlr_output)(output), &box)
		bounds := layout.Bounds{X: int(box.x), Y: int(box.y), Width: int(box.width), Height: int(box.height)}
		return bounds, bounds.Width > 0 && bounds.Height > 0
	}
	return layout.Bounds{}, false
}

func (c *Compositor) focusedWorkspaceID() (uint64, bool) {
	if c.model == nil {
		return 0, false
	}
	for _, workspace := range c.model.Snapshot().Workspaces {
		if workspace.IsFocused {
			return workspace.ID, true
		}
	}
	return 0, false
}

func (c *Compositor) workspaceIDForBind(index uint8, name string) (uint64, bool) {
	if c.model == nil {
		return 0, false
	}
	if name != "" {
		return c.model.WorkspaceIDByName(name)
	}
	snapshot := c.model.Snapshot()
	for _, workspace := range snapshot.Workspaces {
		if workspace.Output == snapshot.FocusedOutput && workspace.Index == index {
			return workspace.ID, true
		}
	}
	return 0, false
}

func (c *Compositor) activeWindowSnapshot() (wm.WindowSnapshot, bool) {
	if c.model == nil {
		return wm.WindowSnapshot{}, false
	}
	id, ok := c.model.ActiveWindow()
	if !ok {
		return wm.WindowSnapshot{}, false
	}
	for _, window := range c.model.Snapshot().Windows {
		if window.ID == id {
			return window, true
		}
	}
	return wm.WindowSnapshot{}, false
}

func workspaceIndex(workspaces []wm.WorkspaceSnapshot, id uint64) int {
	for _, workspace := range workspaces {
		if workspace.ID == id {
			return int(workspace.Index)
		}
	}
	return 0
}

func (c *Compositor) windowSceneTarget(view *toplevelView, window wm.WindowSnapshot) (x, y, width, height int) {
	geometry := view.geometry()
	placement := window.Tile
	x, y = placement.X, placement.Y
	width, height = placement.Width, placement.Height
	if width <= 0 {
		width = geometry.w
	}
	if height <= 0 {
		height = geometry.h
	}
	if !placement.Floating {
		x += (placement.Width - geometry.w) / 2
		y += (placement.Height - geometry.h) / 2
	}
	if window.Fullscreen {
		if bounds, ok := c.outputLayoutBounds(window.Output); ok {
			return bounds.X, bounds.Y, bounds.Width, bounds.Height
		}
	}
	return x, y, width, height
}

func (c *Compositor) defaultColumnWidth(available int) int {
	if available <= 0 {
		return 0
	}
	if c.layoutOptions.FixedColumnWidth > 0 {
		return c.layoutOptions.FixedColumnWidth
	}
	proportion := c.layoutOptions.ColumnWidthProportion
	if proportion <= 0 {
		proportion = 0.5
	}
	width := (float64(available-c.layoutOptions.Gaps) * proportion) - float64(c.layoutOptions.Gaps)
	return max(1, int(math.Round(width)))
}

func (c *Compositor) applyWorkspaceModel() {
	if c.model == nil {
		return
	}
	for _, view := range c.views {
		c.syncWorkspaceWindowSizeHints(view)
	}
	snapshot := c.model.Snapshot()
	activeWorkspaceByID := make(map[uint64]bool, len(snapshot.Workspaces))
	for _, workspace := range snapshot.Workspaces {
		activeWorkspaceByID[workspace.ID] = workspace.IsActive
	}
	activeWindowIDs := activeWindowIDsForWorkspaces(snapshot.Workspaces)
	nextActive := make(map[string]activeWorkspace)
	switched := make(map[string]int)
	for _, workspace := range snapshot.Workspaces {
		if !workspace.IsActive {
			continue
		}
		current := activeWorkspace{id: workspace.ID, index: int(workspace.Index)}
		if previous, ok := c.activeWorkspaces[workspace.Output]; ok && previous.id != current.id {
			direction := 1
			if current.index < previous.index {
				direction = -1
			}
			switched[workspace.Output] = direction
		}
		nextActive[workspace.Output] = current
	}
	windows := make(map[uint64]wm.WindowSnapshot, len(snapshot.Windows))
	for _, window := range snapshot.Windows {
		windows[window.ID] = window
	}
	fullscreenByOutput := make(map[string]*toplevelView)
	for _, view := range c.views {
		if view == nil || view.toplevel == nil || !view.mapped || !bool(view.toplevel.base.surface.mapped) {
			continue
		}
		window, exists := windows[view.id]
		if !exists || !activeWorkspaceByID[window.WorkspaceID] || !window.Fullscreen {
			continue
		}
		// The WM model owns output assignment. A client request is applied to
		// the model before this reconciliation, so key visibility by the same
		// assigned output used below for each window.
		output := window.Output
		if current := fullscreenByOutput[output]; current == nil || view.touched > current.touched {
			fullscreenByOutput[output] = view
		}
	}
	for _, view := range c.views {
		if view == nil || view.tree == nil || !view.mapped {
			continue
		}
		view.workspaceActive = activeWindowIDs[view.id]
		window, exists := windows[view.id]
		if !exists {
			c.cancelViewAnimations(view)
			c.cancelResizeAnimation(view)
			view.presentationVisible = false
			c.setViewSceneEnabled(view, false)
			c.setViewAppearanceEnabled(view, false)
			continue
		}
		oldVisible := view.presentationVisible
		oldWorkspaceID, oldOutput := view.workspaceID, view.outputName
		newlyTracked := oldWorkspaceID == 0
		visible := activeWorkspaceByID[window.WorkspaceID] && window.Tile.Visible
		if fullscreen := fullscreenByOutput[window.Output]; fullscreen != nil {
			visible = fullscreen == view
		}
		view.fullscreen = window.Fullscreen
		view.workspaceID = window.WorkspaceID
		view.workspaceIndex = workspaceIndex(snapshot.Workspaces, window.WorkspaceID)
		view.outputName = window.Output
		targetX, targetY, targetWidth, targetHeight := c.windowSceneTarget(view, window)
		if view.fullscreen {
			c.cancelResizeAnimation(view)
			c.cancelAnimation(animationKey{owner: view.id, property: "x"})
			c.cancelAnimation(animationKey{owner: view.id, property: "y"})
			view.presentationVisible = visible
			c.setViewSceneEnabled(view, visible)
			c.setViewAppearanceEnabled(view, visible)
		} else if direction, changed := switched[window.Output]; changed && oldOutput == window.Output && oldVisible && !visible && oldWorkspaceID == window.WorkspaceID {
			c.cancelResizeAnimation(view)
			view.presentationVisible = false
			c.setViewSceneEnabled(view, true)
			if targetWidth > 0 && targetHeight > 0 {
				C.sysc_workspace_set_size(view.toplevel, C.int(targetWidth), C.int(targetHeight))
			}
			x, y := view.treePosition()
			height := c.workspaceBounds(window.Output).Height
			if height <= 0 {
				height = 1
			}
			c.animateViewPosition(view, x, y-direction*height, c.animationConfig.WorkspaceSwitch, func() {
				if !view.presentationVisible {
					c.setViewSceneEnabled(view, false)
				}
			})
		} else if direction, changed := switched[window.Output]; changed && oldOutput == window.Output && !oldVisible && visible && !newlyTracked {
			c.cancelResizeAnimation(view)
			view.presentationVisible = true
			height := c.workspaceBounds(window.Output).Height
			if height <= 0 {
				height = 1
			}
			c.setViewScenePosition(view, targetX, targetY+direction*height)
			c.setViewSceneEnabled(view, true)
			c.animateViewPosition(view, targetX, targetY, c.animationConfig.WorkspaceSwitch, nil)
			if targetWidth > 0 && targetHeight > 0 {
				C.sysc_workspace_set_size(view.toplevel, C.int(targetWidth), C.int(targetHeight))
			}
		} else {
			view.presentationVisible = visible
			if !visible {
				c.cancelAnimation(animationKey{owner: view.id, property: "x"})
				c.cancelAnimation(animationKey{owner: view.id, property: "y"})
				c.cancelResizeAnimation(view)
				c.setViewSceneEnabled(view, false)
				c.setViewAppearanceEnabled(view, false)
				if targetWidth > 0 && targetHeight > 0 {
					C.sysc_workspace_set_size(view.toplevel, C.int(targetWidth), C.int(targetHeight))
				}
				continue
			}
			if newlyTracked {
				c.setViewScenePosition(view, targetX, targetY)
				c.setViewSceneEnabled(view, true)
			} else {
				kind := c.animationConfig.WindowMovement
				geometry := view.geometry()
				if targetWidth != geometry.w || targetHeight != geometry.h {
					kind = c.animationConfig.WindowResize
				} else if currentX, currentY := view.treePosition(); targetX != currentX && targetY == currentY {
					kind = c.animationConfig.HorizontalViewMovement
				}
				if oldVisible && oldOutput == window.Output {
					c.animateViewPosition(view, targetX, targetY, kind, nil)
				} else {
					c.setViewScenePosition(view, targetX, targetY)
				}
				c.setViewSceneEnabled(view, true)
				if oldVisible && view.openAnimationFinished && targetWidth > 0 && targetHeight > 0 {
					c.animateViewSize(view, targetWidth, targetHeight, c.animationConfig.WindowResize)
				} else {
					c.cancelResizeAnimation(view)
				}
			}
			if targetWidth > 0 && targetHeight > 0 {
				C.sysc_workspace_set_size(view.toplevel, C.int(targetWidth), C.int(targetHeight))
			}
		}
		if visible && !view.openAnimationStarted {
			c.startWindowOpen(view)
		}
		if view.fullscreen {
			if bounds, ok := c.outputLayoutBounds(window.Output); ok {
				if x, y := view.treePosition(); x != bounds.X || y != bounds.Y {
					c.setViewScenePosition(view, bounds.X, bounds.Y)
				}
				if int(view.toplevel.scheduled.width) != bounds.Width || int(view.toplevel.scheduled.height) != bounds.Height {
					C.sysc_workspace_set_size(view.toplevel, C.int(bounds.Width), C.int(bounds.Height))
				}
			}
			continue
		}
	}
	c.activeWorkspaces = nextActive
	for _, view := range c.views {
		if view == nil || view.tree == nil {
			continue
		}
		if view.mapped {
			c.applyViewAppearance(view)
		} else {
			c.syncViewDecorations(view)
		}
	}
	// Workspace activation can change while the mapped-window snapshots stay
	// identical, especially when switching between empty workspaces.
	c.syncOverviewScene()
	c.notifyStateChanged()
}

func (c *Compositor) syncWorkspaceWindowSizeHints(view *toplevelView) bool {
	if c.model == nil || view == nil || view.toplevel == nil || !view.mapped {
		return false
	}
	current := view.toplevel.current
	return c.model.SetWindowSizeHints(view.id,
		int(current.min_width), int(current.max_width),
		int(current.min_height), int(current.max_height))
}

func (c *Compositor) activateWorkspaceFocus() {
	if c.model == nil {
		return
	}
	snapshot := c.model.Snapshot()
	if snapshot.FocusedOutput != "" {
		c.setActiveOutputName(snapshot.FocusedOutput)
	}
	var focusedID uint64
	for _, workspace := range snapshot.Workspaces {
		if workspace.IsFocused && workspace.ActiveWindowID != nil {
			focusedID = *workspace.ActiveWindowID
			break
		}
	}
	if focusedID != 0 {
		for _, view := range c.views {
			if view.id == focusedID && view.mapped {
				c.focusView(view)
				return
			}
		}
	}
	if c.focusedView != nil && c.focusedView.toplevel != nil {
		C.wlr_xdg_toplevel_set_activated(c.focusedView.toplevel, false)
	}
	c.focusedView = nil
	c.onDemandLayerFocus = nil
	c.updateKeyboardFocus()
	c.notifyWindowStateChanged()
}
