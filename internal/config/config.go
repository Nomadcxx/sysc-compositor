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
	Environment  map[string]*string
	Keyboard     Keyboard
	Input        Input
	Layout       Layout
	Overview     Overview
	Gestures     Gestures
	SwitchEvents SwitchEvents
	Cursor       Cursor
	Binds        []Binding
	// OverlayBinds is every bind in order, applied here or not, as the
	// hotkey overlay lists them; HotkeyOverlay is its section.
	OverlayBinds      []OverlayBind
	HotkeyOverlay     HotkeyOverlay
	RecentWindows     RecentWindows
	Workspaces        []WorkspaceConfig
	WindowRules       []WindowRule
	LayerRules        []LayerRule
	Animations        Animations
	Blur              Blur
	PreferNoCSD       bool
	XwaylandSatellite XwaylandSatellite
	// ScreenshotPath is the strftime pattern screenshots are written to; nil
	// (`screenshot-path null`) keeps them in the clipboard only.
	ScreenshotPath *string

	// sawMRUBinds is set once a recent-windows binds block has replaced the
	// default switcher binds.
	sawMRUBinds bool
}

// DefaultScreenshotPath is Niri 26.04's (niri-config/src/misc.rs).
const DefaultScreenshotPath = "~/Pictures/Screenshots/Screenshot from %Y-%m-%d %H-%M-%S.png"

// Blur is the top-level blur section: how ext-background-effect frost is
// drawn. A later section, or one in an include, changes only the fields it
// names, as Niri merges blur parts.
type Blur struct {
	Off        bool
	Passes     int
	Offset     float64
	Noise      float64
	Saturation float64
}

// defaultBlur is Niri 26.04's (niri-config/src/appearance.rs).
func defaultBlur() Blur {
	return Blur{Passes: 3, Offset: 3, Noise: 0.02, Saturation: 1.5}
}

// LoadResult returns the decoded config and paths that can trigger a reload.
// Sources includes missing optional paths so creating one can repair a config.
type LoadResult struct {
	Root    string
	Config  *Config
	Sources []string
}

type Input struct {
	ModKey, ModKeyNested string
	Tablet               Tablet
}

// Tablet is input.tablet, Niri's niri-config/src/input.rs Tablet.
type Tablet struct {
	Off                bool
	MapToOutput        string
	MapToFocusedOutput bool
}

type SwitchEvents struct {
	LidOpen, LidClose, TabletModeOn, TabletModeOff []string
}

type Layout struct {
	Gaps                  int
	ColumnWidthProportion float64
	FixedColumnWidth      int
	InsertHint            InsertHint
	FocusRing             FocusRing
	Shadow                Shadow
}

type Color struct{ R, G, B, A float64 }

type FocusRing struct {
	Off                        bool
	Width                      float64
	ActiveColor, InactiveColor Color
}

type Shadow struct {
	On               bool
	Softness, Spread float64
	OffsetX, OffsetY float64
	Color            Color
}

type CornerRadius [4]float64

type SpringConfig struct {
	DampingRatio float64
	Stiffness    uint32
	Epsilon      float64
}

type Animation struct {
	Off         bool
	Spring      *SpringConfig
	Unsupported bool
}

type Animations struct {
	Off                    bool
	WindowOpen             Animation
	WindowClose            Animation
	WindowResize           Animation
	WindowMovement         Animation
	OverviewOpenClose      Animation
	WorkspaceSwitch        Animation
	HorizontalViewMovement Animation
	// ExitConfirmationOpenClose drives the exit dialog's fade and zoom.
	ExitConfirmationOpenClose Animation
	// RecentWindowsClose fades the window switcher out.
	RecentWindowsClose Animation
}

// HotkeyOverlay is Niri's hotkey-overlay section.
type HotkeyOverlay struct {
	SkipAtStartup bool
	HideNotBound  bool
}

