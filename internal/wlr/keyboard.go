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

static bool sysc_modifier_active(struct wlr_keyboard *keyboard, const char *name) {
	return keyboard != NULL && keyboard->xkb_state != NULL &&
		xkb_state_mod_name_is_active(keyboard->xkb_state, name,
			XKB_STATE_MODS_EFFECTIVE) > 0;
}

static bool sysc_keysym_matches(struct wlr_keyboard *keyboard, uint32_t keycode,
		const char *name) {
	if (keyboard == NULL || keyboard->xkb_state == NULL) {
		return false;
	}
	xkb_keysym_t wanted = xkb_keysym_from_name(name, XKB_KEYSYM_CASE_INSENSITIVE);
	if (wanted == XKB_KEY_NoSymbol) {
		return false;
	}
	const xkb_keysym_t *syms = NULL;
	int count = xkb_state_key_get_syms(keyboard->xkb_state, keycode + 8, &syms);
	for (int i = 0; i < count; i++) {
		if (syms[i] == wanted || xkb_keysym_to_lower(syms[i]) == xkb_keysym_to_lower(wanted)) {
			return true;
		}
	}
	return false;
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
		if c.dispatchKeyBinding(keycode) {
			c.suppressedKeys[keycode] = true
			return
		}
	} else if c.suppressedKeys[keycode] {
		delete(c.suppressedKeys, keycode)
		return
	}
	C.wlr_seat_keyboard_notify_key(c.seat, event.time_msec, event.keycode,
		C.uint32_t(event.state))
}

func (c *Compositor) dispatchKeyBinding(keycode uint32) bool {
	modifiers := c.activeBindModifiers()
	for _, binding := range c.bindings {
		if binding.Chord.Trigger != bind.TriggerKey || binding.Chord.Modifiers != modifiers {
			continue
		}
		name := C.CString(binding.Chord.Key)
		matches := bool(C.sysc_keysym_matches(c.keyboard, C.uint32_t(keycode), name))
		C.free(unsafe.Pointer(name))
		if !matches {
			continue
		}
		for _, action := range binding.Actions {
			c.executeBindAction(action)
		}
		return true
	}
	return false
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
		active := bool(C.sysc_modifier_active(c.keyboard, name))
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
	if action.Name == bind.ActionCloseWindow {
		c.CloseWindow(nil)
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
	case bind.ActionSetWorkspaceName:
		if workspaceID, ok := c.focusedWorkspaceID(); ok {
			c.SetWorkspaceName(workspaceID, action.WorkspaceName)
		}
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
	c.modifiers = uint32(C.wlr_keyboard_get_modifiers(c.keyboard))
	C.wlr_seat_keyboard_notify_modifiers(c.seat, &c.keyboard.modifiers)
	c.notifyStateChanged()
}

// userLayout is the user seat's first xkb layout name, for tests.
func (c *Compositor) userLayout() string {
	return C.GoString(C.sysc_keymap_layout(c.keyboard))
}
