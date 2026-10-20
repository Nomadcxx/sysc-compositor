package wlr

import (
	"testing"
	"time"

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
		keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
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
		keyboard.key(56, false)
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
		keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
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
