package control

import "fmt"

// Evdev keycodes for the injected keyboard, whose keymap is fixed to US
// (internal/wlr seat.go). Names follow xkb keysym spelling where one exists.
const keyShift uint32 = 42

var keyCodes = map[string]uint32{
	"Escape": 1, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6, "6": 7, "7": 8, "8": 9, "9": 10, "0": 11,
	"minus": 12, "equal": 13, "BackSpace": 14, "Tab": 15,
	"q": 16, "w": 17, "e": 18, "r": 19, "t": 20, "y": 21, "u": 22, "i": 23, "o": 24, "p": 25,
	"bracketleft": 26, "bracketright": 27, "Return": 28, "Control": 29,
	"a": 30, "s": 31, "d": 32, "f": 33, "g": 34, "h": 35, "j": 36, "k": 37, "l": 38,
	"semicolon": 39, "apostrophe": 40, "grave": 41, "Shift": keyShift, "backslash": 43,
	"z": 44, "x": 45, "c": 46, "v": 47, "b": 48, "n": 49, "m": 50,
	"comma": 51, "period": 52, "slash": 53, "Alt": 56, "space": 57,
	"Home": 102, "Up": 103, "Left": 105, "Right": 106, "End": 107, "Down": 108, "Delete": 111,
	"Super": 125,
}

// unshifted and shifted map printable US ASCII to key names.
var (
	unshifted = map[rune]string{' ': "space", '-': "minus", '=': "equal", '[': "bracketleft",
		']': "bracketright", ';': "semicolon", '\'': "apostrophe", '`': "grave", '\\': "backslash",
		',': "comma", '.': "period", '/': "slash", '\n': "Return", '\t': "Tab"}
	shifted = map[rune]string{'!': "1", '@': "2", '#': "3", '$': "4", '%': "5", '^': "6", '&': "7",
		'*': "8", '(': "9", ')': "0", '_': "minus", '+': "equal", '{': "bracketleft",
		'}': "bracketright", ':': "semicolon", '"': "apostrophe", '~': "grave", '|': "backslash",
		'<': "comma", '>': "period", '?': "slash"}
)

type stroke struct {
	code  uint32
	shift bool
}

// planText turns text into keystrokes on the US keymap, or refuses it whole.
func planText(text string) ([]stroke, error) {
	var out []stroke
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, stroke{keyCodes[string(r)], false})
		case r >= 'A' && r <= 'Z':
			out = append(out, stroke{keyCodes[string(r-'A'+'a')], true})
		case unshifted[r] != "":
			out = append(out, stroke{keyCodes[unshifted[r]], false})
		case shifted[r] != "":
			out = append(out, stroke{keyCodes[shifted[r]], true})
		default:
			return nil, fmt.Errorf("%q has no key on the US keymap", r)
		}
	}
	return out, nil
}
