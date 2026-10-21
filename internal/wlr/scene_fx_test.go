package wlr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestShadowUsesTopLeftCornerRadius(t *testing.T) {
	radii := config.CornerRadius{4, 8, 12, 2}
	if got := shadowCornerRadius(radii, 0); got != 4 {
		t.Fatalf("shadow corner radius = %d, want top-left radius 4", got)
	}
}

func TestShadowStyleMatchesNiriBoxShadow(t *testing.T) {
	shadow := config.Shadow{Softness: 20, Spread: 4.2, Color: config.Color{R: 0.4, G: 0.3, B: 0.2, A: 0.8}}
	active := makeShadowRenderStyle(shadow, config.CornerRadius{12, 8, 4, 2}, true)
	if active.spread != 5 || active.radius != 17 || active.sigma != 10 {
		t.Fatalf("active shadow style = %+v, want spread 5 radius 17 sigma 10", active)
	}
	inactive := makeShadowRenderStyle(shadow, config.CornerRadius{12, 8, 4, 2}, false)
	if inactive.color[3] != float32(0.8*0.75) {
		t.Errorf("inactive shadow alpha = %v, want configured alpha scaled by 0.75", inactive.color[3])
	}
	square := makeShadowRenderStyle(shadow, config.CornerRadius{}, true)
	if square.radius != 0 {
		t.Errorf("spread rounded a square-corner window: radius = %d, want 0", square.radius)
	}
	contracted := makeShadowRenderStyle(config.Shadow{Spread: -20}, config.CornerRadius{12, 12, 12, 12}, true)
	if contracted.spread != -20 || contracted.radius != 0 {
		t.Errorf("contracted shadow = %+v, want spread -20 with radius 0", contracted)
	}
}

func TestFocusRingTracksEachWorkspaceActiveWindow(t *testing.T) {
	first, second := uint64(1), uint64(2)
	workspaces := []wm.WorkspaceSnapshot{
		{Output: "DP-1", IsActive: true, ActiveWindowID: &first},
		{Output: "HDMI-1", IsActive: true, ActiveWindowID: &second},
	}
	active := activeWindowIDsForWorkspaces(workspaces)
	if !active[1] || !active[2] {
		t.Fatal("active windows on both outputs should have focus rings")
	}
	if active[3] {
		t.Fatal("inactive window should not have a focus ring")
	}
}

