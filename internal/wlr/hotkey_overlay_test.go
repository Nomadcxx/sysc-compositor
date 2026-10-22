package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// TestKeyNameIsXKBCanonical is Niri's keysym parse: case-insensitive, with
// XF86Screensaver and XF86ScreenSaver told apart.
func TestKeyNameIsXKBCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"T": "t", "slash": "slash", "Slash": "slash", "print": "Print", "Page_Down": "Next",
		"XF86AudioRaiseVolume": "XF86AudioRaiseVolume", "XF86Screensaver": "XF86Screensaver",
		"xf86screensaver": "XF86ScreenSaver", "NotAKey": "NotAKey",
	} {
		if got := keyName(in); got != want {
			t.Errorf("keyName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestHotkeyOverlayShowsAtStartupWithoutTakingInput: the overlay opens at
// startup over every output, the pointer still reaches the window under
// it, and the next key or button press closes it.
func TestHotkeyOverlayShowsAtStartupWithoutTakingInput(t *testing.T) {
	cfg, err := config.Parse("overlay.kdl", []byte("binds {\n    Mod+Shift+Slash { show-hotkey-overlay; }\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	c := newNestedWith(t, Options{Binds: cfg.Binds, OverlayBinds: cfg.OverlayBinds, ShowHotkeyOverlayAtStartup: true})
	lockTestWindow(t, c, func(window *toplevelView) {
		var keyboard *fakeKeyboard
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			keyboard.remove()
			pointer.remove()
		})
		onCompositor(t, c, func() {
			keyboard = c.addFakeKeyboard()
			pointer = c.addFakePointer()
			if !c.HotkeyOverlayOpen() || c.hotkeyOverlay.tree == nil {
				t.Fatal("the hotkey overlay is not up at startup")
			}
			drawn := false
			for _, img := range c.hotkeyOverlay.panels {
				drawn = drawn || img != nil
			}
			if !drawn {
				t.Error("no overlay panel was drawn")
			}
			x, y := c.viewCenter(window)
			if surface, _, _ := c.surfaceAt(x, y); c.viewForSurface(surface) != window {
				t.Error("the overlay hides the window from the pointer")
			}
			keyboard.key(keyA, true)
			keyboard.key(keyA, false)
			if c.HotkeyOverlayOpen() || c.hotkeyOverlay.tree != nil {
				t.Fatal("a key press did not close the overlay")
			}

			c.ShowHotkeyOverlay()
			if !c.HotkeyOverlayOpen() || c.hotkeyOverlay.tree == nil {
				t.Fatal("ShowHotkeyOverlay did not open it")
			}
			pointer.moveTo(x, y)
			pointer.button(btnLeft, true)
			pointer.button(btnLeft, false)
			if c.HotkeyOverlayOpen() {
				t.Error("a button press did not close the overlay")
			}
		})
	})
}
