package wlr

import (
	"image"
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestM10BackgroundEffectRegionIsCommittedAndCopied(t *testing.T) {
	c := newNested(t)
	type result struct {
		client *testclient.BackgroundEffectClient
		err    error
	}
	done := make(chan result, 1)
	go func() {
		client, err := testclient.StartBackgroundEffectClient(c.SocketName())
		done <- result{client: client, err: err}
		c.Terminate()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	clientResult := <-done
	if clientResult.err != nil {
		t.Fatal(clientResult.err)
	}
	t.Cleanup(clientResult.client.Close)

	if len(c.backgroundEffects) != 1 {
		t.Fatalf("background effects = %d, want one surface effect", len(c.backgroundEffects))
	}
	for _, effect := range c.backgroundEffects {
		want := []rect{{-5, -4, 15, 14}, {20, 10, 20, 20}}
		if !reflect.DeepEqual(effect.committed, want) {
			t.Fatalf("committed blur region = %v, want copied region %v", effect.committed, want)
		}
		if effect.pendingSet {
			t.Fatal("pending blur region remained unapplied after wl_surface.commit")
		}
	}
}

func TestM10BackgroundEffectRegionIsClippedToSurface(t *testing.T) {
	got := clipSurfaceRegion([]rect{
		{-5, -4, 15, 14}, {20, 10, 20, 20}, {40, 5, 2, 2},
	}, 30, 20)
	want := []rect{{0, 0, 10, 10}, {20, 10, 10, 10}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("clipped blur region = %v, want %v", got, want)
	}
}

// TestWindowBackgroundEffectTakesRoundedGeometry: a window's blur is cut to
// its geometry and rounded where it meets the geometry's corners, with
// Niri's extra pixel of radius; square corners stay square.
func TestWindowBackgroundEffectTakesRoundedGeometry(t *testing.T) {
	geometry := rect{10, 5, 100, 80}
	radii := config.CornerRadius{20, 20, 0, 20}
	gotRects, gotRadii := roundedToGeometry([]rect{
		{0, 0, 130, 100}, // a whole CSD surface, shadow margins included
		{10, 45, 30, 50}, // the bottom-left corner only
		{50, 20, 20, 20}, // inside, touching no corner
		{0, 0, 5, 5},     // outside the geometry
	}, geometry, radii)
	wantRects := []rect{{10, 5, 100, 80}, {10, 45, 30, 40}, {50, 20, 20, 20}}
	wantRadii := []config.CornerRadius{{21, 21, 0, 21}, {0, 0, 0, 21}, {}}
	if !reflect.DeepEqual(gotRects, wantRects) || !reflect.DeepEqual(gotRadii, wantRadii) {
		t.Fatalf("rounded blur = %v %v, want %v %v", gotRects, gotRadii, wantRects, wantRadii)
	}
}

func TestBackgroundEffectReusesNodesAcrossBufferCommits(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	parent := newBackgroundEffectTestScene()
	if parent == nil {
		t.Fatal("creating scene")
	}
	defer sceneFXNodeDestroy(parent)
	anchor := sceneFXCreateRect(parent, 60, 40, [4]float32{1, 1, 1, 1})
	if anchor == nil {
		t.Fatal("creating anchor")
	}
	c := &Compositor{listeners: map[*Listener]struct{}{}}
	effect := &backgroundEffect{}
	region := []rect{{0, 0, 20, 20}, {30, 0, 10, 10}}
	square := make([]config.CornerRadius, len(region))
	c.syncBackgroundEffectNodes(effect, parent, anchor, 0, 0, region, square, true)
	if len(effect.blurNodes) != len(region) {
		t.Fatalf("created %d blur nodes, want %d", len(effect.blurNodes), len(region))
	}
	first, second := effect.blurGone[effect.blurNodes[0]], effect.blurGone[effect.blurNodes[1]]
	sameNodes := func() {
		t.Helper()
		if len(effect.blurNodes) != 2 || effect.blurGone[effect.blurNodes[0]] != first ||
			effect.blurGone[effect.blurNodes[1]] != second {
			t.Fatal("commit replaced the blur nodes")
		}
	}
	c.syncBackgroundEffectNodes(effect, parent, anchor, 0, 0, region, square, true)
	sameNodes()

	region[0].w = 14
	corners := []config.CornerRadius{{4, 0, 0, 0}, {}}
	c.syncBackgroundEffectNodes(effect, parent, anchor, 2, 3, region, corners, true)
	sameNodes()
	if got, want := sceneFXBlurBox(effect.blurNodes[0]), (rect{2, 3, 14, 20}); got != want {
		t.Fatalf("resized blur node = %+v, want %+v", got, want)
	}
	if got := sceneFXBlurTopLeftCorner(effect.blurNodes[0]); got != 4 {
		t.Fatalf("resized blur corner = %d, want 4", got)
	}
	c.syncBackgroundEffectNodes(effect, parent, anchor, 2, 3, region, square, true)
	if got := sceneFXBlurTopLeftCorner(effect.blurNodes[0]); got != 0 {
		t.Fatalf("removed blur corner = %d, want 0", got)
	}
}

// TestBackgroundEffectDrawsFrostUnderTheRegion: a mapped toplevel's blur
// region becomes one SceneFX blur node per rectangle, clipped to the surface
// and kept at the rectangles' offsets from each other; a rectangle outside
// the surface draws nothing. Blur `off` removes the frost and turning it
// back on draws it again.
func TestBackgroundEffectDrawsFrostUnderTheRegion(t *testing.T) {
	c := newNested(t)
	watchdog := time.AfterFunc(10*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mapped := make(chan struct{}, 1)
	c.viewMapped = func(*toplevelView) {
		select {
		case mapped <- struct{}{}:
		default:
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		select {
		case <-mapped:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		// The client's buffer is 200x150.
		if err := client.SetBlurRegion([4]int32{-5, -4, 15, 14}, [4]int32{20, 10, 20, 20},
			[4]int32{500, 500, 5, 5}); err != nil {
			t.Error(err)
			return
		}
		frost := func(label string, want int) {
			t.Helper()
			var effect *backgroundEffect
			for _, e := range c.backgroundEffects {
				effect = e
			}
			if effect == nil {
				t.Errorf("%s: no background effect recorded", label)
				return
			}
			if len(effect.blurNodes) != want {
				t.Errorf("%s: %d blur nodes, want %d", label, len(effect.blurNodes), want)
				return
			}
			if want == 0 {
				return
			}
			first, second := sceneFXBlurBox(effect.blurNodes[0]), sceneFXBlurBox(effect.blurNodes[1])
			if first.w != 10 || first.h != 10 || second.w != 20 || second.h != 20 {
				t.Errorf("%s: blur sizes %dx%d and %dx%d, want 10x10 (clipped) and 20x20",
					label, first.w, first.h, second.w, second.h)
			}
			if second.x-first.x != 20 || second.y-first.y != 10 {
				t.Errorf("%s: blur nodes at %+v and %+v, want the second 20,10 from the first", label, first, second)
			}
		}
		onCompositor(t, c, func() { frost("committed", 2) })
		onCompositor(t, c, func() {
			c.blurConfig.Off = true
			c.applyBlurConfig()
			frost("blur off", 0)
		})
		onCompositor(t, c, func() {
			c.blurConfig.Off = false
			c.applyBlurConfig()
			frost("blur back on", 2)
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestBackdropBlurReusesCacheUntilItsOutputChanges(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	scene := newBackgroundEffectTestScene()
	if scene == nil {
		t.Fatal("creating scene")
	}
	defer sceneFXNodeDestroy(scene)
	c := newBackdropBlurTestCompositor(scene)
	left, right := unsafe.Pointer(new(byte)), unsafe.Pointer(new(byte))
	leftBox, rightBox := image.Rect(0, 0, 80, 60), image.Rect(80, 0, 160, 60)

	a := c.syncBackdropBlur(left, leftBox)
	b := c.syncBackdropBlur(right, rightBox)
	if a == nil || b == nil || a == b || !a.stale || !b.stale {
		t.Fatal("each output needs a separate cache that starts stale")
	}
	if a.node.node.enabled || b.node.node.enabled {
		t.Fatal("an enabled cache lifts occlusion before any repaint needs it")
	}
	// prepareBackdropBlur clears stale when it starts a rebuild.
	a.stale, b.stale = false, false
	if c.syncBackdropBlur(left, leftBox) != a || a.stale {
		t.Fatal("a frame with the same backdrop reused neither its pixels nor its node")
	}
	c.invalidateBackdropBlur(right)
	if a.stale || !b.stale {
		t.Fatal("backdrop damage should mark only the affected cache")
	}
	if b.node.node.enabled || b.node.dirty {
		t.Fatal("invalidation touched the scene, which damages an output with no consumer")
	}
	if c.syncBackdropBlur(left, leftBox.Add(image.Pt(1, 0))) != a || !a.stale {
		t.Fatal("moving an output should invalidate its existing cache")
	}
	c.dropBackdropBlur(right)
	if c.blurBackdrops[right] != nil {
		t.Fatal("retiring an output kept its cache node")
	}
}

// A background layer's popup or shadow can draw on a neighbouring output, so
// the bounds that pick which caches to rebuild cover every enabled node.
func TestBackdropTreeBoundsCoverShadowsAndSkipDisabledNodes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	scene := newBackgroundEffectTestScene()
	if scene == nil {
		t.Fatal("creating scene")
	}
	defer sceneFXNodeDestroy(scene)
	surface := sceneFXCreateRect(scene, 20, 20, [4]float32{0, 0, 0, 1})
	sceneFXNodeSetPosition(surface, 70, 10)
	shadow := sceneFXCreateShadow(scene, 30, 30, 0, 5, [4]float32{0, 0, 0, 1})
	sceneFXNodeSetPosition(shadow, 65, 5)
	if got, want := backdropTreeBounds(scene), image.Rect(65, 5, 95, 35); got != want {
		t.Fatalf("bounds = %v, want %v with the shadow", got, want)
	}
	sceneFXSetNodeEnabled(shadow, false)
	if got, want := backdropTreeBounds(scene), image.Rect(70, 10, 90, 30); got != want {
		t.Fatalf("bounds = %v, want %v without the disabled shadow", got, want)
	}
	sceneFXTreeSetEnabled(scene, false)
	if got := backdropTreeBounds(scene); !got.Empty() {
		t.Fatalf("a disabled tree draws nothing, got %v", got)
	}
}
