package config

import (
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/sblinch/kdl-go/document"
)

type WorkspaceConfig struct {
	Name         string
	OpenOnOutput string
}

type PresetSize struct {
	Fixed         bool
	Value         float64
	WindowChooses bool
}

func (p PresetSize) Resolve(available int) int {
	if p.Fixed {
		return int(p.Value)
	}
	return int(math.Round(float64(available) * p.Value))
}

// ColumnDisplay is the way the windows of a column are stacked.
type ColumnDisplay string

const (
	ColumnDisplayNormal ColumnDisplay = "normal"
	ColumnDisplayTabbed ColumnDisplay = "tabbed"
)

// RelativeTo names the corner or edge a floating position is measured from.
// The names are the ones Niri uses.
type RelativeTo string

const (
	RelativeToTopLeft     RelativeTo = "top-left"
	RelativeToTopRight    RelativeTo = "top-right"
	RelativeToBottomLeft  RelativeTo = "bottom-left"
	RelativeToBottomRight RelativeTo = "bottom-right"
	RelativeToTop         RelativeTo = "top"
	RelativeToBottom      RelativeTo = "bottom"
	RelativeToLeft        RelativeTo = "left"
	RelativeToRight       RelativeTo = "right"
)

// FloatingPosition is where a floating window opens, measured from one of the
// working area's corners or edges.
type FloatingPosition struct {
	X          int
	Y          int
	RelativeTo RelativeTo
}

// RuleMatch is one match line of a window rule. The state properties are
// compared against the window's state when the rule is resolved, the way
// Niri does.
type RuleMatch struct {
	AppID            *regexp.Regexp
	Title            *regexp.Regexp
	IsFocused        *bool
	IsUrgent         *bool
	IsActiveInColumn *bool
	IsFloating       *bool
	AtStartup        *bool
	Unsupported      bool
}

// WindowState is the state a window rule is matched against. The fields carry
// the meaning of Niri's match properties of the same name.
type WindowState struct {
	IsFocused        bool
	IsUrgent         bool
	IsActiveInColumn bool
	IsFloating       bool
	AtStartup        bool
}

// UnmappedWindowState is the state a window that is not on screen has. Niri
// reads the rules of a window that is still unmapped that way: it is not
// focused, not urgent, not floating, and it is the active window of its
// column.
func UnmappedWindowState(atStartup bool) WindowState {
	return WindowState{IsActiveInColumn: true, AtStartup: atStartup}
}

type WindowRule struct {
	BlockOutFrom         *string
	Matches              []RuleMatch
	Excludes             []RuleMatch
	OpenOnOutput         *string
	OpenOnWorkspace      *string
	OpenFloating         *bool
	OpenFullscreen       *bool
	OpenMaximized        *bool
	OpenMaximizedToEdges *bool
	OpenFocused          *bool
	DefaultColumnWidth   *PresetSize
	DefaultWindowHeight  *PresetSize
	GeometryCornerRadius *CornerRadius
	ClipToGeometry       *bool
	MinWidth             *int
	MaxWidth             *int
	MinHeight            *int
	MaxHeight            *int
	DefaultColumnDisplay *ColumnDisplay
	// DefaultFloatingPosition is where a floating window opens.
	DefaultFloatingPosition *FloatingPosition
}

type WindowRuleResult struct {
	BlockOutFrom         *string
	OpenOnOutput         *string
	OpenOnWorkspace      *string
	OpenFloating         *bool
	OpenFullscreen       *bool
	OpenMaximized        *bool
	OpenMaximizedToEdges *bool
	OpenFocused          *bool
	DefaultColumnWidth   *PresetSize
	DefaultWindowHeight  *PresetSize
	GeometryCornerRadius *CornerRadius
	ClipToGeometry       *bool
	MinWidth             *int
	MaxWidth             *int
	MinHeight            *int
	MaxHeight            *int
	DefaultColumnDisplay *ColumnDisplay
	// DefaultFloatingPosition is where a floating window opens.
	DefaultFloatingPosition *FloatingPosition
}

type LayerRule struct {
	Matches             []LayerMatch
	Excludes            []LayerMatch
	PlaceWithinBackdrop *bool
}

type LayerMatch struct {
	Namespace   *regexp.Regexp
	Unsupported bool
}

func (c *Config) ResolveWindowRules(appID, title string, state WindowState) WindowRuleResult {
	return ResolveWindowRules(c.WindowRules, appID, title, state)
}

