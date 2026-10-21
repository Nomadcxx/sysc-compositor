package wlr

import (
	"math"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func activeWindowIDsForWorkspaces(workspaces []wm.WorkspaceSnapshot) map[uint64]bool {
	active := make(map[uint64]bool, len(workspaces))
	for _, workspace := range workspaces {
		if workspace.ActiveWindowID != nil {
			active[*workspace.ActiveWindowID] = true
		}
	}
	return active
}

func fitCornerRadii(radii config.CornerRadius, width, height float64) config.CornerRadius {
	factor := 1.0
	limit := func(size, sum float64) {
		if sum > 0 {
			factor = min(factor, size/sum)
		}
	}
	limit(width, radii[0]+radii[1])
	limit(width, radii[3]+radii[2])
	limit(height, radii[0]+radii[3])
	limit(height, radii[1]+radii[2])
	factor = min(1, factor)
	for i := range radii {
		radii[i] *= factor
	}
	return radii
}

func shadowCornerRadius(radii config.CornerRadius, spread int) int {
	if radii[0] <= 0 {
		return 0
	}
	return int(math.Ceil(max(0, radii[0]+float64(spread))))
}

type shadowRenderStyle struct {
	spread int
	radius int
	sigma  float64
	color  [4]float32
}

func makeShadowRenderStyle(shadow config.Shadow, radii config.CornerRadius, active bool) shadowRenderStyle {
	spread := int(math.Ceil(math.Abs(shadow.Spread)))
	if shadow.Spread < 0 {
		spread = -spread
	}
	radius := shadowCornerRadius(radii, spread)
	color := shadow.Color
	if !active {
		color.A *= 0.75
	}
	// ponytail: SceneFX shadow geometry rounds to logical pixels; M15 mixed-DPI work can align it to physical pixels.
	return shadowRenderStyle{spread: spread, radius: radius, sigma: shadow.Softness / 2, color: premultipliedColor(color)}
}

func sceneFXRadii(radii config.CornerRadius) [4]uint16 {
	var result [4]uint16
	for i, radius := range radii {
		// ponytail: SceneFX stores corner radii as uint16; clamp at its API limit.
		result[i] = uint16(min(uint64(math.Round(radius)), uint64(math.MaxUint16)))
	}
	return result
}

func premultipliedColor(color config.Color) [4]float32 {
	a := float32(color.A)
	return [4]float32{float32(color.R) * a, float32(color.G) * a, float32(color.B) * a, a}
}

func (c *Compositor) applyViewAppearance(view *toplevelView) {
	if view == nil || view.toplevel == nil || view.tree == nil {
		return
	}
	geometry := view.geometry()
	var radii config.CornerRadius
	if !view.fullscreen && view.appearance.GeometryCornerRadius != nil {
		radii = *view.appearance.GeometryCornerRadius
	}
	view.cornerRadii = fitCornerRadii(radii, float64(geometry.w), float64(geometry.h))
	sceneFXSetWindowCorners(view.sceneNode(), view.surfacePointer(), sceneFXRadii(view.cornerRadii))

	view.clipToGeometry = !view.fullscreen && view.appearance.ClipToGeometry != nil && *view.appearance.ClipToGeometry
	sceneFXSetSubsurfaceClip(view.sceneNode(), view.clipToGeometry,
		geometry.x, geometry.y, geometry.w, geometry.h)
	if view.clipToGeometry {
		c.clearPopupClips(view)
	}
	sceneFXSetTreeOpacity(view.sceneNode(), view.presentationOpacity)

	if c.layoutOptions.Shadow.On && view.shadowNode == nil {
		color := premultipliedColor(c.layoutOptions.Shadow.Color)
		view.shadowNode = sceneFXCreateShadow(unsafe.Pointer(c.windowShadowTree),
			max(1, geometry.w), max(1, geometry.h), 0, c.layoutOptions.Shadow.Softness, color)
	}
	if !c.layoutOptions.FocusRing.Off && c.layoutOptions.FocusRing.Width > 0 && view.focusRingNode == nil {
		view.focusRingNode = sceneFXCreateRect(unsafe.Pointer(c.windowFocusTree), 1, 1, [4]float32{})
	}
	c.syncViewDecorations(view)
}

func (c *Compositor) syncViewDecorations(view *toplevelView) {
	if view == nil || view.toplevel == nil || view.tree == nil {
		return
	}
	visible := view.mapped && view.presentationVisible && !view.fullscreen
	geometry := view.geometry()
	visualWidth, visualHeight := geometry.w, geometry.h
	if view.resizeSnapshot != nil {
		visualWidth = max(1, int(math.Round(view.resizeWidth)))
		visualHeight = max(1, int(math.Round(view.resizeHeight)))
	}
	x, y := view.treePosition()

	if view.shadowNode != nil {
		shadow := c.layoutOptions.Shadow
		style := makeShadowRenderStyle(shadow, view.cornerRadii,
			view == c.focusedView && c.activeOutputName == c.outputNameForView(view))
		spread := style.spread
		width, height := max(1, visualWidth+2*spread), max(1, visualHeight+2*spread)
		sceneFXShadowUpdate(view.shadowNode,
			x-spread+int(math.Round(shadow.OffsetX)), y-spread+int(math.Round(shadow.OffsetY)),
			width, height, style.radius, style.sigma, style.color)
		sceneFXSetNodeEnabled(view.shadowNode, visible && shadow.On)
	}

	ring := c.layoutOptions.FocusRing
	if view.focusRingNode == nil {
		return
	}
	ringVisible := visible && view.workspaceActive && !ring.Off && ring.Width > 0
	width := max(1, int(math.Ceil(ring.Width)))
	color := ring.InactiveColor
	if c.activeOutputName == c.outputNameForView(view) {
		color = ring.ActiveColor
	}
	rgba := premultipliedColor(color)
	outerRadii := view.cornerRadii
	for i := range outerRadii {
		outerRadii[i] += float64(width)
	}
	outerRadii = fitCornerRadii(outerRadii, float64(visualWidth+2*width), float64(visualHeight+2*width))
	sceneFXRectUpdateRing(view.focusRingNode, x-width, y-width,
		visualWidth+2*width, visualHeight+2*width, width,
		sceneFXRadii(outerRadii), sceneFXRadii(view.cornerRadii), rgba)
	sceneFXSetNodeEnabled(view.focusRingNode, ringVisible)
}

func (c *Compositor) syncFocusRings() {
	for _, view := range c.views {
		c.syncViewDecorations(view)
	}
}

func (c *Compositor) setViewAppearanceEnabled(view *toplevelView, enabled bool) {
	if view == nil {
		return
	}
	sceneFXSetNodeEnabled(view.shadowNode, enabled)
	sceneFXSetNodeEnabled(view.focusRingNode, enabled)
	if enabled {
		c.syncViewDecorations(view)
	}
}

func (c *Compositor) destroyViewAppearance(view *toplevelView) {
	if view == nil {
		return
	}
	if view.shadowNode != nil {
		sceneFXNodeDestroy(view.shadowNode)
		view.shadowNode = nil
	}
	if view.focusRingNode != nil {
		sceneFXNodeDestroy(view.focusRingNode)
		view.focusRingNode = nil
	}
}
