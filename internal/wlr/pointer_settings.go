package wlr

import (
	"math"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

type deviceCaps struct {
	TapFingers                  int
	HasAccel                    bool
	HasNatural                  bool
	HasLeftHanded               bool
	HasDWT                      bool
	HasDWTP                     bool
	HasMiddleEmulation          bool
	ScrollMethods, ClickMethods uint32
}
type pointerSetting struct {
	Name  string
	Value any
}

// planPointerSettings follows Niri 26.04's apply_libinput_settings order.
// Removing a section restores device defaults instead of retaining old values.
func planPointerSettings(kind pointerKind, cfg config.PointerDevice, caps deviceCaps) []pointerSetting {
	var settings []pointerSetting
	add := func(name string, value any) {
		if !cfg.Set {
			value = "default"
		}
		settings = append(settings, pointerSetting{name, value})
	}
	optional := func(value string) any {
		if value == "" {
			return "default"
		}
		return value
	}
	mode := "enabled"
	if cfg.Off {
		mode = "disabled"
	} else if kind == kindTouchpad && cfg.DisabledOnExternalMouse {
		mode = "disabled-on-external-mouse"
	}
	add("send_events_set_mode", mode)
	if kind == kindTouchpad && caps.TapFingers > 0 {
		add("tap_set_enabled", cfg.Tap)
		if caps.HasDWT {
			add("dwt_set_enabled", cfg.DWT)
		}
		if caps.HasDWTP {
			add("dwtp_set_enabled", cfg.DWTP)
		}
		add("tap_set_drag_lock_enabled", cfg.DragLock)
	}
	if caps.HasNatural {
		add("scroll_set_natural_scroll_enabled", cfg.NaturalScroll)
	}
	if caps.HasAccel {
		add("accel_set_speed", cfg.AccelSpeed)
	}
	left := func() {
		if caps.HasLeftHanded {
			add("left_handed_set", cfg.LeftHanded)
		}
	}
	middle := func() {
		if caps.HasMiddleEmulation {
			add("middle_emulation_set_enabled", cfg.MiddleEmulation)
		}
	}
	if kind == kindTrackball {
		middle()
		left()
	} else {
		left()
		middle()
	}
	if kind == kindTouchpad && caps.TapFingers > 0 {
		var drag any = "default"
		if cfg.Drag != nil {
			drag = *cfg.Drag
		}
		add("tap_set_drag_enabled", drag)
	}
	if caps.HasAccel {
		add("accel_set_profile", optional(cfg.AccelProfile))
	}
	methodMask := map[string]uint32{"no-scroll": 0, "two-finger": 1, "edge": 2, "on-button-down": 4}
	if caps.ScrollMethods != 0 && (!cfg.Set || cfg.ScrollMethod == "" || cfg.ScrollMethod == "no-scroll" || caps.ScrollMethods&methodMask[cfg.ScrollMethod] != 0) {
		add("scroll_set_method", optional(cfg.ScrollMethod))
		if caps.ScrollMethods&4 != 0 && (!cfg.Set || cfg.ScrollMethod == "" || cfg.ScrollMethod == "on-button-down") {
			var button any = "default"
			if cfg.ScrollButton != nil {
				button = *cfg.ScrollButton
			}
			add("scroll_set_button", button)
			add("scroll_set_button_lock", cfg.ScrollButtonLock)
		}
	}
	if kind == kindTouchpad && caps.TapFingers > 0 {
		add("tap_set_button_map", optional(cfg.TapButtonMap))
		clickMask := map[string]uint32{"button-areas": 1, "clickfinger": 2}
		if caps.ClickMethods != 0 && (!cfg.Set || cfg.ClickMethod == "" || caps.ClickMethods&clickMask[cfg.ClickMethod] != 0) {
			add("click_set_method", optional(cfg.ClickMethod))
		}
	}
	return settings
}

func scaleAxis(f config.ScrollFactor, horizontal bool, delta float64, v120 int32) (float64, int32) {
	factor := f.Vertical
	if horizontal {
		factor = f.Horizontal
	}
	scaled := math.Round(float64(v120) * factor)
	scaled = math.Max(-2147483648, math.Min(2147483647, scaled))
	return delta * factor, int32(scaled)
}

func swipeDeltas(natural bool, dx, dy float64) (float64, float64) {
	if natural {
		return -dx, -dy
	}
	return dx, dy
}
