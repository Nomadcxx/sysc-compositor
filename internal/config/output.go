package config

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sblinch/kdl-go/document"
)

// OutputConfig is one `output` section, Niri's niri_config::Output.
type OutputConfig struct {
	// Name is the connector or "make model serial" the section matches.
	Name string
	Off  bool
	// Scale nil lets the compositor guess one from the output's density.
	Scale     *float64
	Transform Transform
	// Position nil places the output automatically.
	Position *OutputPosition
	// Mode nil picks the output's preferred mode.
	Mode     *OutputMode
	Modeline *Modeline
	// VRR nil keeps variable refresh rate off.
	VRR            *VRR
	FocusAtStartup bool
	BackdropColor  *Color
	HotCorners     *HotCorners
	// Layout merges over the config's layout for this output's workspaces.
	// The deprecated output-level background-color folds into it.
	Layout *WorkspaceLayout
}

type OutputPosition struct{ X, Y int }

// OutputMode is Niri's ConfiguredMode: Refresh 0 means none was given.
// Custom asks for a CVT mode the output does not list.
type OutputMode struct {
	Width, Height int
	Refresh       float64
	Custom        bool
}

// Modeline is a VESA CVT modeline: the clock in MHz, the horizontal and
// vertical timings in pixels, and the sync polarities.
type Modeline struct {
	Clock                                  float64
	HDisplay, HSyncStart, HSyncEnd, HTotal int
	VDisplay, VSyncStart, VSyncEnd, VTotal int
	HSyncPositive, VSyncPositive           bool
}

// VRR is a configured variable refresh rate: on, or on only while a window
// that asks for it is shown.
type VRR struct{ OnDemand bool }

// Transform is an output transform, numbered as wl_output_transform.
type Transform uint32

const (
	TransformNormal Transform = iota
	Transform90
	Transform180
	Transform270
	TransformFlipped
	TransformFlipped90
	TransformFlipped180
	TransformFlipped270
)

var transformNames = []string{"normal", "90", "180", "270", "flipped", "flipped-90", "flipped-180", "flipped-270"}

func (t Transform) String() string {
	if int(t) < len(transformNames) {
		return transformNames[t]
	}
	return fmt.Sprintf("Transform(%d)", uint32(t))
}

// ParseTransform reads one of Niri's transform names.
func ParseTransform(name string) (Transform, error) {
	for i, known := range transformNames {
		if name == known {
			return Transform(i), nil
		}
	}
	return 0, fmt.Errorf(`invalid transform %q, can be "normal", "90", "180", "270", "flipped", "flipped-90", "flipped-180" or "flipped-270"`, name)
}

// ParseOutputMode reads Niri's "WIDTHxHEIGHT" or "WIDTHxHEIGHT@REFRESH".
func ParseOutputMode(value string) (OutputMode, error) {
	width, rest, ok := strings.Cut(value, "x")
	if !ok {
		return OutputMode{}, fmt.Errorf("mode %q: no 'x' separator found", value)
	}
	height, refresh, hasRefresh := strings.Cut(rest, "@")
	var mode OutputMode
	var err error
	if mode.Width, err = parseModeSize(width); err != nil {
		return OutputMode{}, fmt.Errorf("mode %q: error parsing width", value)
	}
	if mode.Height, err = parseModeSize(height); err != nil {
		return OutputMode{}, fmt.Errorf("mode %q: error parsing height", value)
	}
	if hasRefresh {
		if mode.Refresh, err = strconv.ParseFloat(refresh, 64); err != nil {
			return OutputMode{}, fmt.Errorf("mode %q: error parsing refresh rate", value)
		}
		if math.IsNaN(mode.Refresh) || math.IsInf(mode.Refresh, 0) || mode.Refresh <= 0 || mode.Refresh > float64(math.MaxInt32)/1000 {
			return OutputMode{}, fmt.Errorf("mode %q: invalid refresh rate", value)
		}
	}
	if mode.Width == 0 || mode.Height == 0 {
		return OutputMode{}, fmt.Errorf("mode %q: dimensions must be above 0", value)
	}
	return mode, nil
}

func parseModeSize(value string) (int, error) {
	size, err := strconv.ParseUint(value, 10, 16)
	return int(size), err
}

// OutputName is an output's identity, matched against a section name as
// Niri's OutputName does. A missing make, model or serial is empty.
type OutputName struct{ Connector, Make, Model, Serial string }

