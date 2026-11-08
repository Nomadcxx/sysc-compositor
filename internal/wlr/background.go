package wlr

import (
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// syncBackgroundColors fills each output below its background layer surfaces
// with the background-color of its active workspace, or the layout's, as
// Niri's workspace background. Unlike Niri's, the fill belongs to the output:
// it changes at once on a workspace switch instead of sliding with the
// workspace, and the overview does not draw it.
func (c *Compositor) syncBackgroundColors() {
	if c.model == nil || c.backgroundTree == nil {
		return
	}
	if c.backgroundNodes == nil {
		c.backgroundNodes = make(map[unsafe.Pointer]unsafe.Pointer)
	}
	active := make(map[string]*string)
	for _, workspace := range c.model.Snapshot().Workspaces {
		if workspace.IsActive {
			active[workspace.Output] = workspace.Name
		}
	}
	for output, node := range c.backgroundNodes {
		if _, ok := c.sceneOutputs[output]; !ok {
			sceneFXNodeDestroy(node)
			delete(c.backgroundNodes, output)
		}
	}
	for output := range c.sceneOutputs {
		color := c.layoutOptions.BackgroundColor
		if name := active[outputName(output)]; name != nil {
			if override, ok := c.layoutOptions.WorkspaceBackgroundColors[config.WorkspaceKey(*name)]; ok {
				color = override
			}
		}
		node := c.backgroundNodes[output]
		if node == nil {
			if node = sceneFXCreateRect(unsafe.Pointer(c.backgroundTree), 1, 1, [4]float32{}); node == nil {
				continue
			}
			sceneFXRectIgnoreInput(node)
			c.backgroundNodes[output] = node
		}
		box := c.outputLayoutBox(output)
		sceneFXRectUpdateFill(node, box.Min.X, box.Min.Y, box.Dx(), box.Dy(), [4]uint16{}, premultipliedColor(color))
		sceneFXSetNodeEnabled(node, color.A > 0 && !box.Empty())
	}
}
