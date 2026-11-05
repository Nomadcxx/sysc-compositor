package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// seatCapabilityTouch is wl_seat's touch capability bit.
const seatCapabilityTouch = 4

// runTouchClients maps one toplevel per client and runs steps with the
// clients and their views. No client binds touch: a test decides which one
// asks for it.
func runTouchClients(t *testing.T, c *Compositor, count int,
	steps func(clients []*testclient.ToplevelClient, views []*toplevelView)) {
	t.Helper()
	watchdog := time.AfterFunc(15*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mapped := make(chan *toplevelView, count)
	c.viewMapped = func(view *toplevelView) {
		select {
		case mapped <- view:
		default:
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		var clients []*testclient.ToplevelClient
		var views []*toplevelView
		for i := 0; i < count; i++ {
			client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				t.Error(err)
				return
			}
			defer client.Close()
			select {
			case view := <-mapped:
				views = append(views, view)
			case <-time.After(2 * time.Second):
				t.Error("a toplevel never mapped")
				return
			}
			clients = append(clients, client)
		}
		steps(clients, views)
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}

// touchDevicePoint is the position over the glass a finger must report to
// land on a layout point. It reads the output's box, so it belongs on the
// compositor thread.
func touchDevicePoint(c *Compositor, lx, ly float64) (float64, float64) {
	box, transform := c.touchOutputBox()
	if box.w == 0 || box.h == 0 {
		return 0, 0
	}
	tx := (lx - float64(box.x)) / float64(box.w)
	ty := (ly - float64(box.y)) / float64(box.h)
	return touchInverseTransformPoint(tx, ty, transform)
}

// TestTouchReachesTheClientThatAskedForIt: a finger on a window reaches the
// client that asked for touch, as Niri's does, with no mouse emulation; a
// client that never asked for touch gets nothing.
func TestTouchReachesTheClientThatAskedForIt(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 2, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		plain, aware := clients[0], clients[1]
		var screen *fakeTouch
		var dx, dy float64
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			c.focusView(views[1])
			lx, ly := viewCenter(views[1])
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		if screen == nil {
			t.Fatal("the fake touch screen did not appear")
		}
		if err := aware.BindTouch(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			screen.down(1, dx, dy)
			screen.frame()
			screen.motion(1, dx+0.01, dy)
			screen.frame()
			screen.up(1)
			screen.frame()
		})
		if err := aware.SyncInput(); err != nil {
			t.Fatal(err)
		}
		ev := aware.TouchEvents()
		if ev.Downs != 1 || ev.Ups != 1 || ev.Motions != 1 {
			t.Errorf("finger on the window: %+v, want one down, one motion and one up", ev)
		}
		if !ev.OnToplevel {
			t.Errorf("the finger did not land on the toplevel: %+v", ev)
		}
		if ev.ID != 1 {
			t.Errorf("touch id %d, want 1", ev.ID)
		}
		if ev.Frames == 0 {
			t.Errorf("the client got no touch frame: %+v", ev)
		}
		if n := aware.InputEvents().PointerButtons; n != 0 {
			t.Errorf("touch produced %d emulated pointer buttons, want none", n)
		}
		if err := plain.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := plain.TouchEvents(); ev.Downs != 0 || ev.Ups != 0 {
			t.Errorf("a client that never asked for touch got %+v", ev)
		}
		if n := plain.InputEvents().PointerButtons; n != 0 {
			t.Errorf("a client that never asked for touch got %d emulated pointer buttons", n)
		}
		onCompositor(t, c, func() { screen.remove() })
	})
}

// TestTouchOffKeepsTheSeatSilent: input.touch off leaves the seat without
// the touch capability, and no finger reaches anything.
func TestTouchOffKeepsTheSeatSilent(t *testing.T) {
	c := newNestedWith(t, Options{Touch: config.Touch{Off: true}})
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		client := clients[0]
		var screen *fakeTouch
		var dx, dy float64
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			lx, ly := viewCenter(views[0])
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		if screen == nil {
			t.Fatal("the fake touch screen did not appear")
		}
		var caps int
		onCompositor(t, c, func() {
			caps = c.SeatCapabilities()
			screen.down(1, dx, dy)
			screen.up(1)
			screen.frame()
		})
		if caps&seatCapabilityTouch != 0 {
			t.Errorf("the seat advertises touch while input.touch is off: %d", caps)
		}
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Downs != 0 || ev.Ups != 0 {
			t.Errorf("a finger reached the window while input.touch is off: %+v", ev)
		}
		onCompositor(t, c, func() { screen.remove() })
	})
}

// TestTouchReachesNothingWhileTheSessionIsLocked: a sealed session keeps
// touch away from the windows, as it keeps the keyboard away.
func TestTouchReachesNothingWhileTheSessionIsLocked(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		client := clients[0]
		var screen *fakeTouch
		var dx, dy float64
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			lx, ly := viewCenter(views[0])
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		if screen == nil {
			t.Fatal("the fake touch screen did not appear")
		}
		if err := client.BindTouch(); err != nil {
			t.Fatal(err)
		}
		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		onCompositor(t, c, func() {
			screen.down(1, dx, dy)
			screen.up(1)
			screen.frame()
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Downs != 0 || ev.Ups != 0 {
			t.Errorf("locked session leaked touch input to the window: %+v", ev)
		}
		onCompositor(t, c, func() { screen.remove() })
	})
}

// TestSessionLockEndsAnActiveTouch: locking ends a finger that started before
// the session was sealed, and later motion never reaches the application.
func TestSessionLockEndsAnActiveTouch(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		client := clients[0]
		var screen *fakeTouch
		var dx, dy float64
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			lx, ly := c.viewCenter(views[0])
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		if screen == nil {
			t.Fatal("the fake touch screen did not appear")
		}
		if err := client.BindTouch(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			screen.down(1, dx, dy)
			screen.frame()
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Downs != 1 {
			t.Fatalf("pre-lock touch events = %+v, want one down", ev)
		}

		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		onCompositor(t, c, func() {
			screen.motion(1, dx+0.01, dy)
			screen.frame()
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Ups != 1 || ev.Motions != 0 {
			t.Errorf("post-lock touch events = %+v, want one up and no motion", ev)
		}
		onCompositor(t, c, func() { screen.remove() })
	})
}

// TestModalOpeningEndsAnActiveTouch: opening a modal UI ends a finger already
// delivered to a window, and later motion never reaches that window.
func TestModalOpeningEndsAnActiveTouch(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		client := clients[0]
		var screen *fakeTouch
		var dx, dy float64
		onCompositor(t, c, func() {
			screen = c.addFakeTouch()
			lx, ly := c.viewCenter(views[0])
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		if screen == nil {
			t.Fatal("the fake touch screen did not appear")
		}
		if err := client.BindTouch(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			screen.down(1, dx, dy)
			screen.frame()
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Downs != 1 {
			t.Fatalf("pre-modal touch events = %+v, want one down", ev)
		}

		onCompositor(t, c, func() {
			if !c.OpenOverview() {
				t.Fatal("could not open overview")
			}
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Ups != 1 || ev.Motions != 0 {
			t.Fatalf("modal-open touch events = %+v, want one up and no motion", ev)
		}

		onCompositor(t, c, func() {
			screen.motion(1, dx+0.01, dy)
			screen.frame()
			screen.remove()
		})
		if err := client.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if ev := client.TouchEvents(); ev.Motions != 0 {
			t.Errorf("post-modal touch events = %+v, want no motion", ev)
		}
	})
}
