package wlr

import (
	"slices"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
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
	cooldowns     map[bind.Chord]uint32
	suppressed    map[uint32]bool
}

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

// bindCoolingDown reports whether binding ran less than its cooldown before
// timeMsec, and otherwise starts a new cooldown, as Niri's handle_bind.
// Times are one input source's milliseconds, so each chord compares like
// with like.
func (c *Compositor) bindCoolingDown(binding bind.Binding, timeMsec uint32) bool {
	if binding.Cooldown <= 0 {
		return false
	}
	if last, ok := c.pointerBinds.cooldowns[binding.Chord]; ok && timeMsec-last < uint32(binding.Cooldown.Milliseconds()) {
		return true
	}
	if c.pointerBinds.cooldowns == nil {
		c.pointerBinds.cooldowns = map[bind.Chord]uint32{}
	}
	c.pointerBinds.cooldowns[binding.Chord] = timeMsec
	return false
}

func (c *Compositor) runTriggerBinding(trigger string, modifiers bind.Modifiers, timeMsec uint32) {
	binding, ok := c.matchTriggerBinding(trigger, modifiers)
	if ok && !c.bindCoolingDown(binding, timeMsec) {
		c.executeKeyBinding(binding)
	}
}

// scrollBinds is Niri's on_pointer_axis bind branch. A wheel or finger
// scroll is the binds' when some bind of that kind uses the held modifiers:
// it accumulates into ticks, runs the bind once per tick and never reaches
// the client. Otherwise the accumulators reset and the scroll goes on.
func (c *Compositor) scrollBinds(wheel, finger, horizontal bool, delta float64, v120 int32, timeMsec uint32) bool {
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
	if !c.hasTriggerBinds(triggers, modifiers) {
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
	ticks := trackers[axis].accumulate(amount)
	for ; ticks > 0; ticks-- {
		c.runTriggerBinding(forward, modifiers, timeMsec)
	}
	for ; ticks < 0; ticks++ {
		c.runTriggerBinding(back, modifiers, timeMsec)
	}
	return true
}

// mouseBinds is Niri's on_pointer_button bind branch: a press that runs a
// bind is the bind's, and so is its release.
func (c *Compositor) mouseBinds(button uint32, pressed bool, timeMsec uint32) bool {
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
	if !c.bindCoolingDown(binding, timeMsec) {
		c.executeKeyBinding(binding)
	}
	return true
}