// OverlayBind is a bind as the hotkey overlay sees it: its key, its first
// action with the arguments and properties that tell actions apart, and its
// hotkey-overlay-title (Title set, or TitleHidden for null).
type OverlayBind struct {
	Chord            bind.Chord
	Action           string
	Args             []string
	SkipConfirmation bool
	Title            *string
	TitleHidden      bool
}

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

type XwaylandSatellite struct {
	Off  bool
	Path string
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
			InsertHint: InsertHint{Color: Color{127.0 / 255, 200.0 / 255, 1, 128.0 / 255}},
			FocusRing:  FocusRing{Width: 4, ActiveColor: Color{127.0 / 255, 200.0 / 255, 1, 1}, InactiveColor: Color{80.0 / 255, 80.0 / 255, 80.0 / 255, 1}},
			Shadow:     Shadow{Softness: 30, Spread: 5, OffsetY: 5, Color: Color{0, 0, 0, 119.0 / 255}}},
		Overview:          Overview{Zoom: 0.5, BackdropColor: Color{0.15, 0.15, 0.15, 1}},
		Gestures:          Gestures{DNDEdgeViewScroll: DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 100, MaxSpeed: 1500}},
		Cursor:            Cursor{Theme: "default", Size: 24},
		Animations:        defaultAnimations(),
		Blur:              defaultBlur(),
		RecentWindows:     defaultRecentWindows(),
		XwaylandSatellite: XwaylandSatellite{Path: "xwayland-satellite"},
		ScreenshotPath:    func() *string { p := DefaultScreenshotPath; return &p }()}
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
	result, err := ParseWithSources(name, src)
	return result.Config, err
}

