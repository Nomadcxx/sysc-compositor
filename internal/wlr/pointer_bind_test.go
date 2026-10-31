package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

const (
	buttonLeft  = 0x110
	buttonRight = 0x111
)

func TestClientCursorSurfaceDestroyDropsListener(t *testing.T) {
	c := newNested(t)
	var pointer *fakePointer
	var cursorListener *Listener
	cursorWasSet := make(chan bool, 1)
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach the test pointer")
			c.Terminate()
			return
		}
		pointer.moveTo(c.viewCenter(view))
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := c.Post(func() {
				cursorListener = c.cursorSurfaceDestroy
				cursorWasSet <- c.cursorClientSet && cursorListener != nil
			}); err != nil {
				cursorWasSet <- false
			}
		}()
	}
	runClient(t, c, []string{"sh", "-c", `weston-terminal & client=$!; sleep 1; kill "$client"; wait "$client"; sleep 1`}, nil, nil)
	if !<-cursorWasSet {
		t.Fatal("the terminal did not set its cursor surface")
	}
	if c.cursorClientSet || c.cursorSurfaceDestroy != nil {
		t.Fatalf("destroyed client cursor is still tracked: set=%v listener=%p", c.cursorClientSet, c.cursorSurfaceDestroy)
	}
	if _, exists := c.listeners[cursorListener]; exists {
		t.Fatal("listener for the destroyed client cursor surface remains registered")
	}
}

