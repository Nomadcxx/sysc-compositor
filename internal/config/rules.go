package config

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/sblinch/kdl-go/document"
)

type WorkspaceConfig struct {
	Name         string
	OpenOnOutput string
	Layout       *WorkspaceLayout
}

// WorkspaceLayout holds the layout overrides a workspace may set. A nil
// field means the workspace keeps the value of its output.
type WorkspaceLayout struct {
	Gaps                  *int
	ColumnWidthProportion *float64
	FixedColumnWidth      *int
	// A nil preset list keeps the output's list, an empty one resets to
	// Niri's default presets.
	PresetColumnWidths   []PresetSize
	PresetWindowHeights  []PresetSize
	DefaultColumnDisplay *ColumnDisplay
	// A nil mode keeps the output's policy.
	CenterFocusedColumn *CenterFocusedColumn
	// A nil flag keeps the output's always-center-single-column.
	AlwaysCenterSingleColumn *bool
	// A nil strut set keeps the output's. When present it replaces the
	// whole set, Niri's merge_clone of the struct, not one edge at a time.
	Struts *Struts
	// A nil colour keeps the output's background-color.
	BackgroundColor *Color
	// Decorations merge over the layout's, field by field.
	Decorations
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

// CenterFocusedColumn is when the view centers the focused column.
type CenterFocusedColumn string

const (
	CenterFocusedColumnNever      CenterFocusedColumn = "never"
	CenterFocusedColumnAlways     CenterFocusedColumn = "always"
	CenterFocusedColumnOnOverflow CenterFocusedColumn = "on-overflow"
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
	AppID              *regexp.Regexp
	Title              *regexp.Regexp
	IsFocused          *bool
	IsUrgent           *bool
	IsActive           *bool
	IsActiveInColumn   *bool
	IsFloating         *bool
	IsWindowCastTarget *bool
	AtStartup          *bool
	Unsupported        bool
}

// WindowState is the state a window rule is matched against. The fields carry
// the meaning of Niri's match properties of the same name.
type WindowState struct {
	IsFocused          bool
	IsUrgent           bool
	IsActive           bool
	IsActiveInColumn   bool
	IsFloating         bool
	IsWindowCastTarget bool
	AtStartup          bool
}

// UnmappedWindowState is the state a window that is not on screen has. Niri
// reads the rules of a window that is still unmapped that way: it is not
// focused, not urgent, not floating, not active, not the target of a window
// screencast, and it is the active window of its column.
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
	// Opacity is clamped to [0, 1] when the window is rendered.
	Opacity              *float64
	MinWidth             *int
	MaxWidth             *int
	MinHeight            *int
	MaxHeight            *int
	DefaultColumnDisplay *ColumnDisplay
	// DefaultFloatingPosition is where a floating window opens.
	DefaultFloatingPosition *FloatingPosition
	// ScrollFactor scales the scroll events delivered to the window, Niri's
	// scroll-factor; nil means one.
	ScrollFactor        *float64
	VariableRefreshRate *bool
	// Decorations merge field by field over the window's workspace layout,
	// and over each other in rule order.
	Decorations
	// DrawBorderWithBackground nil means true for a client-side decorated window.
	DrawBorderWithBackground *bool
	BackgroundEffect         BackgroundEffectRule
	// TiledState nil derives the tiled state from the decoration mode.
	TiledState *bool
	// BabaIsFloat bobs the window up and down, Niri's baba-is-float.
	BabaIsFloat *bool
	Popups      PopupsRule
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
	Opacity              *float64
	MinWidth             *int
	MaxWidth             *int
	MinHeight            *int
	MaxHeight            *int
	DefaultColumnDisplay *ColumnDisplay
	// DefaultFloatingPosition is where a floating window opens.
	DefaultFloatingPosition *FloatingPosition
	// ScrollFactor scales the scroll events delivered to the window, Niri's
	// scroll-factor; nil means one.
	ScrollFactor        *float64
	VariableRefreshRate *bool
	// Decorations merge field by field over the window's workspace layout,
	// and over each other in rule order.
	Decorations
	// DrawBorderWithBackground nil means true for a client-side decorated window.
	DrawBorderWithBackground *bool
	BackgroundEffect         BackgroundEffectRule
	// TiledState nil derives the tiled state from the decoration mode.
	TiledState *bool
	// BabaIsFloat bobs the window up and down, Niri's baba-is-float.
	BabaIsFloat *bool
	Popups      PopupsRule
}

