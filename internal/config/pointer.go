package config

import (
	"fmt"
	"math"
	"slices"

	"github.com/sblinch/kdl-go/document"
)

// ScrollFactor scales pointer axes; both axes default to one.
type ScrollFactor struct{ Vertical, Horizontal float64 }

// PointerDevice is one complete Niri-compatible pointing-device section.
// Set distinguishes an absent section (libinput defaults) from explicit flags.
type PointerDevice struct {
	Set                                           bool
	Off, NaturalScroll                            bool
	AccelSpeed                                    float64
	AccelProfile, ScrollMethod                    string
	ScrollButton                                  *uint32
	ScrollButtonLock, LeftHanded, MiddleEmulation bool
	Tap, DWT, DWTP, DragLock                      bool
	Drag                                          *bool
	ClickMethod, TapButtonMap                     string
	DisabledOnExternalMouse                       bool
	ScrollFactor                                  ScrollFactor
}

// Pointers groups the four independently replaced device sections.
type Pointers struct{ Touchpad, Mouse, Trackpoint, Trackball PointerDevice }

func defaultPointers() Pointers {
	d := PointerDevice{ScrollFactor: ScrollFactor{1, 1}}
	return Pointers{Touchpad: d, Mouse: d, Trackpoint: d, Trackball: d}
}

func pointerDevice(n *document.Node, touchpad bool) (PointerDevice, error) {
	p := PointerDevice{Set: true, ScrollFactor: ScrollFactor{1, 1}}
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return p, fmt.Errorf("takes child nodes only")
	}
	flags := map[string]*bool{
		"off": &p.Off, "natural-scroll": &p.NaturalScroll, "scroll-button-lock": &p.ScrollButtonLock, "left-handed": &p.LeftHanded, "middle-emulation": &p.MiddleEmulation,
	}
	type enum struct {
		dst    *string
		values []string
	}
	enums := map[string]enum{
		"accel-profile": {&p.AccelProfile, []string{"adaptive", "flat"}},
		"scroll-method": {&p.ScrollMethod, []string{"no-scroll", "two-finger", "edge", "on-button-down"}},
	}
	if touchpad {
		flags["tap"], flags["dwt"], flags["dwtp"], flags["drag-lock"], flags["disabled-on-external-mouse"] = &p.Tap, &p.DWT, &p.DWTP, &p.DragLock, &p.DisabledOnExternalMouse
		enums["click-method"] = enum{&p.ClickMethod, []string{"button-areas", "clickfinger"}}
		enums["tap-button-map"] = enum{&p.TapButtonMap, []string{"left-right-middle", "left-middle-right"}}
	}
	seen := make(map[string]bool)
	for _, child := range n.Children {
		name := nodeName(child)
		if seen[name] {
			return p, fmt.Errorf("duplicate node `%s`", name)
		}
		seen[name] = true
		if name == "scroll-factor" && (touchpad || nodeName(n) == "mouse") {
			factor, err := pointerScrollFactor(child)
			if err != nil {
				return p, fmt.Errorf("%s: %w", name, err)
			}
			p.ScrollFactor = factor
			continue
		}
		if len(child.Properties) != 0 || len(child.Children) != 0 {
			return p, fmt.Errorf("`%s` takes no properties or children", name)
		}
		if flag, ok := flags[name]; ok {
			if err := noValue(child); err != nil {
				return p, err
			}
			*flag = true
			continue
		}
		if choice, ok := enums[name]; ok {
			args, err := stringArgs(child, 1, 1)
			if err != nil {
				return p, fmt.Errorf("%s: %w", name, err)
			}
			if !slices.Contains(choice.values, args[0]) {
				return p, fmt.Errorf("%s: unexpected value %q (expected %v)", name, args[0], choice.values)
			}
			*choice.dst = args[0]
			continue
		}
		switch name {
		case "accel-speed":
			value, err := numberArg(child)
			if err != nil {
				return p, err
			}
			if value < -1 || value > 1 {
				return p, fmt.Errorf("accel-speed: must be from -1 to 1")
			}
			p.AccelSpeed = value
		case "scroll-button":
			value, err := intArg(child, 0, math.MaxUint32)
			if err != nil {
				return p, err
			}
			button := uint32(value)
			p.ScrollButton = &button
		case "drag":
			if !touchpad {
				return p, fmt.Errorf("unexpected node `%s`", name)
			}
			value, err := boolArg(child)
			if err != nil {
				return p, fmt.Errorf("drag: %w", err)
			}
			p.Drag = &value
		default:
			return p, fmt.Errorf("unexpected node `%s`", name)
		}
	}
	return p, nil
}

func pointerScrollFactor(n *document.Node) (ScrollFactor, error) {
	factor := ScrollFactor{1, 1}
	if len(n.Arguments) > 1 || len(n.Children) != 0 {
		return factor, fmt.Errorf("takes at most one number and horizontal/vertical properties")
	}
	if len(n.Arguments) == 1 {
		base, err := pointerFactorValue(n.Arguments[0], 0, 100)
		if err != nil {
			return factor, err
		}
		factor = ScrollFactor{base, base}
	}
	for name, value := range n.Properties {
		if name != "vertical" && name != "horizontal" {
			return factor, fmt.Errorf("unexpected property `%s`", name)
		}
		v, err := pointerFactorValue(value, -100, 100)
		if err != nil {
			return factor, fmt.Errorf("%s: %w", name, err)
		}
		if name == "vertical" {
			factor.Vertical = v
		} else {
			factor.Horizontal = v
		}
	}
	return factor, nil
}
func pointerFactorValue(value *document.Value, min, max float64) (float64, error) {
	var n float64
	switch v := value.Value.(type) {
	case int64:
		n = float64(v)
	case float64:
		n = v
	default:
		return 0, fmt.Errorf("must be a number")
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < min || n > max {
		return 0, fmt.Errorf("must be a finite number from %v to %v", min, max)
	}
	return n, nil
}
