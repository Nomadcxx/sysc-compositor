package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server xkbcommon
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <xkbcommon/xkbcommon.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard_group.h>
#include <wlr/types/wlr_seat.h>

// Niri's defaults (niri-config input.rs): repeat after 600 ms, 25 per second.
// Clients repeat for themselves from these; the compositor sends one key per
// physical press (design §7).
static const int sysc_repeat_rate = 25;
static const int sysc_repeat_delay = 600;

// sysc_keyboard_group_create makes the one keyboard the user seat hears. Every
// physical keyboard joins it, so a key from any of them reaches the focused
// client. Empty rule names make xkbcommon use its defaults and the
// XKB_DEFAULT_* environment until KDL supplies xkb settings (M5).
static struct wlr_keyboard_group *sysc_keyboard_group_create(void) {
	struct wlr_keyboard_group *group = wlr_keyboard_group_create();
	if (group == NULL) {
		return NULL;
	}
	struct xkb_context *ctx = xkb_context_new(XKB_CONTEXT_NO_FLAGS);
	struct xkb_rule_names names = {0};
	struct xkb_keymap *keymap = ctx == NULL ? NULL :
		xkb_keymap_new_from_names(ctx, &names, XKB_KEYMAP_COMPILE_NO_FLAGS);
	bool ok = keymap != NULL && wlr_keyboard_set_keymap(&group->keyboard, keymap);
	xkb_keymap_unref(keymap);
	xkb_context_unref(ctx);
	if (!ok) {
		wlr_keyboard_group_destroy(group);
		return NULL;
	}
	wlr_keyboard_set_repeat_info(&group->keyboard, sysc_repeat_rate, sysc_repeat_delay);
	return group;
}

// A keyboard joins only with the group's keymap; a backend keyboard arrives
// with none.
static bool sysc_keyboard_group_join(struct wlr_keyboard_group *group, struct wlr_keyboard *kb) {
	wlr_keyboard_set_keymap(kb, group->keyboard.keymap);
	wlr_keyboard_set_repeat_info(kb, sysc_repeat_rate, sysc_repeat_delay);
	return wlr_keyboard_group_add_keyboard(group, kb);
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// startKeyboardGroup gives the user seat its one keyboard and listens to it.
func (c *Compositor) startKeyboardGroup() error {
	if c.keyboardGroup = C.sysc_keyboard_group_create(); c.keyboardGroup == nil {
		return errors.New("keyboard group: no xkb keymap could be compiled")
	}
	c.keyboard = &c.keyboardGroup.keyboard
	C.wlr_seat_set_keyboard(c.seat, c.keyboard)
	c.keep(
		AddListener(unsafe.Pointer(&c.keyboard.events.key), c.handleKeyboardKey),
		AddListener(unsafe.Pointer(&c.keyboard.events.modifiers), c.handleKeyboardModifiers),
	)
	return nil
}

func (c *Compositor) destroyKeyboardGroup() {
	if c.keyboardGroup != nil {
		C.wlr_keyboard_group_destroy(c.keyboardGroup)
		c.keyboardGroup = nil
		c.keyboard = nil
	}
}

// addKeyboard puts a backend keyboard into the group. The group drops it by
// itself when the device goes.
func (c *Compositor) addKeyboard(kb *C.struct_wlr_keyboard) bool {
	return bool(C.sysc_keyboard_group_join(c.keyboardGroup, kb))
}

// handleKeyboardKey delivers a key from any keyboard to the user seat's focus.
func (c *Compositor) handleKeyboardKey(data unsafe.Pointer) {
	event := (*C.struct_wlr_keyboard_key_event)(data)
	C.wlr_seat_keyboard_notify_key(c.seat, event.time_msec, event.keycode,
		C.uint32_t(event.state))
}

func (c *Compositor) handleKeyboardModifiers(unsafe.Pointer) {
	c.modifiers = uint32(C.wlr_keyboard_get_modifiers(c.keyboard))
	C.wlr_seat_keyboard_notify_modifiers(c.seat, &c.keyboard.modifiers)
}