type LayerRule struct {
	Matches             []LayerMatch
	Excludes            []LayerMatch
	PlaceWithinBackdrop *bool
	// Opacity is clamped to [0, 1] when the layer is rendered.
	Opacity              *float64
	BlockOutFrom         *string
	GeometryCornerRadius *CornerRadius
	// Shadow merges over the layout's shadow turned off, as in Niri.
	Shadow           ShadowRule
	BackgroundEffect BackgroundEffectRule
	Popups           PopupsRule
	BabaIsFloat      *bool
}

// LayerRuleResult is every matching layer rule laid over the last, in order.
type LayerRuleResult struct {
	PlaceWithinBackdrop  bool
	Opacity              *float64
	BlockOutFrom         *string
	GeometryCornerRadius *CornerRadius
	Shadow               ShadowRule
	BackgroundEffect     BackgroundEffectRule
	Popups               PopupsRule
	// BabaIsFloat bobs the surface up and down, Niri's baba-is-float.
	BabaIsFloat bool
}

// PopupsRule is Niri's popups part: the xdg popups of a window or layer
// surface draw with this opacity on top of their parent's, and a blur under
// them takes this corner radius. A nil field keeps the earlier rule's.
type PopupsRule struct {
	// Opacity is clamped to [0, 1] when the popup is rendered.
	Opacity              *float64
	GeometryCornerRadius *CornerRadius
	BackgroundEffect     BackgroundEffectRule
}

func (r PopupsRule) merge(part PopupsRule) PopupsRule {
	applyRuleValue(&r.Opacity, part.Opacity)
	applyRuleValue(&r.GeometryCornerRadius, part.GeometryCornerRadius)
	r.BackgroundEffect.Blur = cmp.Or(part.BackgroundEffect.Blur, r.BackgroundEffect.Blur)
	return r
}

// BlocksCapture is WindowRuleResult.BlocksCapture for a layer.
func (r LayerRuleResult) BlocksCapture(target CaptureTarget) bool {
	return blocksCapture(r.BlockOutFrom, target)
}

// BackgroundEffectRule is Niri's background-effect part. Only blur applies:
// nil follows the surface's ext-background-effect region, true blurs the
// whole surface and false never blurs.
type BackgroundEffectRule struct {
	Blur *bool
}

// LayerMatch is one match or exclude line of a layer rule. Its properties are
// ANDed; a nil property matches every surface.
type LayerMatch struct {
	Namespace *regexp.Regexp
	AtStartup *bool
	Layer     *LayerName
}

// LayerName is a layer-shell layer by its Niri name.
type LayerName string

const (
	LayerBackground LayerName = "background"
	LayerBottom     LayerName = "bottom"
	LayerTop        LayerName = "top"
	LayerOverlay    LayerName = "overlay"
)

// LayerState is the state a layer rule is matched against besides the
// namespace: the surface's current layer and whether the compositor is
// still starting up.
type LayerState struct {
	Layer     LayerName
	AtStartup bool
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
		applyRuleValue(&result.Opacity, rule.Opacity)
		applyRuleValue(&result.BlockOutFrom, rule.BlockOutFrom)
		applyRuleValue(&result.MinWidth, rule.MinWidth)
		applyRuleValue(&result.MaxWidth, rule.MaxWidth)
		applyRuleValue(&result.MinHeight, rule.MinHeight)
		applyRuleValue(&result.MaxHeight, rule.MaxHeight)
		applyRuleValue(&result.DefaultColumnDisplay, rule.DefaultColumnDisplay)
		applyRuleValue(&result.DefaultFloatingPosition, rule.DefaultFloatingPosition)
		applyRuleValue(&result.ScrollFactor, rule.ScrollFactor)
		applyRuleValue(&result.VariableRefreshRate, rule.VariableRefreshRate)
		result.Decorations = result.Decorations.merge(rule.Decorations)
		result.BackgroundEffect.Blur = cmp.Or(rule.BackgroundEffect.Blur, result.BackgroundEffect.Blur)
		applyRuleValue(&result.DrawBorderWithBackground, rule.DrawBorderWithBackground)
		applyRuleValue(&result.TiledState, rule.TiledState)
		applyRuleValue(&result.BabaIsFloat, rule.BabaIsFloat)
		result.Popups = result.Popups.merge(rule.Popups)
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
	return blocksCapture(r.BlockOutFrom, target)
}

