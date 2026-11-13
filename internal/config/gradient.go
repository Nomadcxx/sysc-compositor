package config

import (
	"fmt"
	"math"
	"strings"

	"github.com/sblinch/kdl-go/document"
)

// Gradient is Niri's two-stop linear gradient, including its interpolation space.
type Gradient struct {
	From, To                  Color
	Angle                     int
	RelativeTo, Interpolation string
}

func parseGradient(node *document.Node) (*Gradient, error) {
	if len(node.Arguments) != 0 || len(node.Children) != 0 {
		return nil, fmt.Errorf("gradient takes properties only")
	}
	g := &Gradient{Angle: 180, RelativeTo: "window", Interpolation: "srgb"}
	for key, property := range node.Properties {
		switch key {
		case "from", "to":
			value, ok := property.Value.(string)
			if !ok {
				return nil, fmt.Errorf("gradient %s must be a color string", key)
			}
			var color Color
			var err error
			if value != "transparent" {
				color, err = parseHexColor(value)
			}
			if err != nil {
				return nil, err
			}
			if key == "from" {
				g.From = color
			} else {
				g.To = color
			}
		case "angle":
			value, ok := numericValue(property.Value)
			if !ok || math.IsNaN(value) || value < -32768 || value > 32767 || math.Trunc(value) != value {
				return nil, fmt.Errorf("gradient angle must be an integer from -32768 to 32767")
			}
			g.Angle = int(value)
		case "relative-to":
			value, ok := property.Value.(string)
			if !ok || value != "window" && value != "workspace-view" {
				return nil, fmt.Errorf("gradient relative-to must be window or workspace-view")
			}
			g.RelativeTo = value
		case "in":
			value, ok := property.Value.(string)
			if !ok {
				return nil, fmt.Errorf("gradient in must be a string")
			}
			fields := strings.Fields(value)
			if len(fields) == 0 {
				return nil, fmt.Errorf("gradient in needs a color space")
			}
			switch fields[0] {
			case "srgb", "srgb-linear", "oklab", "oklch":
			default:
				return nil, fmt.Errorf("unknown gradient color space %q", fields[0])
			}
			if len(fields) > 1 {
				if fields[0] != "oklch" || len(fields) != 3 || fields[2] != "hue" {
					return nil, fmt.Errorf("only oklch accepts a hue interpolation method")
				}
				switch fields[1] {
				case "shorter", "longer", "increasing", "decreasing":
				default:
					return nil, fmt.Errorf("unknown hue interpolation %q", fields[1])
				}
			}
			g.Interpolation = strings.Join(fields, " ")
		default:
			return nil, fmt.Errorf("unexpected gradient property %q", key)
		}
	}
	if _, ok := node.Properties["from"]; !ok {
		return nil, fmt.Errorf("gradient needs from")
	}
	if _, ok := node.Properties["to"]; !ok {
		return nil, fmt.Errorf("gradient needs to")
	}
	return g, nil
}
