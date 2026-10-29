package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const evdevButtonStylus = 0x14b

// viewCenter is the middle of a view's window geometry in layout coordinates.
func viewCenter(view *toplevelView) (float64, float64) {
	x, y := view.treePosition()
	g := view.geometry()
	return float64(x + g.w/2), float64(y + g.h/2)
}

// runTabletClients maps one toplevel per entry of bind (true: it binds the
// user seat's tablets), then runs steps with the clients and their views.
func runTabletClients(t *testing.T, c *Compositor, bind []bool,
	steps func(clients []*testclient.ToplevelClient, views []*toplevelView)) {
	t.Helper()
	watchdog := time.AfterFunc(15*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mapped := make(chan *toplevelView, len(bind))
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
		for _, tablets := range bind {
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
			if tablets {
				if err := client.BindTablet(); err != nil {
					t.Error(err)
					return
				}
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

func syncTablet(t *testing.T, client *testclient.ToplevelClient) testclient.TabletEvents {
	t.Helper()
	if err := client.SyncInput(); err != nil {
		t.Fatal(err)
	}
	return client.TabletEvents()
}

// TestTabletToolFocusButtonsAndRemoval: a tablet-aware window gets the pen's
// proximity, motion, pressure, tip and buttons, with no pointer emulation;
// the tip focuses it, as Niri's does; the pad enters with the pen and
// delivers its buttons; both leave on proximity-out and retire when
// unplugged.
func TestTabletToolFocusButtonsAndRemoval(t *testing.T) {
	c := newNested(t)
	runTabletClients(t, c, []bool{false, true}, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		aware := clients[1]
		var pen *fakeTablet
		var pad *fakePad
		var x, y float64
		onCompositor(t, c, func() {
			pen, pad = c.addFakeTablet(0, 0), c.addFakePad(4)
			c.focusView(views[0])
			x, y = viewCenter(views[1])
		})
		if pen == nil || pad == nil {
			t.Fatal("fake tablet devices unavailable")
		}
		if ev := syncTablet(t, aware); ev.TabletsAdded != 1 || ev.PadsAdded != 1 {
			t.Fatalf("seat announced %+v, want one tablet and one pad", ev)
		}
		onCompositor(t, c, func() {
			pen.proximity(x, y, true)
			pen.moveTo(x+4, y+4)
		})
		ev := syncTablet(t, aware)
		if ev.ToolsAdded != 1 || ev.ProximityIns != 1 || !ev.ToolOnToplevel || ev.Motions == 0 || ev.Pressures == 0 {
			t.Fatalf("pen over the window: %+v", ev)
		}
		if ev.PadEnters != 1 || !ev.PadOnToplevel {
			t.Fatalf("pad did not enter with the pen: %+v", ev)
		}
		var focused bool
		onCompositor(t, c, func() {
			pen.tip(true)
			focused = c.keyboardFocusedView() == views[1]
			pen.moveTo(x+8, y+8)
			pen.button(evdevButtonStylus, true)
			pen.button(evdevButtonStylus, false)
			pad.button(0, true)
			pad.button(0, false)
			pen.tip(false)
		})
		ev = syncTablet(t, aware)
		if !focused {
			t.Error("the tip did not focus the window under the pen")
		}
		if ev.Downs != 1 || ev.Ups != 1 || ev.Buttons != 2 || ev.PadButtons != 2 {
			t.Errorf("stroke: %+v, want one down and up, two pen and two pad buttons", ev)
		}
		if n := aware.InputEvents().PointerButtons; n != 0 {
			t.Errorf("tablet-aware window also got %d pointer buttons", n)
		}
		onCompositor(t, c, func() { pen.proximity(x, y, false) })
		if ev = syncTablet(t, aware); ev.ProximityOuts != 1 || ev.PadLeaves != 1 || ev.ToolOnToplevel || ev.PadOnToplevel {
			t.Errorf("proximity out: %+v", ev)
		}
		onCompositor(t, c, func() {
			pad.remove()
			pen.remove()
		})
		if ev = syncTablet(t, aware); ev.PadsRemoved != 1 || ev.TabletsRemoved != 1 {
			t.Errorf("unplugging: %+v, want the pad and tablet removed", ev)
		}
	})
}

// TestTabletEmulatesPointerForPlainClients: a client without tablet-v2 gets
// the tip as a left click, as Mango and Hyprland do; while a compositor UI
// takes the pointer, a tablet-aware window under it gets nothing and the tip
// acts on the UI.
func TestTabletEmulatesPointerForPlainClients(t *testing.T) {
	c := newNested(t)
	runTabletClients(t, c, []bool{false, true}, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		plain, aware := clients[0], clients[1]
		var pen *fakeTablet
		onCompositor(t, c, func() { pen = c.addFakeTablet(0, 0) })
		if pen == nil {
			t.Fatal("fake tablet unavailable")
		}
		// With no other pointer device the tablet gives the seat its pointer:
		// one roundtrip delivers the capability, the next binds wl_pointer.
		for i := 0; i < 2; i++ {
			if err := plain.SyncInput(); err != nil {
				t.Fatal(err)
			}
		}
		onCompositor(t, c, func() {
			x, y := viewCenter(views[0])
			pen.proximity(x, y, true)
			pen.tip(true)
			pen.tip(false)
			pen.proximity(x, y, false)
		})
		if err := plain.SyncInput(); err != nil {
			t.Fatal(err)
		}
		if n := plain.InputEvents().PointerButtons; n != 2 {
			t.Errorf("plain window got %d pointer buttons from the tip, want press and release", n)
		}
		// The colour picker draws nothing over the windows, so only the modal
		// check keeps the pen from the tablet-aware one under it.
		picked := false
		var pickErr error
		onCompositor(t, c, func() {
			if _, pickErr = c.PickColor(func([3]float64, error) { picked = true }); pickErr != nil {
				return
			}
			x, y := viewCenter(views[1])
			pen.proximity(x, y, true)
			pen.moveTo(x+2, y+2)
			pen.tip(true)
			pen.tip(false)
			pen.proximity(x, y, false)
		})
		if pickErr != nil {
			t.Fatal(pickErr)
		}
		if ev := syncTablet(t, aware); ev.ProximityIns != 0 || ev.Downs != 0 {
			t.Errorf("tablet-aware window got pen events through the colour picker: %+v", ev)
		}
		if !picked {
			t.Error("the tip did not pick a colour")
		}
		onCompositor(t, c, pen.remove)
	})
}

// TestTabletRespectsLockAndInjectedSeats: tablets join only the user seat,
// and while the session is locked the window under the pen gets nothing.
func TestTabletRespectsLockAndInjectedSeats(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("remote"); err != nil {
		t.Fatal(err)
	}
	runTabletClients(t, c, []bool{true}, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		aware := clients[0]
		if err := aware.BindOtherSeatTablet(); err != nil {
			t.Fatal(err)
		}
		var pen *fakeTablet
		var pad *fakePad
		onCompositor(t, c, func() { pen, pad = c.addFakeTablet(0, 0), c.addFakePad(2) })
		if pen == nil || pad == nil {
			t.Fatal("fake tablet devices unavailable")
		}
		if ev := syncTablet(t, aware); ev.TabletsAdded != 1 || ev.OtherSeatObjects != 0 {
			t.Fatalf("user seat tablets %d, injected seat objects %d", ev.TabletsAdded, ev.OtherSeatObjects)
		}
		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		onCompositor(t, c, func() {
			x, y := viewCenter(views[0])
			pen.proximity(x, y, true)
			pen.moveTo(x+2, y+2)
			pen.tip(true)
			pen.tip(false)
			pad.button(0, true)
			pen.proximity(x, y, false)
		})
		ev := syncTablet(t, aware)
		if ev.ProximityIns != 0 || ev.Downs != 0 || ev.PadEnters != 0 || ev.PadButtons != 0 {
			t.Errorf("locked session leaked pen input to the window: %+v", ev)
		}
		if n := aware.InputEvents().PointerButtons; n != 0 {
			t.Errorf("locked session leaked %d emulated buttons to the window", n)
		}
		onCompositor(t, c, func() {
			pad.remove()
			pen.remove()
		})
	})
}
