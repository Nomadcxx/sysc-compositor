package wlr

import (
	"fmt"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

const (
	keyF11 = 87 // evdev KEY_F11
	keyF12 = 88 // evdev KEY_F12
)

// TestShortcutsInhibitPassesBindsToTheFocusedClient: while the focused
// toplevel inhibits shortcuts, a normal bind does not run and an
// allow-inhibiting=false bind still does; ToggleKeyboardShortcutsInhibit
// turns the inhibitor off, and then the normal bind runs again.
func TestShortcutsInhibitPassesBindsToTheFocusedClient(t *testing.T) {
	exempt := keyBind(t, "F11", bind.ActionSpawn, "exempt")
	exempt.IgnoresInhibit = true
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "F12", bind.ActionSpawn, "normal"), exempt,
	}})
	var spawned []string
	c.SetSpawnHandlers(func(argv []string) error {
		spawned = append(spawned, argv[0])
		return nil
	}, nil)
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
		if err := client.InhibitShortcuts(); err != nil {
			t.Error(err)
			return
		}
		var keyboard *fakeKeyboard
		defer onCompositor(t, c, func() {
			if keyboard != nil {
				keyboard.remove()
			}
		})
		press := func(key uint32) {
			keyboard.key(key, true)
			keyboard.key(key, false)
		}
		onCompositor(t, c, func() {
			keyboard = c.addFakeKeyboard()
			c.focusView(view)
			if !c.shortcutsInhibited() {
				t.Error("the focused toplevel's inhibitor is not active")
			}
			press(keyF12)
			press(keyF11)
			if got := fmt.Sprint(spawned); got != "[exempt]" {
				t.Errorf("binds run while inhibited = %s, want only the allow-inhibiting=false one", got)
			}
			c.ToggleKeyboardShortcutsInhibit()
			if c.shortcutsInhibited() {
				t.Error("ToggleKeyboardShortcutsInhibit left the inhibitor active")
			}
			press(keyF12)
			if got := fmt.Sprint(spawned); got != "[exempt normal]" {
				t.Errorf("binds run after the toggle = %s, want the normal bind too", got)
			}
			c.ToggleKeyboardShortcutsInhibit()
			if !c.shortcutsInhibited() {
				t.Error("a second ToggleKeyboardShortcutsInhibit did not restore the inhibitor")
			}
		})
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
