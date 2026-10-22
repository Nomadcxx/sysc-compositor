package wlr

import (
	"math"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestLockedPointerStaysAndReportsRelativeMotion: a toplevel under the
// pointer that locks it gets `locked`; user motion then leaves the pointer
// where it is and reaches the client as relative motion. Destroying the
// lock lets the pointer move again.
func TestLockedPointerStaysAndReportsRelativeMotion(t *testing.T) {
	c := newNested(t)
	watchdog := time.AfterFunc(10*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mappedViews := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case mappedViews <- view:
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
		case view = <-mappedViews:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
		})
		var x, y float64
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			x, y = c.viewCenter(view)
			pointer.moveTo(x, y)
		})
		if !client.WaitPointerEnter(2 * time.Second) {
			t.Error("the pointer never entered the toplevel")
			return
		}
		if err := client.LockPointer(); err != nil {
			t.Error(err)
			return
		}
		if !client.WaitPointerLocked(2 * time.Second) {
			t.Error("the pointer lock was never activated")
			return
		}
		onCompositor(t, c, func() {
			if c.activeConstraint == nil {
				t.Error("the compositor holds no active constraint")
			}
			pointer.moveTo(x+40, y+25)
			if math.Abs(float64(c.cursor.x)-x) > 0.5 || math.Abs(float64(c.cursor.y)-y) > 0.5 {
				t.Errorf("a locked pointer moved to (%v,%v), want it to stay at (%v,%v)", c.cursor.x, c.cursor.y, x, y)
			}
		})
		if dx, dy := client.RelativeMotion(300 * time.Millisecond); math.Abs(dx-40) > 1 || math.Abs(dy-25) > 1 {
			t.Errorf("relative motion = (%v,%v), want about (40,25)", dx, dy)
		}

		if err := client.UnlockPointer(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if c.activeConstraint != nil {
				t.Error("the destroyed lock is still active")
			}
			pointer.moveTo(x+40, y+25)
			if math.Abs(float64(c.cursor.x)-(x+40)) > 0.5 || math.Abs(float64(c.cursor.y)-(y+25)) > 0.5 {
				t.Errorf("the released pointer is at (%v,%v), want (%v,%v)", c.cursor.x, c.cursor.y, x+40, y+25)
			}
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
