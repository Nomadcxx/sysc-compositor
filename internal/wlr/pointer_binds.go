package wlr

import (
	"slices"
	"strings"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// Niri's bind triggers for scrolls and buttons, and its tick sizes: a wheel
// detent is 120 v120 units, and finger scrolls tick every 10 units
// (src/niri.rs ScrollTracker::new).
var (
	wheelTriggers  = []string{"WheelScrollUp", "WheelScrollDown", "WheelScrollLeft", "WheelScrollRight"}
	fingerTriggers = []string{"TouchpadScrollUp", "TouchpadScrollDown", "TouchpadScrollLeft", "TouchpadScrollRight"}
	mouseTriggers  = []string{"MouseLeft", "MouseRight", "MouseMiddle", "MouseBack", "MouseForward"}
)

const (
	wheelTick  = 120
	fingerTick = 10
)

// pointerBindState is the scroll accumulators, the per-bind cooldowns and the
// buttons whose press ran a bind, as Niri keeps them in State.
type pointerBindState struct {
	wheel, finger [2]scrollTracker // [vertical, horizontal]
	cooldowns     map[bind.Chord]time.Duration
	suppressed    map[uint32]bool
	// now is the clock cooldowns run on; nil is the monotonic clock. Tests
	// replace it.
	now func() time.Duration
}

// bindClockStart anchors the monotonic clock bind cooldowns are measured on.
var bindClockStart = time.Now()

// mouseTrigger is Niri's trigger for an evdev button code, or "".
func mouseTrigger(button uint32) string {
	switch button {
	case 0x110: // BTN_LEFT
		return "MouseLeft"
	case 0x111: // BTN_RIGHT
		return "MouseRight"
	case 0x112: // BTN_MIDDLE
		return "MouseMiddle"
	case 0x113: // BTN_SIDE
		return "MouseBack"
	case 0x114: // BTN_EXTRA
		return "MouseForward"
	}
	return ""
}

// hasTriggerBinds is Niri's mods_with_wheel_binds / _finger_scroll_binds /
// _mouse_binds membership: some bind on one of triggers uses exactly
// modifiers. Lock and inhibit do not matter here, as in Niri.
func (c *Compositor) hasTriggerBinds(triggers []string, modifiers bind.Modifiers) bool {
	for _, binding := range c.keyBindings(modifiers) {
		if binding.Chord.Modifiers == modifiers && slices.ContainsFunc(triggers, func(t string) bool {
			return strings.EqualFold(t, binding.Chord.Key)
		}) {
			return true
		}
	}
	return false
}

// matchTriggerBinding is the bind on trigger with modifiers that may run now.
func (c *Compositor) matchTriggerBinding(trigger string, modifiers bind.Modifiers) (bind.Binding, bool) {
	for _, binding := range c.keyBindings(modifiers) {
		if binding.Chord.Trigger == bind.TriggerKey || binding.Chord.Modifiers != modifiers ||
			!strings.EqualFold(binding.Chord.Key, trigger) {
			continue
		}
		if c.sessionLocked && !binding.AllowWhenLocked && !allowedWhenLocked(binding) {
			continue
		}
		if !binding.IgnoresInhibit && c.shortcutsInhibited() {
			continue
		}
		return binding, true
	}
	return bind.Binding{}, false
}

// bindCoolingDown reports whether binding ran less than its cooldown ago, and
// otherwise starts a new cooldown, as Niri's handle_bind. Like Niri's timer,
// it runs on the compositor's monotonic clock, not on event timestamps, which
// a virtual device may stamp as it likes.
func (c *Compositor) bindCoolingDown(binding bind.Binding) bool {
	if binding.Cooldown <= 0 {
		return false
	}
	now := time.Since(bindClockStart)
	if c.pointerBinds.now != nil {
		now = c.pointerBinds.now()
	}
	if last, ok := c.pointerBinds.cooldowns[binding.Chord]; ok && now-last < binding.Cooldown {
		return true
	}
	if c.pointerBinds.cooldowns == nil {
		c.pointerBinds.cooldowns = map[bind.Chord]time.Duration{}
	}
	c.pointerBinds.cooldowns[binding.Chord] = now
	return false
}

func (c *Compositor) runTriggerBinding(trigger string, modifiers bind.Modifiers) {
	binding, ok := c.matchTriggerBinding(trigger, modifiers)
	if ok && !c.bindCoolingDown(binding) {
		c.executeKeyBinding(binding)
	}
}

// overviewWheelCooldown is the cooldown of Niri's built-in overview wheel
// binds that switch workspaces or, with Shift, columns.
const overviewWheelCooldown = 50 * time.Millisecond

// overviewWheelBind is Niri's built-in overview bind for a wheel trigger with
// modifiers, if there is one: with no modifiers the wheel switches the
// workspace of the output under the pointer and a horizontal wheel moves the
// column focus of the workspace under it; with Shift a vertical wheel moves
// the column focus. Its chord carries no modifiers, as Niri's does, so Shift
// and plain wheels share a cooldown.
func (c *Compositor) overviewWheelBind(trigger string, modifiers bind.Modifiers) (bind.Binding, func(*wm.Model) bool, bool) {
	x, y := float64(c.cursor.x), float64(c.cursor.y)
	output := c.outputNameAt(x, y)
	workspace := c.workspaceUnderGesture(output)
	workspaceDown := func(m *wm.Model) bool { return m.FocusWorkspaceDownOn(output) }
	workspaceUp := func(m *wm.Model) bool { return m.FocusWorkspaceUpOn(output) }
	columnRight := func(m *wm.Model) bool { return m.FocusColumnRightOnWorkspace(workspace) }
	columnLeft := func(m *wm.Model) bool { return m.FocusColumnLeftOnWorkspace(workspace) }
	var action func(*wm.Model) bool
	var cooldown time.Duration
	switch {
	case modifiers == 0 && trigger == "WheelScrollDown":
		action, cooldown = workspaceDown, overviewWheelCooldown
	case modifiers == 0 && trigger == "WheelScrollUp":
		action, cooldown = workspaceUp, overviewWheelCooldown
	case modifiers == 0 && trigger == "WheelScrollRight":
		action = columnRight
	case modifiers == 0 && trigger == "WheelScrollLeft":
		action = columnLeft
	case modifiers == bind.Modifiers(bind.ModifierShift) && trigger == "WheelScrollDown":
		action, cooldown = columnRight, overviewWheelCooldown
	case modifiers == bind.Modifiers(bind.ModifierShift) && trigger == "WheelScrollUp":
		action, cooldown = columnLeft, overviewWheelCooldown
	default:
		return bind.Binding{}, nil, false
	}
	binding := bind.Binding{Chord: bind.Chord{Key: trigger, Trigger: bind.TriggerScroll}, Cooldown: cooldown}
	return binding, action, true
}

// scrollBinds is Niri's on_pointer_axis bind branch. A wheel or finger
// scroll is the binds' when some bind of that kind uses the held modifiers:
// it accumulates into ticks, runs the bind once per tick and never reaches
// the client. Otherwise the accumulators reset and the scroll goes on. In
// the overview every wheel is the compositor's unless the pointer is on a
// layer surface, and the built-in overview binds come before configured
// ones.
func (c *Compositor) scrollBinds(wheel, finger, horizontal bool, delta float64, v120 int32) bool {
	state := &c.pointerBinds
	trackers, triggers, tick, amount := &state.wheel, wheelTriggers, float64(wheelTick), float64(v120)
	switch {
	case wheel:
	case finger:
		trackers, triggers, tick, amount = &state.finger, fingerTriggers, fingerTick, delta
	default:
		return false
	}
	modifiers := c.activeBindModifiers()
	inOverview := wheel && c.model != nil && c.model.OverviewOpen() && !c.pointerOnLayerSurface()
	if !inOverview && !c.hasTriggerBinds(triggers, modifiers) {
		trackers[0].reset()
		trackers[1].reset()
		return false
	}
	axis, back, forward := 0, triggers[0], triggers[1]
	if horizontal {
		axis, back, forward = 1, triggers[2], triggers[3]
	}
	if trackers[axis].tick == 0 {
		trackers[axis] = newScrollTracker(tick)
	}
	run := func(trigger string) {
		if inOverview {
			if binding, action, ok := c.overviewWheelBind(trigger, modifiers); ok {
				if !c.bindCoolingDown(binding) {
					c.applyWorkspaceFocusAction(action)
				}
				return
			}
		}
		c.runTriggerBinding(trigger, modifiers)
	}
	ticks := trackers[axis].accumulate(amount)
	for ; ticks > 0; ticks-- {
		run(forward)
	}
	for ; ticks < 0; ticks++ {
		run(back)
	}
	return true
}

// mouseBinds is Niri's on_pointer_button bind branch: a press that runs a
// bind is the bind's, and so is its release.
func (c *Compositor) mouseBinds(button uint32, pressed bool) bool {
	state := &c.pointerBinds
	if !pressed {
		if state.suppressed[button] {
			delete(state.suppressed, button)
			return true
		}
		return false
	}
	trigger := mouseTrigger(button)
	modifiers := c.activeBindModifiers()
	if trigger == "" || !c.hasTriggerBinds(mouseTriggers, modifiers) {
		return false
	}
	binding, ok := c.matchTriggerBinding(trigger, modifiers)
	if !ok {
		return false
	}
	if state.suppressed == nil {
		state.suppressed = map[uint32]bool{}
	}
	state.suppressed[button] = true
	if !c.bindCoolingDown(binding) {
		c.executeKeyBinding(binding)
	}
	return true
}
