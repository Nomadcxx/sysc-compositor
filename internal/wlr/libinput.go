package wlr

/*
#cgo pkg-config: libinput
#include <libinput.h>
#include <wlr/backend/libinput.h>
#include <wlr/types/wlr_input_device.h>
*/
import "C"

import (
	"log"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// All methods in this file run on the locked compositor thread, on device
// add or reload. Nested and virtual devices have no libinput handle.
func pointerLibinput(device unsafe.Pointer) *C.struct_libinput_device {
	if device == nil {
		return nil
	}
	input := (*C.struct_wlr_input_device)(device)
	if !bool(C.wlr_input_device_is_libinput(input)) {
		return nil
	}
	return C.wlr_libinput_get_device_handle(input)
}

func (c *Compositor) setPointerConfig(p config.Pointers) {
	c.pointers = p
	for device := range c.pointerDevices {
		c.applyPointerSettings((*C.struct_wlr_input_device)(device))
	}
}

func (c *Compositor) applyPointerSettings(device *C.struct_wlr_input_device) {
	key := unsafe.Pointer(device)
	if c.pointerKinds == nil {
		c.pointerKinds = map[unsafe.Pointer]pointerKind{}
	}
	handle := pointerLibinput(key)
	if handle == nil {
		c.pointerKinds[key] = kindMouse
		return
	}
	caps := deviceCaps{
		TapFingers:         int(C.libinput_device_config_tap_get_finger_count(handle)),
		HasAccel:           C.libinput_device_config_accel_is_available(handle) != 0,
		HasNatural:         C.libinput_device_config_scroll_has_natural_scroll(handle) != 0,
		HasLeftHanded:      C.libinput_device_config_left_handed_is_available(handle) != 0,
		HasDWT:             C.libinput_device_config_dwt_is_available(handle) != 0,
		HasDWTP:            C.libinput_device_config_dwtp_is_available(handle) != 0,
		HasMiddleEmulation: C.libinput_device_config_middle_emulation_is_available(handle) != 0,
		ScrollMethods:      uint32(C.libinput_device_config_scroll_get_methods(handle)),
		ClickMethods:       uint32(C.libinput_device_config_click_get_methods(handle)),
	}
	kind := classifyPointer(caps.TapFingers, udevProperties("/", C.GoString(C.libinput_device_get_sysname(handle))))
	c.pointerKinds[key] = kind
	cfg := c.pointers.Mouse
	switch kind {
	case kindTouchpad:
		cfg = c.pointers.Touchpad
	case kindTrackpoint:
		cfg = c.pointers.Trackpoint
	case kindTrackball:
		cfg = c.pointers.Trackball
	}
	for _, setting := range planPointerSettings(kind, cfg, caps) {
		status := applyLibinputSetting(handle, setting)
		if status != C.LIBINPUT_CONFIG_STATUS_SUCCESS && status != C.LIBINPUT_CONFIG_STATUS_UNSUPPORTED {
			log.Printf("sysc-compositor: input %s: %s: libinput status %d", C.GoString(C.libinput_device_get_sysname(handle)), setting.Name, status)
		}
	}
}

func (c *Compositor) touchpadNaturalScroll(device unsafe.Pointer) bool {
	if c.pointerKinds[device] != kindTouchpad {
		return false
	}
	handle := pointerLibinput(device)
	return handle != nil && C.libinput_device_config_scroll_get_natural_scroll_enabled(handle) != 0
}

func (c *Compositor) pointerScrollFactor(device unsafe.Pointer, finger bool) config.ScrollFactor {
	cfg := c.pointers.Mouse
	if finger && c.pointerKinds[device] == kindTouchpad {
		cfg = c.pointers.Touchpad
	}
	if !cfg.Set {
		return config.ScrollFactor{Vertical: 1, Horizontal: 1}
	}
	return cfg.ScrollFactor
}

// pointerSettingInt translates the plan's booleans and enum spellings. A
// default comes from the device's matching getter, including on reload.
func pointerSettingInt(value any, fallback int) int {
	switch v := value.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case uint32:
		return int(v)
	case string:
		switch v {
		case "enabled":
			return int(C.LIBINPUT_CONFIG_SEND_EVENTS_ENABLED)
		case "disabled":
			return int(C.LIBINPUT_CONFIG_SEND_EVENTS_DISABLED)
		case "disabled-on-external-mouse":
			return int(C.LIBINPUT_CONFIG_SEND_EVENTS_DISABLED_ON_EXTERNAL_MOUSE)
		case "adaptive":
			return int(C.LIBINPUT_CONFIG_ACCEL_PROFILE_ADAPTIVE)
		case "flat":
			return int(C.LIBINPUT_CONFIG_ACCEL_PROFILE_FLAT)
		case "no-scroll":
			return int(C.LIBINPUT_CONFIG_SCROLL_NO_SCROLL)
		case "two-finger":
			return int(C.LIBINPUT_CONFIG_SCROLL_2FG)
		case "edge":
			return int(C.LIBINPUT_CONFIG_SCROLL_EDGE)
		case "on-button-down":
			return int(C.LIBINPUT_CONFIG_SCROLL_ON_BUTTON_DOWN)
		case "button-areas":
			return int(C.LIBINPUT_CONFIG_CLICK_METHOD_BUTTON_AREAS)
		case "clickfinger":
			return int(C.LIBINPUT_CONFIG_CLICK_METHOD_CLICKFINGER)
		case "left-right-middle":
			return int(C.LIBINPUT_CONFIG_TAP_MAP_LRM)
		case "left-middle-right":
			return int(C.LIBINPUT_CONFIG_TAP_MAP_LMR)
		}
	}
	return fallback
}

