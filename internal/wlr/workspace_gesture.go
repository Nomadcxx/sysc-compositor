package wlr

import (
	"math"
	"time"
	"unsafe"
)

// workspaceSwitchKey animates one output's settling workspace switch.
func workspaceSwitchKey(output string) animationKey {
	return animationKey{property: "workspace-switch:" + output}
}

// workspaceOutput keeps workspace projections and sliding windows on their
// own output, including decorations and shader nodes. Synchronous rendering runs between
// hiding these nodes and restoring them on the compositor thread.
func (c *Compositor) workspaceOutput(output string) func() {
	var hidden []unsafe.Pointer
	hide := func(node unsafe.Pointer) {
		if node != nil && sceneFXNodeEnabled(node) {
			hidden = append(hidden, node)
			sceneFXSetNodeEnabled(node, false)
		}
	}
	for node, owner := range c.overviewWorkspaceNodes {
		if owner != output {
			hide(node)
		}
	}
	if c.model != nil {
		for _, view := range c.views {
			if view == nil || view.tree == nil || view.outputName == output {
				continue
			}
			if _, switching := c.model.WorkspaceRenderOffset(view.outputName); !switching {
				continue
			}
			for _, node := range []unsafe.Pointer{view.sceneNode(), view.resizeSnapshot, view.shadowNode, view.borderNode, view.focusRingNode} {
				hide(node)
			}
			if c.animationShaders != nil {
				for _, property := range []string{"open-opacity", "resize-shader"} {
					if effect := c.animationShaders.effects[animationKey{owner: view.id, property: property}]; effect != nil {
						hide(unsafe.Pointer(effect.node))
					}
				}
			}
		}
	}
	return func() {
		for _, node := range hidden {
			sceneFXSetNodeEnabled(node, true)
		}
	}
}

// beginWorkspaceGesture starts Niri's touchpad workspace switch on output,
// catching a switch that is still settling there.
func (c *Compositor) beginWorkspaceGesture(output string) bool {
	if c.model == nil {
		return false
	}
	c.cancelAnimation(workspaceSwitchKey(output))
	return c.model.BeginWorkspaceGesture(output)
}

func (c *Compositor) updateWorkspaceGesture(delta float64, timestamp time.Duration) {
	if c.model != nil && c.model.UpdateWorkspaceGesture(delta, timestamp, c.model.OverviewZoom(c.overviewConfig.Zoom)) {
		c.presentWorkspaceSwitch()
		c.syncOverviewScene()
	}
}

// endWorkspaceGesture activates the workspace the gesture projects to, then
// springs the drawn workspaces into place from the release velocity.
func (c *Compositor) endWorkspaceGesture(timestamp time.Duration) {
	if c.model == nil {
		return
	}
	output, from, velocity, ok := c.model.EndWorkspaceGesture(timestamp, c.model.OverviewZoom(c.overviewConfig.Zoom))
	if !ok {
		return
	}
	c.applyWorkspaceModel()
	c.activateWorkspaceFocus()
	c.animateWorkspaceSwitch(output, from, velocity)
}

func (c *Compositor) animateWorkspaceSwitch(output string, from, velocity float64) {
	c.animateValue(workspaceSwitchKey(output), c.animationConfig.WorkspaceSwitch, from, 0, velocity,
		func(offset float64) {
			c.model.SetWorkspaceRenderOffset(output, offset)
			c.presentWorkspaceSwitch()
			c.syncOverviewScene()
		}, func() {
			c.model.ClearWorkspaceRenderOffset(output)
			c.applyWorkspaceModel()
		})
}

// presentWorkspaceSwitch draws the workspaces within one step of an output's
// switch offset where Niri's vertical strip of workspaces puts them. The
// overview projects that strip itself.
func (c *Compositor) presentWorkspaceSwitch() {
	if c.model == nil || c.model.OverviewProgress() > 0 {
		return
	}
	snapshot := c.model.Snapshot()
	position := make(map[uint64]int, len(snapshot.Workspaces))
	active := make(map[string]int)
	count := make(map[string]int)
	for _, workspace := range snapshot.Workspaces {
		position[workspace.ID] = count[workspace.Output]
		if workspace.IsActive {
			active[workspace.Output] = count[workspace.Output]
		}
		count[workspace.Output]++
	}
	fullscreen := make(map[uint64]uint64)
	for _, window := range snapshot.Windows {
		if window.Fullscreen {
			fullscreen[window.WorkspaceID] = window.ID
		}
	}
	views := make(map[uint64]*toplevelView, len(c.views))
	for _, view := range c.views {
		if view != nil && view.tree != nil && view.mapped {
			views[view.id] = view
		}
	}
	for _, window := range snapshot.Windows {
		offset, switching := c.model.WorkspaceRenderOffset(window.Output)
		view := views[window.ID]
		if !switching || view == nil {
			continue
		}
		steps := float64(position[window.WorkspaceID]-active[window.Output]) - offset
		shown := math.Abs(steps) <= 1 && (window.Tile.Visible || window.Fullscreen)
		if id := fullscreen[window.WorkspaceID]; id != 0 {
			shown = shown && id == window.ID
		}
		// A workspace in view draws, decorations and all, until the switch
		// settles and applyWorkspaceModel restores the model's visibility.
		view.presentationVisible = shown
		if !shown {
			c.setViewSceneEnabled(view, false)
			c.setViewAppearanceEnabled(view, false)
			continue
		}
		x, y, _, _ := c.windowSceneTarget(view, window)
		bounds, _ := c.outputLayoutBounds(window.Output)
		height := bounds.Height + int(math.Round(float64(bounds.Height)*0.1))
		c.cancelAnimation(animationKey{owner: view.id, property: "x"})
		c.cancelAnimation(animationKey{owner: view.id, property: "y"})
		c.setViewScenePosition(view, x, y+int(math.Round(steps*float64(height))))
		c.setViewSceneEnabled(view, true)
		c.setViewAppearanceEnabled(view, true)
	}
}
