package wlr

import (
	"reflect"
	"testing"
	"time"

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
