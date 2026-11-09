package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// parseAction follows the argument groups in niri-ipc 26.4.0's clap enum.
func parseAction(args []string) (ipc.Action, error) {
	if len(args) == 0 {
		return ipc.Action{}, errors.New("an action is required")
	}
	name := ""
	for _, candidate := range ipc.Actions {
		var command strings.Builder
		for i, r := range candidate {
			if r >= 'A' && r <= 'Z' {
				if i > 0 {
					command.WriteByte('-')
				}
				r += 'a' - 'A'
			}
			command.WriteRune(r)
		}
		if command.String() == args[0] {
			name = candidate
			break
		}
	}
	a := ipc.Action{Name: name}
	if name == "" {
		return a, fmt.Errorf("unknown action %q", args[0])
	}
	if name == "Spawn" || name == "SpawnSh" {
		if len(args) < 3 || args[1] != "--" {
			return a, errors.New("the command goes after `--`")
		}
		if name == "Spawn" {
			a.Command = args[2:]
		} else {
			if len(args) != 3 {
				return a, errors.New("spawn-sh takes one command string")
			}
			a.ShellCommand = args[2]
		}
		return a, nil
	}
	allowed := ""
	minArgs, maxArgs := 0, 0
	switch name {
	case "Quit":
		allowed = "skip-confirmation"
	case "DoScreenTransition":
		allowed = "delay-ms"
	case "Screenshot":
		allowed = "show-pointer path"
	case "ScreenshotScreen":
		allowed = "write-to-disk show-pointer path"
	case "ScreenshotWindow":
		allowed = "id write-to-disk show-pointer path"
	case "CloseWindow", "FullscreenWindow", "ToggleWindowedFullscreen", "FocusWindow",
		"ConsumeOrExpelWindowLeft", "ConsumeOrExpelWindowRight", "CenterWindow", "ResetWindowHeight",
		"SwitchPresetWindowWidth", "SwitchPresetWindowWidthBack", "SwitchPresetWindowHeight", "SwitchPresetWindowHeightBack",
		"MaximizeWindowToEdges", "ToggleWindowFloating", "MoveWindowToFloating", "MoveWindowToTiling",
		"ToggleWindowRuleOpacity", "SetDynamicCastWindow", "ToggleWindowUrgent", "SetWindowUrgent", "UnsetWindowUrgent":
		allowed = "id"
	case "FocusWindowInColumn", "FocusColumn", "MoveColumnToIndex", "SetColumnDisplay", "FocusWorkspace", "FocusMonitor", "MoveColumnToMonitor", "SetColumnWidth", "SwitchLayout":
		minArgs, maxArgs = 1, 1
	case "MoveWindowToWorkspaceDown", "MoveWindowToWorkspaceUp", "MoveColumnToWorkspaceDown", "MoveColumnToWorkspaceUp":
		allowed = "focus"
	case "MoveWindowToWorkspace":
		allowed = "window-id focus"
		minArgs, maxArgs = 1, 1
	case "MoveColumnToWorkspace":
		allowed = "focus"
		minArgs, maxArgs = 1, 1
	case "MoveWorkspaceToIndex":
		allowed = "reference"
		minArgs, maxArgs = 1, 1
	case "SetWorkspaceName":
		allowed = "workspace"
		minArgs, maxArgs = 1, 1
	case "UnsetWorkspaceName", "SetDynamicCastMonitor":
		maxArgs = 1
	case "MoveWindowToMonitor", "SetWindowWidth", "SetWindowHeight":
		allowed = "id"
		minArgs, maxArgs = 1, 1
	case "MoveWorkspaceToMonitor":
		allowed = "reference"
		minArgs, maxArgs = 1, 1
	case "MoveFloatingWindow":
		allowed = "id x y"
	case "StopCast":
		allowed = "session-id"
	case "LoadConfigFile":
		allowed = "path"
	}
	positional, options, err := actionOptions(args[1:], allowed)
	if err != nil {
		return a, err
	}
	if len(positional) < minArgs || len(positional) > maxArgs {
		return a, fmt.Errorf("%s expects %d to %d positional arguments", args[0], minArgs, maxArgs)
	}
	for _, key := range []string{"id", "window-id", "session-id"} {
		if value, ok := options[key]; ok {
			id, e := strconv.ParseUint(value, 10, 64)
			if e != nil {
				return a, fmt.Errorf("invalid %s %q", key, value)
			}
			if key == "session-id" {
				a.SessionID = id
			} else {
				a.WindowID = &id
			}
		}
	}
	for _, key := range []string{"reference", "workspace"} {
		if value, ok := options[key]; ok {
			a.Workspace, err = parseWorkspaceReference(value)
			if err != nil {
				return a, err
			}
		}
	}
	if strings.Contains(allowed, "focus") {
		value := true
		if text, ok := options["focus"]; ok {
			value, err = actionBool(text)
			if err != nil {
				return a, err
			}
		}
		a.Focus = &value
	}
	if path, ok := options["path"]; ok {
		if name == "LoadConfigFile" {
			a.ConfigPath = &path
		} else {
			a.Path = &path
		}
	}
	switch name {
	case "Quit":
		_, a.SkipConfirmation = options["skip-confirmation"]
	case "DoScreenTransition":
		if text, ok := options["delay-ms"]; ok {
			delay, e := strconv.ParseUint(text, 10, 16)
			if e != nil {
				return a, fmt.Errorf("invalid delay %q", text)
			}
			value := uint16(delay)
			a.DelayMS = &value
		}
	case "Screenshot", "ScreenshotScreen", "ScreenshotWindow":
		a.ShowPointer = name != "ScreenshotWindow"
		a.WriteToDisk = name != "Screenshot"
		if text, ok := options["show-pointer"]; ok {
			a.ShowPointer, err = actionBool(text)
			if err != nil {
				return a, err
			}
		}
		if text, ok := options["write-to-disk"]; ok {
			a.WriteToDisk, err = actionBool(text)
			if err != nil {
				return a, err
			}
		}
	case "FocusWindow", "ToggleWindowUrgent", "SetWindowUrgent", "UnsetWindowUrgent":
		if a.WindowID == nil {
			return a, errors.New("--id is required")
		}
	case "FocusWindowInColumn":
		value, e := parseUint8(positional[0])
		if e != nil {
			return a, e
		}
		a.ColumnIndex = &value
	case "FocusColumn", "MoveColumnToIndex", "MoveWorkspaceToIndex":
		value, e := strconv.ParseUint(positional[0], 10, 64)
		if e != nil {
			return a, fmt.Errorf("invalid index %q", positional[0])
		}
		a.Index = &value
	case "SetColumnDisplay":
		switch positional[0] {
		case "normal":
			a.ColumnDisplay = "Normal"
		case "tabbed":
			a.ColumnDisplay = "Tabbed"
		default:
			return a, errors.New("display must be normal or tabbed")
		}
	case "FocusWorkspace", "MoveWindowToWorkspace", "MoveColumnToWorkspace":
		a.Workspace, err = parseWorkspaceReference(positional[0])
		if err != nil {
			return a, err
		}
	case "SetWorkspaceName":
		a.WorkspaceName = &positional[0]
	case "UnsetWorkspaceName":
		if len(positional) > 0 {
			a.Workspace, err = parseWorkspaceReference(positional[0])
			if err != nil {
				return a, err
			}
		}
	case "FocusMonitor", "MoveColumnToMonitor", "MoveWindowToMonitor", "MoveWorkspaceToMonitor":
		a.Output = positional[0]
	case "SetColumnWidth", "SetWindowWidth", "SetWindowHeight":
		change, e := actionSizeChange(positional[0])
		if e != nil {
			return a, e
		}
		a.Size = &change
	case "SwitchLayout":
		switch positional[0] {
		case "next":
			a.Layout = "Next"
		case "prev":
			a.Layout = "Prev"
		default:
			value, e := parseUint8(positional[0])
			if e != nil {
				return a, e
			}
			a.Layout, a.LayoutIndex = "Index", &value
		}
	case "MoveFloatingWindow":
		a.X, a.Y = ipc.PositionChange{Kind: ipc.PositionAdjustFixed}, ipc.PositionChange{Kind: ipc.PositionAdjustFixed}
		for _, key := range []string{"x", "y"} {
			if text, ok := options[key]; ok {
				change, e := actionPositionChange(text)
				if e != nil {
					return a, e
				}
				if key == "x" {
					a.X = change
				} else {
					a.Y = change
				}
			}
		}
	case "SetDynamicCastMonitor":
		if len(positional) > 0 {
			a.CastOutput = &positional[0]
		}
	case "StopCast":
		if _, ok := options["session-id"]; !ok {
			return a, errors.New("--session-id is required")
		}
	}
	return a, nil
}

