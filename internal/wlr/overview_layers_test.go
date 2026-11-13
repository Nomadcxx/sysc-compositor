package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

func TestWorkspaceSwitchMovesBackgroundAndBottomLayersWithGap(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	checked := false
	pollThenStop(c, func() bool {
		var background *layerSurface
		for _, layer := range c.layerOrder {
			if _, _, mapped := layer.mappedSize(); mapped && layer.layer == layerBackground {
				background = layer
				break
			}
		}
		if background == nil {
			return false
		}
		checked = true
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		if !c.model.MapWindow(1000, background.outputName) {
			t.Fatal("could not prepare a workspace switch")
		}
		for _, kind := range []int{layerBackground, layerBottom} {
			background.layer = kind
			c.model.SetWorkspaceRenderOffset(background.outputName, 0.5)
			c.presentWorkspaceSwitch()
			c.syncOverviewScene()
			frame, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Fatal(err)
			}
			x, height := frame.Bounds().Dx()/2, frame.Bounds().Dy()
			for _, y := range []int{height / 4, height * 3 / 4} {
				if pixel := frame.NRGBAAt(x, y); pixel.G < 240 || pixel.R > 10 || pixel.B > 10 {
					t.Errorf("layer %d workspace pixel at y=%d: %v, want green", kind, y, pixel)
				}
			}
			// At halfway, each workspace moves 55% of an output height:
			// the 10% gap straddles the centre of the display.
			gap := int(math.Round(float64(height) * 0.1))
			for _, y := range []int{height/2 - gap/4, height/2 + gap/4} {
				if pixel := frame.NRGBAAt(x, y); pixel.R != pixel.G || pixel.G != pixel.B || pixel.G > 40 {
					t.Errorf("layer %d gap pixel at y=%d: %v, wallpaper did not move", kind, y, pixel)
				}
			}
		}
		c.model.ClearWorkspaceRenderOffset(background.outputName)
		c.applyWorkspaceModel()
		c.animationConfig.Off = false
		c.animationConfig.WorkspaceSwitch = config.Animation{Easing: &config.EasingConfig{DurationMS: 1000, Curve: "linear"}}
		if !c.model.FocusWorkspaceDown() {
			t.Error("could not switch workspace from the keyboard path")
			return true
		}
		c.applyWorkspaceModel()
		running := c.animations[workspaceSwitchKey(background.outputName)]
		if running == nil {
			t.Error("keyboard switch did not start the shared workspace animation")
			return true
		}
		c.tickAnimations(running.started.Add(500 * time.Millisecond))
		frame, err := c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Error(err)
			return true
		}
		if pixel := frame.NRGBAAt(frame.Bounds().Dx()/2, frame.Bounds().Dy()/2); pixel.G > 40 {
			t.Errorf("keyboard switch did not expose the gap: %v", pixel)
		}
		c.tickAnimations(running.started.Add(2 * time.Second))
		frame, err = c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Fatal(err)
		}
		if pixel := frame.NRGBAAt(frame.Bounds().Dx()/2, frame.Bounds().Dy()/2); pixel != (color.NRGBA{G: 255, A: 255}) {
			t.Errorf("wallpaper after settling: %v, want green", pixel)
		}
		return true
	})
	runClient(t, c, []string{"swaybg", "-c", "#00ff00"}, nil, nil)
	if !checked {
		t.Fatal("wallpaper never mapped")
	}
}

