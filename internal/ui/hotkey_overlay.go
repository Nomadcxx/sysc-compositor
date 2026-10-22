package ui

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/text"
)

// Niri's hotkey_overlay.rs constants.
const (
	hotkeyPadding      = 8
	hotkeyBorder       = 4
	hotkeyLineInterval = 2
	hotkeyTitle        = "Important Hotkeys"
	// HotkeyOverlayOpacity is the alpha Niri draws the panel with.
	HotkeyOverlayOpacity = 0.9
)

var (
	hotkeyFont      = text.Font{Family: "sans", Size: 14}
	hotkeyKeyBG     = color.NRGBA{47, 47, 47, 255}   // pango background 12000/65535
	hotkeyBorderRGB = color.RGBA{128, 204, 255, 255} // cairo rgb(0.5, 0.8, 1.0)
)

// HotkeyLine is one row of the overlay: the key, or "" when the action is
// not bound, and the action's title as Pango markup.
type HotkeyLine struct {
	Key, Action string
}

// overlayAction is an action as Niri's overlay compares them: by name,
// arguments and quit's skip-confirmation.
type overlayAction struct {
	name string
	args string
	skip bool
}

func actionOf(b config.OverlayBind) overlayAction {
	return overlayAction{b.Action, strings.Join(b.Args, "\x00"), b.SkipConfirmation}
}