// Unlike flag.FlagSet, Niri permits options after positional arguments and
// negative size values. Extract only each action's declared options.
func actionOptions(args []string, allowed string) ([]string, map[string]string, error) {
	var positional []string
	options := map[string]string{}
	accepts := func(key string) bool {
		for _, option := range strings.Fields(allowed) {
			if option == key {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(args); i++ {
		argument := args[i]
		if argument == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(argument) < 2 || argument[0] != '-' || (argument[1] >= '0' && argument[1] <= '9') || argument[1] == '.' {
			positional = append(positional, argument)
			continue
		}
		var key, value string
		var supplied bool
		if strings.HasPrefix(argument, "--") {
			key, value, supplied = strings.Cut(argument[2:], "=")
		} else {
			key = argument[1:2]
			if len(argument) > 2 {
				value = strings.TrimPrefix(argument[2:], "=")
				supplied = true
			}
			switch key {
			case "s":
				key = "skip-confirmation"
			case "d":
				if accepts("delay-ms") {
					key = "delay-ms"
				} else {
					key = "write-to-disk"
				}
			case "p":
				key = "show-pointer"
			}
		}
		if !accepts(key) {
			return nil, nil, fmt.Errorf("unexpected option %q", argument)
		}
		if _, duplicate := options[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate option --%s", key)
		}
		if key == "skip-confirmation" {
			if supplied {
				return nil, nil, fmt.Errorf("--%s takes no value", key)
			}
			value = "true"
		} else if !supplied {
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return nil, nil, fmt.Errorf("--%s needs a value", key)
			}
			value = args[i]
		}
		options[key] = value
	}
	return positional, options, nil
}

func actionBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("expected true or false, got %q", value)
	}
}

