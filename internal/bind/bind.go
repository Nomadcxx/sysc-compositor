// Package bind parses the key and action subset applied by the M5 compositor.
// Key names stay as XKB names so this package does not depend on cgo.
package bind

import (
	"fmt"
	"strconv"
	"strings"
)

type Modifier uint8

const (
	ModifierCtrl Modifier = 1 << iota
	ModifierShift
	ModifierAlt
	ModifierSuper
	ModifierLevel3
	ModifierLevel5
)

type Modifiers uint8

type Trigger uint8

const (
	TriggerKey Trigger = iota
	TriggerMouse
	TriggerScroll
)

type Chord struct {
	Key       string
	Modifiers Modifiers
	UsesMod   bool
	Trigger   Trigger
}

type Action struct {
	Name           string
	Args           []string
	WorkspaceIndex uint8
}

type Binding struct {
	Chord   Chord
	Actions []Action
}

const (
	ActionCloseWindow       = "close-window"
	ActionSpawn             = "spawn"
	ActionSpawnSh           = "spawn-sh"
	ActionFocusColumnLeft   = "focus-column-left"
	ActionFocusColumnRight  = "focus-column-right"
	ActionFocusColumnFirst  = "focus-column-first"
	ActionFocusColumnLast   = "focus-column-last"
	ActionMoveColumnLeft    = "move-column-left"
	ActionMoveColumnRight   = "move-column-right"
	ActionMoveColumnToFirst = "move-column-to-first"
	ActionMoveColumnToLast  = "move-column-to-last"
	ActionFocusWorkspace    = "focus-workspace"
)

// ResolveModKey follows Niri's backend defaults: Super on native/headless,
// Alt in a nested Wayland session. An explicit nested key wins; otherwise the
// nested default flips when mod-key is Alt.
func ResolveModKey(normal, nested string, isNested bool) (Modifier, error) {
	normalKey := ModifierSuper
	if normal != "" {
		parsed, err := parseModifier(normal)
		if err != nil {
			return 0, fmt.Errorf("mod-key: %w", err)
		}
		normalKey = parsed
	}
	if !isNested {
		return normalKey, nil
	}
	if nested != "" {
		parsed, err := parseModifier(nested)
		if err != nil {
			return 0, fmt.Errorf("mod-key-nested: %w", err)
		}
		return parsed, nil
	}
	if normalKey == ModifierAlt {
		return ModifierSuper, nil
	}
	return ModifierAlt, nil
}

func parseModifier(name string) (Modifier, error) {
	switch strings.ToLower(name) {
	case "ctrl", "control":
		return ModifierCtrl, nil
	case "shift":
		return ModifierShift, nil
	case "alt":
		return ModifierAlt, nil
	case "super", "win":
		return ModifierSuper, nil
	case "iso_level3_shift", "mod5":
		return ModifierLevel3, nil
	case "iso_level5_shift", "mod3":
		return ModifierLevel5, nil
	default:
		return 0, fmt.Errorf("invalid modifier %q", name)
	}
}

func ParseModKey(name string) (Modifier, error) {
	return parseModifier(name)
}

// ParseChord parses modifiers followed by an XKB keysym name. compositor is
// the backend-resolved meaning of the special Mod modifier.
func ParseChord(spec string) (Chord, error) {
	parts := strings.Split(spec, "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return Chord{}, fmt.Errorf("empty key name in bind %q", spec)
	}
	key := parts[len(parts)-1]
	if strings.ContainsAny(key, " \t\r\n") || strings.Contains(key, "+") || isModifierName(key) {
		return Chord{}, fmt.Errorf("invalid XKB key name %q", key)
	}
	var modifiers Modifiers
	usesMod := false
	for _, part := range parts[:len(parts)-1] {
		if part == "" {
			return Chord{}, fmt.Errorf("empty modifier in bind %q", spec)
		}
		if strings.EqualFold(part, "mod") {
			if usesMod {
				return Chord{}, fmt.Errorf("duplicate modifier %q in bind %q", part, spec)
			}
			usesMod = true
			continue
		}
		modifier, err := parseModifier(part)
		if err != nil {
			return Chord{}, fmt.Errorf("bind %q: %w", spec, err)
		}
		if modifiers&Modifiers(modifier) != 0 {
			return Chord{}, fmt.Errorf("duplicate modifier %q in bind %q", part, spec)
		}
		modifiers |= Modifiers(modifier)
	}
	return Chord{Key: key, Modifiers: modifiers, UsesMod: usesMod, Trigger: triggerForKey(key)}, nil
}

func triggerForKey(key string) Trigger {
	switch strings.ToLower(key) {
	case "mouseleft", "mouseright", "mousemiddle", "mouseback", "mouseforward":
		return TriggerMouse
	case "wheelscrolldown", "wheelscrollup", "wheelscrollleft", "wheelscrollright",
		"touchpadscrolldown", "touchpadscrollup", "touchpadscrollleft", "touchpadscrollright":
		return TriggerScroll
	default:
		return TriggerKey
	}
}

// ResolveMod replaces the special Mod token after the backend is known.
func (c Chord) ResolveMod(compositor Modifier) Chord {
	if c.UsesMod {
		c.Modifiers |= Modifiers(compositor)
		c.UsesMod = false
	}
	return c
}

func isModifierName(name string) bool {
	_, err := parseModifier(name)
	return err == nil || strings.EqualFold(name, "mod")
}

// ParseAction returns supported=false for valid action names outside M5. The
// config loader keeps those names in its unapplied-action warnings.
func ParseAction(name string, args []string) (Action, bool, error) {
	action := Action{Name: name, Args: append([]string(nil), args...)}
	switch name {
	case ActionCloseWindow, ActionFocusColumnLeft, ActionFocusColumnRight,
		ActionFocusColumnFirst, ActionFocusColumnLast, ActionMoveColumnLeft,
		ActionMoveColumnRight, ActionMoveColumnToFirst, ActionMoveColumnToLast:
		if len(args) != 0 {
			return Action{}, true, fmt.Errorf("%s takes no arguments", name)
		}
		return action, true, nil
	case ActionSpawn:
		if len(args) == 0 {
			return Action{}, true, fmt.Errorf("spawn needs a command")
		}
		return action, true, nil
	case ActionSpawnSh:
		if len(args) != 1 {
			return Action{}, true, fmt.Errorf("spawn-sh takes one command string")
		}
		return action, true, nil
	case ActionFocusWorkspace:
		if len(args) != 1 {
			return Action{}, true, fmt.Errorf("focus-workspace takes one index")
		}
		index, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil || index == 0 {
			return Action{}, true, fmt.Errorf("focus-workspace index must be from 1 to 255")
		}
		action.WorkspaceIndex = uint8(index)
		return action, true, nil
	default:
		return action, false, nil
	}
}

func IsSupportedAction(name string) bool {
	switch name {
	case ActionCloseWindow, ActionSpawn, ActionSpawnSh,
		ActionFocusColumnLeft, ActionFocusColumnRight, ActionFocusColumnFirst,
		ActionFocusColumnLast, ActionMoveColumnLeft, ActionMoveColumnRight,
		ActionMoveColumnToFirst, ActionMoveColumnToLast, ActionFocusWorkspace:
		return true
	default:
		return false
	}
}
