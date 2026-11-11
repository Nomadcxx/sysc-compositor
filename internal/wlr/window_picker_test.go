package wlr

import (
	"math"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestWindowPickerReturnsWindowAndSuppressesItsClick(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		onCompositor(t, c, func() {
			pointer := c.addFakePointer()
			defer pointer.remove()
			x, y := viewCenter(views[0])
			pointer.moveTo(x, y)
			var got uint64
			calls := 0
			cancel, err := c.PickWindow(func(id uint64, err error) {
				calls++
				got = id
				if err != nil {
					t.Error(err)
				}
			})
			if err != nil {
				t.Error(err)
				return
			}
			pointer.button(evdevButtonLeft, true)
			pointer.button(evdevButtonLeft, false)
			cancel()
			if calls != 1 || got != views[0].id || c.pickWindow != nil {
				t.Errorf("pick id=%d calls=%d", got, calls)
			}
			first := 0
			old, err := c.PickWindow(func(id uint64, err error) {
				first++
				if id != 0 || err != nil {
					t.Error("replacement did not cancel with empty result")
				}
			})
			if err != nil {
				t.Error(err)
				return
			}
			second := 0
			current, err := c.PickWindow(func(id uint64, err error) { second++ })
			if err != nil {
				t.Error(err)
				return
			}
			old()
			if first != 1 || second != 0 || c.pickWindow == nil {
				t.Error("old cancel affected new picker")
			}
			current()
			current()
			if second != 1 || c.pickWindow != nil {
				t.Error("cancel did not finish exactly once")
			}
		})
		if err := clients[0].SyncInput(); err != nil {
			t.Fatal(err)
		}
		if got := clients[0].InputEvents().PointerButtons; got != 0 {
			t.Errorf("picker delivered %d button events to client", got)
		}
	})
}

func TestToggleRuleOpacityPreservesPresentationOpacity(t *testing.T) {
	c := newNested(t)
	opacity := .25
	c.windowRules = []config.WindowRule{{Opacity: &opacity}}
	runTouchClients(t, c, 1, func(_ []*testclient.ToplevelClient, views []*toplevelView) {
		onCompositor(t, c, func() {
			view := views[0]
			view.presentationOpacity = .5
			c.syncViewOpacity(view)
			check := func(want float64) {
				t.Helper()
				_, _, got, found := sceneFXTreeFirstBuffer(view.sceneNode())
				if !found || math.Abs(got-want) > .001 {
					t.Errorf("opacity=%g found=%t want=%g", got, found, want)
				}
			}
			check(.125)
			if !c.ToggleWindowRuleOpacity(&view.id) {
				t.Error("toggle did not resolve explicit window")
			}
			check(.5)
			if !c.ToggleWindowRuleOpacity(nil) {
				t.Error("toggle did not resolve focused window")
			}
			check(.125)
			id := view.id + 100
			if c.ToggleWindowRuleOpacity(&id) {
				t.Error("toggle resolved missing ID")
			}
		})
	})
}
