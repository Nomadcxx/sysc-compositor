package wlr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLayerSnapshotWireMappings(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   int
		want LayerKind
	}{
		{"background", layerBackground, LayerBackgroundKind},
		{"bottom", layerBottom, LayerBottomKind},
		{"top", layerTop, LayerTopKind},
		{"overlay", layerOverlay, LayerOverlayKind},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotLayerKind(tc.in); got != tc.want {
				t.Errorf("snapshotLayerKind(%d) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		in   int
		want LayerKeyboardMode
	}{
		{"none", 0, LayerKeyboardNoneMode},
		{"exclusive", 1, LayerKeyboardExclusiveMode},
		{"on-demand", 2, LayerKeyboardOnDemandMode},
	} {
		t.Run("keyboard_"+tc.name, func(t *testing.T) {
			if got := snapshotKeyboardMode(tc.in); got != tc.want {
				t.Errorf("snapshotKeyboardMode(%d) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLayerKeyboardFocusPriority(t *testing.T) {
	for _, tc := range []struct {
		name       string
		candidates []layerFocusCandidate
		hasWindow  bool
		want       int
	}{
		{
			name: "overlay on-demand focus precedes top exclusive",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerTop, output: "DP-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
				{index: 1, layer: layerOverlay, output: "DP-1", keyboard: LayerKeyboardOnDemandMode, mapped: true, onDemandFocused: true},
			},
			hasWindow: true, want: 1,
		},
		{
			name: "top exclusive precedes focused window",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerTop, output: "DP-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
			},
			hasWindow: true, want: 0,
		},
		{
			name: "mapped bottom on-demand precedes focused window",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerBottom, output: "DP-1", keyboard: LayerKeyboardOnDemandMode, mapped: true, onDemandFocused: true},
			},
			hasWindow: true, want: 0,
		},
		{
			name: "keyboard none never receives focus",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerOverlay, output: "DP-1", keyboard: LayerKeyboardNoneMode, mapped: true},
			},
			want: -1,
		},
		{
			name: "unmapped on-demand surface loses focus",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerOverlay, output: "DP-1", keyboard: LayerKeyboardOnDemandMode, onDemandFocused: true},
			},
			want: -1,
		},
		{
			name: "destroyed surface is absent from focus candidates",
			want: -1,
		},
		{
			name: "focused window precedes bottom exclusive",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerBottom, output: "DP-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
			},
			hasWindow: true, want: -1,
		},
		{
			name: "bottom exclusive takes focus without a window",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerBottom, output: "DP-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
			},
			want: 0,
		},
		{
			name: "unmapped exclusive surface is ignored",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerOverlay, output: "DP-1", keyboard: LayerKeyboardExclusiveMode},
			},
			want: -1,
		},
		{
			name: "layer surface on another output cannot take focus",
			candidates: []layerFocusCandidate{
				{index: 0, layer: layerTop, output: "DP-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
				{index: 1, layer: layerOverlay, output: "HDMI-A-1", keyboard: LayerKeyboardExclusiveMode, mapped: true},
			},
			hasWindow: true, want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := chooseLayerKeyboardFocus(tc.candidates, "DP-1", tc.hasWindow); got != tc.want {
				t.Errorf("chooseLayerKeyboardFocus() = %d, want %d", got, tc.want)
			}
		})
	}
}

// pollThenStop runs check on the compositor thread every 20 ms until it
// reports true or three seconds pass, then stops the compositor.
func pollThenStop(c *Compositor, check func() bool) {
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			done := make(chan bool, 1)
			if c.Post(func() { done <- check() }) != nil || <-done {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = c.Post(c.Terminate)
	}()
}

func mappedOn(placed []layerPlacement, layer int) *layerPlacement {
	for i := range placed {
		if placed[i].mapped && placed[i].layer == layer {
			return &placed[i]
		}
	}
	return nil
}

// TestLayerSurfaceFillsItsOutput: swaybg anchors a background surface to every
// edge with size 0x0, which asks the compositor for the whole output.
func TestLayerSurfaceFillsItsOutput(t *testing.T) {
	c := newNested(t)
	var (
		got    *layerPlacement
		output rect
	)
	pollThenStop(c, func() bool {
		output = c.layoutBox()
		got = mappedOn(c.layerPlacements(), layerBackground)
		return got != nil
	})
	runClient(t, c, []string{"swaybg", "-c", "#336699"}, nil, nil)
	if got == nil {
		t.Fatal("swaybg never mapped a background layer surface")
	}
	if got.box != output {
		t.Errorf("background surface box %+v, want the output %+v", got.box, output)
	}
	if got.namespace != "wallpaper" {
		t.Errorf("namespace %q, want swaybg's \"wallpaper\"", got.namespace)
	}
}

