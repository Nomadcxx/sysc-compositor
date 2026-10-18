package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server xkbcommon
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <xkbcommon/xkbcommon.h>
#include <xkbcommon/xkbcommon-keysyms.h>
#include <wlr/backend/session.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard_group.h>
#include <wlr/types/wlr_seat.h>

static size_t sysc_keyboard_layout_count(struct wlr_keyboard *keyboard) {
	return keyboard == NULL || keyboard->keymap == NULL ? 0 :
		xkb_keymap_num_layouts(keyboard->keymap);
}

static const char *sysc_keyboard_layout_name(struct wlr_keyboard *keyboard, uint32_t index) {
	return keyboard == NULL || keyboard->keymap == NULL ? NULL :
		xkb_keymap_layout_get_name(keyboard->keymap, index);
}

static uint32_t sysc_keyboard_active_layout(struct wlr_keyboard *keyboard) {
	return keyboard == NULL || keyboard->xkb_state == NULL ? 0 :
		xkb_state_serialize_layout(keyboard->xkb_state, XKB_STATE_LAYOUT_EFFECTIVE);
}

// Match Niri's hardcoded XF86Switch_VT keysyms. Under the standard XKB
// keymap, Ctrl+Alt+F1 resolves to VT 1 without consulting bind config.
static int sysc_switch_vt_for_key(struct wlr_keyboard *keyboard, uint32_t keycode) {
	if (keyboard == NULL || keyboard->xkb_state == NULL) {
		return 0;
	}
	xkb_keysym_t keysym = xkb_state_key_get_one_sym(keyboard->xkb_state, keycode + 8);
	if (keysym < XKB_KEY_XF86Switch_VT_1 || keysym > XKB_KEY_XF86Switch_VT_12) {
		return 0;
	}
	return (int)(keysym - XKB_KEY_XF86Switch_VT_1 + 1);
}

#include <stdlib.h>

// sysc_compile_keymap compiles from a keymap file's text when there is one,
// else from rule names. A NULL or empty name is xkbcommon's default, which
// also reads XKB_DEFAULT_* from the environment.
static const char *sysc_keymap_layout(struct wlr_keyboard *kb) {
	return kb->keymap == NULL ? NULL : xkb_keymap_layout_get_name(kb->keymap, 0);
}

static struct xkb_keymap *sysc_compile_keymap(const char *rules, const char *model,
		const char *layout, const char *variant, const char *options, const char *text) {
	struct xkb_context *ctx = xkb_context_new(XKB_CONTEXT_NO_FLAGS);
	if (ctx == NULL) {
		return NULL;
	}
	struct xkb_keymap *keymap;
	if (text != NULL) {
		keymap = xkb_keymap_new_from_string(ctx, text, XKB_KEYMAP_FORMAT_TEXT_V1,
			XKB_KEYMAP_COMPILE_NO_FLAGS);
	} else {
		struct xkb_rule_names names = {
			.rules = rules, .model = model, .layout = layout,
			.variant = variant, .options = options,
		};
		keymap = xkb_keymap_new_from_names(ctx, &names, XKB_KEYMAP_COMPILE_NO_FLAGS);
	}
	xkb_context_unref(ctx);
	return keymap;
}

// sysc_keyboard_group_create makes the one keyboard the user seat hears. Every
// physical keyboard joins it, so a key from any of them reaches the focused
// client. It takes its own reference to keymap.
static struct wlr_keyboard_group *sysc_keyboard_group_create(struct xkb_keymap *keymap,
		int rate, int delay) {
	struct wlr_keyboard_group *group = wlr_keyboard_group_create();
	if (group == NULL) {
		return NULL;
	}
	if (!wlr_keyboard_set_keymap(&group->keyboard, keymap)) {
		wlr_keyboard_group_destroy(group);
		return NULL;
	}
	wlr_keyboard_set_repeat_info(&group->keyboard, rate, delay);
	return group;
}

