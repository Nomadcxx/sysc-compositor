// Package bind parses the key and action subset applied by the compositor.
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
	WorkspaceName  string
	ColumnIndex    uint8
	ColumnDisplay  string
	Output         string
	Focus          bool
}

type Binding struct {
	Chord   Chord
	Actions []Action
	Repeat  bool
	// AllowWhenLocked lets the bind's spawn actions run while the session is
	// locked (Niri's allow-when-locked=true).
	AllowWhenLocked bool
	// IgnoresInhibit keeps the bind working while the focused surface
	// inhibits shortcuts (Niri's allow-inhibiting=false).
	IgnoresInhibit bool
}

const (
	ActionCloseWindow                    = "close-window"
	ActionSpawn                          = "spawn"
	ActionSpawnSh                        = "spawn-sh"
	ActionFocusColumnLeft                = "focus-column-left"
	ActionFocusColumnRight               = "focus-column-right"
	ActionFocusColumnFirst               = "focus-column-first"
	ActionFocusColumnLast                = "focus-column-last"
	ActionMoveColumnLeft                 = "move-column-left"
	ActionMoveColumnRight                = "move-column-right"
	ActionMoveColumnToFirst              = "move-column-to-first"
	ActionMoveColumnToLast               = "move-column-to-last"
	ActionFocusWorkspace                 = "focus-workspace"
	ActionConsumeWindowIntoColumn        = "consume-window-into-column"
	ActionExpelWindowFromColumn          = "expel-window-from-column"
	ActionConsumeOrExpelWindowLeft       = "consume-or-expel-window-left"
	ActionConsumeOrExpelWindowRight      = "consume-or-expel-window-right"
	ActionToggleColumnTabbedDisplay      = "toggle-column-tabbed-display"
	ActionSetColumnDisplay               = "set-column-display"
	ActionFocusWindowInColumn            = "focus-window-in-column"
	ActionFocusWindowDown                = "focus-window-down"
	ActionFocusWindowUp                  = "focus-window-up"
	ActionMoveWindowDown                 = "move-window-down"
	ActionMoveWindowUp                   = "move-window-up"
	ActionToggleWindowFloating           = "toggle-window-floating"
	ActionToggleOverview                 = "toggle-overview"
	ActionToggleKeyboardShortcutsInhibit = "toggle-keyboard-shortcuts-inhibit"
	ActionPowerOffMonitors               = "power-off-monitors"
	ActionPowerOnMonitors                = "power-on-monitors"
	ActionOpenOverview                   = "open-overview"
	ActionCloseOverview                  = "close-overview"
	ActionMoveWindowToFloating           = "move-window-to-floating"
	ActionMoveWindowToTiling             = "move-window-to-tiling"
	ActionFocusFloating                  = "focus-floating"
	ActionFocusTiling                    = "focus-tiling"
	ActionSwitchFocusFloatingAndTiling   = "switch-focus-between-floating-and-tiling"
	ActionFullscreenWindow               = "fullscreen-window"
	ActionToggleWindowedFullscreen       = "toggle-windowed-fullscreen"
	ActionSetWorkspaceName               = "set-workspace-name"
	ActionUnsetWorkspaceName             = "unset-workspace-name"
	ActionMoveWindowToWorkspace          = "move-window-to-workspace"
	ActionMoveWindowToWorkspaceDown      = "move-window-to-workspace-down"
	ActionMoveWindowToWorkspaceUp        = "move-window-to-workspace-up"
	ActionMoveColumnToWorkspace          = "move-column-to-workspace"
	ActionMoveColumnToWorkspaceDown      = "move-column-to-workspace-down"
	ActionMoveColumnToWorkspaceUp        = "move-column-to-workspace-up"
	ActionMoveWindowToMonitor            = "move-window-to-monitor"
	ActionMoveColumnToMonitor            = "move-column-to-monitor"
	ActionMoveWorkspaceToMonitor         = "move-workspace-to-monitor"
	ActionFocusMonitor                   = "focus-monitor"
	ActionMoveWindowToMonitorLeft        = "move-window-to-monitor-left"
	ActionMoveWindowToMonitorRight       = "move-window-to-monitor-right"
	ActionMoveWindowToMonitorUp          = "move-window-to-monitor-up"
	ActionMoveWindowToMonitorDown        = "move-window-to-monitor-down"
	ActionMoveWindowToMonitorPrevious    = "move-window-to-monitor-previous"
	ActionMoveWindowToMonitorNext        = "move-window-to-monitor-next"
	ActionMoveColumnToMonitorLeft        = "move-column-to-monitor-left"
	ActionMoveColumnToMonitorRight       = "move-column-to-monitor-right"
	ActionMoveColumnToMonitorUp          = "move-column-to-monitor-up"
	ActionMoveColumnToMonitorDown        = "move-column-to-monitor-down"
	ActionMoveColumnToMonitorPrevious    = "move-column-to-monitor-previous"
	ActionMoveColumnToMonitorNext        = "move-column-to-monitor-next"
	ActionMoveWorkspaceToMonitorLeft     = "move-workspace-to-monitor-left"
	ActionMoveWorkspaceToMonitorRight    = "move-workspace-to-monitor-right"
	ActionMoveWorkspaceToMonitorUp       = "move-workspace-to-monitor-up"
	ActionMoveWorkspaceToMonitorDown     = "move-workspace-to-monitor-down"
	ActionMoveWorkspaceToMonitorPrevious = "move-workspace-to-monitor-previous"
	ActionMoveWorkspaceToMonitorNext     = "move-workspace-to-monitor-next"
	ActionFocusMonitorLeft               = "focus-monitor-left"
	ActionFocusMonitorRight              = "focus-monitor-right"
	ActionFocusMonitorUp                 = "focus-monitor-up"
	ActionFocusMonitorDown               = "focus-monitor-down"
	ActionFocusMonitorPrevious           = "focus-monitor-previous"
	ActionFocusMonitorNext               = "focus-monitor-next"
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

// ParseAction returns supported=false for valid action names outside the
// implemented Niri subset. The config loader keeps those names in warnings.
func ParseAction(name string, args []string) (Action, bool, error) {
	action := Action{Name: name, Args: append([]string(nil), args...)}
	switch name {
	case ActionCloseWindow, ActionFocusColumnLeft, ActionFocusColumnRight,
		ActionFocusColumnFirst, ActionFocusColumnLast, ActionMoveColumnLeft,
		ActionMoveColumnRight, ActionMoveColumnToFirst, ActionMoveColumnToLast,
		ActionConsumeWindowIntoColumn, ActionExpelWindowFromColumn,
		ActionConsumeOrExpelWindowLeft, ActionConsumeOrExpelWindowRight,
		ActionToggleColumnTabbedDisplay, ActionFocusWindowDown, ActionFocusWindowUp,
		ActionMoveWindowDown, ActionMoveWindowUp, ActionToggleWindowFloating,
		ActionMoveWindowToFloating, ActionMoveWindowToTiling,
		ActionFocusFloating, ActionFocusTiling, ActionSwitchFocusFloatingAndTiling,
		ActionFullscreenWindow, ActionToggleWindowedFullscreen,
		ActionToggleOverview, ActionOpenOverview, ActionCloseOverview,
		ActionToggleKeyboardShortcutsInhibit, ActionPowerOffMonitors, ActionPowerOnMonitors:
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
		if err := parseWorkspaceReference(&action, args); err != nil {
			return Action{}, true, err
		}
		return action, true, nil
	case ActionMoveColumnToWorkspace, ActionMoveWindowToWorkspace:
		if err := parseWorkspaceReference(&action, args); err != nil {
			return Action{}, true, err
		}
		action.Focus = true
		return action, true, nil
	case ActionMoveWindowToWorkspaceDown, ActionMoveWindowToWorkspaceUp,
		ActionMoveColumnToWorkspaceDown, ActionMoveColumnToWorkspaceUp:
		if len(args) != 0 {
			return Action{}, true, fmt.Errorf("%s takes no arguments", name)
		}
		action.Focus = true
		return action, true, nil
	case ActionSetWorkspaceName:
		if len(args) != 1 || args[0] == "" {
			return Action{}, true, fmt.Errorf("set-workspace-name takes one non-empty name")
		}
		action.WorkspaceName = args[0]
		return action, true, nil
	case ActionUnsetWorkspaceName:
		if len(args) != 0 {
			return Action{}, true, fmt.Errorf("unset-workspace-name takes no arguments")
		}
		return action, true, nil
	case ActionMoveWindowToMonitor, ActionMoveColumnToMonitor, ActionMoveWorkspaceToMonitor, ActionFocusMonitor:
		if len(args) != 1 || args[0] == "" {
			return Action{}, true, fmt.Errorf("%s takes one non-empty output name", name)
		}
		action.Output = args[0]
		return action, true, nil
	case ActionMoveWindowToMonitorLeft, ActionMoveWindowToMonitorRight,
		ActionMoveWindowToMonitorUp, ActionMoveWindowToMonitorDown,
		ActionMoveWindowToMonitorPrevious, ActionMoveWindowToMonitorNext,
		ActionMoveColumnToMonitorLeft, ActionMoveColumnToMonitorRight,
		ActionMoveColumnToMonitorUp, ActionMoveColumnToMonitorDown,
		ActionMoveColumnToMonitorPrevious, ActionMoveColumnToMonitorNext,
		ActionMoveWorkspaceToMonitorLeft, ActionMoveWorkspaceToMonitorRight,
		ActionMoveWorkspaceToMonitorUp, ActionMoveWorkspaceToMonitorDown,
		ActionMoveWorkspaceToMonitorPrevious, ActionMoveWorkspaceToMonitorNext,
		ActionFocusMonitorLeft, ActionFocusMonitorRight, ActionFocusMonitorUp,
		ActionFocusMonitorDown, ActionFocusMonitorPrevious, ActionFocusMonitorNext:
		if len(args) != 0 {
			return Action{}, true, fmt.Errorf("%s takes no arguments", name)
		}
		return action, true, nil
	case ActionFocusWindowInColumn:
		if len(args) != 1 {
			return Action{}, true, fmt.Errorf("focus-window-in-column takes one index")
		}
		index, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil || index == 0 {
			return Action{}, true, fmt.Errorf("focus-window-in-column index must be from 1 to 255")
		}
		action.ColumnIndex = uint8(index)
		return action, true, nil
	case ActionSetColumnDisplay:
		if len(args) != 1 || (args[0] != "normal" && args[0] != "tabbed") {
			return Action{}, true, fmt.Errorf("set-column-display takes `normal` or `tabbed`")
		}
		action.ColumnDisplay = args[0]
		return action, true, nil
	default:
		return action, false, nil
	}
}

func parseWorkspaceReference(action *Action, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%s takes one index or name", action.Name)
	}
	index, err := strconv.ParseUint(args[0], 10, 64)
	if err == nil {
		if index == 0 || index > 255 {
			return fmt.Errorf("%s index must be from 1 to 255", action.Name)
		}
		action.WorkspaceIndex = uint8(index)
		return nil
	}
	if args[0] == "" {
		return fmt.Errorf("%s name must not be empty", action.Name)
	}
	action.WorkspaceName = args[0]
	return nil
}