// Matches reports whether target names this output: its connector, or "make
// model serial" with Unknown for a missing part, ignoring ASCII case. An
// output with no make, model and serial matches by connector only.
func (n OutputName) Matches(target string) bool {
	if asciiEqualFold(target, n.Connector) {
		return true
	}
	if n.Make == "" && n.Model == "" && n.Serial == "" {
		return false
	}
	unknown := func(s string) string {
		if s == "" {
			return "Unknown"
		}
		return s
	}
	return asciiEqualFold(target, unknown(n.Make)+" "+unknown(n.Model)+" "+unknown(n.Serial))
}

// Compare orders outputs as Niri places them: those with a make, model or
// serial first, by those and then by connector; the others by connector.
func (n OutputName) Compare(other OutputName) int {
	missing := n.Make == "" && n.Model == "" && n.Serial == ""
	otherMissing := other.Make == "" && other.Model == "" && other.Serial == ""
	switch {
	case missing && otherMissing:
		return cmp.Compare(n.Connector, other.Connector)
	case missing:
		return 1
	case otherMissing:
		return -1
	}
	return cmp.Or(cmp.Compare(n.Make, other.Make), cmp.Compare(n.Model, other.Model),
		cmp.Compare(n.Serial, other.Serial), cmp.Compare(n.Connector, other.Connector))
}

// FindOutput returns the first section that matches the output, as Niri's
// Outputs::find does, or nil.
func (c *Config) FindOutput(name OutputName) *OutputConfig {
	for i := range c.Outputs {
		if name.Matches(c.Outputs[i].Name) {
			return &c.Outputs[i]
		}
	}
	return nil
}

func (c *Config) output(n *document.Node) error {
	names, err := stringArgs(n, 1, 1)
	if err != nil {
		return fmt.Errorf("output needs one name: %w", err)
	}
	output := OutputConfig{Name: names[0]}
	var backgroundColor *Color
	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "off", "focus-at-startup":
			on, err := flagArg(child)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if name == "off" {
				output.Off = on
			} else {
				output.FocusAtStartup = on
			}
		case "scale":
			value, err := numberArg(child)
			if err != nil {
				return err
			}
			if value < 0 || value > 10 {
				return fmt.Errorf("`scale` must be from 0 to 10")
			}
			output.Scale = &value
		case "transform":
			value, err := stringArgs(child, 1, 1)
			if err != nil {
				return fmt.Errorf("transform: %w", err)
			}
			if output.Transform, err = ParseTransform(value[0]); err != nil {
				return err
			}
		case "position":
			position, err := outputPosition(child)
			if err != nil {
				return fmt.Errorf("position: %w", err)
			}
			output.Position = &position
		case "mode":
			mode, err := outputModeNode(child)
			if err != nil {
				return err
			}
			output.Mode = &mode
		case "modeline":
			modeline, err := modelineNode(child)
			if err != nil {
				return fmt.Errorf("modeline: %w", err)
			}
			output.Modeline = &modeline
		case "variable-refresh-rate":
			vrr, err := vrrNode(child)
			if err != nil {
				return fmt.Errorf("variable-refresh-rate: %w", err)
			}
			output.VRR = &vrr
		case "background-color", "backdrop-color":
			color, err := colorArg(child)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if name == "background-color" {
				backgroundColor = &color
			} else {
				output.BackdropColor = &color
			}
		case "hot-corners":
			corners, err := parseHotCorners(child)
			if err != nil {
				return fmt.Errorf("hot-corners: %w", err)
			}
			output.HotCorners = &corners
		case "layout":
			layout, err := c.workspaceLayout(child, "output.layout.")
			if err != nil {
				return err
			}
			output.Layout = layout
		default:
			return fmt.Errorf("unexpected output child `%s`", name)
		}
	}
	// Niri's reload_output_config lets the deprecated key fill in the
	// layout's background-color when the layout leaves it unset.
	if backgroundColor != nil {
		if output.Layout == nil {
			output.Layout = &WorkspaceLayout{}
		}
		if output.Layout.BackgroundColor == nil {
			output.Layout.BackgroundColor = backgroundColor
		}
	}
	c.Outputs = append(c.Outputs, output)
	return nil
}