// HotkeyLines is Niri's collect_actions and format_bind: the actions the
// overlay lists, in its order, each with the key that triggers it. modKey is
// what Mod means in this session; keyName gives an XKB key's canonical name.
func HotkeyLines(binds []config.OverlayBind, hideNotBound bool, modKey bind.Modifier, keyName func(string) string) []HotkeyLine {
	bound := func(match func(config.OverlayBind) bool) (config.OverlayBind, bool) {
		for _, b := range binds {
			if match(b) {
				return b, true
			}
		}
		return config.OverlayBind{}, false
	}
	named := func(name string) func(config.OverlayBind) bool {
		return func(b config.OverlayBind) bool { return b.Action == name }
	}

	actions := []overlayAction{{name: bind.ActionShowHotkeyOverlay}}
	// Prefer quit without skip-confirmation, then with it.
	quit := overlayAction{name: bind.ActionQuit}
	if _, ok := bound(func(b config.OverlayBind) bool { return actionOf(b) == quit }); !ok {
		if b, ok := bound(named(bind.ActionQuit)); ok {
			quit = actionOf(b)
		}
	}
	actions = append(actions, quit)
	for _, name := range []string{bind.ActionCloseWindow, bind.ActionFocusColumnLeft, bind.ActionFocusColumnRight,
		bind.ActionMoveColumnLeft, bind.ActionMoveColumnRight, "focus-workspace-down", "focus-workspace-up"} {
		actions = append(actions, overlayAction{name: name})
	}
	// Prefer moving the column to another workspace, then the window.
	for _, dir := range []string{"down", "up"} {
		column, window := "move-column-to-workspace-"+dir, "move-window-to-workspace-"+dir
		if b, ok := bound(named(column)); ok {
			actions = append(actions, actionOf(b))
		} else if _, ok := bound(named(window)); ok {
			actions = append(actions, overlayAction{name: window})
		} else {
			actions = append(actions, overlayAction{name: column})
		}
	}
	for _, name := range []string{"switch-preset-column-width", "maximize-column",
		bind.ActionConsumeOrExpelWindowLeft, bind.ActionConsumeOrExpelWindowRight,
		bind.ActionToggleWindowFloating, bind.ActionSwitchFocusFloatingAndTiling, bind.ActionToggleOverview} {
		actions = append(actions, overlayAction{name: name})
	}
	// The screenshot UI shows only when bound.
	if b, ok := bound(named(bind.ActionScreenshot)); ok {
		actions = append(actions, actionOf(b))
	}
	for _, b := range binds {
		if b.Title != nil && !slices.Contains(actions, actionOf(b)) {
			actions = append(actions, actionOf(b))
		}
	}
	// Spawns bound with Mod or Super on a key, so volume keys and the like
	// stay out.
	for _, b := range binds {
		spawn := b.Action == bind.ActionSpawn || b.Action == bind.ActionSpawnSh
		mod := b.Chord.UsesMod || b.Chord.Modifiers&bind.Modifiers(bind.ModifierSuper) != 0
		if spawn && mod && b.Chord.Trigger == bind.TriggerKey && !slices.Contains(actions, actionOf(b)) {
			actions = append(actions, actionOf(b))
		}
	}
	if hideNotBound {
		actions = slices.DeleteFunc(actions, func(a overlayAction) bool {
			_, ok := bound(func(b config.OverlayBind) bool { return actionOf(b) == a })
			return !ok
		})
	}

	var lines []HotkeyLine
	for _, a := range actions {
		var custom, plain *config.OverlayBind
		hidden := false
		for i := range binds {
			b := &binds[i]
			if actionOf(*b) != a {
				continue
			}
			switch {
			case b.Title != nil:
				if custom == nil {
					custom = b
				}
			case b.TitleHidden:
				hidden = true
			default:
				if plain == nil {
					plain = b
				}
			}
		}
		if custom == nil && hidden {
			continue
		}
		line := HotkeyLine{Action: actionTitle(a)}
		if b := custom; b != nil || plain != nil {
			if b == nil {
				b = plain
			}
			line.Key = KeyName(b.Chord, modKey, keyName)
			if b.Title != nil {
				line.Action = *b.Title
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// actionTitle is Niri's action_name.
func actionTitle(a overlayAction) string {
	switch a.name {
	case bind.ActionQuit:
		return "Exit sysc-compositor"
	case bind.ActionShowHotkeyOverlay:
		return "Show Important Hotkeys"
	case bind.ActionCloseWindow:
		return "Close Focused Window"
	case bind.ActionFocusColumnLeft:
		return "Focus Column to the Left"
	case bind.ActionFocusColumnRight:
		return "Focus Column to the Right"
	case bind.ActionMoveColumnLeft:
		return "Move Column Left"
	case bind.ActionMoveColumnRight:
		return "Move Column Right"
	case "focus-workspace-down":
		return "Switch Workspace Down"
	case "focus-workspace-up":
		return "Switch Workspace Up"
	case bind.ActionMoveColumnToWorkspaceDown:
		return "Move Column to Workspace Down"
	case bind.ActionMoveColumnToWorkspaceUp:
		return "Move Column to Workspace Up"
	case bind.ActionMoveWindowToWorkspaceDown:
		return "Move Window to Workspace Down"
	case bind.ActionMoveWindowToWorkspaceUp:
		return "Move Window to Workspace Up"
	case "switch-preset-column-width":
		return "Switch Preset Column Widths"
	case "maximize-column":
		return "Maximize Column"
	case bind.ActionConsumeOrExpelWindowLeft:
		return "Consume or Expel Window Left"
	case bind.ActionConsumeOrExpelWindowRight:
		return "Consume or Expel Window Right"
	case bind.ActionToggleWindowFloating:
		return "Move Window Between Floating and Tiling"
	case bind.ActionSwitchFocusFloatingAndTiling:
		return "Switch Focus Between Floating and Tiling"
	case bind.ActionToggleOverview:
		return "Open the Overview"
	case bind.ActionScreenshot:
		return "Take a Screenshot"
	case bind.ActionSpawn:
		first, _, _ := strings.Cut(a.args, "\x00")
		return "Spawn <span face='monospace' bgcolor='#000000'>" + escapeMarkup(first) + "</span>"
	case bind.ActionSpawnSh:
		fields := strings.Fields(a.args)
		first := ""
		if len(fields) > 0 {
			first = fields[0]
		}
		return "Spawn <span face='monospace' bgcolor='#000000'>" + escapeMarkup(first) + "</span>"
	}
	return "FIXME: Unknown"
}

// escapeMarkup escapes text for Pango markup. Niri interpolates the command
// raw, so markup characters there break its parse; here they show as typed.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "&apos;").Replace(s)
}

// KeyName is Niri's key_name: the modifiers, Mod first as what it means in
// this session, then the key prettified.
func KeyName(chord bind.Chord, modKey bind.Modifier, keyName func(string) string) string {
	var b strings.Builder
	mods := chord.Modifiers
	names := []struct {
		mod  bind.Modifier
		name string
	}{
		{bind.ModifierSuper, "Super"}, {bind.ModifierCtrl, "Ctrl"}, {bind.ModifierShift, "Shift"},
		{bind.ModifierAlt, "Alt"}, {bind.ModifierLevel3, "Mod5"}, {bind.ModifierLevel5, "Mod3"},
	}
	if chord.UsesMod {
		for _, n := range names {
			if n.mod == modKey {
				b.WriteString(n.name + " + ")
			}
		}
	}
	for _, n := range names {
		if mods&bind.Modifiers(n.mod) != 0 && !(chord.UsesMod && n.mod == modKey) {
			b.WriteString(n.name + " + ")
		}
	}
	b.WriteString(prettyKey(chord, keyName))
	return b.String()
}

func prettyKey(chord bind.Chord, keyName func(string) string) string {
	if chord.Trigger != bind.TriggerKey {
		// MouseLeft → Mouse Left, WheelScrollDown → Wheel Scroll Down.
		var b strings.Builder
		for i, r := range chord.Key {
			if i > 0 && r >= 'A' && r <= 'Z' {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	name := keyName(chord.Key)
	switch name {
	case "slash":
		name = "/"
	case "comma":
		name = ","
	case "period":
		name = "."
	case "minus":
		name = "-"
	case "equal":
		name = "="
	case "grave":
		name = "`"
	case "bracketleft":
		name = "["
	case "bracketright":
		name = "]"
	case "adiaeresis":
		name = "Ä"
	case "ediaeresis":
		name = "Ë"
	case "idiaeresis":
		name = "Ï"
	case "odiaeresis":
		name = "Ö"
	case "udiaeresis":
		name = "Ü"
	case "ydiaeresis":
		name = "Ÿ"
	case "wdiaeresis":
		name = "Ẅ"
	case "Next":
		name = "Page Down"
	case "Prior":
		name = "Page Up"
	case "Print":
		name = "PrtSc"
	case "Return":
		name = "Enter"
	case "space":
		name = "Space"
	}
	if len(name) == 1 && name[0] < 0x80 {
		name = strings.ToUpper(name)
	}
	return name
}

// HotkeyOverlay draws the overlay panel for lines at scale: a bold centred
// title, then a column of keys on a grey background beside their actions.
func HotkeyOverlay(lines []HotkeyLine, scale float64) (*image.RGBA, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	padding := physical(scale, hotkeyPadding)
	interval := physical(scale, hotkeyLineInterval)
	title, err := text.NewLayout([]text.Span{{Text: hotkeyTitle, Style: text.Style{Bold: true}}}, hotkeyFont, scale, text.AlignLeft)
	if err != nil {
		return nil, err
	}
	keyStyle := text.Style{Family: "Monospace", BG: &hotkeyKeyBG}
	keys := make([]*text.Layout, len(lines))
	acts := make([]*text.Layout, len(lines))
	keyWidth, actWidth := 0, 0
	height := title.Size().Y + padding
	for i, line := range lines {
		key := line.Key
		if key == "" {
			key = "(not bound)"
		}
		if keys[i], err = text.NewLayout([]text.Span{{Text: " " + key + " ", Style: keyStyle}}, hotkeyFont, scale, text.AlignLeft); err != nil {
			return nil, err
		}
		spans, err := text.ParseMarkup(line.Action)
		if err != nil {
			spans = text.Plain(line.Action)
		}
		if acts[i], err = text.NewLayout(spans, hotkeyFont, scale, text.AlignLeft); err != nil {
			return nil, err
		}
		keyWidth = max(keyWidth, keys[i].Size().X)
		actWidth = max(actWidth, acts[i].Size().X)
		height += max(keys[i].Size().Y, acts[i].Size().Y)
	}
	height += (len(lines) - 1) * interval
	width := keyWidth + padding + actWidth + 2*padding
	height += 2 * padding

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(exitDialogBackground), image.Point{}, draw.Src)
	title.Draw(img, image.Pt((width-title.Size().X)/2, padding), color.White)
	y := padding + title.Size().Y + padding
	for i := range lines {
		keys[i].Draw(img, image.Pt(padding, y), color.White)
		acts[i].Draw(img, image.Pt(padding+keyWidth+padding, y), color.White)
		y += max(keys[i].Size().Y, acts[i].Size().Y) + interval
	}
	strokeBorder(img, int(math.Round(hotkeyBorder/2*scale)), hotkeyBorderRGB)
	return img, nil
}
