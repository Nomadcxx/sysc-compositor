package wlr

import (
	"reflect"
	"testing"

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