func actionSizeChange(text string) (ipc.SizeChange, error) {
	var change ipc.SizeChange
	if text == "" {
		return change, errors.New("size must not be empty")
	}
	relative := text[0] == '+' || text[0] == '-'
	if strings.HasSuffix(text, "%") {
		value, err := parseFiniteFloat(strings.TrimSuffix(text, "%"), "size")
		if err != nil {
			return change, err
		}
		change.Kind, change.Value = ipc.SizeSetProportion, value
		if relative {
			change.Kind = ipc.SizeAdjustProportion
		}
	} else {
		value, err := strconv.ParseInt(text, 10, 32)
		if err != nil {
			return change, fmt.Errorf("invalid size %q", text)
		}
		change.Kind, change.Value = ipc.SizeSetFixed, float64(value)
		if relative {
			change.Kind = ipc.SizeAdjustFixed
		}
	}
	return change, nil
}

func actionPositionChange(text string) (ipc.PositionChange, error) {
	if text == "" {
		return ipc.PositionChange{}, errors.New("position must not be empty")
	}
	relative := text[0] == '+' || text[0] == '-'
	proportion := strings.HasSuffix(text, "%")
	value, err := parseFiniteFloat(strings.TrimSuffix(text, "%"), "position")
	if err != nil {
		return ipc.PositionChange{}, err
	}
	kind := ipc.PositionSetFixed
	if proportion {
		kind = ipc.PositionSetProportion
	}
	if relative {
		if proportion {
			kind = ipc.PositionAdjustProportion
		} else {
			kind = ipc.PositionAdjustFixed
		}
	}
	return ipc.PositionChange{Kind: kind, Value: value}, nil
}