func ResolveWindowRules(rules []WindowRule, appID, title string, state WindowState) WindowRuleResult {
	var result WindowRuleResult
	for _, rule := range rules {
		if !rule.matches(appID, title, state) {
			continue
		}
		applyRuleValue(&result.OpenOnOutput, rule.OpenOnOutput)
		applyRuleValue(&result.OpenOnWorkspace, rule.OpenOnWorkspace)
		applyRuleValue(&result.OpenFloating, rule.OpenFloating)
		applyRuleValue(&result.OpenFullscreen, rule.OpenFullscreen)
		applyRuleValue(&result.OpenMaximized, rule.OpenMaximized)
		applyRuleValue(&result.OpenMaximizedToEdges, rule.OpenMaximizedToEdges)
		applyRuleValue(&result.OpenFocused, rule.OpenFocused)
		applyRuleValue(&result.DefaultColumnWidth, rule.DefaultColumnWidth)
		applyRuleValue(&result.DefaultWindowHeight, rule.DefaultWindowHeight)
		applyRuleValue(&result.GeometryCornerRadius, rule.GeometryCornerRadius)
		applyRuleValue(&result.ClipToGeometry, rule.ClipToGeometry)
		applyRuleValue(&result.BlockOutFrom, rule.BlockOutFrom)
		applyRuleValue(&result.MinWidth, rule.MinWidth)
		applyRuleValue(&result.MaxWidth, rule.MaxWidth)
		applyRuleValue(&result.MinHeight, rule.MinHeight)
		applyRuleValue(&result.MaxHeight, rule.MaxHeight)
		applyRuleValue(&result.DefaultColumnDisplay, rule.DefaultColumnDisplay)
		applyRuleValue(&result.DefaultFloatingPosition, rule.DefaultFloatingPosition)
	}
	return result
}

type CaptureTarget uint8

const (
	CaptureDisplay CaptureTarget = iota
	CaptureScreen
	CaptureCast
)

// BlocksCapture follows Niri's RenderTarget: screen-capture protects all
// capture targets, while screencast protects only PipeWire streams.
func (r WindowRuleResult) BlocksCapture(target CaptureTarget) bool {
	return target != CaptureDisplay && r.BlockOutFrom != nil &&
		(*r.BlockOutFrom == "screen-capture" || target == CaptureCast && *r.BlockOutFrom == "screencast")
}