func TestWorkspaceSwitchRoutesInputToMovingBottomLayer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bar.json")
	if err := os.WriteFile(path, []byte(`{"layer":"bottom","position":"top","height":30,"modules-left":[],"modules-center":[],"modules-right":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := newNested(t)
	checked := false
	pollThenStop(c, func() bool {
		for _, layer := range c.layerOrder {
			_, height, mapped := layer.mappedSize()
			if !mapped || layer.layer != layerBottom {
				continue
			}
			checked = true
			c.model.MapWindow(1000, layer.outputName)
			bounds, _ := c.outputLayoutBounds(layer.outputName)
			c.model.SetWorkspaceRenderOffset(layer.outputName, 0.5)
			c.syncOverviewScene()
			x := float64(bounds.X + bounds.Width/2)
			y := float64(bounds.Y+int(math.Round(float64(bounds.Height)*1.1*0.5))) + float64(height)/2
			surface, _, sy := c.surfaceAt(x, y)
			if surface != layer.surface.surface || math.Abs(float64(sy)-float64(height)/2) > 1 {
				t.Errorf("moving bottom layer hit: surface=%p y=%v, want %p y=%v", surface, sy, layer.surface.surface, float64(height)/2)
			}
			if surface, _, _ := c.surfaceAt(x, float64(bounds.Y+bounds.Height/2)); surface != nil {
				t.Error("workspace gap routed input to a stationary surface")
			}
			return true
		}
		return false
	})
	runClient(t, c, []string{"waybar", "-c", path}, nil, nil)
	if !checked {
		t.Fatal("bottom-layer bar never mapped")
	}
}

func TestSceneClipPreservesTransformedPixels(t *testing.T) {
	c := newNested(t)
	checked := false
	pollThenStop(c, func() bool {
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		if output == nil {
			return false
		}
		checked = true
		img := image.NewRGBA(image.Rect(0, 0, 120, 80))
		for y := range 80 {
			for x := range 120 {
				img.SetRGBA(x, y, color.RGBA{R: uint8(100 + x), G: uint8(100 + y), A: 255})
			}
		}
		sceneFXTreeSetEnabled(unsafe.Pointer(c.overviewTree), true)
		clip := image.Rect(25, 30, 75, 100)
		for transform := range 8 {
			node := sceneImage(c.overviewTree, img)
			if node == nil {
				t.Error("could not create a scene image")
				return true
			}
			node.transform = uint32(transform)
			sceneImagePlace(node, image.Rect(0, 0, 100, 160), 1)
			before, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return true
			}
			sceneFXClipTree(unsafe.Pointer(c.overviewTree), rect{clip.Min.X, clip.Min.Y, clip.Dx(), clip.Dy()}, [4]uint16{})
			after, err := c.captureOutputForTarget(output, config.CaptureDisplay)
			if err != nil {
				t.Error(err)
				return true
			}
			for _, p := range []image.Point{{40, 40}, {60, 80}, {10, 20}, {80, 110}} {
				got, want := after.NRGBAAt(p.X, p.Y), (color.NRGBA{A: 255})
				if p.In(clip) {
					want = before.NRGBAAt(p.X, p.Y)
				}
				if got != want {
					t.Errorf("transform %d pixel %v = %v, want %v", transform, p, got, want)
				}
			}
			sceneFXNodeDestroy(unsafe.Pointer(&node.node))
		}
		return true
	})
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("no output for clipping check")
	}
}

func TestOverviewProjectsBackgroundLayersAndBackdropRules(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	checked := false
	pollThenStop(c, func() bool {
		var background *layerSurface
		for _, layer := range c.layerOrder {
			if _, _, mapped := layer.mappedSize(); mapped && layer.layer == layerBackground {
				background = layer
				break
			}
		}
		if background == nil {
			return false
		}
		c.OpenOverview()
		frame, ok := c.model.OverviewFrame(background.outputName, c.model.OverviewZoom(c.overviewConfig.Zoom))
		if !ok {
			t.Error("output missing")
			return true
		}
		count := func() int {
			n := 0
			for _, projection := range c.overviewProjections {
				if projection.layer == background {
					n++
				}
			}
			return n
		}
		if got := count(); got != len(frame.Workspaces) {
			t.Errorf("background copies=%d want=%d", got, len(frame.Workspaces))
		}
		background.rules.PlaceWithinBackdrop = true
		c.syncOverviewScene()
		if !background.withinBackdrop() || count() != 1 {
			t.Error("eligible backdrop layer was duplicated into workspaces")
		}
		background.layer = layerBottom
		c.syncOverviewScene()
		if background.withinBackdrop() || count() != len(frame.Workspaces) {
			t.Error("bottom layer incorrectly honored background-only backdrop rule")
		}
		checked = true
		return true
	})
	runClient(t, c, []string{"swaybg", "-c", "#00ff00"}, nil, nil)
	if !checked {
		t.Fatal("background layer was never projected")
	}
}

func TestOverviewWorkspaceShadowRendersAndTurnsOff(t *testing.T) {
	options := func(off bool) Options {
		source := `overview { zoom 0.5; backdrop-color "#ffffff"; workspace-shadow { softness 10; spread 30; offset x=40 y=0; color "#ff0000"; }; }`
		if off {
			source = `overview { zoom 0.5; backdrop-color "#ffffff"; workspace-shadow { off; }; }`
		}
		cfg, err := config.Parse("shadow.kdl", []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		return Options{Overview: cfg.Overview, Animations: config.Animations{Off: true}}
	}
	c := newNestedWith(t, options(false))
	checked := false
	pollThenStop(c, func() bool {
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		if output == nil {
			return false
		}
		checked = true
		c.OpenOverview()
		frame, _ := c.model.OverviewFrame(outputName(output), c.model.OverviewZoom(c.overviewConfig.Zoom))
		var bounds = frame.Workspaces[0].Bounds
		for _, workspace := range frame.Workspaces {
			if workspace.Active {
				bounds = workspace.Bounds
			}
		}
		x, y := bounds.X+bounds.Width+10-frame.Output.X, bounds.Y+bounds.Height/2-frame.Output.Y
		pixels, err := c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Error(err)
			return true
		}
		if p := pixels.NRGBAAt(x, y); p.R < 200 || p.G > 80 || p.B > 80 {
			t.Errorf("workspace shadow pixel = %v, want red", p)
		}
		if err := c.Reconfigure(options(true)); err != nil {
			t.Error(err)
			return true
		}
		pixels, err = c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Error(err)
			return true
		}
		if p := pixels.NRGBAAt(x, y); p.R < 245 || p.G < 245 || p.B < 245 {
			t.Errorf("disabled workspace shadow = %v, want white backdrop", p)
		}
		return true
	})
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("no output for workspace shadow check")
	}
}

func TestInsertHintGradientRendersAndRestoresSolidColor(t *testing.T) {
	c := newNested(t)
	checked := false
	pollThenStop(c, func() bool {
		var output unsafe.Pointer
		for output = range c.sceneOutputs {
			break
		}
		if output == nil {
			return false
		}
		checked = true
		name := outputName(output)
		c.model.MapWindow(1000, name)
		c.model.MapWindow(1001, name)
		c.model.BeginWindowDrag(1000, 100, 100)
		drag, _ := c.model.UpdateWindowDrag(120, 100, name)
		if drag.Target == nil {
			t.Error("no insertion hint target")
			return true
		}
		bounds := drag.Target.Hint
		cfg, err := config.Parse("gradient.kdl", []byte(`layout { insert-hint { gradient from="#ff0000" to="#0000ff" angle=90; }; }`))
		if err != nil {
			t.Error(err)
			return true
		}
		c.layoutOptions.InsertHint = cfg.Layout.InsertHint
		c.syncInsertHint()
		pixels, err := c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Error(err)
			return true
		}
		left := pixels.NRGBAAt(bounds.X+bounds.Width/10, bounds.Y+bounds.Height/2)
		right := pixels.NRGBAAt(bounds.X+bounds.Width*9/10, bounds.Y+bounds.Height/2)
		if left.R < 200 || left.B > 60 || right.B < 200 || right.R > 60 {
			t.Errorf("hint endpoints left=%v right=%v, want red to blue", left, right)
		}
		c.layoutOptions.InsertHint = config.InsertHint{Color: config.Color{G: 1, A: 1}}
		c.syncInsertHint()
		pixels, err = c.captureOutputForTarget(output, config.CaptureDisplay)
		if err != nil {
			t.Error(err)
			return true
		}
		if p := pixels.NRGBAAt(bounds.X+bounds.Width/2, bounds.Y+bounds.Height/2); p != (color.NRGBA{G: 255, A: 255}) {
			t.Errorf("solid hint after removal: %v", p)
		}
		return true
	})
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("no output for gradient check")
	}
}

func TestOverviewShadowExtremeNegativeSpreadIsEmpty(t *testing.T) {
	cfg, err := config.Parse("shadow.kdl", []byte(`overview { backdrop-color "#ffffff"; workspace-shadow { softness 0; spread -1024; color "#ff0000"; }; }`))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{Overview: cfg.Overview})
	c.OpenOverview()
	if len(c.overviewWorkspaceNodes) != 1 {
		t.Errorf("empty shadow allocated native nodes: got %d, want only the workspace fill", len(c.overviewWorkspaceNodes))
	}
	var output unsafe.Pointer
	for output = range c.sceneOutputs {
		break
	}
	pixels, err := c.captureOutputForTarget(output, config.CaptureDisplay)
	if err != nil {
		t.Fatal(err)
	}
	if pixel := pixels.NRGBAAt(pixels.Bounds().Dx()/8, pixels.Bounds().Dy()/2); pixel != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("empty negative-spread shadow drew %v", pixel)
	}
}