func applyLibinputSetting(d *C.struct_libinput_device, s pointerSetting) C.enum_libinput_config_status {
	switch s.Name {
	case "send_events_set_mode":
		return C.libinput_device_config_send_events_set_mode(d, C.uint32_t(pointerSettingInt(s.Value, int(C.libinput_device_config_send_events_get_default_mode(d)))))
	case "tap_set_enabled":
		return C.libinput_device_config_tap_set_enabled(d, C.enum_libinput_config_tap_state(pointerSettingInt(s.Value, int(C.libinput_device_config_tap_get_default_enabled(d)))))
	case "dwt_set_enabled":
		return C.libinput_device_config_dwt_set_enabled(d, C.enum_libinput_config_dwt_state(pointerSettingInt(s.Value, int(C.libinput_device_config_dwt_get_default_enabled(d)))))
	case "dwtp_set_enabled":
		return C.libinput_device_config_dwtp_set_enabled(d, C.enum_libinput_config_dwtp_state(pointerSettingInt(s.Value, int(C.libinput_device_config_dwtp_get_default_enabled(d)))))
	case "tap_set_drag_lock_enabled":
		return C.libinput_device_config_tap_set_drag_lock_enabled(d, C.enum_libinput_config_drag_lock_state(pointerSettingInt(s.Value, int(C.libinput_device_config_tap_get_default_drag_lock_enabled(d)))))
	case "scroll_set_natural_scroll_enabled":
		return C.libinput_device_config_scroll_set_natural_scroll_enabled(d, C.int(pointerSettingInt(s.Value, int(C.libinput_device_config_scroll_get_default_natural_scroll_enabled(d)))))
	case "accel_set_speed":
		speed, ok := s.Value.(float64)
		if !ok {
			speed = float64(C.libinput_device_config_accel_get_default_speed(d))
		}
		return C.libinput_device_config_accel_set_speed(d, C.double(speed))
	case "left_handed_set":
		return C.libinput_device_config_left_handed_set(d, C.int(pointerSettingInt(s.Value, int(C.libinput_device_config_left_handed_get_default(d)))))
	case "middle_emulation_set_enabled":
		return C.libinput_device_config_middle_emulation_set_enabled(d, C.enum_libinput_config_middle_emulation_state(pointerSettingInt(s.Value, int(C.libinput_device_config_middle_emulation_get_default_enabled(d)))))
	case "tap_set_drag_enabled":
		return C.libinput_device_config_tap_set_drag_enabled(d, C.enum_libinput_config_drag_state(pointerSettingInt(s.Value, int(C.libinput_device_config_tap_get_default_drag_enabled(d)))))
	case "accel_set_profile":
		return C.libinput_device_config_accel_set_profile(d, C.enum_libinput_config_accel_profile(pointerSettingInt(s.Value, int(C.libinput_device_config_accel_get_default_profile(d)))))
	case "scroll_set_method":
		return C.libinput_device_config_scroll_set_method(d, C.enum_libinput_config_scroll_method(pointerSettingInt(s.Value, int(C.libinput_device_config_scroll_get_default_method(d)))))
	case "scroll_set_button":
		return C.libinput_device_config_scroll_set_button(d, C.uint32_t(pointerSettingInt(s.Value, int(C.libinput_device_config_scroll_get_default_button(d)))))
	case "scroll_set_button_lock":
		// Niri applies this flag only to button scrolling, including when that
		// method was selected by the device default.
		if s.Value != "default" && C.libinput_device_config_scroll_get_method(d) != C.LIBINPUT_CONFIG_SCROLL_ON_BUTTON_DOWN {
			return C.LIBINPUT_CONFIG_STATUS_SUCCESS
		}
		return C.libinput_device_config_scroll_set_button_lock(d, C.enum_libinput_config_scroll_button_lock_state(pointerSettingInt(s.Value, int(C.libinput_device_config_scroll_get_default_button_lock(d)))))
	case "tap_set_button_map":
		return C.libinput_device_config_tap_set_button_map(d, C.enum_libinput_config_tap_button_map(pointerSettingInt(s.Value, int(C.libinput_device_config_tap_get_default_button_map(d)))))
	case "click_set_method":
		return C.libinput_device_config_click_set_method(d, C.enum_libinput_config_click_method(pointerSettingInt(s.Value, int(C.libinput_device_config_click_get_default_method(d)))))
	}
	return C.LIBINPUT_CONFIG_STATUS_UNSUPPORTED
}
