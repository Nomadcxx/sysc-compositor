package config

import (
	"fmt"
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/sblinch/kdl-go/document"
)

// RecentWindows is Niri's recent-windows section: the window switcher and
// the focus timestamps it orders windows by.
type RecentWindows struct {
	Off bool
	// DebounceMS is how long a window keeps focus before its focus timestamp
	// updates; OpenDelayMS is how long the switcher waits before it shows.
	DebounceMS, OpenDelayMS int
	Highlight               MRUHighlight
	Previews                MRUPreviews
	Binds                   []MRUBind
}

type MRUHighlight struct {
	ActiveColor, UrgentColor Color
	Padding, CornerRadius    float64
}

type MRUPreviews struct {
	MaxHeight, MaxScale float64
}

// MRUScope is the windows the switcher lists.
type MRUScope string

const (
	MRUScopeAll       MRUScope = "all"
	MRUScopeOutput    MRUScope = "output"
	MRUScopeWorkspace MRUScope = "workspace"
)

// MRUBind is one recent-windows bind: next-window, or previous-window when
// Backward. An empty Scope keeps the switcher's last scope; FilterAppID lists
// only the selected window's app id.
type MRUBind struct {
	Chord          bind.Chord
	Backward       bool
	Scope          MRUScope
	FilterAppID    bool
	IgnoresInhibit bool
}

// defaultRecentWindows is Niri 26.04's (niri-config/src/recent_windows.rs).
func defaultRecentWindows() RecentWindows {
	var binds []MRUBind
	for _, base := range []bind.Chord{
		{Modifiers: bind.Modifiers(bind.ModifierAlt)},
		{UsesMod: true},
	} {
		for _, key := range []string{"Tab", "grave"} {
			for _, backward := range []bool{false, true} {
				chord := base
				chord.Key = key
				if backward {
					chord.Modifiers |= bind.Modifiers(bind.ModifierShift)
				}
				binds = append(binds, MRUBind{Chord: chord, Backward: backward, FilterAppID: key == "grave"})
			}
		}
	}
	return RecentWindows{
		DebounceMS: 750, OpenDelayMS: 150,
		Highlight: MRUHighlight{
			ActiveColor: Color{0.6, 0.6, 0.6, 1}, UrgentColor: Color{1, 0.6, 0.6, 1}, Padding: 30,
		},
		Previews: MRUPreviews{MaxHeight: 480, MaxScale: 0.5},
		Binds:    binds,
	}
}

