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
	// Binds open and drive the switcher: each runs one mru-advance, as
	// Niri turns MruBinds into Binds.
	Binds []bind.Binding
}

type MRUHighlight struct {
	ActiveColor, UrgentColor Color
	Padding, CornerRadius    float64
}

type MRUPreviews struct {
	MaxHeight, MaxScale float64
}

// mruBinding is a recent-windows bind: next-window, or previous-window when
// backward. A next-window or previous-window without filter= lists every app
// id, and one without scope= keeps the switcher's scope.
func mruBinding(chord bind.Chord, backward bool, scope, filter string) bind.Binding {
	return bind.Binding{Chord: chord, Repeat: true, Actions: []bind.Action{{
		Name: bind.ActionMRUAdvance, Backward: backward, MRUScope: scope, MRUFilter: filter,
	}}}
}

// defaultRecentWindows is Niri 26.04's (niri-config/src/recent_windows.rs).
func defaultRecentWindows() RecentWindows {
	var binds []bind.Binding
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
				filter := "all"
				if key == "grave" {
					filter = "app-id"
				}
				binds = append(binds, mruBinding(chord, backward, "", filter))
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
			var binds []bind.Binding
			if binds, err = mruBinds(child); err == nil {
				if !c.sawMRUBinds {
					c.sawMRUBinds = true
					r.Binds = nil
				}
				r.Binds = slices.DeleteFunc(r.Binds, func(old bind.Binding) bool {
					return slices.ContainsFunc(binds, func(b bind.Binding) bool { return b.Chord == old.Chord })
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

func mruBinds(n *document.Node) ([]bind.Binding, error) {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return nil, fmt.Errorf("`binds` takes child nodes only")
	}
	var binds []bind.Binding
	for _, node := range n.Children {
		b, err := mruBind(node)
		if err != nil {
			return nil, fmt.Errorf("bind %q: %w", nodeName(node), err)
		}
		if slices.ContainsFunc(binds, func(seen bind.Binding) bool { return seen.Chord == b.Chord }) {
			return nil, fmt.Errorf("duplicate bind %q", nodeName(node))
		}
		binds = append(binds, b)
	}
	return binds, nil
}

func mruBind(node *document.Node) (bind.Binding, error) {
	chord, err := bind.ParseChord(nodeName(node))
	if err != nil {
		return bind.Binding{}, err
	}
	// The switcher stays open while any modifier is held, so Niri requires one.
	if chord.Modifiers == 0 && !chord.UsesMod {
		return bind.Binding{}, fmt.Errorf("keybind must have a modifier key")
	}
	if chord.Trigger != bind.TriggerKey {
		return bind.Binding{}, fmt.Errorf("key must be a keyboard key")
	}
	if len(node.Arguments) != 0 {
		return bind.Binding{}, fmt.Errorf("no arguments expected for this node")
	}
	ignoresInhibit := false
	for key, property := range node.Properties {
		switch key {
		case "allow-inhibiting":
			allow, ok := property.Value.(bool)
			if !ok {
				return bind.Binding{}, fmt.Errorf("`allow-inhibiting` must be boolean")
			}
			ignoresInhibit = !allow
		case "hotkey-overlay-title":
			// Niri's hotkey overlay lists the general binds only.
			if _, ok := property.Value.(string); !ok && property.Value != nil {
				return bind.Binding{}, fmt.Errorf("`hotkey-overlay-title` must be a string or null")
			}
		default:
			return bind.Binding{}, fmt.Errorf("unexpected property `%s`", key)
		}
	}
	if len(node.Children) != 1 {
		return bind.Binding{}, fmt.Errorf("expected exactly one action")
	}
	action := node.Children[0]
	backward := false
	switch nodeName(action) {
	case "next-window":
	case "previous-window":
		backward = true
	default:
		return bind.Binding{}, fmt.Errorf("unexpected action `%s`", nodeName(action))
	}
	if len(action.Arguments) != 0 || len(action.Children) != 0 {
		return bind.Binding{}, fmt.Errorf("`%s` takes properties only", nodeName(action))
	}
	scope, filter := "", "all"
	for key, property := range action.Properties {
		value, ok := property.Value.(string)
		switch {
		case key == "scope" && ok && (value == "all" || value == "output" || value == "workspace"):
			scope = value
		case key == "filter" && ok && value == "app-id":
			filter = value
		case key == "scope" || key == "filter":
			return bind.Binding{}, fmt.Errorf("invalid `%s` %v", key, property.Value)
		default:
			return bind.Binding{}, fmt.Errorf("unexpected property `%s`", key)
		}
	}
	b := mruBinding(chord, backward, scope, filter)
	b.IgnoresInhibit = ignoresInhibit
	return b, nil
}