func TestNestedModPointerGrabsUseEdgeResizeAndCursorImages(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	var pointer *fakePointer
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		keyboard = c.addFakeKeyboard()
		pointer = c.addFakePointer()
		if keyboard == nil || pointer == nil {
			t.Error("could not attach fake input devices")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		if !c.modifierActive(c.modKey) {
			t.Error("nested Alt is not active as the compositor Mod key")
		}
		x, y := view.treePosition()
		pointer.moveTo(float64(x+1), float64(y+1))
		if got := c.viewAtCursor(); got != view {
			t.Errorf("view under pointer = %p, want %p at cursor (%f,%f)", got, view, c.cursor.x, c.cursor.y)
		}
		pointer.button(buttonRight, true)
		if c.pointerGrab == nil || c.pointerGrab.mode != grabResize {
			t.Error("nested Mod+RMB did not begin a resize grab")
		} else {
			if c.pointerGrab.edges != edgeLeft|edgeTop {
				t.Errorf("resize edges = %b, want left+top", c.pointerGrab.edges)
			}
			if c.drawnImage != "nw-resize" {
				t.Errorf("resize cursor image = %q, want nw-resize", c.drawnImage)
			}
		}
		pointer.button(buttonRight, false)
		if c.pointerGrab != nil {
			t.Error("resize grab remained active after button release")
		}
		if c.drawnImage != "default" {
			t.Errorf("cursor after resize = %q, want default", c.drawnImage)
		}
		pointer.moveTo(c.viewCenter(view))
		pointer.button(buttonLeft, true)
		if c.pointerGrab == nil || c.pointerGrab.mode != grabMove || c.drawnImage != "grabbing" {
			t.Errorf("Mod+LMB grab = (%+v, %q), want move and grabbing cursor", c.pointerGrab, c.drawnImage)
		}
		pointer.button(buttonLeft, false)
		keyboard.key(c.modKeycode(), false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if len(c.Windows()) != 1 {
		t.Fatalf("mapped windows = %d, want one", len(c.Windows()))
	}
}

func TestModDragPersistsFloatingWindowPositionOnRelease(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	var pointer *fakePointer
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		keyboard = c.addFakeKeyboard()
		pointer = c.addFakePointer()
		if keyboard == nil || pointer == nil {
			t.Error("could not attach fake input devices")
			c.Terminate()
			return
		}
		if !c.SetWindowFloating(view.id, true) {
			t.Error("SetWindowFloating() = false, want mapped window floating")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		pointer.moveTo(c.viewCenter(view))
		pointer.button(buttonLeft, true)
		if c.pointerGrab == nil || c.pointerGrab.mode != grabMove {
			t.Error("Mod+LMB did not start a move grab")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(int(c.cursor.x)+80), float64(int(c.cursor.y)+60))
		wantX, wantY := view.treePosition()
		pointer.button(buttonLeft, false)
		if c.pointerGrab != nil {
			t.Error("move grab remained active after release")
		}

		var window *wm.WindowSnapshot
		for _, candidate := range c.WorkspaceSnapshot().Windows {
			if candidate.ID == view.id {
				window = &candidate
				break
			}
		}
		if window == nil {
			t.Fatal("moved window missing from workspace snapshot")
		}
		if !window.Tile.Floating || window.Tile.X != wantX || window.Tile.Y != wantY {
			t.Errorf("floating model position = (%d,%d), want released scene position (%d,%d)",
				window.Tile.X, window.Tile.Y, wantX, wantY)
		}
		c.applyWorkspaceModel()
		if gotX, gotY := view.treePosition(); gotX != wantX || gotY != wantY {
			t.Errorf("scene position after model reapply = (%d,%d), want (%d,%d)", gotX, gotY, wantX, wantY)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

const buttonMiddle = 0x112 // BTN_MIDDLE

// pointerBindHarness maps one toplevel under binds, puts a fake pointer over
// it and a fake keyboard on the seat, then runs fn off the compositor thread.
func pointerBindHarness(t *testing.T, binds string, fn func(c *Compositor, client *testclient.ToplevelClient,
	pointer *fakePointer, keyboard *fakeKeyboard, view *toplevelView)) {
	t.Helper()
	cfg, err := config.Parse("pointer-binds.kdl", []byte(binds))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{Binds: cfg.Binds})
	watchdog := time.AfterFunc(15*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mapped := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case mapped <- view:
		default:
		}
	}
	go func() {
		defer c.Terminate()
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		var view *toplevelView
		select {
		case view = <-mapped:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		var pointer *fakePointer
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() {
			pointer.remove()
			keyboard.remove()
		})
		onCompositor(t, c, func() {
			pointer, keyboard = c.addFakePointer(), c.addFakeKeyboard()
			pointer.moveTo(c.viewCenter(view))
		})
		if !client.WaitPointerEnter(2 * time.Second) {
			t.Error("the pointer never entered the toplevel")
			return
		}
		fn(c, client, pointer, keyboard, view)
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}

func focusedWorkspaceID(c *Compositor) uint64 {
	for _, workspace := range c.WorkspaceSnapshot().Workspaces {
		if workspace.IsFocused {
			return workspace.ID
		}
	}
	return 0
}

func TestWheelBindsInterceptOnlyBoundModifiers(t *testing.T) {
	pointerBindHarness(t, `
binds {
    Ctrl+WheelScrollDown cooldown-ms=150 { focus-workspace-down; }
    Ctrl+WheelScrollUp cooldown-ms=150 { focus-workspace-up; }
    Ctrl+WheelScrollRight { focus-column-right; }
}
`, func(c *Compositor, client *testclient.ToplevelClient, pointer *fakePointer, keyboard *fakeKeyboard, view *toplevelView) {
		var first uint64
		axes := func() int {
			if err := client.SyncInput(); err != nil {
				t.Error(err)
			}
			return client.InputEvents().PointerAxes
		}
		before := axes()
		onCompositor(t, c, func() {
			first = focusedWorkspaceID(c)
			keyboard.key(keyLeftCtrl, true)
			pointer.wheelAt(1000, false, 120)
			if focusedWorkspaceID(c) == first {
				t.Error("Ctrl+wheel down did not switch the workspace")
			}
			pointer.wheelAt(1050, false, -120)
			if focusedWorkspaceID(c) != first {
				t.Error("Ctrl+wheel up did not switch back")
			}
			pointer.wheelAt(1100, false, 120)
			if focusedWorkspaceID(c) != first {
				t.Error("a wheel down 100 ms after the last one ran inside its 150 ms cooldown")
			}
			pointer.wheelAt(1200, false, 120)
			if focusedWorkspaceID(c) == first {
				t.Error("a wheel down 200 ms after the last one did not run")
			}
			pointer.wheelAt(1300, false, -120)
			pointer.wheelAt(1400, false, 60)
			pointer.wheelAt(1410, false, 60)
			if focusedWorkspaceID(c) == first {
				t.Error("two half detents did not add up to one tick")
			}
			pointer.wheelAt(1600, false, -120)
			keyboard.key(keyLeftCtrl, false)
		})
		if got := axes(); got != before {
			t.Errorf("the client got %d axis events from bound Ctrl+wheel scrolls, want 0", got-before)
		}
		onCompositor(t, c, func() {
			pointer.moveTo(c.viewCenter(view))
			pointer.wheelAt(2000, false, 120)
		})
		if got := axes(); got == before {
			t.Error("an unmodified wheel never reached the client")
		}
		before = axes()
		onCompositor(t, c, func() {
			keyboard.key(keyLeftShift, true)
			pointer.wheelAt(2100, false, 120)
			keyboard.key(keyLeftShift, false)
		})
		if got := axes(); got == before {
			t.Error("Shift+wheel, which no bind uses, never reached the client")
		}
	})
}

func TestMouseBindSuppressesItsRelease(t *testing.T) {
	pointerBindHarness(t, `
binds {
    Ctrl+MouseMiddle { close-window; }
}
`, func(c *Compositor, client *testclient.ToplevelClient, pointer *fakePointer, keyboard *fakeKeyboard, _ *toplevelView) {
		buttons := func() int {
			if err := client.SyncInput(); err != nil {
				t.Error(err)
			}
			return client.InputEvents().PointerButtons
		}
		before := buttons()
		onCompositor(t, c, func() {
			keyboard.key(keyLeftCtrl, true)
			pointer.button(buttonMiddle, true)
			keyboard.key(keyLeftCtrl, false)
			pointer.button(buttonMiddle, false)
		})
		if got := buttons(); got != before {
			t.Errorf("the client got %d button events from a Ctrl+middle bind, want none", got-before)
		}
		if !client.CloseRequested() {
			t.Error("Ctrl+MouseMiddle did not run close-window")
		}
		onCompositor(t, c, func() {
			pointer.button(buttonMiddle, true)
			pointer.button(buttonMiddle, false)
		})
		if got := buttons(); got != before+2 {
			t.Errorf("a plain middle click gave the client %d button events, want 2", got-before)
		}
	})
}

func TestTouchpadScrollBindsUseFingerTicks(t *testing.T) {
	pointerBindHarness(t, `
binds {
    Ctrl+TouchpadScrollDown { focus-workspace-down; }
}
`, func(c *Compositor, client *testclient.ToplevelClient, pointer *fakePointer, keyboard *fakeKeyboard, _ *toplevelView) {
		onCompositor(t, c, func() {
			first := focusedWorkspaceID(c)
			keyboard.key(keyLeftCtrl, true)
			pointer.fingerScrollAt(1000, 4)
			pointer.fingerScrollAt(1010, 4)
			if focusedWorkspaceID(c) != first {
				t.Error("8 units of finger scroll ran a bind before Niri's 10-unit tick")
			}
			pointer.fingerScrollAt(1020, 4)
			if focusedWorkspaceID(c) == first {
				t.Error("12 units of finger scroll did not run the bind")
			}
			keyboard.key(keyLeftCtrl, false)
		})
	})
}
