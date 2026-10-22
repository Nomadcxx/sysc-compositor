// Package config reads niri's KDL config (KDL v1, as niri 26.04's knuffel
// parses it). The sections built so far are decoded; every other section
// niri knows is accepted and not applied yet; a node niri does not know is an
// error, as it is in niri.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/sblinch/kdl-go"
	"github.com/sblinch/kdl-go/document"
)

// Config is the part of niri's config the compositor applies.
type Config struct {
	SpawnAtStartup   [][]string
	SpawnShAtStartup []string
	Warnings         []string
	UnappliedActions []string
	// Environment is applied to spawned commands; a nil value unsets.
	Environment map[string]*string
	Keyboard    Keyboard
	Input       Input
	Layout      Layout
	Overview    Overview
	Gestures    Gestures
	Cursor      Cursor
	Binds       []Binding
	Workspaces  []WorkspaceConfig
	WindowRules []WindowRule
	LayerRules  []LayerRule
}

type Input struct {
	ModKey, ModKeyNested string
}

type Layout struct {
	Gaps                  int
	ColumnWidthProportion float64
	FixedColumnWidth      int
	InsertHint            InsertHint
}

type Color struct{ R, G, B, A float64 }

type Overview struct {
	Zoom          float64
	BackdropColor Color
}

type Gestures struct {
	HotCorners        HotCorners
	DNDEdgeViewScroll DNDEdgeViewScroll
}

type HotCorners struct {
	TopLeft, TopRight, BottomLeft, BottomRight bool
	Off                                        bool
}

type DNDEdgeViewScroll struct {
	TriggerWidth float64
	DelayMS      int
	MaxSpeed     float64
}

type InsertHint struct {
	Off   bool
	Color Color
}

type Cursor struct {
	Theme string
	Size  int
}

type Binding = bind.Binding

// Keyboard is input.keyboard: the xkb names and the repeat clients use.
type Keyboard struct {
	Rules, Model, Layout, Variant, Options, File string
	RepeatDelay, RepeatRate                      int
}

// Default is what niri runs with when no config applies.
func Default() *Config {
	return &Config{Environment: map[string]*string{},
		Keyboard: Keyboard{RepeatDelay: 600, RepeatRate: 25},
		Layout: Layout{Gaps: 16, ColumnWidthProportion: 0.5,
			InsertHint: InsertHint{Color: Color{127.0 / 255, 200.0 / 255, 1, 128.0 / 255}}},
		Overview: Overview{Zoom: 0.5, BackdropColor: Color{0.15, 0.15, 0.15, 1}},
		Gestures: Gestures{DNDEdgeViewScroll: DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 100, MaxSpeed: 1500}},
		Cursor:   Cursor{Theme: "default", Size: 24}}
}

// topLevel is every top-level node niri 26.04 decodes (niri-config/src/lib.rs).
var topLevel = map[string]bool{
	"input": true, "output": true, "spawn-at-startup": true, "spawn-sh-at-startup": true,
	"layout": true, "prefer-no-csd": true, "cursor": true, "screenshot-path": true,
	"clipboard": true, "hotkey-overlay": true, "config-notification": true, "animations": true,
	"blur": true, "gestures": true, "overview": true, "environment": true,
	"xwayland-satellite": true, "window-rule": true, "layer-rule": true, "binds": true,
	"switch-events": true, "debug": true, "workspace": true, "recent-windows": true,
	"include": true,
}

