package ui

import (
	"image/color"
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func overlayLines(t *testing.T, kdl string, hideNotBound bool) []HotkeyLine {
	t.Helper()
	cfg, err := config.Parse("overlay.kdl", []byte(kdl))
	if err != nil {
		t.Fatal(err)
	}
	return HotkeyLines(cfg.OverlayBinds, hideNotBound, bind.ModifierSuper, func(k string) string { return k })
}

// lineFor is Niri's test check(): " key : title" for the line titled or
// keyed like want, or "None".
func lineFor(lines []HotkeyLine, title string) string {
	for _, l := range lines {
		if l.Action == title || strings.HasPrefix(l.Action, title) {
			key := l.Key
			if key == "" {
				key = "(not bound)"
			}
			return " " + key + " : " + l.Action
		}
	}
	return "None"
}

// TestHotkeyFormatBindMatchesNiri ports Niri's test_format_bind snapshots,
// with the custom titles found by key since they replace the action's name.
func TestHotkeyFormatBindMatchesNiri(t *testing.T) {
	for _, tc := range []struct {
		kdl, find, want string
	}{
		{``, "Take a Screenshot", "None"}, // not bound: the screenshot row is left out
		{"binds {\nMod+P { screenshot; }\n}", "Take a Screenshot", " Super + P : Take a Screenshot"},
		{"binds {\nMod+P hotkey-overlay-title=\"Hello\" { screenshot; }\n}", "Hello", " Super + P : Hello"},
		{"binds {\nMod+P { screenshot; }\nPrint { screenshot; }\n}", "Take a Screenshot", " Super + P : Take a Screenshot"},
		{"binds {\nMod+P { screenshot; }\nPrint hotkey-overlay-title=\"My Cool Bind\" { screenshot; }\n}", "My Cool Bind", " PrtSc : My Cool Bind"},
		{"binds {\nMod+P hotkey-overlay-title=\"First\" { screenshot; }\nPrint hotkey-overlay-title=\"My Cool Bind\" { screenshot; }\n}", "First", " Super + P : First"},
		{"binds {\nMod+P { screenshot; }\nPrint hotkey-overlay-title=null { screenshot; }\n}", "Take a Screenshot", "None"},
		{"binds {\nMod+P hotkey-overlay-title=\"Hello\" { screenshot; }\nPrint hotkey-overlay-title=null { screenshot; }\n}", "Hello", " Super + P : Hello"},
	} {
		if got := lineFor(overlayLines(t, tc.kdl, false), tc.find); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.kdl, got, tc.want)
		}
	}
}

func TestHotkeyLinesFollowNiriOrderAndFilters(t *testing.T) {
	lines := overlayLines(t, `
binds {
    Mod+Shift+Slash { show-hotkey-overlay; }
    Mod+Shift+E { quit; }
    Ctrl+Alt+Delete { quit skip-confirmation=true; }
    Mod+Q { close-window; }
    Mod+Page_Down { focus-workspace-down; }
    Mod+Ctrl+Page_Down { move-window-to-workspace-down; }
    Mod+R { switch-preset-column-width; }
    Mod+T hotkey-overlay-title="Open a Terminal: alacritty" { spawn "alacritty"; }
    Mod+D { spawn-sh "fuzzel --dmenu"; }
    XF86AudioRaiseVolume { spawn "wpctl" "set-volume"; }
    Mod+WheelScrollDown { spawn "never"; }
}
`, false)
	want := []HotkeyLine{
		{"Super + Shift + Slash", "Show Important Hotkeys"}, // keyName is identity here; xkb makes it "/"
		{"Super + Shift + E", "Exit sysc-compositor"},
		{"Super + Q", "Close Focused Window"},
		{"", "Focus Column to the Left"},
		{"", "Focus Column to the Right"},
		{"", "Move Column Left"},
		{"", "Move Column Right"},
		{"Super + Page_Down", "Switch Workspace Down"},
		{"", "Switch Workspace Up"},
		{"Super + Ctrl + Page_Down", "Move Window to Workspace Down"},
		{"", "Move Column to Workspace Up"},
		{"Super + R", "Switch Preset Column Widths"},
		{"", "Maximize Column"},
		{"", "Consume or Expel Window Left"},
		{"", "Consume or Expel Window Right"},
		{"", "Move Window Between Floating and Tiling"},
		{"", "Switch Focus Between Floating and Tiling"},
		{"", "Open the Overview"},
		{"Super + T", "Open a Terminal: alacritty"},
		{"Super + D", "Spawn <span face='monospace' bgcolor='#000000'>fuzzel</span>"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, lines[i], want[i])
		}
	}

	only := overlayLines(t, "hotkey-overlay { hide-not-bound; }\nbinds {\nCtrl+Alt+Delete { quit skip-confirmation=true; }\n}\n", true)
	if len(only) != 1 || only[0] != (HotkeyLine{"Ctrl + Alt + Delete", "Exit sysc-compositor"}) {
		t.Errorf("hide-not-bound with only quit skip-confirmation bound = %+v", only)
	}
}