func (r WindowRule) matches(appID, title string, state WindowState) bool {
	if hasUnsupportedWindowMatch(r.Matches) || hasUnsupportedWindowMatch(r.Excludes) {
		return false
	}
	if len(r.Matches) != 0 {
		matched := false
		for _, ruleMatch := range r.Matches {
			if ruleMatch.matches(appID, title, state) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, ruleMatch := range r.Excludes {
		if ruleMatch.matches(appID, title, state) {
			return false
		}
	}
	return true
}

// matches reports whether the match line holds for the window. The state
// properties are compared against the window's state.
func (m RuleMatch) matches(appID, title string, state WindowState) bool {
	if m.Unsupported {
		return false
	}
	if m.AppID != nil && !m.AppID.MatchString(appID) {
		return false
	}
	if m.Title != nil && !m.Title.MatchString(title) {
		return false
	}
	if m.IsFocused != nil && *m.IsFocused != state.IsFocused {
		return false
	}
	if m.IsUrgent != nil && *m.IsUrgent != state.IsUrgent {
		return false
	}
	if m.IsActiveInColumn != nil && *m.IsActiveInColumn != state.IsActiveInColumn {
		return false
	}
	if m.IsFloating != nil && *m.IsFloating != state.IsFloating {
		return false
	}
	if m.AtStartup != nil && *m.AtStartup != state.AtStartup {
		return false
	}
	return true
}

func hasUnsupportedWindowMatch(matches []RuleMatch) bool {
	for _, match := range matches {
		if match.Unsupported {
			return true
		}
	}
	return false
}

func (c *Config) ResolveLayerRules(namespace string) bool {
	return ResolveLayerRules(c.LayerRules, namespace)
}

func ResolveLayerRules(rules []LayerRule, namespace string) bool {
	withinBackdrop := false
	for _, rule := range rules {
		if !rule.matches(namespace) {
			continue
		}
		if rule.PlaceWithinBackdrop != nil {
			withinBackdrop = *rule.PlaceWithinBackdrop
		}
	}
	return withinBackdrop
}

func (r LayerRule) matches(namespace string) bool {
	if hasUnsupportedLayerMatch(r.Matches) || hasUnsupportedLayerMatch(r.Excludes) {
		return false
	}
	if len(r.Matches) != 0 {
		matched := false
		for _, ruleMatch := range r.Matches {
			if ruleMatch.matches(namespace) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, ruleMatch := range r.Excludes {
		if ruleMatch.matches(namespace) {
			return false
		}
	}
	return true
}

func (m LayerMatch) matches(namespace string) bool {
	return !m.Unsupported && (m.Namespace == nil || m.Namespace.MatchString(namespace))
}

func hasUnsupportedLayerMatch(matches []LayerMatch) bool {
	for _, match := range matches {
		if match.Unsupported {
			return true
		}
	}
	return false
}

func applyRuleValue[T any](target **T, value *T) {
	if value == nil {
		return
	}
	copy := *value
	*target = &copy
}

func (c *Config) workspace(node *document.Node) error {
	args, err := stringArgs(node, 1, 1)
	if err != nil {
		return err
	}
	name := args[0]
	if name == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	for _, existing := range c.Workspaces {
		if asciiEqualFold(existing.Name, name) {
			return fmt.Errorf("duplicate workspace name %q", name)
		}
	}
	workspace := WorkspaceConfig{Name: name}
	for _, child := range node.Children {
		switch nodeName(child) {
		case "open-on-output":
			value, err := stringArgs(child, 1, 1)
			if err != nil {
				return fmt.Errorf("open-on-output: %w", err)
			}
			workspace.OpenOnOutput = value[0]
		case "layout":
			c.addWarning("workspace.layout")
		default:
			return fmt.Errorf("unexpected workspace child `%s`", nodeName(child))
		}
	}
	c.Workspaces = append(c.Workspaces, workspace)
	return nil
}

func (c *Config) windowRule(node *document.Node) error {
	rule := WindowRule{}
	for _, child := range node.Children {
		var err error
		switch nodeName(child) {
		case "match", "exclude":
			var match RuleMatch
			match, err = parseWindowMatch(child)
			if nodeName(child) == "match" {
				rule.Matches = append(rule.Matches, match)
			} else {
				rule.Excludes = append(rule.Excludes, match)
			}
		case "open-on-output", "open-on-workspace":
			var value []string
			value, err = stringArgs(child, 1, 1)
			if err == nil {
				if nodeName(child) == "open-on-output" {
					rule.OpenOnOutput = &value[0]
				} else {
					rule.OpenOnWorkspace = &value[0]
				}
			}
		case "open-floating", "open-fullscreen", "open-maximized", "open-maximized-to-edges", "open-focused":
			var value bool
			value, err = boolArg(child)
			if err == nil {
				switch nodeName(child) {
				case "open-floating":
					rule.OpenFloating = &value
				case "open-fullscreen":
					rule.OpenFullscreen = &value
				case "open-maximized":
					rule.OpenMaximized = &value
				case "open-maximized-to-edges":
					rule.OpenMaximizedToEdges = &value
				case "open-focused":
					rule.OpenFocused = &value
				}
			}
		case "min-width", "max-width", "min-height", "max-height":
			var value int
			value, err = intArg(child, 0, 65535)
			if err == nil {
				switch nodeName(child) {
				case "min-width":
					rule.MinWidth = &value
				case "max-width":
					rule.MaxWidth = &value
				case "min-height":
					rule.MinHeight = &value
				case "max-height":
					rule.MaxHeight = &value
				}
			}
		case "default-column-display":
			var value []string
			value, err = stringArgs(child, 1, 1)
			if err == nil {
				display := ColumnDisplay(value[0])
				if display != ColumnDisplayNormal && display != ColumnDisplayTabbed {
					err = fmt.Errorf("expected normal or tabbed")
				} else {
					rule.DefaultColumnDisplay = &display
				}
			}
		case "default-floating-position":
			var value FloatingPosition
			value, err = parseFloatingPosition(child)
			if err == nil {
				rule.DefaultFloatingPosition = &value
			}
		case "default-column-width", "default-window-height":
			var value PresetSize
			value, err = parsePresetSize(child)
			if err == nil {
				if nodeName(child) == "default-column-width" {
					rule.DefaultColumnWidth = &value
				} else {
					rule.DefaultWindowHeight = &value
				}
			}
		case "geometry-corner-radius":
			var value CornerRadius
			value, err = parseCornerRadius(child)
			if err == nil {
				rule.GeometryCornerRadius = &value
			}
		case "clip-to-geometry":
			var value bool
			value, err = boolArg(child)
			if err == nil {
				rule.ClipToGeometry = &value
			}
		case "block-out-from":
			var value []string
			value, err = stringArgs(child, 1, 1)
			if err == nil {
				if value[0] != "screen-capture" && value[0] != "screencast" {
					err = fmt.Errorf("expected screen-capture or screencast")
				} else {
					rule.BlockOutFrom = &value[0]
				}
			}
		default:
			c.addWarning("window-rule")
		}
		if err != nil {
			return fmt.Errorf("%s: %w", nodeName(child), err)
		}
		if (nodeName(child) == "match" || nodeName(child) == "exclude") &&
			((nodeName(child) == "match" && rule.Matches[len(rule.Matches)-1].Unsupported) ||
				(nodeName(child) == "exclude" && rule.Excludes[len(rule.Excludes)-1].Unsupported)) {
			c.addWarning("window-rule.match")
		}
	}
	c.WindowRules = append(c.WindowRules, rule)
	return nil
}

func parseCornerRadius(node *document.Node) (CornerRadius, error) {
	if len(node.Properties) != 0 || len(node.Children) != 0 || (len(node.Arguments) != 1 && len(node.Arguments) != 4) {
		return CornerRadius{}, fmt.Errorf("geometry-corner-radius takes one or four numbers")
	}
	var radius CornerRadius
	for i, arg := range node.Arguments {
		value, ok := numericValue(arg.Value)
		if !ok || value < 0 {
			return CornerRadius{}, fmt.Errorf("corner radius %d must be a finite non-negative number", i+1)
		}
		radius[i] = value
	}
	if len(node.Arguments) == 1 {
		radius[1], radius[2], radius[3] = radius[0], radius[0], radius[0]
	}
	return radius, nil
}

func parseWindowMatch(node *document.Node) (RuleMatch, error) {
	if len(node.Arguments) != 0 || len(node.Children) != 0 {
		return RuleMatch{}, fmt.Errorf("match takes properties only")
	}
	var match RuleMatch
	for key, property := range node.Properties {
		value, ok := property.Value.(string)
		if key == "app-id" || key == "title" {
			if !ok {
				return RuleMatch{}, fmt.Errorf("%s must be a regular expression string", key)
			}
			compiled, err := regexp.Compile(value)
			if err != nil {
				return RuleMatch{}, fmt.Errorf("%s regular expression: %w", key, err)
			}
			if key == "app-id" {
				match.AppID = compiled
			} else {
				match.Title = compiled
			}
			continue
		}
		switch key {
		case "is-focused", "is-urgent", "is-active-in-column", "is-floating", "at-startup":
			value, ok := property.Value.(bool)
			if !ok {
				return RuleMatch{}, fmt.Errorf("%s must be boolean", key)
			}
			switch key {
			case "is-focused":
				match.IsFocused = &value
			case "is-urgent":
				match.IsUrgent = &value
			case "is-active-in-column":
				match.IsActiveInColumn = &value
			case "is-floating":
				match.IsFloating = &value
			case "at-startup":
				match.AtStartup = &value
			}
		case "is-active", "is-window-cast-target":
			if _, ok := property.Value.(bool); !ok {
				return RuleMatch{}, fmt.Errorf("%s must be boolean", key)
			}
			match.Unsupported = true
		default:
			return RuleMatch{}, fmt.Errorf("unexpected match property `%s`", key)
		}
	}
	return match, nil
}

// parseFloatingPosition reads the position a floating window opens at. The
// position is measured from the corner or the edge that relative-to names.
func parseFloatingPosition(node *document.Node) (FloatingPosition, error) {
	if len(node.Arguments) != 0 || len(node.Children) != 0 {
		return FloatingPosition{}, fmt.Errorf("default-floating-position takes properties only")
	}
	position := FloatingPosition{RelativeTo: RelativeToTopLeft}
	for key, property := range node.Properties {
		switch key {
		case "x", "y":
			value, ok := numericValue(property.Value)
			if !ok {
				return FloatingPosition{}, fmt.Errorf("%s must be a number", key)
			}
			if value < -65535 || value > 65535 {
				return FloatingPosition{}, fmt.Errorf("%s must be from -65535 to 65535", key)
			}
			if key == "x" {
				position.X = int(math.Round(value))
			} else {
				position.Y = int(math.Round(value))
			}
		case "relative-to":
			text, ok := property.Value.(string)
			if !ok {
				return FloatingPosition{}, fmt.Errorf("relative-to must be a string")
			}
			relative := RelativeTo(text)
			switch relative {
			case RelativeToTopLeft, RelativeToTopRight, RelativeToBottomLeft, RelativeToBottomRight,
				RelativeToTop, RelativeToBottom, RelativeToLeft, RelativeToRight:
				position.RelativeTo = relative
			default:
				return FloatingPosition{}, fmt.Errorf("relative-to must be one of top-left, top-right, bottom-left, bottom-right, top, bottom, left, right")
			}
		default:
			return FloatingPosition{}, fmt.Errorf("unexpected default-floating-position property `%s`", key)
		}
	}
	return position, nil
}

func (c *Config) layerRule(node *document.Node) error {
	rule := LayerRule{}
	for _, child := range node.Children {
		switch nodeName(child) {
		case "match", "exclude":
			match, err := parseLayerMatch(child)
			if err != nil {
				return fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if nodeName(child) == "match" {
				rule.Matches = append(rule.Matches, match)
			} else {
				rule.Excludes = append(rule.Excludes, match)
			}
			if match.Unsupported {
				c.addWarning("layer-rule.match")
			}
		case "place-within-backdrop":
			value, err := boolArg(child)
			if err != nil {
				return fmt.Errorf("place-within-backdrop: %w", err)
			}
			rule.PlaceWithinBackdrop = &value
		default:
			c.addWarning("layer-rule")
		}
	}
	c.LayerRules = append(c.LayerRules, rule)
	return nil
}

func parseLayerMatch(node *document.Node) (LayerMatch, error) {
	if len(node.Arguments) != 0 || len(node.Children) != 0 {
		return LayerMatch{}, fmt.Errorf("match takes properties only")
	}
	var match LayerMatch
	for key, property := range node.Properties {
		value, ok := property.Value.(string)
		if key == "namespace" {
			if !ok {
				return LayerMatch{}, fmt.Errorf("namespace must be a regular expression string")
			}
			compiled, err := regexp.Compile(value)
			if err != nil {
				return LayerMatch{}, fmt.Errorf("namespace regular expression: %w", err)
			}
			match.Namespace = compiled
			continue
		}
		switch key {
		case "at-startup", "layer":
			if key == "at-startup" {
				if _, ok := property.Value.(bool); !ok {
					return LayerMatch{}, fmt.Errorf("at-startup must be boolean")
				}
			} else if !ok {
				return LayerMatch{}, fmt.Errorf("layer must be a string")
			}
			match.Unsupported = true
		default:
			return LayerMatch{}, fmt.Errorf("unexpected match property `%s`", key)
		}
	}
	return match, nil
}

func parsePresetSize(node *document.Node) (PresetSize, error) {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 || len(node.Children) > 1 {
		return PresetSize{}, fmt.Errorf("takes exactly one fixed or proportion child")
	}
	if len(node.Children) == 0 {
		return PresetSize{WindowChooses: true}, nil
	}
	child := node.Children[0]
	switch nodeName(child) {
	case "fixed":
		value, err := intArg(child, 1, 1<<31-1)
		if err != nil {
			return PresetSize{}, err
		}
		return PresetSize{Fixed: true, Value: float64(value)}, nil
	case "proportion":
		value, err := proportionArg(child)
		if err != nil {
			return PresetSize{}, err
		}
		return PresetSize{Value: value}, nil
	default:
		return PresetSize{}, fmt.Errorf("expected fixed or proportion, got `%s`", nodeName(child))
	}
}

func proportionArg(node *document.Node) (float64, error) {
	if len(node.Arguments) != 1 {
		return 0, fmt.Errorf("`%s` takes one decimal number", nodeName(node))
	}
	value, ok := node.Arguments[0].Value.(float64)
	if !ok {
		return 0, fmt.Errorf("`%s` must be a decimal number", nodeName(node))
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("`%s` must be a finite decimal number", nodeName(node))
	}
	return min(10000, max(0, value)), nil
}

func boolArg(node *document.Node) (bool, error) {
	if len(node.Arguments) != 1 || len(node.Children) != 0 || len(node.Properties) != 0 {
		return false, fmt.Errorf("takes one boolean argument")
	}
	value, ok := node.Arguments[0].Value.(bool)
	if !ok {
		return false, fmt.Errorf("argument must be boolean")
	}
	return value, nil
}

func (c *Config) addWarning(warning string) {
	if !slices.Contains(c.Warnings, warning) {
		c.Warnings = append(c.Warnings, warning)
	}
}

func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		left, right := a[i], b[i]
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		if left != right {
			return false
		}
	}
	return true
}