// recentWindows merges one recent-windows section, as Niri merges its parts:
// the first binds block clears the default binds, and a later one replaces
// the binds whose keys it names.
func (c *Config) recentWindows(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("`%s` takes child nodes only", nodeName(n))
	}
	r := &c.RecentWindows
	var on, off bool
	for _, child := range n.Children {
		name := nodeName(child)
		var err error
		switch name {
		case "on", "off":
			if err = noValue(child); err == nil {
				on, off = on || name == "on", off || name == "off"
			}
		case "debounce-ms":
			r.DebounceMS, err = intArg(child, 0, 65535)
		case "open-delay-ms":
			r.OpenDelayMS, err = intArg(child, 0, 65535)
		case "highlight":
			err = r.highlight(child)
		case "previews":
			err = r.previews(child)
		case "binds":
			var binds []MRUBind
			if binds, err = mruBinds(child); err == nil {
				if !c.sawMRUBinds {
					c.sawMRUBinds = true
					r.Binds = nil
				}
				r.Binds = slices.DeleteFunc(r.Binds, func(old MRUBind) bool {
					return slices.ContainsFunc(binds, func(b MRUBind) bool { return b.Chord == old.Chord })
				})
				r.Binds = append(r.Binds, binds...)
			}
		default:
			return fmt.Errorf("unexpected node `%s`", name)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if on {
		r.Off = false
	}
	if off {
		r.Off = true
	}
	return nil
}

func (r *RecentWindows) highlight(n *document.Node) error {
	for _, child := range n.Children {
		var err error
		switch nodeName(child) {
		case "active-color":
			r.Highlight.ActiveColor, err = colorArg(child)
		case "urgent-color":
			r.Highlight.UrgentColor, err = colorArg(child)
		case "padding":
			r.Highlight.Padding, err = boundedNumber(child, 0, 65535)
		case "corner-radius":
			r.Highlight.CornerRadius, err = boundedNumber(child, 0, 65535)
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
		if err != nil {
			return fmt.Errorf("%s: %w", nodeName(child), err)
		}
	}
	return nil
}

func (r *RecentWindows) previews(n *document.Node) error {
	for _, child := range n.Children {
		var err error
		switch nodeName(child) {
		case "max-height":
			r.Previews.MaxHeight, err = boundedNumber(child, 1, 65535)
		case "max-scale":
			r.Previews.MaxScale, err = boundedNumber(child, 0, 1)
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
		if err != nil {
			return fmt.Errorf("%s: %w", nodeName(child), err)
		}
	}
	return nil
}

func mruBinds(n *document.Node) ([]MRUBind, error) {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return nil, fmt.Errorf("`binds` takes child nodes only")
	}
	var binds []MRUBind
	for _, node := range n.Children {
		b, err := mruBind(node)
		if err != nil {
			return nil, fmt.Errorf("bind %q: %w", nodeName(node), err)
		}
		if slices.ContainsFunc(binds, func(seen MRUBind) bool { return seen.Chord == b.Chord }) {
			return nil, fmt.Errorf("duplicate bind %q", nodeName(node))
		}
		binds = append(binds, b)
	}
	return binds, nil
}

func mruBind(node *document.Node) (MRUBind, error) {
	chord, err := bind.ParseChord(nodeName(node))
	if err != nil {
		return MRUBind{}, err
	}
	// The switcher stays open while any modifier is held, so Niri requires one.
	if chord.Modifiers == 0 && !chord.UsesMod {
		return MRUBind{}, fmt.Errorf("keybind must have a modifier key")
	}
	if chord.Trigger != bind.TriggerKey {
		return MRUBind{}, fmt.Errorf("key must be a keyboard key")
	}
	if len(node.Arguments) != 0 {
		return MRUBind{}, fmt.Errorf("no arguments expected for this node")
	}
	b := MRUBind{Chord: chord}
	for key, property := range node.Properties {
		switch key {
		case "allow-inhibiting":
			allow, ok := property.Value.(bool)
			if !ok {
				return MRUBind{}, fmt.Errorf("`allow-inhibiting` must be boolean")
			}
			b.IgnoresInhibit = !allow
		case "hotkey-overlay-title":
			// Niri's hotkey overlay lists the general binds only.
			if _, ok := property.Value.(string); !ok && property.Value != nil {
				return MRUBind{}, fmt.Errorf("`hotkey-overlay-title` must be a string or null")
			}
		default:
			return MRUBind{}, fmt.Errorf("unexpected property `%s`", key)
		}
	}
	if len(node.Children) != 1 {
		return MRUBind{}, fmt.Errorf("expected exactly one action")
	}
	action := node.Children[0]
	switch nodeName(action) {
	case "next-window":
	case "previous-window":
		b.Backward = true
	default:
		return MRUBind{}, fmt.Errorf("unexpected action `%s`", nodeName(action))
	}
	if len(action.Arguments) != 0 || len(action.Children) != 0 {
		return MRUBind{}, fmt.Errorf("`%s` takes properties only", nodeName(action))
	}
	for key, property := range action.Properties {
		value, ok := property.Value.(string)
		switch {
		case key == "scope" && ok && (value == "all" || value == "output" || value == "workspace"):
			b.Scope = MRUScope(value)
		case key == "filter" && ok && value == "app-id":
			b.FilterAppID = true
		case key == "scope" || key == "filter":
			return MRUBind{}, fmt.Errorf("invalid `%s` %v", key, property.Value)
		default:
			return MRUBind{}, fmt.Errorf("unexpected property `%s`", key)
		}
	}
	return b, nil
}