// Parse decodes one config file's text. name is used in errors.
func Parse(name string, src []byte) (*Config, error) {
	doc, err := kdl.Parse(strings.NewReader(string(src)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	root, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve config path: %w", name, err)
	}
	stack := map[string]bool{filepath.Clean(root): true}
	nodes, err := expandIncludes(root, doc.Nodes, stack, 0)
	if err != nil {
		return nil, err
	}
	return decode(name, nodes)
}

func decode(name string, nodes []*document.Node) (*Config, error) {
	c := Default()
	for _, n := range nodes {
		node := nodeName(n)
		if !topLevel[node] {
			return nil, fmt.Errorf("%s: unexpected node `%s`", name, node)
		}
		var err error
		switch node {
		case "include":
			err = errors.New("include must be loaded from a config path")
		case "spawn-at-startup":
			var argv []string
			if argv, err = stringArgs(n, 1, -1); err == nil {
				c.SpawnAtStartup = append(c.SpawnAtStartup, argv)
			}
		case "spawn-sh-at-startup":
			var argv []string
			if argv, err = stringArgs(n, 1, 1); err == nil {
				c.SpawnShAtStartup = append(c.SpawnShAtStartup, argv[0])
			}
		case "environment":
			err = c.environment(n)
		case "input":
			err = c.input(n)
		case "layout":
			err = c.layout(n)
		case "overview":
			err = c.overview(n)
		case "gestures":
			err = c.gestures(n)
		case "cursor":
			err = c.cursor(n)
		case "binds":
			err = c.binds(n)
		case "workspace":
			err = c.workspace(n)
		case "window-rule":
			err = c.windowRule(n)
		case "layer-rule":
			err = c.layerRule(n)
		default:
			c.addWarning(node)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", name, node, err)
		}
	}
	return c, nil
}

const maxIncludeDepth = 255

func expandIncludes(name string, nodes []*document.Node, stack map[string]bool, depth int) ([]*document.Node, error) {
	var expanded []*document.Node
	for _, node := range nodes {
		if nodeName(node) != "include" {
			expanded = append(expanded, node)
			continue
		}
		paths, err := stringArgs(node, 1, 1)
		if err != nil {
			return nil, fmt.Errorf("%s: include: %w", name, err)
		}
		if len(node.Children) != 0 {
			return nil, fmt.Errorf("%s: include: unexpected child node", name)
		}
		optional := false
		for key, value := range node.Properties {
			if key != "optional" {
				return nil, fmt.Errorf("%s: include: unexpected property `%s`", name, key)
			}
			var ok bool
			optional, ok = value.Value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s: include: `optional` must be boolean", name)
			}
		}
		path, err := resolveIncludePath(filepath.Dir(name), paths[0])
		if err != nil {
			return nil, fmt.Errorf("%s: include %q: %w", name, paths[0], err)
		}
		if depth >= maxIncludeDepth {
			return nil, fmt.Errorf("%s: include %q: recursion limit (%d) reached", name, paths[0], maxIncludeDepth)
		}
		if stack[path] {
			return nil, fmt.Errorf("%s: include %q: recursive include", name, paths[0])
		}
		content, err := os.ReadFile(path)
		if err != nil {
			if optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("%s: include %q: read %s: %w", name, paths[0], path, err)
		}
		doc, err := kdl.Parse(strings.NewReader(string(content)))
		if err != nil {
			return nil, fmt.Errorf("%s: include %q: %w", name, paths[0], err)
		}
		stack[path] = true
		children, expandErr := expandIncludes(path, doc.Nodes, stack, depth+1)
		delete(stack, path)
		if expandErr != nil {
			return nil, expandErr
		}
		expanded = append(expanded, children...)
	}
	return expanded, nil
}

