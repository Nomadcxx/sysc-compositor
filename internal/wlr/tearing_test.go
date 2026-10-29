package wlr

import (
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestTearingHintKeepsVsyncPresentation: a focused fullscreen window that asks
// for async presentation, which other compositors tear for after an opt-in,
// has its hint recorded, and its output keeps committing vsynced frames, as
// Niri's always are. Destroying the control drops the hint.
func TestTearingHintKeepsVsyncPresentation(t *testing.T) {
	c := newNested(t)
	if len(c.sceneOutputs) != 1 {
		t.Fatal("tearing check requires one initial output")
	}
	var output unsafe.Pointer
	for o := range c.sceneOutputs {
		output = o
	}
	var commits, torn atomic.Int32
	observer := observeTearingCommitsForTest(output, func(tore bool) {
		commits.Add(1)
		if tore {
			torn.Add(1)
		}
	})
	c.keep(observer)
	watchdog := time.AfterFunc(10*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	views := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case views <- view:
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
		var view *toplevelView
		select {
		case view = <-views:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		var fullscreen, focused bool
		onCompositor(t, c, func() {
			fullscreen = c.SetFullscreen(view.id, true)
			focused = c.keyboardFocusedView() == view
		})
		if !fullscreen || !focused {
			t.Errorf("fullscreen=%v focused=%v, want the focused fullscreen case", fullscreen, focused)
			return
		}
		if err := client.SetTearingHint(1); err != nil {
			t.Error(err)
			return
		}
		var hint uint32
		onCompositor(t, c, func() { hint = tearingHintForTest(c, view) })
		if hint != 1 {
			t.Errorf("recorded presentation hint %d, want async (1)", hint)
			return
		}
		// Each paint is its own frame: paints sent together share one commit.
		for i, color := range []uint32{0xffdc2828, 0xff28dc28, 0xff2828dc} {
			before := commits.Load()
			if err := client.Paint(color, 200+i, 150); err != nil {
				t.Error(err)
				return
			}
			deadline := time.Now().Add(2 * time.Second)
			for commits.Load() == before && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if commits.Load() == before {
				t.Errorf("paint %d produced no output commit", i)
				return
			}
		}
		if n := torn.Load(); n != 0 {
			t.Errorf("%d output commits tore for an async hint", n)
			return
		}
		if err := client.DestroyTearingControl(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { hint = tearingHintForTest(c, view) })
		if hint != 0 {
			t.Errorf("hint %d outlived its tearing control, want vsync (0)", hint)
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
