package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <wlr/interfaces/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard.h>

static const struct wlr_keyboard_impl sysc_fake_keyboard_impl = {
	.name = "sysc-fake-keyboard",
};

// A keyboard as a backend would announce one: no keymap, so the compositor
// has to give it one.
static struct wlr_keyboard *sysc_fake_keyboard_create(void) {
	struct wlr_keyboard *kb = calloc(1, sizeof(*kb));
	if (kb != NULL) {
		wlr_keyboard_init(kb, &sysc_fake_keyboard_impl, "sysc-fake-keyboard");
	}
	return kb;
}

static void sysc_fake_keyboard_destroy(struct wlr_keyboard *kb) {
	wlr_keyboard_finish(kb);
	free(kb);
}

static void sysc_fake_key(struct wlr_keyboard *kb, uint32_t time, uint32_t keycode, bool pressed) {
	struct wlr_keyboard_key_event event = {
		.time_msec = time,
		.keycode = keycode,
		.update_state = true,
		.state = pressed ? WL_KEYBOARD_KEY_STATE_PRESSED : WL_KEYBOARD_KEY_STATE_RELEASED,
	};
	wlr_keyboard_notify_key(kb, &event);
}
*/
import "C"

import "unsafe"

// fakeKeyboard is an in-process keyboard attached to the user seat the way a
// backend keyboard is, for tests: hardware keyboards cannot be pressed from a
// test, and the nested backend's one keyboard is the host's.
type fakeKeyboard struct{ kb *C.struct_wlr_keyboard }

func (c *Compositor) addFakeKeyboard() *fakeKeyboard {
	kb := C.sysc_fake_keyboard_create()
	if kb == nil {
		return nil
	}
	c.handleNewInput(unsafe.Pointer(&kb.base))
	return &fakeKeyboard{kb: kb}
}

func (f *fakeKeyboard) key(keycode uint32, pressed bool) {
	C.sysc_fake_key(f.kb, nowMsec(), C.uint32_t(keycode), C.bool(pressed))
}

func (f *fakeKeyboard) remove() {
	if f.kb != nil {
		C.sysc_fake_keyboard_destroy(f.kb)
		f.kb = nil
	}
}
