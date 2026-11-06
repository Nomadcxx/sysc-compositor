package config

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sblinch/kdl-go/document"
)

func TestPointerDeviceSectionsDecode(t *testing.T) {
	cfg, err := Parse("pointers.kdl", []byte(`input {
 touchpad { tap; dwt; dwtp; drag-lock; natural-scroll; accel-speed 0.2; accel-profile "flat"; scroll-method "two-finger"; click-method "clickfinger"; tap-button-map "left-middle-right"; drag false; disabled-on-external-mouse; scroll-factor 1.5; }
 mouse { left-handed; middle-emulation; scroll-button 4294967295; scroll-button-lock; scroll-factor vertical=2.0 horizontal=-1.0; }
 trackball { off; natural-scroll; accel-speed -1; accel-profile "adaptive"; scroll-method "on-button-down"; scroll-button 0; }
}`))
	if err != nil {
		t.Fatal(err)
	}
	pad := cfg.Input.Touchpad
	if !pad.Set || !pad.Tap || !pad.DWT || !pad.DWTP || !pad.DragLock || !pad.NaturalScroll || pad.AccelSpeed != 0.2 || pad.AccelProfile != "flat" || pad.ScrollMethod != "two-finger" || pad.ClickMethod != "clickfinger" || pad.TapButtonMap != "left-middle-right" || !pad.DisabledOnExternalMouse || pad.Drag == nil || *pad.Drag || pad.ScrollFactor != (ScrollFactor{1.5, 1.5}) {
		t.Fatalf("touchpad: %+v", pad)
	}
	mouse := cfg.Input.Mouse
	if !mouse.Set || !mouse.LeftHanded || !mouse.MiddleEmulation || !mouse.ScrollButtonLock || mouse.ScrollButton == nil || *mouse.ScrollButton != math.MaxUint32 || mouse.ScrollFactor != (ScrollFactor{2, -1}) {
		t.Fatalf("mouse: %+v", mouse)
	}
	if cfg.Input.Trackpoint.Set || !cfg.Input.Trackball.Set || !cfg.Input.Trackball.Off || cfg.Input.Trackball.AccelSpeed != -1 || cfg.Input.Trackball.ScrollButton == nil || *cfg.Input.Trackball.ScrollButton != 0 {
		t.Fatalf("other pointers: %+v", cfg.Input.Pointers)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings: %v", cfg.Warnings)
	}
}
func TestPointerDeviceSectionsReplaceAcrossIncludes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config.kdl")
	child := filepath.Join(dir, "t.kdl")
	if err := os.WriteFile(root, []byte("input { touchpad { tap; natural-scroll; scroll-factor 2; }; mouse { left-handed; }; }\ninclude \"t.kdl\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte(`input { touchpad { dwt; }; }`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Input.Touchpad.Tap || cfg.Input.Touchpad.NaturalScroll || !cfg.Input.Touchpad.DWT || cfg.Input.Touchpad.ScrollFactor != (ScrollFactor{1, 1}) || !cfg.Input.Mouse.LeftHanded {
		t.Fatalf("replacement: %+v", cfg.Input.Pointers)
	}
}
func TestPointerDeviceSectionsDefaultsAndRemoval(t *testing.T) {
	cfg, err := Parse("unset.kdl", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, dev := range []PointerDevice{cfg.Input.Touchpad, cfg.Input.Mouse, cfg.Input.Trackpoint, cfg.Input.Trackball} {
		if dev.Set || dev.ScrollFactor != (ScrollFactor{1, 1}) {
			t.Fatalf("unset: %+v", dev)
		}
	}
	cfg, err = Parse("empty.kdl", []byte(`input { touchpad {}; mouse {}; trackpoint {}; trackball {}; }`))
	if err != nil {
		t.Fatal(err)
	}
	for _, dev := range []PointerDevice{cfg.Input.Touchpad, cfg.Input.Mouse, cfg.Input.Trackpoint, cfg.Input.Trackball} {
		if !dev.Set || dev.ScrollFactor != (ScrollFactor{1, 1}) {
			t.Fatalf("empty: %+v", dev)
		}
	}
}
func TestPointerDeviceSectionsRejectBadValues(t *testing.T) {
	for _, tc := range []struct{ device, field string }{
		{"touchpad", `accel-speed 1.5`}, {"mouse", `accel-speed -1.01`}, {"touchpad", `accel-profile "fast"`}, {"touchpad", `scroll-method "three"`}, {"touchpad", `tap 1`},
		{"mouse", `tap`}, {"trackpoint", `dwt`}, {"trackball", `drag false`}, {"mouse", `click-method "clickfinger"`}, {"touchpad", `click-method "bad"`}, {"touchpad", `tap-button-map "bad"`},
		{"mouse", `scroll-button -1`}, {"mouse", `scroll-button 4294967296`}, {"mouse", `scroll-button 1.5`}, {"mouse", `scroll-button 1 extra=true`}, {"touchpad", `drag 1`},
		{"touchpad", `scroll-factor -1`}, {"mouse", `scroll-factor 101`}, {"mouse", `scroll-factor vertical=-101`}, {"mouse", `scroll-factor horizontal=101`}, {"mouse", `scroll-factor vertical="2"`}, {"mouse", `scroll-factor other=2`}, {"mouse", `scroll-factor 1 2`},
		{"trackpoint", `scroll-factor 2`}, {"trackball", `scroll-factor vertical=2`}, {"touchpad", `tap extra=true`}, {"touchpad", `tap { bad; }`}, {"touchpad", `accel-profile "flat" extra=true`}, {"touchpad", `unknown`}, {"touchpad", `tap; tap`},
	} {
		t.Run(tc.device+" "+tc.field, func(t *testing.T) {
			_, err := Parse("bad.kdl", []byte("input { "+tc.device+" { "+tc.field+"; }; }"))
			field := strings.TrimSuffix(strings.Fields(tc.field)[0], ";")
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("%s: %v", field, err)
			}
		})
	}
	for _, text := range []string{`input { touchpad 1 {}; }`, `input { mouse extra=true {}; }`, `input { touchpad {}; touchpad {}; }`} {
		if _, err := Parse("bad.kdl", []byte(text)); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}
func TestPointerDeviceScrollFactorPropertiesOverrideBase(t *testing.T) {
	for _, tc := range []struct {
		text string
		want ScrollFactor
	}{
		{`scroll-factor 2 vertical=-1`, ScrollFactor{-1, 2}},
		{`scroll-factor horizontal=-2`, ScrollFactor{1, -2}},
		{`scroll-factor 0 vertical=100 horizontal=-100`, ScrollFactor{100, -100}},
		{`scroll-factor`, ScrollFactor{1, 1}},
	} {
		cfg, err := Parse("factor.kdl", []byte("input { mouse { "+tc.text+"; }; }"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Input.Mouse.ScrollFactor != tc.want {
			t.Fatalf("%s: %+v", tc.text, cfg.Input.Mouse.ScrollFactor)
		}
	}
}
func TestPointerDeviceRejectsNonFiniteNumbers(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		field := &document.Node{Name: &document.Value{Value: "accel-speed"}, Arguments: []*document.Value{{Value: value}}}
		node := &document.Node{Name: &document.Value{Value: "touchpad"}, Children: []*document.Node{field}}
		if _, err := pointerDevice(node, true); err == nil {
			t.Fatalf("accepted speed %v", value)
		}
		field.Name = &document.Value{Value: "scroll-factor"}
		if _, err := pointerDevice(node, true); err == nil {
			t.Fatalf("accepted factor %v", value)
		}
	}
}
func TestPointerDeviceEmptySectionResetsEveryField(t *testing.T) {
	old, err := Parse("old.kdl", []byte(`input { touchpad { tap; dwt; natural-scroll; drag false; scroll-button 4; }; }`))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := Parse("empty.kdl", []byte(`input { touchpad {}; }`))
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(old.Input.Touchpad, empty.Input.Touchpad) || empty.Input.Touchpad.Drag != nil || empty.Input.Touchpad.ScrollButton != nil {
		t.Fatal("old settings survived")
	}
}