func outputPosition(n *document.Node) (OutputPosition, error) {
	if len(n.Arguments) != 0 || len(n.Children) != 0 || len(n.Properties) != 2 {
		return OutputPosition{}, fmt.Errorf("takes the properties x= and y=")
	}
	var position OutputPosition
	for key, target := range map[string]*int{"x": &position.X, "y": &position.Y} {
		property, ok := n.Properties[key]
		if !ok {
			return OutputPosition{}, fmt.Errorf("missing property `%s`", key)
		}
		value, ok := property.Value.(int64)
		if !ok || value < -(1<<31) || value > 1<<31-1 {
			return OutputPosition{}, fmt.Errorf("`%s` must be a whole number", key)
		}
		*target = int(value)
	}
	return position, nil
}

func outputModeNode(n *document.Node) (OutputMode, error) {
	if len(n.Children) != 0 {
		return OutputMode{}, fmt.Errorf("mode takes no children")
	}
	custom := false
	for key, property := range n.Properties {
		value, ok := property.Value.(bool)
		if key != "custom" || !ok {
			return OutputMode{}, fmt.Errorf("mode: unexpected property `%s`", key)
		}
		custom = value
	}
	value, err := stringArgs(n, 1, 1)
	if err != nil {
		return OutputMode{}, fmt.Errorf("mode: %w", err)
	}
	mode, err := ParseOutputMode(value[0])
	if err != nil {
		return OutputMode{}, err
	}
	if custom && mode.Refresh <= 0 {
		return OutputMode{}, fmt.Errorf("mode %q: a custom mode needs a refresh rate above 0", value[0])
	}
	mode.Custom = custom
	return mode, nil
}

func modelineNode(n *document.Node) (Modeline, error) {
	if len(n.Children) != 0 || len(n.Properties) != 0 || len(n.Arguments) != 11 {
		return Modeline{}, fmt.Errorf("takes the clock, eight timings and two sync polarities")
	}
	var m Modeline
	switch clock := n.Arguments[0].Value.(type) {
	case float64:
		m.Clock = clock
	case int64:
		m.Clock = float64(clock)
	default:
		return Modeline{}, fmt.Errorf("the clock must be a number")
	}
	if math.IsNaN(m.Clock) || math.IsInf(m.Clock, 0) || m.Clock <= 0 || m.Clock > float64(math.MaxUint32)/1000 {
		return Modeline{}, fmt.Errorf("the clock must be positive and fit a DRM pixel clock")
	}
	timings := []*int{&m.HDisplay, &m.HSyncStart, &m.HSyncEnd, &m.HTotal,
		&m.VDisplay, &m.VSyncStart, &m.VSyncEnd, &m.VTotal}
	for i, target := range timings {
		value, ok := n.Arguments[i+1].Value.(int64)
		if !ok || value < 0 || value > 1<<16-1 {
			return Modeline{}, fmt.Errorf("timing %d must be a whole number from 0 to 65535", i+1)
		}
		*target = int(value)
	}
	if !(m.HDisplay < m.HSyncStart && m.HSyncStart < m.HSyncEnd && m.HSyncEnd < m.HTotal) {
		return Modeline{}, fmt.Errorf("the horizontal timings must increase: hdisplay < hsync_start < hsync_end < htotal")
	}
	if !(m.VDisplay < m.VSyncStart && m.VSyncStart < m.VSyncEnd && m.VSyncEnd < m.VTotal) {
		return Modeline{}, fmt.Errorf("the vertical timings must increase: vdisplay < vsync_start < vsync_end < vtotal")
	}
	polarity := func(arg any, positive, negative string) (bool, error) {
		switch arg {
		case positive:
			return true, nil
		case negative:
			return false, nil
		}
		return false, fmt.Errorf("invalid sync polarity %v, can be %q or %q", arg, positive, negative)
	}
	var err error
	if m.HSyncPositive, err = polarity(n.Arguments[9].Value, "+hsync", "-hsync"); err != nil {
		return Modeline{}, err
	}
	if m.VSyncPositive, err = polarity(n.Arguments[10].Value, "+vsync", "-vsync"); err != nil {
		return Modeline{}, err
	}
	return m, nil
}

func vrrNode(n *document.Node) (VRR, error) {
	if len(n.Arguments) != 0 || len(n.Children) != 0 {
		return VRR{}, fmt.Errorf("takes only the on-demand= property")
	}
	var vrr VRR
	for key, property := range n.Properties {
		value, ok := property.Value.(bool)
		if key != "on-demand" || !ok {
			return VRR{}, fmt.Errorf("unexpected property `%s`", key)
		}
		vrr.OnDemand = value
	}
	return vrr, nil
}