func blocksCapture(blockOutFrom *string, target CaptureTarget) bool {
	return target != CaptureDisplay && blockOutFrom != nil &&
		(*blockOutFrom == "screen-capture" || target == CaptureCast && *blockOutFrom == "screencast")
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
	if m.IsActive != nil && *m.IsActive != state.IsActive {
		return false
	}
	if m.IsActiveInColumn != nil && *m.IsActiveInColumn != state.IsActiveInColumn {
		return false
	}
	if m.IsFloating != nil && *m.IsFloating != state.IsFloating {
		return false
	}
	if m.IsWindowCastTarget != nil && *m.IsWindowCastTarget != state.IsWindowCastTarget {
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

func (c *Config) ResolveLayerRules(namespace string, state LayerState) LayerRuleResult {
	return ResolveLayerRules(c.LayerRules, namespace, state)
}

func ResolveLayerRules(rules []LayerRule, namespace string, state LayerState) LayerRuleResult {
	var result LayerRuleResult
	for _, rule := range rules {
		if !rule.matches(namespace, state) {
			continue
		}
		if rule.PlaceWithinBackdrop != nil {
			result.PlaceWithinBackdrop = *rule.PlaceWithinBackdrop
		}
		if rule.BabaIsFloat != nil {
			result.BabaIsFloat = *rule.BabaIsFloat
		}
		applyRuleValue(&result.Opacity, rule.Opacity)
		applyRuleValue(&result.BlockOutFrom, rule.BlockOutFrom)
		applyRuleValue(&result.GeometryCornerRadius, rule.GeometryCornerRadius)
		result.Shadow = result.Shadow.merge(rule.Shadow)
		result.BackgroundEffect.Blur = cmp.Or(rule.BackgroundEffect.Blur, result.BackgroundEffect.Blur)
		result.Popups = result.Popups.merge(rule.Popups)
	}
	return result
}

func (r LayerRule) matches(namespace string, state LayerState) bool {
	if len(r.Matches) != 0 {
		matched := false
		for _, ruleMatch := range r.Matches {
			if ruleMatch.matches(namespace, state) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, ruleMatch := range r.Excludes {
		if ruleMatch.matches(namespace, state) {
			return false
		}
	}
	return true
}

func (m LayerMatch) matches(namespace string, state LayerState) bool {
	return (m.Namespace == nil || m.Namespace.MatchString(namespace)) &&
		(m.AtStartup == nil || *m.AtStartup == state.AtStartup) &&
		(m.Layer == nil || *m.Layer == state.Layer)
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
			layout, err := c.workspaceLayout(child, "workspace.layout.")
			if err != nil {
				return err
			}
			workspace.Layout = layout
		default:
			return fmt.Errorf("unexpected workspace child `%s`", nodeName(child))
		}
	}
	c.Workspaces = append(c.Workspaces, workspace)
	return nil
}

// workspaceLayout parses the layout part of a workspace. Niri merges it over
// the layout of the output, so only the keys it sets override the output's.
// prefix names the section in warnings.
func (c *Config) workspaceLayout(node *document.Node, prefix string) (*WorkspaceLayout, error) {
	layout := &WorkspaceLayout{}
	for _, child := range node.Children {
		switch nodeName(child) {
		case "gaps":
			value, err := intArg(child, 0, 1<<16-1)
			if err != nil {
				return nil, fmt.Errorf("gaps: %w", err)
			}
			layout.Gaps = &value
		case "default-column-width":
			for _, width := range child.Children {
				switch nodeName(width) {
				case "proportion":
					value, err := floatArg(width, 0, 100)
					if err != nil {
						return nil, fmt.Errorf("default-column-width: %w", err)
					}
					layout.ColumnWidthProportion = &value
					layout.FixedColumnWidth = nil
				case "fixed":
					value, err := intArg(width, 1, 1<<16-1)
					if err != nil {
						return nil, fmt.Errorf("default-column-width: %w", err)
					}
					layout.FixedColumnWidth = &value
					layout.ColumnWidthProportion = nil
				}
			}
		case "preset-column-widths":
			presets, err := presetSizeList(child)
			if err != nil {
				return nil, fmt.Errorf("preset-column-widths: %w", err)
			}
			layout.PresetColumnWidths = presets
		case "preset-window-heights":
			presets, err := presetSizeList(child)
			if err != nil {
				return nil, fmt.Errorf("preset-window-heights: %w", err)
			}
			layout.PresetWindowHeights = presets
		case "default-column-display":
			display, err := columnDisplayArg(child)
			if err != nil {
				return nil, fmt.Errorf("default-column-display: %w", err)
			}
			layout.DefaultColumnDisplay = &display
		case "center-focused-column":
			mode, err := centerFocusedColumnArg(child)
			if err != nil {
				return nil, fmt.Errorf("center-focused-column: %w", err)
			}
			layout.CenterFocusedColumn = &mode
		case "always-center-single-column":
			on, err := flagArg(child)
			if err != nil {
				return nil, fmt.Errorf("always-center-single-column: %w", err)
			}
			layout.AlwaysCenterSingleColumn = &on
		case "struts":
			struts, err := strutsBlock(child)
			if err != nil {
				return nil, err
			}
			layout.Struts = &struts
		case "focus-ring", "border":
			rule, err := c.borderRule(child, prefix+nodeName(child))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if nodeName(child) == "focus-ring" {
				layout.FocusRing = layout.FocusRing.merge(rule)
			} else {
				layout.Border = layout.Border.merge(rule)
			}
		case "shadow":
			rule, err := c.shadowRule(child)
			if err != nil {
				return nil, fmt.Errorf("shadow: %w", err)
			}
			layout.Shadow = layout.Shadow.merge(rule)
		case "tab-indicator":
			part, err := c.tabIndicatorPart(child, prefix+"tab-indicator", false)
			if err != nil {
				return nil, fmt.Errorf("tab-indicator: %w", err)
			}
			layout.TabIndicator = layout.TabIndicator.merge(part)
		case "background-color":
			color, err := colorArg(child)
			if err != nil {
				return nil, fmt.Errorf("background-color: %w", err)
			}
			layout.BackgroundColor = &color
		default:
			c.addWarning(prefix + nodeName(child))
		}
	}
	return layout, nil
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
		case "scroll-factor":
			var value float64
			value, err = floatArg(child, 0, 100)
			if err == nil {
				rule.ScrollFactor = &value
			}
		case "opacity":
			var value float64
			value, err = numberArg(child)
			if err == nil {
				rule.Opacity = &value
			}
		case "clip-to-geometry", "draw-border-with-background", "tiled-state", "baba-is-float", "variable-refresh-rate":
			var value bool
			value, err = boolArg(child)
			if err == nil {
				switch nodeName(child) {
				case "clip-to-geometry":
					rule.ClipToGeometry = &value
				case "draw-border-with-background":
					rule.DrawBorderWithBackground = &value
				case "tiled-state":
					rule.TiledState = &value
				case "variable-refresh-rate":
					rule.VariableRefreshRate = &value
				default:
					rule.BabaIsFloat = &value
				}
			}
		case "focus-ring", "border":
			var part BorderRule
			part, err = c.borderRule(child, "window-rule."+nodeName(child))
			if err == nil {
				if nodeName(child) == "focus-ring" {
					rule.FocusRing = rule.FocusRing.merge(part)
				} else {
					rule.Border = rule.Border.merge(part)
				}
			}
		case "shadow":
			var part ShadowRule
			part, err = c.shadowRule(child)
			if err == nil {
				rule.Shadow = rule.Shadow.merge(part)
			}
		case "tab-indicator":
			var part TabIndicatorPart
			part, err = c.tabIndicatorPart(child, "window-rule.tab-indicator", true)
			if err == nil {
				rule.TabIndicator = rule.TabIndicator.merge(part)
			}
		case "block-out-from":
			var value string
			value, err = blockOutFromArg(child)
			if err == nil {
				rule.BlockOutFrom = &value
			}
		case "background-effect":
			var part BackgroundEffectRule
			part, err = c.backgroundEffectRule(child, "window-rule.background-effect")
			if err == nil {
				rule.BackgroundEffect.Blur = cmp.Or(part.Blur, rule.BackgroundEffect.Blur)
			}
		case "popups":
			var part PopupsRule
			part, err = c.popupsRule(child, "window-rule.popups")
			if err == nil {
				rule.Popups = rule.Popups.merge(part)
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
		case "is-focused", "is-urgent", "is-active", "is-active-in-column", "is-floating", "is-window-cast-target", "at-startup":
			value, ok := property.Value.(bool)
			if !ok {
				return RuleMatch{}, fmt.Errorf("%s must be boolean", key)
			}
			switch key {
			case "is-focused":
				match.IsFocused = &value
			case "is-urgent":
				match.IsUrgent = &value
			case "is-active":
				match.IsActive = &value
			case "is-active-in-column":
				match.IsActiveInColumn = &value
			case "is-floating":
				match.IsFloating = &value
			case "is-window-cast-target":
				match.IsWindowCastTarget = &value
			case "at-startup":
				match.AtStartup = &value
			}
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
		case "place-within-backdrop", "baba-is-float":
			value, err := boolArg(child)
			if err != nil {
				return fmt.Errorf("%s: %w", nodeName(child), err)
			}
			if nodeName(child) == "place-within-backdrop" {
				rule.PlaceWithinBackdrop = &value
			} else {
				rule.BabaIsFloat = &value
			}
		case "opacity":
			value, err := numberArg(child)
			if err != nil {
				return fmt.Errorf("opacity: %w", err)
			}
			rule.Opacity = &value
		case "block-out-from":
			value, err := blockOutFromArg(child)
			if err != nil {
				return fmt.Errorf("block-out-from: %w", err)
			}
			rule.BlockOutFrom = &value
		case "geometry-corner-radius":
			value, err := parseCornerRadius(child)
			if err != nil {
				return fmt.Errorf("geometry-corner-radius: %w", err)
			}
			rule.GeometryCornerRadius = &value
		case "shadow":
			part, err := c.shadowRule(child)
			if err != nil {
				return fmt.Errorf("shadow: %w", err)
			}
			rule.Shadow = rule.Shadow.merge(part)
		case "background-effect":
			part, err := c.backgroundEffectRule(child, "layer-rule.background-effect")
			if err != nil {
				return fmt.Errorf("background-effect: %w", err)
			}
			rule.BackgroundEffect.Blur = cmp.Or(part.Blur, rule.BackgroundEffect.Blur)
		case "popups":
			part, err := c.popupsRule(child, "layer-rule.popups")
			if err != nil {
				return fmt.Errorf("popups: %w", err)
			}
			rule.Popups = rule.Popups.merge(part)
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
		case "at-startup":
			atStartup, ok := property.Value.(bool)
			if !ok {
				return LayerMatch{}, fmt.Errorf("at-startup must be boolean")
			}
			match.AtStartup = &atStartup
		case "layer":
			layer := LayerName(value)
			if !ok || (layer != LayerBackground && layer != LayerBottom && layer != LayerTop && layer != LayerOverlay) {
				return LayerMatch{}, fmt.Errorf("layer must be \"background\", \"bottom\", \"top\" or \"overlay\"")
			}
			match.Layer = &layer
		default:
			return LayerMatch{}, fmt.Errorf("unexpected match property `%s`", key)
		}
	}
	return match, nil
}

// presetSizeList parses a cycle of preset sizes: fixed pixel sizes and
// proportional shares of the working size. An empty list is valid and means
// Niri's default presets.
func presetSizeList(node *document.Node) ([]PresetSize, error) {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return nil, fmt.Errorf("takes only fixed and proportion children")
	}
	presets := make([]PresetSize, 0, len(node.Children))
	for _, child := range node.Children {
		switch nodeName(child) {
		case "fixed":
			value, err := intArg(child, 1, 1<<31-1)
			if err != nil {
				return nil, err
			}
			presets = append(presets, PresetSize{Fixed: true, Value: float64(value)})
		case "proportion":
			value, err := proportionArg(child)
			if err != nil {
				return nil, err
			}
			presets = append(presets, PresetSize{Value: value})
		default:
			return nil, fmt.Errorf("expected fixed or proportion, got `%s`", nodeName(child))
		}
	}
	return presets, nil
}

