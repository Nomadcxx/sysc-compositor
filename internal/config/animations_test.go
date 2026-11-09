package config

import (
	"strings"
	"testing"
)

func TestAnimationEasingSlowdownAndShadersAreParsed(t *testing.T) {
	cfg, err := Parse("animations.kdl", []byte(`animations {
 slowdown 2
 window-open { duration-ms 100; curve "cubic-bezier" 0 1 0 1; custom-shader "vec4 open_color(vec3 c, vec3 s) { return vec4(1.0); }"; }
 config-notification-open-close { duration-ms 200; curve "linear"; }
 screenshot-ui-open { duration-ms 250; curve "ease-out-cubic"; }
}`))
	if err != nil {
		t.Fatal(err)
	}
	open := cfg.Animations.WindowOpen
	if cfg.Animations.Slowdown == nil || *cfg.Animations.Slowdown != 2 || open.Easing == nil || open.Easing.DurationMS != 100 || open.Easing.Curve != "cubic-bezier" || open.Easing.CubicBezier != [4]float64{0, 1, 0, 1} || open.CustomShader == nil {
		t.Fatalf("animation config = %+v, open=%+v", cfg.Animations, open)
	}
	if cfg.Animations.ConfigNotificationOpenClose.Easing == nil || cfg.Animations.ScreenshotUIOpen.Easing == nil {
		t.Fatal("UI easing omitted")
	}
	for _, source := range []string{
		`animations { window-open { curve "unknown"; }; }`,
		`animations { window-open { curve "cubic-bezier" -0.1 0 1 1; }; }`,
		`animations { window-open { curve "cubic-bezier" 0 0 1; }; }`,
	} {
		if _, err := Parse("bad.kdl", []byte(source)); err == nil || !strings.Contains(err.Error(), "curve") {
			t.Fatalf("invalid easing = %q, err %v", source, err)
		}
	}
}