// ParseWithSources parses text and records its root and included paths.
func ParseWithSources(name string, src []byte) (LoadResult, error) {
	result := LoadResult{}
	root, err := filepath.Abs(name)
	if err != nil {
		return result, fmt.Errorf("%s: resolve config path: %w", name, err)
	}
	root = filepath.Clean(root)
	result.Root = root
	result.Sources = append(result.Sources, root)
	doc, err := kdl.Parse(strings.NewReader(string(src)))
	if err != nil {
		return result, fmt.Errorf("%s: %w", name, err)
	}
	stack := map[string]bool{root: true}
	seenSources := map[string]struct{}{root: {}}
	nodes, err := expandIncludes(root, doc.Nodes, stack, 0, &result.Sources, seenSources)
	if err != nil {
		return result, err
	}
	result.Config, err = decode(name, nodes)
	return result, err
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
		case "prefer-no-csd":
			if len(n.Arguments) == 0 {
				err = noValue(n)
				if err == nil {
					c.PreferNoCSD = true
				}
			} else {
				c.PreferNoCSD, err = boolArg(n)
			}
		case "animations":
			err = c.animations(n)
		case "blur":
			err = c.blur(n)
		case "xwayland-satellite":
			err = c.xwaylandSatellite(n)
		case "screenshot-path":
			err = c.screenshotPath(n)
		case "overview":
			err = c.overview(n)
		case "gestures":
			err = c.gestures(n)
		case "cursor":
			err = c.cursor(n)
		case "binds":
			err = c.binds(n)
		case "hotkey-overlay":
			err = c.hotkeyOverlay(n)
		case "recent-windows":
			err = c.recentWindows(n)
		case "switch-events":
			err = c.switchEvents(n)
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

func expandIncludes(name string, nodes []*document.Node, stack map[string]bool, depth int, sources *[]string, seenSources map[string]struct{}) ([]*document.Node, error) {
	var expanded []*document.Node
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		kind := nodeName(node)
		if !multipleTopLevelNodes(kind) {
			if _, exists := seen[kind]; exists {
				return nil, fmt.Errorf("%s: duplicate node `%s`", name, kind)
			}
			seen[kind] = struct{}{}
		}
		if kind != "include" {
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
		if _, exists := seenSources[path]; !exists {
			*sources = append(*sources, path)
			seenSources[path] = struct{}{}
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
		children, expandErr := expandIncludes(path, doc.Nodes, stack, depth+1, sources, seenSources)
		delete(stack, path)
		if expandErr != nil {
			return nil, expandErr
		}
		expanded = append(expanded, children...)
	}
	return expanded, nil
}

func multipleTopLevelNodes(name string) bool {
	switch name {
	case "output", "spawn-at-startup", "spawn-sh-at-startup", "window-rule", "layer-rule", "workspace", "include":
		return true
	default:
		return false
	}
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
		if nodeName(child) == "tablet" {
			if err := c.tablet(child); err != nil {
				return fmt.Errorf("tablet: %w", err)
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

// tablet decodes input.tablet. left-handed and calibration-matrix are
// libinput device settings, which no device class applies yet; Niri ignores
// a matrix that is not six numbers.
func (c *Config) tablet(n *document.Node) error {
	for _, child := range n.Children {
		var err error
		switch name := nodeName(child); name {
		case "off", "map-to-focused-output", "left-handed":
			if err = noValue(child); err != nil {
				break
			}
			switch name {
			case "off":
				c.Input.Tablet.Off = true
			case "map-to-focused-output":
				c.Input.Tablet.MapToFocusedOutput = true
			default:
				c.addWarning("input.tablet." + name)
			}
		case "map-to-output":
			var value []string
			if value, err = stringArgs(child, 1, 1); err == nil {
				c.Input.Tablet.MapToOutput = value[0]
			}
		case "calibration-matrix":
			for _, arg := range child.Arguments {
				switch arg.Value.(type) {
				case int64, float64:
				default:
					err = fmt.Errorf("calibration-matrix: argument %v is not a number", arg.Value)
				}
			}
			if err == nil {
				c.addWarning("input.tablet." + name)
			}
		default:
			err = fmt.Errorf("unknown node `%s`", name)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", nodeName(child), err)
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
		case "focus-ring":
			if err := c.focusRing(child); err != nil {
				return fmt.Errorf("focus-ring: %w", err)
			}
		case "shadow":
			if err := c.shadow(child); err != nil {
				return fmt.Errorf("shadow: %w", err)
			}
		default:
			c.addWarning("layout." + nodeName(child))
		}
	}
	return nil
}

func defaultAnimations() Animations {
	spring := func(stiffness uint32, epsilon float64) Animation {
		return Animation{Spring: &SpringConfig{DampingRatio: 1, Stiffness: stiffness, Epsilon: epsilon}}
	}
	return Animations{
		WindowOpen:                Animation{Unsupported: true}, // Niri's default is easing.
		WindowClose:               Animation{Unsupported: true}, // Niri's default is easing.
		WindowResize:              spring(800, 0.0001),
		WindowMovement:            spring(800, 0.0001),
		OverviewOpenClose:         spring(800, 0.0001),
		WorkspaceSwitch:           spring(1000, 0.0001),
		HorizontalViewMovement:    spring(800, 0.0001),
		ExitConfirmationOpenClose: Animation{Spring: &SpringConfig{DampingRatio: 0.6, Stiffness: 500, Epsilon: 0.01}},
		RecentWindowsClose:        spring(800, 0.001),
	}
}

func (c *Config) animations(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("animations takes child nodes only")
	}
	var off, on bool
	for _, child := range n.Children {
		name := nodeName(child)
		switch name {
		case "off", "on":
			if err := noValue(child); err != nil {
				return err
			}
			off = off || name == "off"
			on = on || name == "on"
		case "slowdown":
			value, err := numberArg(child)
			if err != nil || value < 0 || value > math.MaxInt32 {
				return fmt.Errorf("`slowdown` must be a number from 0 to %d", int64(math.MaxInt32))
			}
			c.addWarning("animations.slowdown")
		case "window-open", "window-close", "window-resize", "window-movement", "overview-open-close", "workspace-switch", "horizontal-view-movement",
			"exit-confirmation-open-close", "recent-windows-close":
			setting, err := c.parseAnimation(name, child)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			c.setAnimation(name, setting)
		case "config-notification-open-close", "screenshot-ui-open":
			c.addWarning("animations." + name)
		default:
			return fmt.Errorf("unexpected node `%s`", name)
		}
	}
	if off {
		c.Animations.Off = true
	}
	if on {
		c.Animations.Off = false
	}
	return nil
}

func (c *Config) parseAnimation(name string, node *document.Node) (Animation, error) {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return Animation{}, fmt.Errorf("takes child nodes only")
	}
	setting := defaultAnimation(name)
	sawSpring, sawEasing := false, false
	for _, child := range node.Children {
		switch nodeName(child) {
		case "off":
			if err := noValue(child); err != nil {
				return Animation{}, err
			}
			setting.Off = true
		case "spring":
			if sawEasing {
				return Animation{}, fmt.Errorf("cannot combine spring and easing settings")
			}
			if sawSpring {
				return Animation{}, fmt.Errorf("duplicate spring node")
			}
			spring, err := parseSpringConfig(child)
			if err != nil {
				return Animation{}, err
			}
			setting.Spring = &spring
			setting.Unsupported = false
			sawSpring = true
		case "duration-ms", "curve":
			if sawSpring {
				return Animation{}, fmt.Errorf("cannot combine spring and easing settings")
			}
			if err := validateEasingChild(child); err != nil {
				return Animation{}, err
			}
			setting.Spring = nil
			setting.Unsupported = true
			sawEasing = true
			c.addWarning("animations." + name + ".easing")
		case "custom-shader":
			if name != "window-open" && name != "window-close" && name != "window-resize" {
				return Animation{}, fmt.Errorf("unexpected node `custom-shader`")
			}
			if len(child.Properties) != 0 || len(child.Children) != 0 {
				return Animation{}, fmt.Errorf("`custom-shader` takes one string argument")
			}
			if _, err := stringArgs(child, 1, 1); err != nil {
				return Animation{}, err
			}
			c.addWarning("animations." + name + ".custom-shader")
		default:
			return Animation{}, fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	if setting.Unsupported && !setting.Off && !sawEasing {
		c.addWarning("animations." + name + ".easing")
	}
	return setting, nil
}

func defaultAnimation(name string) Animation {
	defaults := defaultAnimations()
	switch name {
	case "window-open":
		return defaults.WindowOpen
	case "window-close":
		return defaults.WindowClose
	case "window-resize":
		return defaults.WindowResize
	case "window-movement":
		return defaults.WindowMovement
	case "overview-open-close":
		return defaults.OverviewOpenClose
	case "workspace-switch":
		return defaults.WorkspaceSwitch
	case "exit-confirmation-open-close":
		return defaults.ExitConfirmationOpenClose
	case "recent-windows-close":
		return defaults.RecentWindowsClose
	default:
		return defaults.HorizontalViewMovement
	}
}

func (c *Config) setAnimation(name string, setting Animation) {
	switch name {
	case "window-open":
		c.Animations.WindowOpen = setting
	case "window-close":
		c.Animations.WindowClose = setting
	case "window-resize":
		c.Animations.WindowResize = setting
	case "window-movement":
		c.Animations.WindowMovement = setting
	case "overview-open-close":
		c.Animations.OverviewOpenClose = setting
	case "workspace-switch":
		c.Animations.WorkspaceSwitch = setting
	case "horizontal-view-movement":
		c.Animations.HorizontalViewMovement = setting
	case "exit-confirmation-open-close":
		c.Animations.ExitConfirmationOpenClose = setting
	case "recent-windows-close":
		c.Animations.RecentWindowsClose = setting
	}
}

func validateEasingChild(node *document.Node) error {
	if nodeName(node) == "duration-ms" {
		if len(node.Children) != 0 || len(node.Properties) != 0 {
			return fmt.Errorf("`duration-ms` takes one number")
		}
		_, err := intArg(node, 0, math.MaxUint32)
		return err
	}
	if len(node.Arguments) != 1 || len(node.Children) != 0 || len(node.Properties) != 0 {
		return fmt.Errorf("`curve` takes one easing name")
	}
	if _, ok := node.Arguments[0].Value.(string); !ok {
		return fmt.Errorf("`curve` must be a string")
	}
	return nil
}

func parseSpringConfig(node *document.Node) (SpringConfig, error) {
	if len(node.Arguments) != 0 || len(node.Children) != 0 {
		return SpringConfig{}, fmt.Errorf("spring takes properties only")
	}
	var result SpringConfig
	seen := map[string]bool{}
	for key, property := range node.Properties {
		seen[key] = true
		switch key {
		case "damping-ratio", "epsilon":
			value, ok := numericValue(property.Value)
			if !ok {
				return SpringConfig{}, fmt.Errorf("%s must be a number", key)
			}
			if key == "damping-ratio" {
				if value < 0.1 || value > 10 {
					return SpringConfig{}, fmt.Errorf("damping ratio must be between 0.1 and 10")
				}
				result.DampingRatio = value
			} else {
				if value < 0.00001 || value > 0.1 {
					return SpringConfig{}, fmt.Errorf("epsilon must be between 0.00001 and 0.1")
				}
				result.Epsilon = value
			}
		case "stiffness":
			value, ok := property.Value.(int64)
			if !ok || value < 1 || value > math.MaxUint32 {
				return SpringConfig{}, fmt.Errorf("stiffness must be a whole number from 1 to %d", uint64(math.MaxUint32))
			}
			result.Stiffness = uint32(value)
		default:
			return SpringConfig{}, fmt.Errorf("unexpected property `%s`", key)
		}
	}
	for _, key := range []string{"damping-ratio", "stiffness", "epsilon"} {
		if !seen[key] {
			return SpringConfig{}, fmt.Errorf("missing property `%s`", key)
		}
	}
	return result, nil
}

func numericValue(value any) (float64, bool) {
	var number float64
	switch value := value.(type) {
	case int64:
		number = float64(value)
	case float64:
		number = value
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func (c *Config) focusRing(node *document.Node) error {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return fmt.Errorf("focus-ring takes child nodes only")
	}
	for _, child := range node.Children {
		switch nodeName(child) {
		case "off", "on":
			if err := noValue(child); err != nil {
				return err
			}
			c.Layout.FocusRing.Off = nodeName(child) == "off"
		case "width":
			value, err := boundedNumber(child, 0, 65535)
			if err != nil {
				return err
			}
			c.Layout.FocusRing.Width = value
		case "active-color", "inactive-color":
			value, err := colorArg(child)
			if err != nil {
				return fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if nodeName(child) == "active-color" {
				c.Layout.FocusRing.ActiveColor = value
			} else {
				c.Layout.FocusRing.InactiveColor = value
			}
		case "urgent-color", "active-gradient", "inactive-gradient", "urgent-gradient":
			c.addWarning("layout.focus-ring." + nodeName(child))
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func (c *Config) shadow(node *document.Node) error {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return fmt.Errorf("shadow takes child nodes only")
	}
	for _, child := range node.Children {
		switch nodeName(child) {
		case "off", "on":
			if err := noValue(child); err != nil {
				return err
			}
			c.Layout.Shadow.On = nodeName(child) == "on"
		case "softness":
			value, err := boundedNumber(child, 0, 1024)
			if err != nil {
				return err
			}
			c.Layout.Shadow.Softness = value
		case "spread":
			value, err := boundedNumber(child, -1024, 1024)
			if err != nil {
				return err
			}
			c.Layout.Shadow.Spread = value
		case "offset":
			if len(child.Arguments) != 0 || len(child.Children) != 0 {
				return fmt.Errorf("offset takes x and y properties only")
			}
			for key, property := range child.Properties {
				value, ok := numericValue(property.Value)
				if !ok || value < -65535 || value > 65535 {
					return fmt.Errorf("offset %s must be a number from -65535 to 65535", key)
				}
				switch key {
				case "x":
					c.Layout.Shadow.OffsetX = value
				case "y":
					c.Layout.Shadow.OffsetY = value
				default:
					return fmt.Errorf("unexpected offset property `%s`", key)
				}
			}
		case "color":
			value, err := colorArg(child)
			if err != nil {
				return fmt.Errorf("color: %w", err)
			}
			c.Layout.Shadow.Color = value
		case "inactive-color", "draw-behind-window":
			c.addWarning("layout.shadow." + nodeName(child))
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func boundedNumber(node *document.Node, minValue, maxValue float64) (float64, error) {
	value, err := numberArg(node)
	if err != nil {
		return 0, err
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("`%s` must be a number from %v to %v", nodeName(node), minValue, maxValue)
	}
	return value, nil
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

func (c *Config) xwaylandSatellite(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("takes children only")
	}
	on := false
	seen := make(map[string]bool)
	for _, child := range n.Children {
		name := nodeName(child)
		if seen[name] {
			return fmt.Errorf("duplicate setting %q", name)
		}
		seen[name] = true
		switch name {
		case "off", "on":
			if err := noValue(child); err != nil {
				return err
			}
			if name == "on" {
				on = true
			} else {
				c.XwaylandSatellite.Off = true
			}
		case "path":
			args, err := stringArgs(child, 1, 1)
			if err != nil {
				return err
			}
			if args[0] == "" || len(child.Children) != 0 || len(child.Properties) != 0 {
				return fmt.Errorf("path needs one nonempty executable")
			}
			c.XwaylandSatellite.Path = args[0]
		default:
			return fmt.Errorf("unknown setting %q", name)
		}
	}
	if on {
		c.XwaylandSatellite.Off = false
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

func (c *Config) blur(n *document.Node) error {
	for _, child := range n.Children {
		var err error
		switch name := nodeName(child); name {
		case "off", "on":
			if err = noValue(child); err == nil {
				c.Blur.Off = name == "off"
			}
		case "passes":
			c.Blur.Passes, err = intArg(child, 1, 31)
		case "offset":
			c.Blur.Offset, err = blurNumber(child, 1000)
		case "noise":
			c.Blur.Noise, err = blurNumber(child, 1)
		case "saturation":
			c.Blur.Saturation, err = blurNumber(child, 10)
		default:
			err = fmt.Errorf("unknown setting %q", name)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", nodeName(child), err)
		}
	}
	return nil
}

func (c *Config) screenshotPath(n *document.Node) error {
	if len(n.Arguments) != 1 || len(n.Children) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("takes one path or null")
	}
	switch path := n.Arguments[0].Value.(type) {
	case nil:
		c.ScreenshotPath = nil
	case string:
		c.ScreenshotPath = &path
	default:
		return fmt.Errorf("must be a string or null")
	}
	return nil
}

// blurNumber is a blur setting from 0 to max inclusive.
func blurNumber(n *document.Node, max float64) (float64, error) {
	value, err := numberArg(n)
	if err != nil {
		return 0, err
	}
	if value < 0 || value > max {
		return 0, fmt.Errorf("`%s` must be from 0 to %v", nodeName(n), max)
	}
	return value, nil
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
	var parsed []Binding
	var overlay []OverlayBind
	overrides := make(map[bind.Chord]struct{}, len(n.Children))
	for _, node := range n.Children {
		chord, err := bind.ParseChord(nodeName(node))
		if err != nil {
			return fmt.Errorf("bind %q: %w", nodeName(node), err)
		}
		if _, duplicate := overrides[chord]; duplicate {
			return fmt.Errorf("duplicate bind %q", nodeName(node))
		}
		overrides[chord] = struct{}{}
		if len(node.Children) == 0 {
			return fmt.Errorf("bind %q: expected at least one action", nodeName(node))
		}
		entry, err := overlayBind(chord, node)
		if err != nil {
			return fmt.Errorf("bind %q: %w", nodeName(node), err)
		}
		overlay = append(overlay, entry)
		binding := Binding{Chord: chord, Repeat: true}
		if property, ok := node.Properties["repeat"]; ok {
			repeat, ok := property.Value.(bool)
			if !ok {
				return fmt.Errorf("bind %q: `repeat` must be boolean", nodeName(node))
			}
			binding.Repeat = repeat
		}
		if property, ok := node.Properties["allow-when-locked"]; ok {
			allow, ok := property.Value.(bool)
			if !ok {
				return fmt.Errorf("bind %q: `allow-when-locked` must be boolean", nodeName(node))
			}
			binding.AllowWhenLocked = allow
		}
		if property, ok := node.Properties["allow-inhibiting"]; ok {
			allow, ok := property.Value.(bool)
			if !ok {
				return fmt.Errorf("bind %q: `allow-inhibiting` must be boolean", nodeName(node))
			}
			binding.IgnoresInhibit = !allow
		}
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
			if name == bind.ActionScreenshotScreen || name == bind.ActionScreenshotWindow || name == bind.ActionScreenshot {
				action.WriteToDisk, action.ShowPointer = true, name != bind.ActionScreenshotWindow
				for key, property := range actionNode.Properties {
					value, ok := property.Value.(bool)
					if !ok {
						return fmt.Errorf("bind %q action %s: `%s` must be boolean", nodeName(node), name, key)
					}
					switch key {
					case "write-to-disk":
						if name == bind.ActionScreenshot {
							return fmt.Errorf("bind %q action %s: unexpected property `%s`", nodeName(node), name, key)
						}
						action.WriteToDisk = value
					case "show-pointer":
						action.ShowPointer = value
					default:
						return fmt.Errorf("bind %q action %s: unexpected property `%s`", nodeName(node), name, key)
					}
				}
			}
			if name == bind.ActionQuit {
				for key, property := range actionNode.Properties {
					value, ok := property.Value.(bool)
					if key != "skip-confirmation" || !ok {
						return fmt.Errorf("bind %q action %s: unexpected property `%s`", nodeName(node), name, key)
					}
					action.SkipConfirmation = value
				}
			}
			if chord.Trigger != bind.TriggerKey {
				c.addUnappliedAction(name)
				continue
			}
			binding.Actions = append(binding.Actions, action)
		}
		if len(binding.Actions) > 0 {
			parsed = append(parsed, binding)
		}
	}
	kept := c.Binds[:0]
	for _, binding := range c.Binds {
		if _, replaced := overrides[binding.Chord]; !replaced {
			kept = append(kept, binding)
		}
	}
	c.Binds = append(kept, parsed...)
	keptOverlay := c.OverlayBinds[:0]
	for _, entry := range c.OverlayBinds {
		if _, replaced := overrides[entry.Chord]; !replaced {
			keptOverlay = append(keptOverlay, entry)
		}
	}
	c.OverlayBinds = append(keptOverlay, overlay...)
	return nil
}

// overlayBind reads what the hotkey overlay needs from a bind node.
func overlayBind(chord bind.Chord, node *document.Node) (OverlayBind, error) {
	action := node.Children[0]
	entry := OverlayBind{Chord: chord, Action: nodeName(action)}
	// Arguments that are not strings or integers belong to actions this
	// compositor does not apply; the overlay only names them.
	entry.Args, _ = actionArgs(action)
	if skip, ok := action.Properties["skip-confirmation"]; ok && entry.Action == bind.ActionQuit {
		entry.SkipConfirmation, _ = skip.Value.(bool)
	}
	if property, ok := node.Properties["hotkey-overlay-title"]; ok {
		switch title := property.Value.(type) {
		case nil:
			entry.TitleHidden = true
		case string:
			entry.Title = &title
		default:
			return entry, fmt.Errorf("`hotkey-overlay-title` must be a string or null")
		}
	}
	return entry, nil
}

func (c *Config) hotkeyOverlay(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("`%s` takes child nodes only", nodeName(n))
	}
	for _, child := range n.Children {
		if err := noValue(child); err != nil {
			return err
		}
		switch nodeName(child) {
		case "skip-at-startup":
			c.HotkeyOverlay.SkipAtStartup = true
		case "hide-not-bound":
			c.HotkeyOverlay.HideNotBound = true
		default:
			return fmt.Errorf("unexpected node `%s`", nodeName(child))
		}
	}
	return nil
}

func (c *Config) switchEvents(n *document.Node) error {
	if len(n.Arguments) != 0 || len(n.Properties) != 0 {
		return fmt.Errorf("`%s` takes no arguments or properties", nodeName(n))
	}
	seen := make(map[string]bool, len(n.Children))
	for _, event := range n.Children {
		name := nodeName(event)
		if seen[name] {
			return fmt.Errorf("duplicate switch event `%s`", name)
		}
		seen[name] = true
		if len(event.Arguments) != 0 || len(event.Properties) != 0 || len(event.Children) != 1 {
			return fmt.Errorf("switch event `%s` needs one spawn action", name)
		}
		spawn := event.Children[0]
		if nodeName(spawn) != "spawn" || len(spawn.Children) != 0 || len(spawn.Properties) != 0 {
			return fmt.Errorf("switch event `%s` only supports spawn", name)
		}
		argv, err := stringArgs(spawn, 1, -1)
		if err != nil {
			return fmt.Errorf("switch event `%s`: %w", name, err)
		}
		switch name {
		case "lid-open":
			c.SwitchEvents.LidOpen = argv
		case "lid-close":
			c.SwitchEvents.LidClose = argv
		case "tablet-mode-on":
			c.SwitchEvents.TabletModeOn = argv
		case "tablet-mode-off":
			c.SwitchEvents.TabletModeOff = argv
		default:
			return fmt.Errorf("unexpected switch event `%s`", name)
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
	if len(n.Arguments) != 1 || len(n.Children) != 0 || len(n.Properties) != 0 {
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

// Path is where niri reads its config: one named file (-c, $NIRI_CONFIG or
// load-config-file), or else Search, the user's and then the system config,
// searched again on every load so a config created after startup is found.
type Path struct {
	Named  string
	Search []string
}

// Locate is the config path niri would read: the -c flag, then $NIRI_CONFIG
// (empty means unset), then the user's and the system config.
func Locate(flag string, getenv func(string) string) Path {
	if flag != "" {
		return Path{Named: flag}
	}
	if p := getenv("NIRI_CONFIG"); p != "" {
		return Path{Named: p}
	}
	var search []string
	if x := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		search = append(search, filepath.Join(x, "niri", "config.kdl"))
	} else if h := getenv("HOME"); h != "" {
		search = append(search, filepath.Join(h, ".config", "niri", "config.kdl"))
	}
	return Path{Search: append(search, "/etc/niri/config.kdl")}
}

// Resolve is the file to load now. It returns "" when nothing was named and
// no searched file exists.
func (p Path) Resolve() string {
	if p.Named != "" {
		return p.Named
	}
	for _, candidate := range p.Search {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// Load reads and parses path; "" is the default config.
func Load(path string) (*Config, error) {
	result, err := LoadWithSources(path)
	return result.Config, err
}

// LoadWithSources reads a config and returns the root and include paths used
// during parsing, even when a read or parse fails.
func LoadWithSources(path string) (LoadResult, error) {
	if path == "" {
		return LoadResult{Config: Default()}, nil
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return LoadResult{}, err
	}
	root = filepath.Clean(root)
	src, err := os.ReadFile(root)
	if err != nil {
		return LoadResult{Root: root, Sources: []string{root}}, err
	}
	return ParseWithSources(root, src)
}
