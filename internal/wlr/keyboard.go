package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server xkbcommon
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdint.h>
#include <wayland-server-core.h>
#include <xkbcommon/xkbcommon.h>
#include <xkbcommon/xkbcommon-keysyms.h>
#include <wlr/backend/session.h>
#include <wlr/interfaces/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_keyboard_group.h>
#include <wlr/types/wlr_input_method_v2.h>
#include <wlr/types/wlr_seat.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);

static int sysc_keyboard_repeat_timer(void *data) {
	sysc_listener_fire((uintptr_t)data, NULL);
	return 0;
}

static struct wl_event_source *sysc_keyboard_repeat_source(
		struct wl_event_loop *loop, uintptr_t token) {
	return wl_event_loop_add_timer(loop, sysc_keyboard_repeat_timer, (void *)token);
}

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

static bool sysc_keyboard_switch_layout(struct wlr_keyboard *keyboard, uint32_t index) {
	if (keyboard == NULL || keyboard->xkb_state == NULL ||
		index >= xkb_keymap_num_layouts(keyboard->keymap) ||
		index == xkb_state_serialize_layout(keyboard->xkb_state, XKB_STATE_LAYOUT_EFFECTIVE)) {
		return false;
	}
	wlr_keyboard_notify_modifiers(keyboard, keyboard->modifiers.depressed,
		keyboard->modifiers.latched, keyboard->modifiers.locked, index);
	return true;
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

// sysc_keyboard_a11y_key reads what an assistive technology is told about a
// key: its keysym, the effective modifier mask and its character. wlroots
// emits the key event before it updates XKB state, so this is the state the
// key was pressed or released in, as Niri reads it.
static void sysc_keyboard_a11y_key(struct wlr_keyboard *keyboard, uint32_t keycode,
		uint32_t *keysym, uint32_t *mods, uint32_t *unichar) {
	*keysym = *mods = *unichar = 0;
	if (keyboard == NULL || keyboard->xkb_state == NULL) {
		return;
	}
	xkb_keycode_t code = keycode + 8;
	*keysym = xkb_state_key_get_one_sym(keyboard->xkb_state, code);
	*mods = xkb_state_serialize_mods(keyboard->xkb_state, XKB_STATE_MODS_EFFECTIVE);
	*unichar = xkb_state_key_get_utf32(keyboard->xkb_state, code);
}

static bool sysc_modifier_active(struct wlr_keyboard *keyboard, const char *name) {
	return keyboard != NULL && keyboard->xkb_state != NULL &&
		xkb_state_mod_name_is_active(keyboard->xkb_state, name,
			XKB_STATE_MODS_EFFECTIVE) > 0;
}

static bool sysc_keysym_is_ascii_or_unprintable(xkb_keysym_t sym) {
	return xkb_keysym_to_utf32(sym) < 128;
}

static xkb_keysym_t sysc_raw_sym(struct xkb_keymap *keymap, xkb_keycode_t code,
		xkb_layout_index_t layout) {
	const xkb_keysym_t *syms = NULL;
	return xkb_keymap_key_get_syms_by_level(keymap, code, layout, 0, &syms) > 0 ?
		syms[0] : XKB_KEY_NoSymbol;
}

// The keysym a bind names, as Niri's find_bind takes it: smithay's
// raw_latin_sym_or_raw_current_sym. That is the key's unshifted keysym on its
// layout, unless that is a non-ASCII character and another layout gives the
// key an ASCII one, so binds work the same on every layout.
static xkb_keysym_t sysc_bind_keysym(struct xkb_state *state, uint32_t keycode) {
	struct xkb_keymap *keymap = xkb_state_get_keymap(state);
	xkb_keycode_t code = keycode + 8;
	xkb_layout_index_t current = xkb_state_key_get_layout(state, code);
	xkb_keysym_t base = sysc_raw_sym(keymap, code, current);
	if (base == XKB_KEY_NoSymbol || sysc_keysym_is_ascii_or_unprintable(base)) {
		return base;
	}
	for (xkb_layout_index_t layout = 0; layout < xkb_keymap_num_layouts(keymap); layout++) {
		xkb_keysym_t sym = layout == current ? XKB_KEY_NoSymbol : sysc_raw_sym(keymap, code, layout);
		if (sym != XKB_KEY_NoSymbol && xkb_keysym_to_utf32(sym) != 0 &&
				sysc_keysym_is_ascii_or_unprintable(sym)) {
			return sym;
		}
	}
	return base;
}

static bool sysc_keysym_matches(struct wlr_keyboard *keyboard, uint32_t keycode,
		const char *name) {
	if (keyboard == NULL || keyboard->xkb_state == NULL) {
		return false;
	}
	xkb_keysym_t wanted = xkb_keysym_from_name(name, XKB_KEYSYM_CASE_INSENSITIVE);
	xkb_keysym_t sym = sysc_bind_keysym(keyboard->xkb_state, keycode);
	return wanted != XKB_KEY_NoSymbol && sym != XKB_KEY_NoSymbol &&
		xkb_keysym_to_lower(sym) == xkb_keysym_to_lower(wanted);
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
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
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

// SwitchLayout changes the XKB group on the user seat and notifies clients.
// target is Next, Prev, or Index; an invalid index or unknown target is a no-op.
func (c *Compositor) SwitchLayout(target string, index *uint8) bool {
	count := uint32(C.sysc_keyboard_layout_count(c.keyboard))
	if count == 0 {
		return false
	}
	current := uint32(C.sysc_keyboard_active_layout(c.keyboard))
	var next uint32
	switch target {
	case "Next":
		next = (current + 1) % count
	case "Prev":
		next = (current + count - 1) % count
	case "Index":
		if index == nil || uint32(*index) >= count {
			return false
		}
		next = uint32(*index)
	default:
		return false
	}
	return bool(C.sysc_keyboard_switch_layout(c.keyboard, C.uint32_t(next)))
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
	c.seatKeyboard = newNativeKeyboard("sysc-user-seat-keyboard")
	if c.seatKeyboard == nil || !c.activateKeyboard(c.keyboard) {
		return errors.New("keyboard group: the user seat keyboard could not be initialized")
	}
	C.wlr_seat_set_keyboard(c.seat, c.seatKeyboard)
	c.keep(
		AddListener(unsafe.Pointer(&c.keyboard.events.key), c.handleKeyboardKey),
		AddListener(unsafe.Pointer(&c.keyboard.events.modifiers), c.handleKeyboardModifiers),
		AddListener(unsafe.Pointer(&c.keyboardGroup.events.enter), c.keyboardGroupEntered),
		AddListener(unsafe.Pointer(&c.keyboardGroup.events.leave), c.keyboardGroupLeft),
		AddListener(unsafe.Pointer(&c.keyboard.events.keymap), func(unsafe.Pointer) { c.refreshActiveKeyboard(c.keyboard) }),
		AddListener(unsafe.Pointer(&c.keyboard.events.repeat_info), func(unsafe.Pointer) { c.refreshActiveKeyboard(c.keyboard) }),
	)
	return nil
}

func (c *Compositor) reconfigureKeyboard(k Keyboard) error {
	if c.keyboard == nil {
		return errors.New("keyboard group: no user keyboard")
	}
	keymap, err := compileKeymap(k)
	if err != nil {
		return err
	}
	defer C.xkb_keymap_unref(keymap)
	delay, rate := k.RepeatDelay, k.RepeatRate
	if delay == 0 && rate == 0 {
		delay, rate = defaultRepeatDelay, defaultRepeatRate
	}
	if !bool(C.wlr_keyboard_set_keymap(c.keyboard, keymap)) {
		return errors.New("keyboard group: the new keymap could not be set")
	}
	C.wlr_keyboard_set_repeat_info(c.keyboard, C.int(rate), C.int(delay))
	for device := range c.keyboardDevices {
		keyboard := C.wlr_keyboard_from_input_device((*C.struct_wlr_input_device)(device))
		if keyboard == nil {
			continue
		}
		if !bool(C.wlr_keyboard_set_keymap(keyboard, keymap)) {
			return errors.New("keyboard group: an input keyboard rejected the new keymap")
		}
		C.wlr_keyboard_set_repeat_info(keyboard, C.int(rate), C.int(delay))
	}
	for keyboard := range c.virtualKeyboards {
		C.wlr_keyboard_set_repeat_info(keyboard, C.int(rate), C.int(delay))
	}
	return nil
}

func (c *Compositor) destroyKeyboardGroup() {
	destroyNativeKeyboard(c.seatKeyboard)
	c.seatKeyboard, c.activeKeyboard = nil, nil
	if c.keyboardGroup != nil {
		C.wlr_keyboard_group_destroy(c.keyboardGroup)
		c.keyboardGroup = nil
		c.keyboard = nil
	}
}

// activateKeyboard mirrors source metadata while the stable seat keyboard owns
// application-held keycodes. Native late wl_keyboard bindings see that union.
func (c *Compositor) activateKeyboard(source *C.struct_wlr_keyboard) bool {
	if source == nil || source.keymap == nil || c.seatKeyboard == nil {
		return false
	}
	keyboard := c.seatKeyboard
	previous := keyboard.modifiers
	changed := keyboard.keymap != source.keymap
	if changed && !bool(C.wlr_keyboard_set_keymap(keyboard, source.keymap)) {
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not mirror the seat keymap")
		return false
	}
	C.wlr_keyboard_set_repeat_info(keyboard, source.repeat_info.rate, source.repeat_info.delay)
	C.wlr_keyboard_notify_modifiers(keyboard, source.modifiers.depressed,
		source.modifiers.latched, source.modifiers.locked, source.modifiers.group)
	c.activeKeyboard = source
	if changed || previous != keyboard.modifiers {
		C.wlr_seat_keyboard_notify_modifiers(c.seat, &keyboard.modifiers)
	}
	return true
}

func (c *Compositor) refreshActiveKeyboard(source *C.struct_wlr_keyboard) {
	if c.activeKeyboard == source {
		c.activateKeyboard(source)
	}
}

// updateSeatKey retains wlroots' fixed held-key capacity; a dropped press has
// no application release. Deferred group joins update state without a press.
func (c *Compositor) updateSeatKey(event *C.struct_wlr_keyboard_key_event, notify bool) bool {
	keyboard := c.seatKeyboard
	if keyboard == nil {
		return false
	}
	pressed := event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED
	held := false
	for i := 0; i < int(keyboard.num_keycodes); i++ {
		if keyboard.keycodes[i] == event.keycode {
			held = true
			break
		}
	}
	if pressed == held {
		return held
	}
	if pressed && int(keyboard.num_keycodes) == len(keyboard.keycodes) {
		return false
	}
	copy := *event
	copy.update_state = false
	C.wlr_keyboard_notify_key(keyboard, &copy)
	if notify {
		C.wlr_seat_keyboard_notify_key(c.seat, event.time_msec, event.keycode, C.uint32_t(event.state))
	}
	return true
}

func (c *Compositor) keyboardGroupEntered(data unsafe.Pointer) {
	array := (*C.struct_wl_array)(data)
	keys := unsafe.Slice((*C.uint32_t)(array.data), int(array.size)/4)
	if c.keyboardRoutes == nil {
		c.keyboardRoutes = make(map[*C.struct_wlr_keyboard]map[uint32]keyboardRoute)
	}
	routes := c.keyboardRoutes[c.keyboard]
	if routes == nil {
		routes = make(map[uint32]keyboardRoute)
		c.keyboardRoutes[c.keyboard] = routes
	}
	for _, key := range keys {
		grab := c.inputMethodGrab(c.keyboard)
		routes[uint32(key)] = keyboardRoute{grab: grab, announced: c.keyDestinationHeld(uint32(key), grab)}
	}
}

func (c *Compositor) keyboardGroupLeft(data unsafe.Pointer) {
	array := (*C.struct_wl_array)(data)
	if array.size != 0 && c.keyboardRepeatKeyboard == c.keyboard {
		c.stopKeyboardRepeat()
	}
	for _, key := range unsafe.Slice((*C.uint32_t)(array.data), int(array.size)/4) {
		event := C.struct_wlr_keyboard_key_event{keycode: key, time_msec: nowMsec(), state: C.WL_KEYBOARD_KEY_STATE_RELEASED}
		delete(c.suppressedKeys, uint32(key))
		c.forwardKeyboardKey(c.keyboard, &event)
	}
}

// A joining keyboard must not fabricate a press to the old focus. Its deferred
// state becomes visible when a later focus receives the native held-key array.
func (c *Compositor) prepareKeyboardFocus() {
	for source, routes := range c.keyboardRoutes {
		for key, route := range routes {
			if route.announced || route.grab != nil {
				continue
			}
			if route.grab = c.inputMethodGrab(source); route.grab != nil {
				routes[key] = route
				continue
			}
			event := C.struct_wlr_keyboard_key_event{keycode: C.uint32_t(key), state: C.WL_KEYBOARD_KEY_STATE_PRESSED}
			route.announced = c.keyDestinationHeld(key, nil) || c.updateSeatKey(&event, false)
			routes[key] = route
		}
	}
}

// addKeyboard puts a backend keyboard into the group. The group drops it by
// itself when the device goes.
func (c *Compositor) addKeyboard(kb *C.struct_wlr_keyboard) bool {
	return bool(C.sysc_keyboard_group_join(c.keyboardGroup, kb))
}

// SetKeyboardMonitor routes the user seat's keys through an assistive
// technology's grabs before anything else sees them. nil removes it. Call it
// on the compositor thread.
func (c *Compositor) SetKeyboardMonitor(monitor func(a11y.Key, time.Duration) a11y.Decision) {
	c.keyboardMonitor = monitor
}

// handleKeyboardKey delivers a key from any keyboard to the user seat's focus.
func (c *Compositor) handleKeyboardKey(data unsafe.Pointer) {
	c.handleKeyboardKeyFrom(c.keyboard, c.suppressedKeys, data)
}

func (c *Compositor) currentKeyboard() *C.struct_wlr_keyboard {
	if c.bindingKeyboard != nil {
		return c.bindingKeyboard
	}
	if c.activeKeyboard != nil {
		return c.activeKeyboard
	}
	return c.keyboard
}

func (c *Compositor) handleKeyboardKeyFrom(keyboard *C.struct_wlr_keyboard, suppressed map[uint32]bool, data unsafe.Pointer) {
	previous := c.bindingKeyboard
	c.bindingKeyboard = keyboard
	defer func() { c.bindingKeyboard = previous }()
	event := (*C.struct_wlr_keyboard_key_event)(data)
	keycode := uint32(event.keycode)
	if keyboard == c.keyboard {
		c.userActivity()
		if c.keyboardMonitor != nil && c.a11yConsumedKey(keyboard, suppressed, event) {
			return
		}
	}
	if event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED {
		// Niri closes the hotkey overlay after the press is handled, so a
		// press still reaches its bind or client, and one that opens the
		// overlay leaves it open.
		if c.hotkeyOverlay.open {
			defer c.hideHotkeyOverlay()
		}
		// A key may have been released while the session was inactive. A fresh
		// press starts a new suppression lifetime for this evdev keycode.
		delete(suppressed, keycode)
		if vt := int(C.sysc_switch_vt_for_key(keyboard, event.keycode)); vt != 0 && keyboard == c.keyboard {
			suppressed[keycode] = true
			if c.session != nil && !bool(C.wlr_session_change_vt(c.session, C.uint(vt))) {
				fmt.Fprintf(os.Stderr, "sysc-compositor: switching to VT %d failed\n", vt)
			}
			return
		}
		if c.exitDialogKeyPress(keycode) {
			suppressed[keycode] = true
			return
		}
		if c.pickColor != nil && c.keysymIs(keycode, "Escape") {
			c.finishColorPick([3]float64{}, errors.New("no color picked"))
			suppressed[keycode] = true
			return
		}
		if c.shotUI != nil {
			suppressed[keycode] = true
			if binding, ok := c.shotUIKey(keycode); ok && binding.Repeat {
				c.startKeyboardRepeat(binding)
			}
			return
		}
		if binding, matched := c.dispatchKeyBinding(keycode); matched {
			suppressed[keycode] = true
			if binding.Repeat {
				c.startKeyboardRepeat(binding)
			}
			return
		}
	} else {
		if c.keyboardRepeatKeyboard == keyboard {
			c.stopKeyboardRepeat()
		}
		if suppressed[keycode] {
			delete(suppressed, keycode)
			return
		}
	}
	// Typing into the focused window marks it recently used at once.
	c.applyPendingWindowFocus()
	c.forwardKeyboardKey(keyboard, event)
}

// a11yConsumedKey offers a user-seat key to the assistive technology's grabs
// before VT switching, dialogs and binds see it, as Niri does. It reports
// whether the grabs kept the key.
func (c *Compositor) a11yConsumedKey(keyboard *C.struct_wlr_keyboard, suppressed map[uint32]bool,
	event *C.struct_wlr_keyboard_key_event) bool {
	var keysym, mods, unichar C.uint32_t
	C.sysc_keyboard_a11y_key(keyboard, event.keycode, &keysym, &mods, &unichar)
	key := a11y.Key{Time: time.Duration(event.time_msec) * time.Millisecond,
		Released: event.state != C.WL_KEYBOARD_KEY_STATE_PRESSED, Mods: uint32(mods),
		Keysym: uint32(keysym), Unichar: uint32(unichar), Keycode: uint16(event.keycode + 8)}
	decision := c.keyboardMonitor(key, time.Duration(keyboard.repeat_info.delay)*time.Millisecond)
	if decision == a11y.Pass {
		return false
	}
	if decision == a11y.ModifierFirstPress {
		// wlr_keyboard_notify_key reads this after its listeners return, so
		// a single tap of the screen reader's modifier never reaches XKB.
		event.update_state = false
	}
	// Niri also forwards grabbed modifier releases, because smithay lists
	// grabbed keys as held in wl_keyboard.enter. The seat keyboard here holds
	// only keys that reached a client, so a release goes where its press went.
	if keycode := uint32(event.keycode); key.Released {
		delete(suppressed, keycode)
	} else {
		suppressed[keycode] = true
	}
	return true
}

// keysymIs reports whether keycode is the named bind key on the user
// keyboard.
func (c *Compositor) keysymIs(keycode uint32, name string) bool {
	return keysymMatches(c.currentKeyboard(), keycode, name)
}

// keysymMatches reports whether keycode on kb is the key a bind names.
func keysymMatches(kb *C.struct_wlr_keyboard, keycode uint32, name string) bool {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return bool(C.sysc_keysym_matches(kb, C.uint32_t(keycode), cname))
}

func (c *Compositor) dispatchKeyBinding(keycode uint32) (bind.Binding, bool) {
	binding, ok := c.matchKeyBinding(keycode)
	if ok {
		c.executeKeyBinding(binding)
	}
	return binding, ok
}

// matchKeyBinding is the bind a user key press triggers now, if any.
func (c *Compositor) matchKeyBinding(keycode uint32) (bind.Binding, bool) {
	modifiers := c.activeBindModifiers()
	for _, binding := range c.keyBindings(modifiers) {
		if binding.Chord.Trigger != bind.TriggerKey || binding.Chord.Modifiers != modifiers {
			continue
		}
		if c.sessionLocked && !binding.AllowWhenLocked && !allowedWhenLocked(binding) {
			continue
		}
		if !binding.IgnoresInhibit && c.shortcutsInhibited() {
			continue
		}
		if !c.keysymIs(keycode, binding.Chord.Key) {
			continue
		}
		return binding, true
	}
	return bind.Binding{}, false
}

// allowedWhenLocked is Niri's allowed_when_locked for the implemented
// actions: a bind of quit alone runs while the session is sealed, so the exit
// dialog can open over the lock screen.
func allowedWhenLocked(binding bind.Binding) bool {
	for _, action := range binding.Actions {
		if action.Name != bind.ActionQuit {
			return false
		}
	}
	return len(binding.Actions) > 0
}

// executeKeyBinding runs a bind's actions. While the session is sealed only
// spawn actions, which Niri's allow-when-locked covers, and quit run.
func (c *Compositor) executeKeyBinding(binding bind.Binding) {
	if c.shotUI != nil {
		c.runShotBinding(binding)
		return
	}
	for _, action := range binding.Actions {
		if c.sessionLocked && action.Name != bind.ActionSpawn && action.Name != bind.ActionSpawnSh &&
			action.Name != bind.ActionQuit {
			continue
		}
		c.executeBindAction(action)
	}
}

func (c *Compositor) startKeyboardRepeat(binding bind.Binding) {
	c.stopKeyboardRepeat()
	keyboard := c.currentKeyboard()
	rate := int(keyboard.repeat_info.rate)
	if rate <= 0 {
		return
	}
	token := Register(func(unsafe.Pointer) { c.handleKeyboardRepeat() })
	source := C.sysc_keyboard_repeat_source(c.loop, C.uintptr_t(token))
	if source == nil {
		Unregister(token)
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not create keyboard bind repeat timer")
		return
	}
	c.keyboardRepeatSource = source
	c.keyboardRepeatKeyboard = keyboard
	c.keyboardRepeatToken = token
	c.keyboardRepeatBinding = binding
	// ponytail: Wayland timers use whole milliseconds, so round up to avoid
	// exceeding the requested rate; fractional cadence needs deadline accumulation.
	c.keyboardRepeatInterval = (1000 + rate - 1) / rate
	if C.wl_event_source_timer_update(source, C.int(keyboard.repeat_info.delay)) < 0 {
		c.stopKeyboardRepeat()
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not start keyboard bind repeat timer")
	}
}

func (c *Compositor) handleKeyboardRepeat() {
	source := c.keyboardRepeatSource
	if source == nil {
		return
	}
	c.executeKeyBinding(c.keyboardRepeatBinding)
	if c.keyboardRepeatSource == source && C.wl_event_source_timer_update(source, C.int(c.keyboardRepeatInterval)) < 0 {
		c.stopKeyboardRepeat()
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not restart keyboard bind repeat timer")
	}
}

func (c *Compositor) stopKeyboardRepeat() {
	if c.keyboardRepeatSource != nil {
		C.wl_event_source_remove(c.keyboardRepeatSource)
		c.keyboardRepeatSource = nil
	}
	if c.keyboardRepeatToken != 0 {
		Unregister(c.keyboardRepeatToken)
		c.keyboardRepeatToken = 0
	}
	c.keyboardRepeatBinding = bind.Binding{}
	c.keyboardRepeatKeyboard = nil
	c.keyboardRepeatInterval = 0
}

func (c *Compositor) activeBindModifiers() bind.Modifiers {
	var modifiers bind.Modifiers
	for _, item := range []struct {
		bit  bind.Modifier
		name string
	}{
		{bind.ModifierCtrl, "Control"}, {bind.ModifierShift, "Shift"},
		{bind.ModifierAlt, "Mod1"}, {bind.ModifierSuper, "Mod4"},
		{bind.ModifierLevel3, "Mod5"}, {bind.ModifierLevel5, "Mod3"},
	} {
		name := C.CString(item.name)
		active := bool(C.sysc_modifier_active(c.currentKeyboard(), name))
		C.free(unsafe.Pointer(name))
		if active {
			modifiers |= bind.Modifiers(item.bit)
		}
	}
	return modifiers
}

func (c *Compositor) modifierActive(modifier bind.Modifier) bool {
	return c.activeBindModifiers()&bind.Modifiers(modifier) != 0
}

func (c *Compositor) executeBindAction(action bind.Action) {
	if c.runSwitcherAction(action) {
		return
	}
	if action.Name == bind.ActionCloseWindow {
		c.CloseWindow(nil)
		return
	}
	if action.Name == bind.ActionQuit {
		c.Quit(action.SkipConfirmation)
		return
	}
	if action.Name == bind.ActionShowHotkeyOverlay {
		if !c.hotkeyOverlay.open {
			c.ShowHotkeyOverlay()
			c.announceHotkeyOverlay()
		}
		return
	}
	if action.Name == bind.ActionSpawn {
		if c.spawn != nil {
			if err := c.spawn(action.Args); err != nil {
				fmt.Fprintf(os.Stderr, "sysc-compositor: bind spawn: %v\n", err)
			}
		}
		return
	}
	if action.Name == bind.ActionSpawnSh {
		if c.spawnSh != nil {
			if err := c.spawnSh(action.Args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "sysc-compositor: bind spawn-sh: %v\n", err)
			}
		}
		return
	}
	if c.model == nil {
		return
	}
	switch action.Name {
	case bind.ActionSetDynamicCastWindow:
		c.SetDynamicCastWindow(nil)
		return
	case bind.ActionSetDynamicCastMonitor:
		var name *string
		if len(action.Args) != 0 {
			name = &action.Args[0]
		}
		c.SetDynamicCastMonitor(name)
		return
	case bind.ActionClearDynamicCastTarget:
		c.ClearDynamicCastTarget()
		return
	case bind.ActionSetWorkspaceName:
		if workspaceID, ok := c.focusedWorkspaceID(); ok {
			c.SetWorkspaceName(workspaceID, action.WorkspaceName)
		}
		return
	case bind.ActionFocusWindowPrevious:
		c.FocusWindowPrevious()
		return
	case bind.ActionUnsetWorkspaceName:
		if workspaceID, ok := c.focusedWorkspaceID(); ok {
			c.UnsetWorkspaceName(workspaceID)
		}
		return
	case bind.ActionMoveWindowToWorkspace, bind.ActionMoveColumnToWorkspace:
		workspaceID, ok := c.workspaceIDForBind(action.WorkspaceIndex, action.WorkspaceName)
		if !ok {
			return
		}
		if action.Name == bind.ActionMoveWindowToWorkspace {
			c.MoveWindowToWorkspace(nil, workspaceID, action.Focus)
		} else {
			c.MoveColumnToWorkspace(workspaceID, action.Focus)
		}
		return
	case bind.ActionMoveWindowToMonitor:
		c.MoveWindowToMonitor(nil, action.Output)
		return
	case bind.ActionMoveColumnToMonitor:
		c.MoveColumnToMonitor(action.Output)
		return
	case bind.ActionMoveWorkspaceToMonitor:
		if workspaceID, ok := c.focusedWorkspaceID(); ok {
			c.MoveWorkspaceToOutput(workspaceID, action.Output)
		}
		return
	case bind.ActionFocusMonitor:
		c.FocusMonitor(action.Output)
		return
	case bind.ActionMoveWindowToMonitorLeft:
		c.MoveWindowToMonitorDirection(nil, "left")
		return
	case bind.ActionMoveWindowToMonitorRight:
		c.MoveWindowToMonitorDirection(nil, "right")
		return
	case bind.ActionMoveWindowToMonitorUp:
		c.MoveWindowToMonitorDirection(nil, "up")
		return
	case bind.ActionMoveWindowToMonitorDown:
		c.MoveWindowToMonitorDirection(nil, "down")
		return
	case bind.ActionMoveWindowToMonitorPrevious:
		c.MoveWindowToMonitorDirection(nil, "previous")
		return
	case bind.ActionMoveWindowToMonitorNext:
		c.MoveWindowToMonitorDirection(nil, "next")
		return
	case bind.ActionMoveColumnToMonitorLeft:
		c.MoveColumnToMonitorDirection("left")
		return
	case bind.ActionMoveColumnToMonitorRight:
		c.MoveColumnToMonitorDirection("right")
		return
	case bind.ActionMoveColumnToMonitorUp:
		c.MoveColumnToMonitorDirection("up")
		return
	case bind.ActionMoveColumnToMonitorDown:
		c.MoveColumnToMonitorDirection("down")
		return
	case bind.ActionMoveColumnToMonitorPrevious:
		c.MoveColumnToMonitorDirection("previous")
		return
	case bind.ActionMoveColumnToMonitorNext:
		c.MoveColumnToMonitorDirection("next")
		return
	case bind.ActionMoveWorkspaceToMonitorLeft:
		c.MoveWorkspaceToMonitorDirection("left")
		return
	case bind.ActionMoveWorkspaceToMonitorRight:
		c.MoveWorkspaceToMonitorDirection("right")
		return
	case bind.ActionMoveWorkspaceToMonitorUp:
		c.MoveWorkspaceToMonitorDirection("up")
		return
	case bind.ActionMoveWorkspaceToMonitorDown:
		c.MoveWorkspaceToMonitorDirection("down")
		return
	case bind.ActionMoveWorkspaceToMonitorPrevious:
		c.MoveWorkspaceToMonitorDirection("previous")
		return
	case bind.ActionMoveWorkspaceToMonitorNext:
		c.MoveWorkspaceToMonitorDirection("next")
		return
	case bind.ActionFocusMonitorLeft:
		c.FocusMonitorDirection("left")
		return
	case bind.ActionFocusMonitorRight:
		c.FocusMonitorDirection("right")
		return
	case bind.ActionFocusMonitorUp:
		c.FocusMonitorDirection("up")
		return
	case bind.ActionFocusMonitorDown:
		c.FocusMonitorDirection("down")
		return
	case bind.ActionFocusMonitorPrevious:
		c.FocusMonitorDirection("previous")
		return
	case bind.ActionFocusMonitorNext:
		c.FocusMonitorDirection("next")
		return
	case bind.ActionToggleWindowFloating:
		c.ToggleWindowFloating()
		return
	case bind.ActionToggleOverview:
		c.ToggleOverview()
	case bind.ActionToggleKeyboardShortcutsInhibit:
		c.ToggleKeyboardShortcutsInhibit()
	case bind.ActionPowerOffMonitors:
		if err := c.PowerOffMonitors(); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: power-off-monitors: %v\n", err)
		}
	case bind.ActionScreenshotScreen:
		if err := c.ScreenshotScreen(action.WriteToDisk, action.ShowPointer, ""); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: screenshot-screen: %v\n", err)
		}
	case bind.ActionScreenshot:
		if err := c.Screenshot(action.ShowPointer, ""); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: screenshot: %v\n", err)
		}
	case bind.ActionScreenshotWindow:
		if err := c.ScreenshotWindow(nil, action.WriteToDisk, action.ShowPointer, ""); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: screenshot-window: %v\n", err)
		}
	case bind.ActionPowerOnMonitors:
		if err := c.PowerOnMonitors(); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: power-on-monitors: %v\n", err)
		}
		return
	case bind.ActionOpenOverview:
		c.OpenOverview()
		return
	case bind.ActionCloseOverview:
		c.CloseOverview()
		return
	case bind.ActionMoveWindowToFloating, bind.ActionMoveWindowToTiling:
		id, ok := c.model.ActiveWindow()
		if !ok {
			return
		}
		c.SetWindowFloating(id, action.Name == bind.ActionMoveWindowToFloating)
		return
	case bind.ActionFocusFloating:
		c.FocusFloating()
		return
	case bind.ActionFocusTiling:
		c.FocusTiling()
		return
	case bind.ActionSwitchFocusFloatingAndTiling:
		c.SwitchFocusFloatingAndTiling()
		return
	case bind.ActionFullscreenWindow:
		c.ToggleFullscreen()
		return
	case bind.ActionToggleWindowedFullscreen:
		c.ToggleWindowedFullscreen()
		return
	}
	changed, focus := false, false
	switch action.Name {
	case bind.ActionFocusColumnLeft:
		changed, focus = c.model.FocusColumnLeft(), true
	case bind.ActionFocusColumnRight:
		changed, focus = c.model.FocusColumnRight(), true
	case bind.ActionFocusColumnFirst:
		changed, focus = c.model.FocusColumnFirst(), true
	case bind.ActionFocusColumnLast:
		changed, focus = c.model.FocusColumnLast(), true
	case bind.ActionMoveColumnLeft:
		changed = c.model.MoveColumnLeft()
	case bind.ActionMoveColumnRight:
		changed = c.model.MoveColumnRight()
	case bind.ActionMoveColumnToFirst:
		changed = c.model.MoveColumnToFirst()
	case bind.ActionMoveColumnToLast:
		changed = c.model.MoveColumnToLast()
	case bind.ActionFocusWorkspace:
		if action.WorkspaceName != "" {
			changed = c.model.FocusWorkspaceName(action.WorkspaceName)
		} else {
			changed = c.model.FocusWorkspace("", action.WorkspaceIndex)
		}
		focus = true
	case bind.ActionConsumeWindowIntoColumn:
		changed = c.model.ConsumeWindowIntoColumn()
	case bind.ActionExpelWindowFromColumn:
		changed = c.model.ExpelWindowFromColumn()
	case bind.ActionConsumeOrExpelWindowLeft:
		changed = c.model.ConsumeOrExpelWindowLeft(nil)
	case bind.ActionConsumeOrExpelWindowRight:
		changed = c.model.ConsumeOrExpelWindowRight(nil)
	case bind.ActionToggleColumnTabbedDisplay:
		changed = c.model.ToggleColumnTabbedDisplay()
	case bind.ActionSetColumnDisplay:
		display := layout.ColumnDisplayNormal
		if action.ColumnDisplay == "tabbed" {
			display = layout.ColumnDisplayTabbed
		}
		changed = c.model.SetColumnDisplay(display)
	case bind.ActionFocusWindowInColumn:
		changed, focus = c.model.FocusWindowInColumn(action.ColumnIndex), true
	case bind.ActionFocusWindowDown:
		changed, focus = c.model.FocusWindowDown(), true
	case bind.ActionFocusWindowUp:
		changed, focus = c.model.FocusWindowUp(), true
	case bind.ActionMoveWindowDown:
		changed = c.model.MoveWindowDown()
	case bind.ActionMoveWindowUp:
		changed = c.model.MoveWindowUp()
	case bind.ActionMoveWindowToWorkspaceDown:
		changed, focus = c.model.MoveWindowToWorkspaceDown(action.Focus), action.Focus
	case bind.ActionMoveWindowToWorkspaceUp:
		changed, focus = c.model.MoveWindowToWorkspaceUp(action.Focus), action.Focus
	case bind.ActionMoveColumnToWorkspaceDown:
		changed, focus = c.model.MoveColumnToWorkspaceDown(action.Focus), action.Focus
	case bind.ActionMoveColumnToWorkspaceUp:
		changed, focus = c.model.MoveColumnToWorkspaceUp(action.Focus), action.Focus
	}
	if changed {
		c.applyWorkspaceModel()
		if focus {
			c.activateWorkspaceFocus()
		} else {
			c.notifyWindowStateChanged()
		}
	}
}

func (c *Compositor) handleKeyboardModifiers(unsafe.Pointer) {
	c.handleKeyboardModifiersFrom(c.keyboard)
}

func (c *Compositor) handleKeyboardModifiersFrom(keyboard *C.struct_wlr_keyboard) {
	previous := c.bindingKeyboard
	c.bindingKeyboard = keyboard
	defer func() { c.bindingKeyboard = previous }()
	c.modifiers = uint32(C.wlr_keyboard_get_modifiers(keyboard))
	if grab := c.inputMethodGrab(keyboard); grab != nil {
		C.wlr_input_method_keyboard_grab_v2_set_keyboard(grab, keyboard)
		C.wlr_input_method_keyboard_grab_v2_send_modifiers(grab, &keyboard.modifiers)
	} else {
		c.activateKeyboard(keyboard)
	}
	c.switcherModifiersReleased()
	c.notifyStateChanged()
}

// userLayout is the user seat's first xkb layout name, for tests.
func (c *Compositor) userLayout() string {
	return C.GoString(C.sysc_keymap_layout(c.keyboard))
}