// columnDisplayArg reads a column display mode: normal or tabbed.
func columnDisplayArg(node *document.Node) (ColumnDisplay, error) {
	value, err := stringArgs(node, 1, 1)
	if err != nil {
		return "", err
	}
	display := ColumnDisplay(value[0])
	if display != ColumnDisplayNormal && display != ColumnDisplayTabbed {
		return "", fmt.Errorf("expected normal or tabbed")
	}
	return display, nil
}

func centerFocusedColumnArg(node *document.Node) (CenterFocusedColumn, error) {
	value, err := stringArgs(node, 1, 1)
	if err != nil {
		return "", err
	}
	mode := CenterFocusedColumn(value[0])
	if mode != CenterFocusedColumnNever && mode != CenterFocusedColumnAlways &&
		mode != CenterFocusedColumnOnOverflow {
		return "", fmt.Errorf("expected never, always or on-overflow")
	}
	return mode, nil
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

func blockOutFromArg(node *document.Node) (string, error) {
	value, err := stringArgs(node, 1, 1)
	if err != nil {
		return "", err
	}
	if value[0] != "screen-capture" && value[0] != "screencast" {
		return "", fmt.Errorf("expected screen-capture or screencast")
	}
	return value[0], nil
}

// popupsRule parses Niri's popups part of a window or layer rule.
func (c *Config) popupsRule(node *document.Node, prefix string) (PopupsRule, error) {
	var rule PopupsRule
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return rule, fmt.Errorf("popups takes child nodes only")
	}
	for _, child := range node.Children {
		var err error
		switch nodeName(child) {
		case "opacity":
			var value float64
			value, err = numberArg(child)
			rule.Opacity = &value
		case "geometry-corner-radius":
			var value CornerRadius
			value, err = parseCornerRadius(child)
			rule.GeometryCornerRadius = &value
		case "background-effect":
			var part BackgroundEffectRule
			part, err = c.backgroundEffectRule(child, prefix+".background-effect")
			rule.BackgroundEffect.Blur = cmp.Or(part.Blur, rule.BackgroundEffect.Blur)
		default:
			err = fmt.Errorf("unexpected key")
		}
		if err != nil {
			return PopupsRule{}, fmt.Errorf("%s: %w", nodeName(child), err)
		}
	}
	return rule, nil
}

