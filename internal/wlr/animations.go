package wlr

import (
	"math"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/animation"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

const animationFrame = 16 * time.Millisecond

type animationKey struct {
	owner    uint64
	property string
}

type runningAnimation struct {
	spring   animation.Spring
	easing   *animation.Easing
	slowdown float64
	target   float64
	started  time.Time
	set      func(float64)
	done     func()
}

type activeWorkspace struct {
	id    uint64
	index int
}

func (c *Compositor) animationSlowdown() float64 {
	if c.animationConfig.Slowdown == nil {
		return 1
	}
	return math.Max(.001, *c.animationConfig.Slowdown)
}

func (a *runningAnimation) rate() float64 {
	if a.slowdown <= 0 {
		return 1
	}
	return a.slowdown
}

func (a *runningAnimation) duration() time.Duration {
	duration := a.spring.Duration()
	if a.easing != nil {
		duration = a.easing.Duration()
	}
	return time.Duration(math.Min(float64(math.MaxInt64), float64(duration)*a.rate()))
}

func (a *runningAnimation) valueAt(elapsed time.Duration) float64 {
	elapsed = time.Duration(float64(elapsed) / a.rate())
	if a.easing != nil {
		return a.easing.ValueAt(elapsed)
	}
	return a.spring.ValueAt(elapsed)
}

func (a *runningAnimation) velocityAt(elapsed time.Duration) float64 {
	elapsed = time.Duration(float64(elapsed) / a.rate())
	if a.easing != nil {
		return a.easing.VelocityAt(elapsed) / a.rate()
	}
	return a.spring.VelocityAt(elapsed) / a.rate()
}

func (c *Compositor) scheduleAnimationTick() {
	if c.animationTimer != nil || len(c.animations) == 0 && !c.bobbing() {
		return
	}
	c.animationTimer = time.AfterFunc(animationFrame, func() {
		_ = c.Post(func() { c.tickAnimations(time.Now()) })
	})
}

func (c *Compositor) tickAnimations(now time.Time) {
	c.animationTimer = nil
	for key, current := range c.animations {
		if c.animations[key] != current {
			continue
		}
		elapsed := now.Sub(current.started)
		if elapsed >= current.duration() {
			delete(c.animations, key)
			current.set(current.valueAt(elapsed))
			if current.done != nil {
				current.done()
			}
			continue
		}
		current.set(current.valueAt(elapsed))
	}
	c.syncBobbing(now)
	if c.switcher.dirty {
		c.syncSwitcher()
	}
	// Drawn tabs follow their column while it moves.
	if len(c.tabIndicatorHits) > 0 {
		c.syncTabIndicators()
	}
	c.damageCaptureSources()
	c.scheduleAnimationTick()
}

func (c *Compositor) animationValue(key animationKey, fallback float64, now time.Time) (float64, float64, bool) {
	current := c.animations[key]
	if current == nil {
		return fallback, 0, false
	}
	elapsed := max(time.Duration(0), now.Sub(current.started))
	return current.valueAt(elapsed), current.velocityAt(elapsed), true
}

func (c *Compositor) hasAnimation(key animationKey) bool {
	return c.animations[key] != nil
}

func (c *Compositor) cancelAnimation(key animationKey) {
	delete(c.animations, key)
	c.finishAnimationShaderEffect(key)
}

// offsetAnimation moves a running animation's ends by delta, as Niri's
// Animation::offset: the same motion, shifted.
func (c *Compositor) offsetAnimation(key animationKey, delta float64) {
	if current := c.animations[key]; current != nil {
		current.spring = current.spring.Offset(delta)
		if current.easing != nil {
			offset := current.easing.Offset(delta)
			current.easing = &offset
		}
		current.target += delta
	}
}

func (c *Compositor) cancelViewAnimations(view *toplevelView) {
	if view == nil {
		return
	}
	for key := range c.animations {
		if key.owner == view.id {
			delete(c.animations, key)
			c.finishAnimationShaderEffect(key)
		}
	}
}

// animateValue samples an in-flight spring before retargeting, so a changing
// layout continues from the scene's current position and velocity.
func (c *Compositor) animateValue(key animationKey, kind config.Animation,
	from, to, velocity float64, set func(float64), done func()) {
	now := time.Now()
	if current := c.animations[key]; current != nil {
		if current.target == to {
			if current.done == nil {
				current.done = done
			}
			return
		}
		elapsed := max(time.Duration(0), now.Sub(current.started))
		from = current.valueAt(elapsed)
		velocity = current.velocityAt(elapsed)
		delete(c.animations, key)
	}
	if c.animationConfig.Off || kind.Off || (kind.Spring == nil && kind.Easing == nil) {
		set(to)
		if done != nil {
			done()
		}
		return
	}
	current := &runningAnimation{target: to, started: now, set: set, done: done, slowdown: c.animationSlowdown()}
	var err error
	if kind.Easing != nil {
		e := kind.Easing
		easing, failure := animation.NewEasing(from, to, time.Duration(e.DurationMS)*time.Millisecond, e.Curve, e.CubicBezier)
		current.easing, err = &easing, failure
	} else {
		spring := kind.Spring
		current.spring, err = animation.NewSpring(from, to, velocity*current.slowdown, spring.DampingRatio, float64(spring.Stiffness), spring.Epsilon)
	}
	if err != nil || current.duration() == 0 {
		set(to)
		if done != nil {
			done()
		}
		return
	}
	set(from)
	c.animations[key] = current
	c.scheduleAnimationTick()
}

func (c *Compositor) animateOverviewProgress(from, to, velocity float64) {
	key := animationKey{property: "overview"}
	c.animateValue(key, c.animationConfig.OverviewOpenClose, from, to, velocity, func(value float64) {
		c.model.SetOverviewAnimationProgress(value)
		c.syncOverviewScene()
	}, nil)
}

func (c *Compositor) animateViewPosition(view *toplevelView, x, y int, kind config.Animation, done func()) {
	if view == nil || view.tree == nil {
		return
	}
	currentX, currentY := view.layoutPosition()
	xKey := animationKey{owner: view.id, property: "x"}
	yKey := animationKey{owner: view.id, property: "y"}
	if kind.Off || (kind.Spring == nil && kind.Easing == nil) || c.animationConfig.Off {
		c.setViewScenePosition(view, x, y)
		if done != nil {
			done()
		}
		return
	}
	if currentX == x && !c.hasAnimation(xKey) && currentY == y && !c.hasAnimation(yKey) {
		if done != nil {
			done()
		}
		return
	}
	finish := func() {
		if !c.hasAnimation(xKey) && !c.hasAnimation(yKey) && done != nil {
			done()
		}
	}
	if currentX != x || c.hasAnimation(xKey) {
		c.animateValue(xKey, kind, float64(currentX), float64(x), 0, func(value float64) {
			_, py := view.layoutPosition()
			c.setViewScenePosition(view, int(math.Round(value)), py)
		}, finish)
	}
	if currentY != y || c.hasAnimation(yKey) {
		c.animateValue(yKey, kind, float64(currentY), float64(y), 0, func(value float64) {
			px, _ := view.layoutPosition()
			c.setViewScenePosition(view, px, int(math.Round(value)))
		}, finish)
	}
	if !c.hasAnimation(xKey) && !c.hasAnimation(yKey) && done != nil {
		done()
	}
}

// setViewScenePosition puts the view where the layout says, plus its
// baba-is-float offset.
func (c *Compositor) setViewScenePosition(view *toplevelView, x, y int) {
	if view == nil || view.tree == nil {
		return
	}
	y += view.bobY
	sceneFXNodeSetPosition(unsafe.Pointer(&view.tree.node), x, y)
	if view.resizeSnapshot != nil {
		sceneFXTreeSetPosition(view.resizeSnapshot, x, y)
	}
	c.syncViewDecorations(view)
	c.syncAnimationShaderPosition(view)
}

func (c *Compositor) setViewSceneEnabled(view *toplevelView, enabled bool) {
	if view == nil || view.tree == nil {
		return
	}
	if view.resizeSnapshot != nil {
		sceneFXSetNodeEnabled(view.resizeSnapshot, enabled)
	}
	sceneFXSetNodeEnabled(view.sceneNode(), enabled)
	if c.animationShaders != nil {
		for _, property := range []string{"open-opacity", "resize-shader"} {
			if effect := c.animationShaders.effects[animationKey{owner: view.id, property: property}]; effect != nil {
				sceneFXSetNodeEnabled(unsafe.Pointer(&effect.node.node), enabled)
			}
		}
	}
}

// syncViewOpacity draws the live window at its presentation opacity, or hides
// it while a resize snapshot stands in for it. It hides by opacity rather than
// by disabling the node, so the window stays hit-testable and keeps getting
// frame callbacks during the resize spring.
func (c *Compositor) syncViewOpacity(view *toplevelView) {
	ruleOpacity := 1.0
	if !view.fullscreen && !view.ignoreRuleOpacity && view.appearance.Opacity != nil {
		ruleOpacity = clampOpacity(*view.appearance.Opacity)
	}
	opacity := clampOpacity(view.presentationOpacity) * ruleOpacity
	if view.resizeSnapshot != nil || c.animationShaderOwnsView(view) {
		opacity = 0
	}
	sceneFXSetTreeOpacity(view.sceneNode(), opacity)
	if view.toplevel != nil {
		c.syncPopupOpacity(view.toplevel.base.surface, opacity, view.appearance.Popups.Opacity)
	}
}

func (c *Compositor) startWindowOpen(view *toplevelView) {
	if view == nil || view.openAnimationStarted {
		return
	}
	view.openAnimationStarted = true
	if c.animationConfig.Off || c.animationConfig.WindowOpen.Off || (c.animationConfig.WindowOpen.Spring == nil && c.animationConfig.WindowOpen.Easing == nil) {
		view.presentationOpacity = 1
		view.openAnimationFinished = true
		return
	}
	view.openAnimationFinished = false
	view.presentationOpacity = 0
	key := animationKey{owner: view.id, property: "open-opacity"}
	c.beginWindowOpenShader(view, key)
	c.animateValue(key, c.animationConfig.WindowOpen, 0, 1, 0, func(value float64) {
		progress := value
		value = clampOpacity(value)
		view.presentationOpacity = value
		c.updateWindowOpenShader(view, key, progress)
		c.syncViewOpacity(view)
	}, func() {
		c.finishAnimationShaderEffect(key)
		view.openAnimationFinished = true
		if view.mapped && view.presentationVisible {
			c.applyWorkspaceModel()
		}
	})
}

func clampOpacity(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func closeSnapshotKey(id uint64) animationKey {
	return animationKey{owner: id, property: "close-opacity"}
}

func (c *Compositor) startCloseSnapshot(id uint64, node unsafe.Pointer) {
	if node == nil {
		return
	}
	if c.animationConfig.Off || c.animationConfig.WindowClose.Off || (c.animationConfig.WindowClose.Spring == nil && c.animationConfig.WindowClose.Easing == nil) {
		sceneFXNodeDestroy(node)
		delete(c.closingNodes, id)
		return
	}
	sceneFXSetNodeEnabled(node, true)
	key := closeSnapshotKey(id)
	c.beginCloseShader(key, node)
	opacity := 1.0
	c.animateValue(key, c.animationConfig.WindowClose, 1, 0, 0, func(value float64) {
		progress := 1 - value
		value = clampOpacity(value)
		if !c.updateCloseShader(key, progress) {
			sceneFXSetNodeEnabled(node, true)
			if opacity > 0 {
				sceneFXScaleTreeOpacity(node, value/opacity)
			}
		}
		opacity = value
	}, func() {
		c.finishAnimationShaderEffect(key)
		sceneFXNodeDestroy(node)
		delete(c.closingNodes, id)
	})
}

func (c *Compositor) nextAnimationOwner() uint64 {
	c.nextAnimationID++
	return c.nextAnimationID
}

func (c *Compositor) resizeKeys(view *toplevelView) (animationKey, animationKey) {
	return animationKey{owner: view.id, property: "resize-width"},
		animationKey{owner: view.id, property: "resize-height"}
}

func (c *Compositor) cancelResizeAnimation(view *toplevelView) {
	if view == nil {
		return
	}
	w, h := c.resizeKeys(view)
	c.cancelAnimation(animationKey{owner: view.id, property: "resize-shader"})
	c.cancelAnimation(w)
	c.cancelAnimation(h)
	if view.resizeSnapshot != nil {
		sceneFXNodeDestroy(view.resizeSnapshot)
		view.resizeSnapshot = nil
	}
	view.resizeTargetWidth, view.resizeTargetHeight = 0, 0
	if view.tree != nil {
		sceneFXSetNodeEnabled(view.sceneNode(), view.presentationVisible)
		c.syncViewOpacity(view)
	}
}

func (c *Compositor) animateViewSize(view *toplevelView, width, height int, kind config.Animation) {
	if view == nil || view.tree == nil || view.fullscreen || width <= 0 || height <= 0 {
		return
	}
	geometry := view.geometry()
	if view.resizeSnapshot == nil {
		if float64(width) == float64(geometry.w) && float64(height) == float64(geometry.h) {
			return
		}
		if c.animationConfig.Off || kind.Off || (kind.Spring == nil && kind.Easing == nil) {
			return
		}
		var count int
		c.rememberCaptureNodes(view)
		view.resizeSnapshot, count = sceneFXCloneView(unsafe.Pointer(c.windowAnimationTree), unsafe.Pointer(view.tree))
		if count == 0 || view.resizeSnapshot == nil {
			view.resizeSnapshot = nil
			return
		}
		c.rememberCaptureTree(view.resizeSnapshot, view.captureIdentity)
		x, y := view.treePosition()
		sceneFXTreeSetPosition(view.resizeSnapshot, x, y)
		view.resizeBaseWidth, view.resizeBaseHeight = max(1, geometry.w), max(1, geometry.h)
		view.resizeWidth, view.resizeHeight = float64(view.resizeBaseWidth), float64(view.resizeBaseHeight)
		view.resizeScaleX, view.resizeScaleY = 1, 1
		c.syncViewOpacity(view)
	}
	if view.resizeTargetWidth == width && view.resizeTargetHeight == height && c.hasAnimation(animationKey{owner: view.id, property: "resize-shader"}) {
		return
	}
	view.resizeTargetWidth, view.resizeTargetHeight = width, height
	if c.beginResizeShader(view, kind) {
		return
	}
	wKey, hKey := c.resizeKeys(view)
	if view.resizeWidth != float64(width) || c.hasAnimation(wKey) {
		c.animateValue(wKey, kind, view.resizeWidth, float64(width), 0, func(value float64) {
			view.resizeWidth = value
			c.syncResizeScale(view)
		}, func() { c.finishResizeIfSettled(view) })
	}
	if view.resizeHeight != float64(height) || c.hasAnimation(hKey) {
		c.animateValue(hKey, kind, view.resizeHeight, float64(height), 0, func(value float64) {
			view.resizeHeight = value
			c.syncResizeScale(view)
		}, func() { c.finishResizeIfSettled(view) })
	}
	if !c.hasAnimation(wKey) && !c.hasAnimation(hKey) {
		c.finishResizeIfSettled(view)
	}
}

func (c *Compositor) syncResizeScale(view *toplevelView) {
	if view == nil || view.resizeSnapshot == nil {
		return
	}
	x := max(0.01, view.resizeWidth/float64(view.resizeBaseWidth))
	y := max(0.01, view.resizeHeight/float64(view.resizeBaseHeight))
	sceneFXScaleTree(view.resizeSnapshot, view.resizeScaleX, view.resizeScaleY, x, y)
	view.resizeScaleX, view.resizeScaleY = x, y
	c.syncViewDecorations(view)
}

func (c *Compositor) finishResizeIfSettled(view *toplevelView) {
	if view == nil || view.resizeSnapshot == nil {
		return
	}
	wKey, hKey := c.resizeKeys(view)
	if c.hasAnimation(wKey) || c.hasAnimation(hKey) || c.hasAnimation(animationKey{owner: view.id, property: "resize-shader"}) {
		return
	}
	sceneFXNodeDestroy(view.resizeSnapshot)
	view.resizeSnapshot = nil
	view.resizeTargetWidth, view.resizeTargetHeight = 0, 0
	sceneFXSetNodeEnabled(view.sceneNode(), view.presentationVisible)
	c.syncViewOpacity(view)
}
