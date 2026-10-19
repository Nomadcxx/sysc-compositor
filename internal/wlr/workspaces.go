package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_output_layout.h>
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
	snapshot := c.model.Snapshot()
	activeWorkspace := make(map[uint64]bool, len(snapshot.Workspaces))
	for _, workspace := range snapshot.Workspaces {
		activeWorkspace[workspace.ID] = workspace.IsActive
	}
	windows := make(map[uint64]wm.WindowSnapshot, len(snapshot.Windows))
	for _, window := range snapshot.Windows {
		windows[window.ID] = window
	}
	for _, view := range c.views {
		if view == nil || view.tree == nil || !view.mapped {
			continue
		}
		window, exists := windows[view.id]
		if !exists {
			C.sysc_workspace_set_enabled(view.tree, false)
			continue
		}
		C.sysc_workspace_set_enabled(view.tree, C.bool(activeWorkspace[window.WorkspaceID]))
		placement := window.Tile
		C.sysc_workspace_set_position(view.tree, C.int(placement.X), C.int(placement.Y))
		if placement.Width > 0 && placement.Height > 0 {
			C.sysc_workspace_set_size(view.toplevel, C.int(placement.Width), C.int(placement.Height))
		}
	}
	// Workspace activation can change while the mapped-window snapshots stay
	// identical, especially when switching between empty workspaces.
	c.notifyStateChanged()
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