// A keyboard joins only with the group's keymap and repeat; a backend
// keyboard arrives with neither. Clients repeat for themselves from these;
// the compositor sends one key per physical press (design §7).
static bool sysc_keyboard_group_join(struct wlr_keyboard_group *group, struct wlr_keyboard *kb) {
	wlr_keyboard_set_keymap(kb, group->keyboard.keymap);
	wlr_keyboard_set_repeat_info(kb, group->keyboard.repeat_info.rate,
		group->keyboard.repeat_info.delay);
	return wlr_keyboard_group_add_keyboard(group, kb);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

// Keyboard is the user seat's keymap and repeat: niri's input.keyboard.
// Empty names are xkbcommon's defaults; a zero repeat is niri's default.
type Keyboard struct {
	Rules, Model, Layout, Variant, Options string
	// File is a keymap file, already ~-expanded, read instead of the names.
	File                    string
	RepeatDelay, RepeatRate int
}

type KeyboardLayoutsSnapshot struct {
	Names      []string
	CurrentIdx uint8
}

// KeyboardLayoutsSnapshot copies configured layouts and the active index for
// Niri's EventStream. Call it on the compositor thread.
func (c *Compositor) KeyboardLayoutsSnapshot() *KeyboardLayoutsSnapshot {
	count := int(C.sysc_keyboard_layout_count(c.keyboard))
	if count == 0 {
		return nil
	}
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := C.sysc_keyboard_layout_name(c.keyboard, C.uint32_t(i))
		if name != nil {
			names = append(names, C.GoString(name))
		}
	}
	if len(names) == 0 {
		return nil
	}
	return &KeyboardLayoutsSnapshot{
		Names: names, CurrentIdx: uint8(C.sysc_keyboard_active_layout(c.keyboard)),
	}
}

// Niri's defaults (niri-config input.rs): repeat after 600 ms, 25 per second.
const (
	defaultRepeatDelay = 600
	defaultRepeatRate  = 25
)

// compileKeymap builds k's keymap. A keymap that does not compile falls back
// to the default, as niri's does, so a bad config never leaves the seat
// without a keyboard.
func compileKeymap(k Keyboard) (*C.struct_xkb_keymap, error) {
	if k.File != "" {
		text, err := os.ReadFile(k.File)
		if err == nil {
			ctext := C.CString(string(text))
			keymap := C.sysc_compile_keymap(nil, nil, nil, nil, nil, ctext)
			C.free(unsafe.Pointer(ctext))
			if keymap != nil {
				return keymap, nil
			}
		}
		os.Stderr.WriteString("sysc-compositor: keymap file " + k.File + " did not compile; using the default\n")
	} else if k.Rules+k.Model+k.Layout+k.Variant+k.Options != "" {
		names := []*C.char{C.CString(k.Rules), C.CString(k.Model), C.CString(k.Layout),
			C.CString(k.Variant), C.CString(k.Options)}
		keymap := C.sysc_compile_keymap(names[0], names[1], names[2], names[3], names[4], nil)
		for _, n := range names {
			C.free(unsafe.Pointer(n))
		}
		if keymap != nil {
			return keymap, nil
		}
		os.Stderr.WriteString("sysc-compositor: the configured xkb keymap did not compile; using the default\n")
	}
	if keymap := C.sysc_compile_keymap(nil, nil, nil, nil, nil, nil); keymap != nil {
		return keymap, nil
	}
	return nil, errors.New("keyboard group: no xkb keymap could be compiled")
}

// startKeyboardGroup gives the user seat its one keyboard and listens to it.
func (c *Compositor) startKeyboardGroup(k Keyboard) error {
	keymap, err := compileKeymap(k)
	if err != nil {
		return err
	}
	delay, rate := k.RepeatDelay, k.RepeatRate
	if delay == 0 && rate == 0 {
		delay, rate = defaultRepeatDelay, defaultRepeatRate
	}
	c.keyboardGroup = C.sysc_keyboard_group_create(keymap, C.int(rate), C.int(delay))
	C.xkb_keymap_unref(keymap)
	if c.keyboardGroup == nil {
		return errors.New("keyboard group: the keymap could not be set")
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
	keycode := uint32(event.keycode)
	if event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED {
		// A key may have been released while the session was inactive. A fresh
		// press starts a new suppression lifetime for this evdev keycode.
		delete(c.suppressedKeys, keycode)
		if vt := int(C.sysc_switch_vt_for_key(c.keyboard, event.keycode)); vt != 0 {
			c.suppressedKeys[keycode] = true
			if c.session != nil && !bool(C.wlr_session_change_vt(c.session, C.uint(vt))) {
				fmt.Fprintf(os.Stderr, "sysc-compositor: switching to VT %d failed\n", vt)
			}
			return
		}
	} else if c.suppressedKeys[keycode] {
		delete(c.suppressedKeys, keycode)
		return
	}
	C.wlr_seat_keyboard_notify_key(c.seat, event.time_msec, event.keycode,
		C.uint32_t(event.state))
}

func (c *Compositor) handleKeyboardModifiers(unsafe.Pointer) {
	c.modifiers = uint32(C.wlr_keyboard_get_modifiers(c.keyboard))
	C.wlr_seat_keyboard_notify_modifiers(c.seat, &c.keyboard.modifiers)
	c.notifyStateChanged()
}

// userLayout is the user seat's first xkb layout name, for tests.
func (c *Compositor) userLayout() string {
	return C.GoString(C.sysc_keymap_layout(c.keyboard))
}