func TestHotkeyKeyNameMatchesNiri(t *testing.T) {
	canon := func(k string) string {
		return map[string]string{"t": "t", "Return": "Return", "slash": "slash", "Print": "Print", "Next": "Next", "space": "space"}[k]
	}
	for _, tc := range []struct {
		spec string
		mod  bind.Modifier
		want string
	}{
		{"Mod+t", bind.ModifierSuper, "Super + T"},
		{"Mod+Shift+Return", bind.ModifierAlt, "Alt + Shift + Enter"},
		{"Mod+Super+slash", bind.ModifierSuper, "Super + /"},
		{"Mod+Super+slash", bind.ModifierAlt, "Alt + Super + /"},
		{"Ctrl+Print", bind.ModifierSuper, "Ctrl + PrtSc"},
		{"Mod+Next", bind.ModifierSuper, "Super + Page Down"},
		{"Mod5+space", bind.ModifierSuper, "Mod5 + Space"},
		{"Mod+WheelScrollDown", bind.ModifierSuper, "Super + Wheel Scroll Down"},
	} {
		chord, err := bind.ParseChord(tc.spec)
		if err != nil {
			t.Fatal(err)
		}
		if got := KeyName(chord, tc.mod, canon); got != tc.want {
			t.Errorf("KeyName(%s, Mod=%v) = %q, want %q", tc.spec, tc.mod, got, tc.want)
		}
	}
}

func TestHotkeyOverlayPanelLaysOutKeysAndActions(t *testing.T) {
	lines := []HotkeyLine{{"Super + Shift + /", "Show Important Hotkeys"}, {"", "Open the Overview"},
		{"Super + T", "Spawn <span face='monospace' bgcolor='#000000'>alacritty</span>"}}
	img, err := HotkeyOverlay(lines, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 250 || b.Dy() < 4*14+3*8 {
		t.Fatalf("panel is %v", b)
	}
	if img.RGBAAt(0, 0) != hotkeyBorderRGB || img.RGBAAt(1, b.Dy()/2) != hotkeyBorderRGB || img.RGBAAt(2, b.Dy()/2) != exitDialogBackground {
		t.Error("border is not Niri's 2px blue frame")
	}
	keyBG, black := 0, 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			switch img.RGBAAt(x, y) {
			case color.RGBA{47, 47, 47, 255}:
				keyBG++
			case color.RGBA{0, 0, 0, 255}:
				black++
			}
		}
	}
	if keyBG < 300 || black < 50 {
		t.Errorf("key background pixels %d, spawn command background pixels %d", keyBG, black)
	}
	if empty, err := HotkeyOverlay(nil, 1); empty != nil || err != nil {
		t.Errorf("no lines drew %v, %v", empty, err)
	}
}
