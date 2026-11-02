package wlr

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

var allPointerCaps = deviceCaps{TapFingers: 3, HasAccel: true, HasNatural: true, HasLeftHanded: true, HasDWT: true, HasDWTP: true, HasMiddleEmulation: true, ScrollMethods: 7, ClickMethods: 3}

func TestPlanTouchpadSettings(t *testing.T) {
	cfg := config.PointerDevice{Set: true, Tap: true, DWT: true, NaturalScroll: true, AccelSpeed: 0.2, ClickMethod: "clickfinger"}
	// Niri 26.04 apply_libinput_settings, src/input/mod.rs:4674.
	want := []pointerSetting{
		{"send_events_set_mode", "enabled"}, {"tap_set_enabled", true}, {"dwt_set_enabled", true}, {"dwtp_set_enabled", false}, {"tap_set_drag_lock_enabled", false},
		{"scroll_set_natural_scroll_enabled", true}, {"accel_set_speed", 0.2}, {"left_handed_set", false}, {"middle_emulation_set_enabled", false},
		{"tap_set_drag_enabled", "default"}, {"accel_set_profile", "default"}, {"scroll_set_method", "default"}, {"scroll_set_button", "default"}, {"scroll_set_button_lock", false},
		{"tap_set_button_map", "default"}, {"click_set_method", "clickfinger"},
	}
	if got := planPointerSettings(kindTouchpad, cfg, allPointerCaps); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan=%#v want %#v", got, want)
	}
}
func TestPlanUnsetSectionUsesDefaults(t *testing.T) {
	got := planPointerSettings(kindTouchpad, config.PointerDevice{}, allPointerCaps)
	if len(got) != 16 {
		t.Fatalf("default plan=%#v", got)
	}
	for _, setting := range got {
		if setting.Value != "default" {
			t.Errorf("%s=%v, want default", setting.Name, setting.Value)
		}
	}
}
func TestPlanSkipsUnsupported(t *testing.T) {
	got := planPointerSettings(kindMouse, config.PointerDevice{Set: true, DWT: true, AccelSpeed: 0.3, ScrollMethod: "two-finger"}, deviceCaps{HasAccel: true, ScrollMethods: 4})
	want := []pointerSetting{{"send_events_set_mode", "enabled"}, {"accel_set_speed", 0.3}, {"accel_set_profile", "default"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unsupported plan=%#v want %#v", got, want)
	}
}
func TestPlanOffAndExternalMouse(t *testing.T) {
	for _, tc := range []struct {
		cfg  config.PointerDevice
		mode string
	}{
		{config.PointerDevice{Set: true, Off: true, DisabledOnExternalMouse: true}, "disabled"},
		{config.PointerDevice{Set: true, DisabledOnExternalMouse: true}, "disabled-on-external-mouse"},
	} {
		if got := planPointerSettings(kindTouchpad, tc.cfg, allPointerCaps)[0]; got.Value != tc.mode {
			t.Errorf("mode=%v want %s", got.Value, tc.mode)
		}
	}
}
func TestScaleAxis(t *testing.T) {
	for _, tc := range []struct {
		factor     config.ScrollFactor
		horizontal bool
		delta      float64
		v120       int32
	}{
		{config.ScrollFactor{Vertical: 2, Horizontal: -1}, false, 30, 240},
		{config.ScrollFactor{Vertical: 2, Horizontal: -1}, true, -15, -120},
		{config.ScrollFactor{Vertical: 1.5, Horizontal: 1.5}, false, 22.5, 180},
		{config.ScrollFactor{Vertical: 0.3, Horizontal: 0.3}, false, 4.5, 36},
		{config.ScrollFactor{Vertical: 0, Horizontal: 1}, false, 0, 0},
	} {
		delta, v120 := scaleAxis(tc.factor, tc.horizontal, 15, 120)
		if delta != tc.delta || v120 != tc.v120 {
			t.Errorf("scale(%+v,%t)=(%v,%v) want (%v,%v)", tc.factor, tc.horizontal, delta, v120, tc.delta, tc.v120)
		}
	}
}
func TestSwipeDirectionFollowsNaturalScroll(t *testing.T) {
	for _, natural := range []bool{false, true} {
		dx, dy := swipeDeltas(natural, 3, -5)
		wantX, wantY := 3.0, -5.0
		if natural {
			wantX, wantY = -3, 5
		}
		if dx != wantX || dy != wantY {
			t.Errorf("natural=%t deltas=(%v,%v)", natural, dx, dy)
		}
	}
}

func TestPointerScrollFactorsDefaultAndDeviceSelection(t *testing.T) {
	c := &Compositor{}
	if got := c.pointerScrollFactor(nil, true); got != (config.ScrollFactor{Vertical: 1, Horizontal: 1}) {
		t.Fatalf("zero Options factor=%+v", got)
	}
	var marker byte
	device := unsafe.Pointer(&marker)
	c.pointers = config.Pointers{
		Mouse:    config.PointerDevice{Set: true, ScrollFactor: config.ScrollFactor{Vertical: 2, Horizontal: -1}},
		Touchpad: config.PointerDevice{Set: true, ScrollFactor: config.ScrollFactor{Vertical: 0.3, Horizontal: 0}},
	}
	c.pointerKinds = map[unsafe.Pointer]pointerKind{device: kindTouchpad}
	for _, finger := range []bool{false, true} {
		want := c.pointers.Mouse.ScrollFactor
		if finger {
			want = c.pointers.Touchpad.ScrollFactor
		}
		if got := c.pointerScrollFactor(device, finger); got != want {
			t.Errorf("finger=%t factor=%+v want %+v", finger, got, want)
		}
	}
	c.pointerKinds[device] = kindTrackpoint
	if got := c.pointerScrollFactor(device, true); got != c.pointers.Mouse.ScrollFactor {
		t.Fatal("trackpoint did not use mouse factor")
	}
	if got := c.pointerScrollFactor(nil, true); got != c.pointers.Mouse.ScrollFactor {
		t.Fatal("virtual pointer did not use mouse factor")
	}
	if c.touchpadNaturalScroll(nil) {
		t.Fatal("virtual pointer has natural swipe inversion")
	}
}

func TestPointerConfigurationReloadRemovesOverrides(t *testing.T) {
	cfg := config.Pointers{Touchpad: config.PointerDevice{Set: true, NaturalScroll: true, AccelSpeed: 0.3, ScrollFactor: config.ScrollFactor{Vertical: 2, Horizontal: 2}}}
	c := &Compositor{}
	c.setPointerConfig(normalizeOptions(Options{Pointers: cfg}).Pointers)
	if !reflect.DeepEqual(c.pointers, cfg) {
		t.Fatal("Options lost pointer settings")
	}
	c.setPointerConfig(normalizeOptions(Options{}).Pointers)
	for _, setting := range planPointerSettings(kindTouchpad, c.pointers.Touchpad, allPointerCaps) {
		if setting.Value != "default" {
			t.Errorf("removed override %s retained %v", setting.Name, setting.Value)
		}
	}
	if got := c.pointerScrollFactor(nil, true); got != (config.ScrollFactor{Vertical: 1, Horizontal: 1}) {
		t.Fatalf("removed scroll factor=%+v", got)
	}
}

func TestScaleAxisRoundsAndSaturatesWheelValues(t *testing.T) {
	for _, tc := range []struct {
		factor      float64
		wheel, want int32
	}{
		{0.0042, 120, 1}, {-0.0042, 120, -1}, {100, 2147483647, 2147483647}, {100, -2147483648, -2147483648},
	} {
		_, got := scaleAxis(config.ScrollFactor{Vertical: tc.factor, Horizontal: 1}, false, 15, tc.wheel)
		if got != tc.want {
			t.Errorf("factor=%v wheel=%v got=%v want=%v", tc.factor, tc.wheel, got, tc.want)
		}
	}
}