func TestLayerSnapshotIncludesOnlyMappedSurface(t *testing.T) {
	c := newNested(t)
	var mapped, afterClose []LayerSnapshot
	pollThenStop(c, func() bool {
		mapped = c.LayerSnapshot()
		if len(mapped) == 0 {
			return false
		}
		c.closeLayerSurfaces()
		afterClose = c.LayerSnapshot()
		return true
	})
	runClient(t, c, []string{"swaybg", "-c", "#336699"}, nil, nil)
	if len(mapped) != 1 {
		t.Fatalf("mapped LayerSnapshot() = %+v, want one mapped surface", mapped)
	}
	if mapped[0].Namespace != "wallpaper" || mapped[0].Layer != LayerBackgroundKind || mapped[0].KeyboardInteractivity != LayerKeyboardNoneMode {
		t.Errorf("mapped layer = %+v", mapped[0])
	}
	if mapped[0].Output == "" {
		t.Error("mapped layer has no output name")
	}
	if len(afterClose) != 0 {
		t.Errorf("LayerSnapshot() after close = %+v, want empty", afterClose)
	}
}

// TestExclusiveZoneShrinksUsableArea: a 30 px top bar takes 30 px from the
// top of the usable area, and gives it back when it goes.
func TestExclusiveZoneShrinksUsableArea(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.jsonc")
	style := filepath.Join(dir, "style.css")
	if err := os.WriteFile(config, []byte(`{"layer":"top","position":"top","height":30,`+
		`"modules-left":[],"modules-center":[],"modules-right":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(style, []byte("* { min-height: 0; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newNested(t)
	var (
		bar            *layerPlacement
		output         rect
		during, closed rect
		closedOK       bool
	)
	pollThenStop(c, func() bool {
		output = c.layoutBox()
		if bar = mappedOn(c.layerPlacements(), layerTop); bar == nil {
			return false
		}
		// The client sets its exclusive zone once it knows its size, which
		// can be a commit after the first map; wait for it.
		if during = c.usableArea(); during == output {
			return false
		}
		// Checked in the same turn: the client could open a new bar later.
		c.closeLayerSurfaces()
		closed = c.usableArea()
		closedOK = len(c.layerPlacements()) == 0
		return true
	})
	lines := runClient(t, c, []string{"waybar", "-c", config, "-s", style}, nil, nil)
	if bar == nil {
		t.Fatalf("waybar never mapped a top layer surface; it said:\n%s", strings.Join(lines, "\n"))
	}
	want := rect{output.x, output.y + 30, output.w, output.h - 30}
	if during != want {
		t.Errorf("usable area with the bar %+v, want %+v", during, want)
	}
	if bar.box != (rect{output.x, output.y, output.w, 30}) {
		t.Errorf("bar box %+v, want the top 30 px of %+v", bar.box, output)
	}
	if !closedOK {
		t.Fatal("closing the bar left a layer surface behind")
	}
	if closed != output {
		t.Errorf("usable area after the bar closed %+v, want the whole output %+v", closed, output)
	}
}

// TestOutputFollowsHostResize: the host compositor sizes the nested window
// (Niri tiles it) and asks the output to match. The output must take that
// size, and a background layer must be configured to fill it; ignoring the
// request left every frame after the first undrawn.
func TestOutputFollowsHostResize(t *testing.T) {
	c := newNested(t)
	var (
		requested rect
		got       *layerPlacement
		output    rect
	)
	pollThenStop(c, func() bool {
		requested = c.requestedOutputSize()
		output = c.layoutBox()
		got = mappedOn(c.layerPlacements(), layerBackground)
		return requested.w > 0 && got != nil && got.box == output && output.w == requested.w
	})
	runClient(t, c, []string{"swaybg", "-c", "#336699"}, nil, nil)
	if requested.w == 0 {
		t.Fatal("the host never asked the nested output to resize (Niri always sizes a new window)")
	}
	if output.w != requested.w || output.h != requested.h {
		t.Errorf("output %dx%d, but the host asked for %dx%d", output.w, output.h, requested.w, requested.h)
	}
	if got == nil || got.box != output {
		t.Errorf("background surface %+v, want the resized output %+v", got, output)
	}
}

// TestClientContentIsDrawn: once a client maps, the next output frame commits
// the scene with it. M0 looked its scene output up by the wrong key, so no
// frame ever drew the scene and the nested window stayed black.
func TestClientContentIsDrawn(t *testing.T) {
	c := newNested(t)
	var before, after int
	mappedAt := -1
	pollThenStop(c, func() bool {
		if mappedAt < 0 {
			if mappedOn(c.layerPlacements(), layerBackground) == nil {
				return false
			}
			mappedAt = c.scenesDrawn()
			before = mappedAt
			return false
		}
		after = c.scenesDrawn()
		return after > before
	})
	runClient(t, c, []string{"swaybg", "-c", "#336699"}, nil, nil)
	if mappedAt < 0 {
		t.Fatal("swaybg never mapped")
	}
	if after <= before {
		t.Fatalf("no scene was drawn after the background mapped (%d drawn before, %d after)", before, after)
	}
}
