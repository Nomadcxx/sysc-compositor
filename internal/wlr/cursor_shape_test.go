package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const (
	cursorShapeText = 9  // wp_cursor_shape_device_v1.shape.text
	cursorShapeMove = 13 // wp_cursor_shape_device_v1.shape.move
)

// TestCursorShapeFollowsOnlyThePointersClient: the client the user seat's
// pointer is over draws a named theme image through cursor-shape-v1; once
// the pointer has left it, its requests change nothing.
func TestCursorShapeFollowsOnlyThePointersClient(t *testing.T) {
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
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
		})

		var view *toplevelView
		select {
		case view = <-mappedViews:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			pointer.moveTo(c.viewCenter(view))
		})
		if !client.WaitPointerEnter(2 * time.Second) {
			t.Error("the pointer never entered the toplevel")
			return
		}
		if err := client.SetCursorShape(cursorShapeText); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if c.drawnImage != "text" {
				t.Errorf("drawn pointer image = %q, want the client's text shape", c.drawnImage)
			}
		})

		// Away from every surface the default image is drawn, and the client
		// the pointer left cannot change it.
		onCompositor(t, c, func() {
			out := c.OutputSnapshot()[0].Logical
			pointer.moveTo(float64(out.X+int32(out.Width)-2), float64(out.Y+int32(out.Height)-2))
			if surface, _, _ := c.surfaceAt(float64(c.cursor.x), float64(c.cursor.y)); surface != nil {
				t.Error("the probe point is over a surface; the test needs empty space")
			}
		})
		if err := client.SetCursorShape(cursorShapeMove); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if c.drawnImage != "default" {
				t.Errorf("drawn pointer image = %q after a client the pointer left asked for a shape, want default", c.drawnImage)
			}
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