func resolveIncludePath(base, path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		rest := strings.TrimPrefix(path, "~")
		rest = strings.TrimPrefix(rest, string(filepath.Separator))
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (c *Config) environment(n *document.Node) error {
	for _, v := range n.Children {
		if len(v.Arguments) != 1 || len(v.Children) != 0 {
			return fmt.Errorf("`%s` takes one value or null", nodeName(v))
		}
		switch val := v.Arguments[0].Value.(type) {
		case nil:
			c.Environment[nodeName(v)] = nil
		case string:
			c.Environment[nodeName(v)] = &val
		default:
			return fmt.Errorf("`%s` must be a string or null", nodeName(v))
		}
	}
	return nil
}

// input decodes input.keyboard. Its other children (mouse, touchpad, …) are
// accepted and not applied yet.
func (c *Config) input(n *document.Node) error {
	for _, child := range n.Children {
		if nodeName(child) == "mod-key" || nodeName(child) == "mod-key-nested" {
			value, err := stringArgs(child, 1, 1)
			if err != nil {
				return fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if _, err := bind.ParseModKey(value[0]); err != nil {
				return fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if nodeName(child) == "mod-key" {
				c.Input.ModKey = value[0]
			} else {
				c.Input.ModKeyNested = value[0]
			}
			continue
		}
		if nodeName(child) != "keyboard" {
			continue
		}
		kb := child
		for _, k := range kb.Children {
			var err error
			switch nodeName(k) {
			case "xkb":
				err = c.xkb(k)
			case "repeat-delay":
				c.Keyboard.RepeatDelay, err = intArg(k, 0, 1<<16-1)
			case "repeat-rate":
				c.Keyboard.RepeatRate, err = intArg(k, 0, 255)
			}
			if err != nil {
				return fmt.Errorf("keyboard: %w", err)
			}
		}
	}
	return nil
}

func (c *Config) layout(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "gaps":
			value, err := intArg(child, 0, 1<<16-1)
			if err != nil {
				return err
			}
			c.Layout.Gaps = value
		case "default-column-width":
			for _, width := range child.Children {
				switch nodeName(width) {
				case "proportion":
					value, err := floatArg(width, 0, 100)
					if err != nil {
						return fmt.Errorf("default-column-width: %w", err)
					}
					c.Layout.ColumnWidthProportion = value
					c.Layout.FixedColumnWidth = 0
				case "fixed":
					value, err := intArg(width, 1, 1<<16-1)
					if err != nil {
						return fmt.Errorf("default-column-width: %w", err)
					}
					c.Layout.FixedColumnWidth = value
					c.Layout.ColumnWidthProportion = 0
				}
			}
		case "insert-hint":
			if err := c.insertHint(child); err != nil {
				return fmt.Errorf("insert-hint: %w", err)
			}
		}
	}
	return nil
}

func (c *Config) overview(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "zoom":
			value, err := floatArg(child, 0, 1)
			if err != nil {
				return err
			}
			c.Overview.Zoom = value
		case "backdrop-color":
			value, err := colorArg(child)
			if err != nil {
				return fmt.Errorf("backdrop-color: %w", err)
			}
			c.Overview.BackdropColor = value
		case "workspace-shadow":
			c.addWarning("overview.workspace-shadow")
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func (c *Config) gestures(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "hot-corners":
			corners, err := parseHotCorners(child)
			if err != nil {
				return fmt.Errorf("hot-corners: %w", err)
			}
			c.Gestures.HotCorners = corners
		case "dnd-edge-view-scroll":
			if err := c.dndEdgeViewScroll(child); err != nil {
				return fmt.Errorf("dnd-edge-view-scroll: %w", err)
			}
		case "dnd-edge-workspace-switch":
			c.addWarning("gestures.dnd-edge-workspace-switch")
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func (c *Config) dndEdgeViewScroll(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "trigger-width":
			value, err := numberArg(child)
			if err != nil || value < 0 || value > 65535 {
				return fmt.Errorf("`trigger-width` must be a number from 0 to 65535")
			}
			c.Gestures.DNDEdgeViewScroll.TriggerWidth = value
		case "delay-ms":
			value, err := intArg(child, 0, 1<<16-1)
			if err != nil {
				return err
			}
			c.Gestures.DNDEdgeViewScroll.DelayMS = value
		case "max-speed":
			value, err := numberArg(child)
			if err != nil || value < 0 || value > 1_000_000 {
				return fmt.Errorf("`max-speed` must be a number from 0 to 1000000")
			}
			c.Gestures.DNDEdgeViewScroll.MaxSpeed = value
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func parseHotCorners(n *document.Node) (HotCorners, error) {
	var corners HotCorners
	for _, child := range n.Children {
		if err := noValue(child); err != nil {
			return HotCorners{}, err
		}
		switch nodeName(child) {
		case "off":
			corners.Off = true
		case "top-left":
			corners.TopLeft = true
		case "top-right":
			corners.TopRight = true
		case "bottom-left":
			corners.BottomLeft = true
		case "bottom-right":
			corners.BottomRight = true
		default:
			return HotCorners{}, fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return corners, nil
}

func (c *Config) insertHint(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "off":
			if err := noValue(child); err != nil {
				return err
			}
			c.Layout.InsertHint.Off = true
		case "on":
			if err := noValue(child); err != nil {
				return err
			}
			c.Layout.InsertHint.Off = false
		case "color":
			value, err := colorArg(child)
			if err != nil {
				return fmt.Errorf("color: %w", err)
			}
			c.Layout.InsertHint.Color = value
		case "gradient":
			c.addWarning("layout.insert-hint.gradient")
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func noValue(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 || len(n.Children) != 0 {
		return fmt.Errorf("`%s` takes no arguments or properties", nodeName(n))
	}
	return nil
}

func colorArg(n *document.Node) (Color, error) {
	if len(n.Arguments) == 1 {
		value, ok := n.Arguments[0].Value.(string)
		if !ok {
			return Color{}, fmt.Errorf("color must be a hex string or four integers")
		}
		if len(n.Children) != 0 || len(n.Properties) != 0 {
			return Color{}, fmt.Errorf("color cannot have children or properties")
		}
		return parseHexColor(value)
	}
	if len(n.Arguments) != 4 || len(n.Children) != 0 || len(n.Properties) != 0 {
		return Color{}, fmt.Errorf("color needs a hex string or four integers")
	}
	var components [4]float64
	for i, arg := range n.Arguments {
		value, ok := arg.Value.(int64)
		if !ok || value < 0 || value > 255 {
			return Color{}, fmt.Errorf("color component %d must be an integer from 0 to 255", i+1)
		}
		components[i] = float64(value) / 255
	}
	return Color{components[0], components[1], components[2], components[3]}, nil
}

// ponytail: hex forms cover this config's colors; CSS names and rgb()/hsl() need a parser dependency.
func parseHexColor(value string) (Color, error) {
	if len(value) < 2 || value[0] != '#' {
		return Color{}, fmt.Errorf("color %q must use #RGB, #RGBA, #RRGGBB, or #RRGGBBAA", value)
	}
	hex := value[1:]
	var rgba [4]uint8
	switch len(hex) {
	case 3, 4:
		for i := range hex {
			nibble, err := strconv.ParseUint(hex[i:i+1], 16, 8)
			if err != nil {
				return Color{}, fmt.Errorf("invalid hex color %q", value)
			}
			rgba[i] = uint8(nibble * 17)
		}
		if len(hex) == 3 {
			rgba[3] = 255
		}
	case 6, 8:
		for i := range len(hex) / 2 {
			component, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
			if err != nil {
				return Color{}, fmt.Errorf("invalid hex color %q", value)
			}
			rgba[i] = uint8(component)
		}
		if len(hex) == 6 {
			rgba[3] = 255
		}
	default:
		return Color{}, fmt.Errorf("color %q must use #RGB, #RGBA, #RRGGBB, or #RRGGBBAA", value)
	}
	return Color{float64(rgba[0]) / 255, float64(rgba[1]) / 255, float64(rgba[2]) / 255, float64(rgba[3]) / 255}, nil
}

func (c *Config) cursor(n *document.Node) error {
	for _, child := range n.Children {
		switch nodeName(child) {
		case "xcursor-theme":
			value, err := stringArgs(child, 1, 1)
			if err != nil {
				return fmt.Errorf("xcursor-theme: %w", err)
			}
			c.Cursor.Theme = value[0]
		case "xcursor-size":
			value, err := intArg(child, 0, 255)
			if err != nil {
				return err
			}
			c.Cursor.Size = value
		}
	}
	return nil
}

func (c *Config) binds(n *document.Node) error {
	if !slices.Contains(c.Warnings, "binds") {
		c.Warnings = append(c.Warnings, "binds")
	}
	for _, node := range n.Children {
		chord, err := bind.ParseChord(nodeName(node))
		if err != nil {
			return fmt.Errorf("bind %q: %w", nodeName(node), err)
		}
		if len(node.Children) == 0 {
			return fmt.Errorf("bind %q: expected at least one action", nodeName(node))
		}
		binding := Binding{Chord: chord}
		for _, actionNode := range node.Children {
			name := nodeName(actionNode)
			if !bind.IsSupportedAction(name) {
				c.addUnappliedAction(name)
				continue
			}
			args, err := actionArgs(actionNode)
			if err != nil {
				return fmt.Errorf("bind %q action %s: %w", nodeName(node), name, err)
			}
			action, _, err := bind.ParseAction(name, args)
			if err != nil {
				return fmt.Errorf("bind %q action %s: %w", nodeName(node), name, err)
			}
			if name == bind.ActionMoveColumnToWorkspace || name == bind.ActionMoveWindowToWorkspace ||
				name == bind.ActionMoveColumnToWorkspaceDown || name == bind.ActionMoveColumnToWorkspaceUp ||
				name == bind.ActionMoveWindowToWorkspaceDown || name == bind.ActionMoveWindowToWorkspaceUp {
				for key, property := range actionNode.Properties {
					if key != "focus" {
						return fmt.Errorf("bind %q action %s: unexpected property `%s`", nodeName(node), name, key)
					}
					focus, ok := property.Value.(bool)
					if !ok {
						return fmt.Errorf("bind %q action %s: `focus` must be boolean", nodeName(node), name)
					}
					action.Focus = focus
				}
			}
			if chord.Trigger != bind.TriggerKey {
				c.addUnappliedAction(name)
				continue
			}
			binding.Actions = append(binding.Actions, action)
		}
		if len(binding.Actions) > 0 {
			c.Binds = append(c.Binds, binding)
		}
	}
	return nil
}

func (c *Config) addUnappliedAction(name string) {
	if !slices.Contains(c.UnappliedActions, name) {
		c.UnappliedActions = append(c.UnappliedActions, name)
	}
}

func actionArgs(n *document.Node) ([]string, error) {
	args := make([]string, 0, len(n.Arguments))
	for _, arg := range n.Arguments {
		switch value := arg.Value.(type) {
		case string:
			args = append(args, value)
		case int64:
			args = append(args, strconv.FormatInt(value, 10))
		default:
			return nil, fmt.Errorf("argument %v must be a string or integer", arg.Value)
		}
	}
	return args, nil
}

func (c *Config) xkb(n *document.Node) error {
	fields := map[string]*string{
		"rules": &c.Keyboard.Rules, "model": &c.Keyboard.Model, "layout": &c.Keyboard.Layout,
		"variant": &c.Keyboard.Variant, "options": &c.Keyboard.Options, "file": &c.Keyboard.File,
	}
	for _, f := range n.Children {
		dst, ok := fields[nodeName(f)]
		if !ok {
			return fmt.Errorf("xkb: unexpected node `%s`", nodeName(f))
		}
		v, err := stringArgs(f, 1, 1)
		if err != nil {
			return fmt.Errorf("xkb %s: %w", nodeName(f), err)
		}
		*dst = v[0]
	}
	return nil
}

func nodeName(n *document.Node) string { return n.Name.ValueString() }

// stringArgs reads a node's string arguments, between min and max of them
// (max < 0 is unbounded).
func stringArgs(n *document.Node, min, max int) ([]string, error) {
	if len(n.Arguments) < min || (max >= 0 && len(n.Arguments) > max) {
		return nil, fmt.Errorf("wrong number of arguments (%d)", len(n.Arguments))
	}
	out := make([]string, 0, len(n.Arguments))
	for _, a := range n.Arguments {
		s, ok := a.Value.(string)
		if !ok {
			return nil, fmt.Errorf("argument %v is not a string", a.Value)
		}
		out = append(out, s)
	}
	return out, nil
}

func intArg(n *document.Node, min, max int64) (int, error) {
	if len(n.Arguments) != 1 {
		return 0, fmt.Errorf("`%s` takes one number", nodeName(n))
	}
	v, ok := n.Arguments[0].Value.(int64)
	if !ok || v < min || v > max {
		return 0, fmt.Errorf("`%s` must be a whole number from %d to %d", nodeName(n), min, max)
	}
	return int(v), nil
}

func floatArg(n *document.Node, min, max float64) (float64, error) {
	value, err := numberArg(n)
	if err != nil {
		return 0, err
	}
	if value <= min || value > max {
		return 0, fmt.Errorf("`%s` must be greater than %v and at most %v", nodeName(n), min, max)
	}
	return value, nil
}

func numberArg(n *document.Node) (float64, error) {
	if len(n.Arguments) != 1 {
		return 0, fmt.Errorf("`%s` takes one number", nodeName(n))
	}
	var value float64
	switch v := n.Arguments[0].Value.(type) {
	case int64:
		value = float64(v)
	case float64:
		value = v
	default:
		return 0, fmt.Errorf("`%s` must be a number", nodeName(n))
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("`%s` must be a finite number", nodeName(n))
	}
	return value, nil
}

// Locate is the config path niri would read: the -c flag, then $NIRI_CONFIG
// (empty means unset), then the user's and the system config. It returns ""
// when no file exists and none was named.
func Locate(flag string, getenv func(string) string) string {
	if flag != "" {
		return flag
	}
	if p := getenv("NIRI_CONFIG"); p != "" {
		return p
	}
	var candidates []string
	if x := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		candidates = append(candidates, filepath.Join(x, "niri", "config.kdl"))
	} else if h := getenv("HOME"); h != "" {
		candidates = append(candidates, filepath.Join(h, ".config", "niri", "config.kdl"))
	}
	candidates = append(candidates, "/etc/niri/config.kdl")
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Load reads and parses path; "" is the default config.
func Load(path string) (*Config, error) {
	if path == "" {
		return Default(), nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, src)
}