func TestSceneFXRendererRendersNestedSceneEffects(t *testing.T) {
	c := newNested(t)
	var created bool
	c.viewMapped = func(*toplevelView) {
		if !c.usesSceneFXRenderer() {
			t.Error("renderer is not a SceneFX renderer")
			c.Terminate()
			return
		}
		color := [4]float32{0, 0, 0, 0.5}
		shadow := sceneFXCreateShadow(unsafe.Pointer(c.scene), 100, 100, 16, 8, color)
		if shadow == nil {
			t.Error("could not create SceneFX shadow node")
		} else {
			sceneFXNodeSetPosition(shadow, 10, 10)
		}
		rect := sceneFXCreateRoundedRect(unsafe.Pointer(c.scene), 100, 100, 16, color)
		if rect == nil {
			t.Error("could not create SceneFX rectangle node")
		}
		created = shadow != nil && rect != nil
		time.AfterFunc(150*time.Millisecond, c.Terminate)
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !created {
		t.Fatal("nested client did not create SceneFX nodes")
	}
	if c.drawn == 0 {
		t.Fatal("nested SceneFX scene rendered no output frames")
	}
}

func TestWindowEffectsRenderForMappedToplevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m8.kdl")
	source := []byte(`
prefer-no-csd
layout {
    focus-ring {
        width 4
        active-color "#7fc8ff"
        inactive-color "#505050"
    }
    shadow {
        on
        softness 20
        spread 5
        offset x=0 y=5
    }
}
animations {
    window-open { spring damping-ratio=1.0 stiffness=700 epsilon=0.0001; }
    window-close { spring damping-ratio=1.0 stiffness=700 epsilon=0.1; }
    window-resize { spring damping-ratio=1.0 stiffness=800 epsilon=0.001; }
    window-movement { spring damping-ratio=1.0 stiffness=600 epsilon=0.0001; }
    overview-open-close { spring damping-ratio=1.0 stiffness=900 epsilon=0.0001; }
    workspace-switch { spring damping-ratio=1.0 stiffness=800 epsilon=0.00001; }
    horizontal-view-movement { spring damping-ratio=1.0 stiffness=700 epsilon=0.0001; }
}
window-rule {
    geometry-corner-radius 20
    clip-to-geometry true
}
`)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	layout := &LayoutOptions{
		Gaps: cfg.Layout.Gaps, ColumnWidthProportion: cfg.Layout.ColumnWidthProportion,
		FixedColumnWidth: cfg.Layout.FixedColumnWidth, InsertHint: cfg.Layout.InsertHint,
		FocusRing: cfg.Layout.FocusRing, Shadow: cfg.Layout.Shadow,
	}
	c := newNestedWith(t, Options{
		Layout:      layout,
		Animations:  cfg.Animations,
		WindowRules: cfg.WindowRules,
		PreferNoCSD: cfg.PreferNoCSD,
	})
	var mapped bool
	c.viewMapped = func(view *toplevelView) {
		mapped = true
		if !reflect.DeepEqual(c.animationConfig, cfg.Animations) {
			t.Errorf("compositor animations = %+v, want parsed M8 settings %+v", c.animationConfig, cfg.Animations)
		}
		if c.layoutOptions.FocusRing != cfg.Layout.FocusRing || c.layoutOptions.Shadow != cfg.Layout.Shadow || !c.preferNoCSD {
			t.Errorf("compositor appearance = focus:%+v shadow:%+v prefer-no-csd:%v", c.layoutOptions.FocusRing, c.layoutOptions.Shadow, c.preferNoCSD)
		}
		expectSceneField(t, view, "shadowNode", true)
		expectSceneField(t, view, "focusRingNode", true)
		expectFloatArray(t, view, "cornerRadii", []float64{20, 20, 20, 20})
		if got := sceneFXTreeRoundedBufferCornerRadii(unsafe.Pointer(view.tree)); got != [4]uint16{20, 20, 20, 20} {
			t.Errorf("mapped buffer corner radii = %v, want [20 20 20 20]", got)
		}
		expectBoolField(t, view, "clipToGeometry", true)
		time.AfterFunc(150*time.Millisecond, c.Terminate)
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !mapped {
		t.Fatal("nested toplevel did not map")
	}
	if c.drawn == 0 {
		t.Fatal("nested compositor rendered no output frames")
	}
}

func TestFullscreenSuppressesWindowAppearance(t *testing.T) {
	defaults := config.Default()
	radius := config.CornerRadius{12, 8, 4, 2}
	clip := true
	c := newNestedWith(t, Options{
		Layout: &LayoutOptions{
			Gaps: defaults.Layout.Gaps, ColumnWidthProportion: defaults.Layout.ColumnWidthProportion,
			InsertHint: defaults.Layout.InsertHint,
			FocusRing: config.FocusRing{Width: 3,
				ActiveColor: config.Color{R: 1, A: 1}, InactiveColor: config.Color{B: 1, A: 1}},
			Shadow: config.Shadow{On: true, Softness: 8, Spread: 3,
				Color: config.Color{A: 0.5}},
		},
		WindowRules: []config.WindowRule{{GeometryCornerRadius: &radius, ClipToGeometry: &clip}},
	})
	var checked bool
	c.viewMapped = func(view *toplevelView) {
		checked = true
		if !c.SetFullscreen(view.id, true) {
			t.Error("SetFullscreen() = false, want fullscreen enabled")
			c.Terminate()
			return
		}
		if sceneFXNodeEnabled(view.shadowNode) {
			t.Error("fullscreen window shadow remained enabled")
		}
		if sceneFXNodeEnabled(view.focusRingNode) {
			t.Error("fullscreen focus ring remained enabled")
		}
		if view.clipToGeometry {
			t.Error("fullscreen view remained clipped to its window geometry")
		}
		if view.cornerRadii != (config.CornerRadius{}) {
			t.Errorf("fullscreen corner radii = %v, want square corners", view.cornerRadii)
		}
		time.AfterFunc(150*time.Millisecond, c.Terminate)
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested toplevel did not map")
	}
}

func expectSceneField(t *testing.T, value any, name string, wantNonNil bool) {
	t.Helper()
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Errorf("toplevel view has no %s scene field", name)
		return
	}
	if field.Kind() == reflect.Array || field.Kind() == reflect.Slice {
		if field.Len() == 0 {
			t.Errorf("toplevel view %s is empty", name)
		}
		for i := 0; i < field.Len(); i++ {
			if field.Index(i).IsNil() == wantNonNil {
				t.Errorf("toplevel view %s[%d] nil = %v, want non-nil %v", name, i, field.Index(i).IsNil(), wantNonNil)
			}
		}
		return
	}
	if field.IsNil() == wantNonNil {
		t.Errorf("toplevel view %s nil = %v, want non-nil %v", name, field.IsNil(), wantNonNil)
	}
}

func expectFloatArray(t *testing.T, value any, name string, want []float64) {
	t.Helper()
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Errorf("toplevel view has no %s field", name)
		return
	}
	if field.Len() != len(want) {
		t.Errorf("toplevel view %s has length %d, want %d", name, field.Len(), len(want))
		return
	}
	for i, expected := range want {
		if got := field.Index(i).Float(); got != expected {
			t.Errorf("toplevel view %s[%d] = %v, want %v", name, i, got, expected)
		}
	}
}

func expectBoolField(t *testing.T, value any, name string, want bool) {
	t.Helper()
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Errorf("toplevel view has no %s field", name)
		return
	}
	if got := field.Bool(); got != want {
		t.Errorf("toplevel view %s = %v, want %v", name, got, want)
	}
}