// backgroundEffectRule parses Niri's background-effect part. xray, noise and
// saturation are checked and warn: SceneFX's blur takes noise and
// saturation for the whole scene, and xray needs an optimized blur pass.
func (c *Config) backgroundEffectRule(node *document.Node, prefix string) (BackgroundEffectRule, error) {
	var rule BackgroundEffectRule
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return rule, fmt.Errorf("background-effect takes child nodes only")
	}
	for _, child := range node.Children {
		switch name := nodeName(child); name {
		case "blur", "xray":
			value, err := boolArg(child)
			if err != nil {
				return rule, fmt.Errorf("%s: %w", name, err)
			}
			if name == "blur" {
				rule.Blur = &value
			} else {
				c.addWarning(prefix + ".xray")
			}
		case "noise", "saturation":
			if _, err := boundedNumber(child, 0, 1000); err != nil {
				return rule, err
			}
			c.addWarning(prefix + "." + name)
		default:
			return rule, fmt.Errorf("unexpected node `%s`", name)
		}
	}
	return rule, nil
}

// WorkspaceKey folds a workspace name the way names compare: ASCII letters
// ignore case, as in Niri.
func WorkspaceKey(name string) string {
	key := []byte(name)
	for i, b := range key {
		if b >= 'A' && b <= 'Z' {
			key[i] = b + 'a' - 'A'
		}
	}
	return string(key)
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