func IsSupportedAction(name string) bool {
	switch name {
	case ActionCloseWindow, ActionSpawn, ActionSpawnSh,
		ActionFocusColumnLeft, ActionFocusColumnRight, ActionFocusColumnFirst,
		ActionFocusColumnLast, ActionMoveColumnLeft, ActionMoveColumnRight,
		ActionMoveColumnToFirst, ActionMoveColumnToLast, ActionFocusWorkspace,
		ActionConsumeWindowIntoColumn, ActionExpelWindowFromColumn,
		ActionConsumeOrExpelWindowLeft, ActionConsumeOrExpelWindowRight,
		ActionToggleColumnTabbedDisplay, ActionSetColumnDisplay, ActionFocusWindowInColumn,
		ActionFocusWindowDown, ActionFocusWindowUp, ActionMoveWindowDown, ActionMoveWindowUp,
		ActionToggleWindowFloating, ActionMoveWindowToFloating, ActionMoveWindowToTiling,
		ActionFocusFloating, ActionFocusTiling, ActionSwitchFocusFloatingAndTiling,
		ActionFullscreenWindow, ActionToggleWindowedFullscreen,
		ActionToggleOverview, ActionOpenOverview, ActionCloseOverview,
		ActionToggleKeyboardShortcutsInhibit, ActionPowerOffMonitors, ActionPowerOnMonitors,
		ActionSetWorkspaceName, ActionUnsetWorkspaceName,
		ActionMoveWindowToWorkspace, ActionMoveWindowToWorkspaceDown, ActionMoveWindowToWorkspaceUp,
		ActionMoveColumnToWorkspace, ActionMoveColumnToWorkspaceDown, ActionMoveColumnToWorkspaceUp,
		ActionMoveWindowToMonitor, ActionMoveColumnToMonitor, ActionMoveWorkspaceToMonitor, ActionFocusMonitor,
		ActionMoveWindowToMonitorLeft, ActionMoveWindowToMonitorRight,
		ActionMoveWindowToMonitorUp, ActionMoveWindowToMonitorDown,
		ActionMoveWindowToMonitorPrevious, ActionMoveWindowToMonitorNext,
		ActionMoveColumnToMonitorLeft, ActionMoveColumnToMonitorRight,
		ActionMoveColumnToMonitorUp, ActionMoveColumnToMonitorDown,
		ActionMoveColumnToMonitorPrevious, ActionMoveColumnToMonitorNext,
		ActionMoveWorkspaceToMonitorLeft, ActionMoveWorkspaceToMonitorRight,
		ActionMoveWorkspaceToMonitorUp, ActionMoveWorkspaceToMonitorDown,
		ActionMoveWorkspaceToMonitorPrevious, ActionMoveWorkspaceToMonitorNext,
		ActionFocusMonitorLeft, ActionFocusMonitorRight, ActionFocusMonitorUp,
		ActionFocusMonitorDown, ActionFocusMonitorPrevious, ActionFocusMonitorNext:
		return true
	default:
		return false
	}
}
