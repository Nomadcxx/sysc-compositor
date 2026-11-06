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

// shadowRenderStyle maps Niri's shadow onto a SceneFX shadow node. Niri
// blurs the tile grown by spread with sigma softness/2 (layout/shadow.rs).
// SceneFX blurs the node inset by its blur_sigma with sigma blur_sigma/2, so
// blur_sigma is the softness and the node is the shadow box grown by it.
type shadowRenderStyle struct {
	// inset is how far the node's edge lies outside the tile's.
	inset     int
	radius    int
	blurSigma float64
	color     [4]float32
}

func makeShadowRenderStyle(shadow config.Shadow, radii config.CornerRadius, active bool) shadowRenderStyle {
	spread := int(math.Ceil(math.Abs(shadow.Spread)))
	if shadow.Spread < 0 {
		spread = -spread
	}
	softness := max(0, shadow.Softness)
	radius := shadowCornerRadius(radii, spread)
	color := shadow.Color
	switch {
	case active:
	case shadow.InactiveColor != nil:
		color = *shadow.InactiveColor
	default:
		color.A *= 0.75
	}
	// ponytail: SceneFX shadow geometry rounds to logical pixels; M15 mixed-DPI work can align it to physical pixels.
	return shadowRenderStyle{inset: spread + int(math.Ceil(softness)), radius: radius,
		blurSigma: softness, color: premultipliedColor(color)}
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

// viewDecorations is a window's focus ring, border and shadow after its
// workspace layout's parts and its window rules' parts are laid over the
// layout's, in Niri's order.
type viewDecorations struct {
	ring, border config.FocusRing
	shadow       config.Shadow
	// withBackground fills the window's rectangle under it with the border's
	// colour, or the focus ring's when the border is off.
	withBackground bool
}

// resolveViewDecorations resolves the window's decorations and reports whether
// its border width changed, which resizes the window inside its tile.
func (c *Compositor) resolveViewDecorations(view *toplevelView, workspaceName *string) bool {
	var workspace config.Decorations
	if workspaceName != nil {
		workspace = c.layoutOptions.WorkspaceDecorations[config.WorkspaceKey(*workspaceName)]
	}
	window := view.appearance.Decorations
	before := view.borderWidth()
	view.decorations = viewDecorations{
		ring:           c.layoutOptions.FocusRing.Merged(workspace.FocusRing).Merged(window.FocusRing),
		border:         c.layoutOptions.Border.Merged(workspace.Border).Merged(window.Border),
		shadow:         c.layoutOptions.Shadow.Merged(workspace.Shadow).Merged(window.Shadow),
		withBackground: !c.serverSideDecorated(view),
	}
	if value := view.appearance.DrawBorderWithBackground; value != nil {
		view.decorations.withBackground = *value
	}
	return view.borderWidth() != before
}

// borderWidth is the border the window's tile holds on each side. Niri draws
// none in fullscreen or maximized to the edges.
func (v *toplevelView) borderWidth() int {
	if border := v.decorations.border; !border.Off && !v.fullscreen && !v.maximized {
		return decorationWidth(border.Width)
	}
	return 0
}

// decorationWidth rounds a nonzero width to whole pixels, at least one.
// ponytail: rounds logical pixels where Niri rounds physical ones; they differ
// only at a fractional scale.
func decorationWidth(width float64) int {
	if width <= 0 {
		return 0
	}
	return max(1, int(math.Round(width)))
}

// decorationColor is Niri's choice: urgent, then active, then inactive.
func decorationColor(decoration config.FocusRing, active, urgent bool) config.Color {
	switch {
	case urgent:
		return decoration.UrgentColor
	case active:
		return decoration.ActiveColor
	}
	return decoration.InactiveColor
}

func expandRadii(radii config.CornerRadius, by int) config.CornerRadius {
	for i := range radii {
		radii[i] += float64(by)
	}
	return radii
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
	c.syncViewOpacity(view)

	decorations := view.decorations
	if decorations.shadow.On && view.shadowNode == nil {
		color := premultipliedColor(decorations.shadow.Color)
		view.shadowNode = sceneFXCreateShadow(unsafe.Pointer(c.windowShadowTree),
			max(1, geometry.w), max(1, geometry.h), 0, decorations.shadow.Softness, color)
	}
	ringOn := !decorations.ring.Off && decorations.ring.Width > 0
	if ringOn && view.focusRingNode == nil {
		view.focusRingNode = sceneFXCreateRect(unsafe.Pointer(c.windowFocusTree), 1, 1, [4]float32{})
	}
	borderOn := !decorations.border.Off && decorations.border.Width > 0
	if (borderOn || ringOn && decorations.withBackground) && view.borderNode == nil {
		view.borderNode = sceneFXCreateRect(unsafe.Pointer(c.windowBorderTree), 1, 1, [4]float32{})
		sceneFXRectIgnoreInput(view.borderNode)
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
	decorations := view.decorations
	// Niri's is_active: the active window of a workspace on the focused output.
	active := view.workspaceActive && c.activeOutputName == c.outputNameForView(view)

	// The tile is the window plus the border on each side.
	border := view.borderWidth()
	tileX, tileY := x-border, y-border
	tileWidth, tileHeight := visualWidth+2*border, visualHeight+2*border
	tileRadii := view.cornerRadii
	if border > 0 {
		tileRadii = fitCornerRadii(expandRadii(view.cornerRadii, border), float64(tileWidth), float64(tileHeight))
	}

	if view.shadowNode != nil {
		shadow := decorations.shadow
		style := makeShadowRenderStyle(shadow, tileRadii,
			view == c.focusedView && c.activeOutputName == c.outputNameForView(view))
		inset := style.inset
		offsetX, offsetY := int(math.Round(shadow.OffsetX)), int(math.Round(shadow.OffsetY))
		width, height := max(1, tileWidth+2*inset), max(1, tileHeight+2*inset)
		sceneFXShadowUpdate(view.shadowNode, tileX-inset+offsetX, tileY-inset+offsetY,
			width, height, style.radius, style.blurSigma, style.color)
		// Niri cuts the shadow out from under the tile unless it is drawn behind.
		if shadow.DrawBehindWindow {
			sceneFXShadowClip(view.shadowNode, 0, 0, 0, 0, [4]uint16{})
		} else {
			sceneFXShadowClip(view.shadowNode, inset-offsetX, inset-offsetY,
				tileWidth, tileHeight, sceneFXRadii(tileRadii))
		}
		sceneFXSetNodeEnabled(view.shadowNode, visible && shadow.On)
	}

	ring := decorations.ring
	ringVisible := visible && view.workspaceActive && !ring.Off && ring.Width > 0
	if view.focusRingNode != nil {
		width := decorationWidth(ring.Width)
		rgba := premultipliedColor(decorationColor(ring, active, view.urgent))
		outerRadii := fitCornerRadii(expandRadii(tileRadii, width),
			float64(tileWidth+2*width), float64(tileHeight+2*width))
		sceneFXRectUpdateRing(view.focusRingNode, tileX-width, tileY-width,
			tileWidth+2*width, tileHeight+2*width, width,
			sceneFXRadii(outerRadii), sceneFXRadii(tileRadii), rgba)
		sceneFXSetNodeEnabled(view.focusRingNode, ringVisible)
	}

	if view.borderNode != nil {
		switch {
		case border > 0:
			rgba := premultipliedColor(decorationColor(decorations.border, active, view.urgent))
			if decorations.withBackground {
				sceneFXRectUpdateFill(view.borderNode, tileX, tileY, tileWidth, tileHeight,
					sceneFXRadii(tileRadii), rgba)
			} else {
				sceneFXRectUpdateRing(view.borderNode, tileX, tileY, tileWidth, tileHeight, border,
					sceneFXRadii(tileRadii), sceneFXRadii(view.cornerRadii), rgba)
			}
			sceneFXSetNodeEnabled(view.borderNode, visible)
		case ringVisible && decorations.border.Off && decorations.withBackground:
			// With the border off, the focus ring's background fills the window.
			rgba := premultipliedColor(decorationColor(ring, active, view.urgent))
			sceneFXRectUpdateFill(view.borderNode, x, y, visualWidth, visualHeight,
				sceneFXRadii(view.cornerRadii), rgba)
			sceneFXSetNodeEnabled(view.borderNode, true)
		default:
			sceneFXSetNodeEnabled(view.borderNode, false)
		}
	}
}

func (c *Compositor) syncFocusRings() {
	for _, view := range c.views {
		c.syncViewDecorations(view)
	}
	c.syncTabIndicators()
}

func (c *Compositor) setViewAppearanceEnabled(view *toplevelView, enabled bool) {
	if view == nil {
		return
	}
	sceneFXSetNodeEnabled(view.shadowNode, enabled)
	sceneFXSetNodeEnabled(view.focusRingNode, enabled)
	sceneFXSetNodeEnabled(view.borderNode, enabled)
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
	if view.borderNode != nil {
		sceneFXNodeDestroy(view.borderNode)
		view.borderNode = nil
	}
}
