package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestTextInputRelaysToTheInputMethod: the focused toplevel's text input
// gets enter; enabling it activates the input method with its surrounding
// text; the input method's commit reaches the text input; and an
// input-method popup is drawn under the text cursor rectangle.
func TestTextInputRelaysToTheInputMethod(t *testing.T) {
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
		if err := client.StartTextInput(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { c.focusView(view) })
		if entered, _, _, _ := client.TextInputState(200 * time.Millisecond); !entered {
			t.Error("the focused toplevel's text input got no enter")
			return
		}
		if err := client.EnableTextInput("hello", 10, 20, 2, 16); err != nil {
			t.Error(err)
			return
		}
		if _, active, surrounding, _ := client.TextInputState(200 * time.Millisecond); !active || surrounding != "hello" {
			t.Errorf("input method active = %v with surrounding %q, want active with \"hello\"", active, surrounding)
		}
		if err := client.InputMethodCommit("é"); err != nil {
			t.Error(err)
			return
		}
		if _, _, _, committed := client.TextInputState(200 * time.Millisecond); committed != "é" {
			t.Errorf("text input committed %q, want the input method's \"é\"", committed)
		}

		if err := client.InputMethodPopup(40, 30); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			if len(c.inputPopups) != 1 {
				t.Errorf("input popups = %d, want 1", len(c.inputPopups))
				return
			}
			sx, sy, found := c.surfaceLayoutPosition(viewSurface(view))
			if !found {
				t.Error("the toplevel's surface is not in the scene")
				return
			}
			for _, p := range c.inputPopups {
				x, y := sceneTreePosition(p.tree)
				if x != sx+10 || y != sy+20+16 {
					t.Errorf("input popup at (%d,%d), want under the cursor rectangle at (%d,%d)", x, y, sx+10, sy+36)
				}
			}
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
