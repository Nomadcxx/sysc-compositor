package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// LayoutFromConfig converts global layout values at the config/runtime boundary.
func LayoutFromConfig(cfg *config.Config) *LayoutOptions {
	return &LayoutOptions{
		Gaps: cfg.Layout.Gaps, ColumnWidthProportion: cfg.Layout.ColumnWidthProportion,
		FixedColumnWidth:         cfg.Layout.FixedColumnWidth,
		PresetColumnWidths:       presetSizes(cfg.Layout.PresetColumnWidths),
		PresetWindowHeights:      presetSizes(cfg.Layout.PresetWindowHeights),
		DefaultColumnDisplay:     configColumnDisplay(cfg.Layout.DefaultColumnDisplay),
		CenterFocusedColumn:      centerFocusedColumn(cfg.Layout.CenterFocusedColumn),
		AlwaysCenterSingleColumn: cfg.Layout.AlwaysCenterSingleColumn,
		EmptyWorkspaceAboveFirst: cfg.Layout.EmptyWorkspaceAboveFirst,
		Struts:                   struts(cfg.Layout.Struts),
		InsertHint:               cfg.Layout.InsertHint,
		FocusRing:                cfg.Layout.FocusRing, Border: cfg.Layout.Border, Shadow: cfg.Layout.Shadow,
		WorkspaceDecorations: workspaceDecorations(cfg.Workspaces),
		TabIndicator:         cfg.Layout.TabIndicator, TabSpace: wm.TabSpaceFromConfig(cfg.Layout.TabIndicator),
		BackgroundColor:           cfg.Layout.BackgroundColor,
		WorkspaceBackgroundColors: workspaceBackgroundColors(cfg.Workspaces),
	}
}

// workspaceLayout converts the layout override of a workspace for the model.
func WorkspaceLayoutFromConfig(layout *config.WorkspaceLayout, tabIndicator config.TabIndicator) *wm.WorkspaceLayout {
	if layout == nil {
		return nil
	}
	effective := tabIndicator.Merged(layout.TabIndicator)
	return &wm.WorkspaceLayout{ConfigTabIndicator: &effective, TabIndicatorPart: layout.TabIndicator,
		TabSpace: workspaceTabSpace(layout.TabIndicator, tabIndicator),
		Gaps:     layout.Gaps, ColumnWidthProportion: layout.ColumnWidthProportion,
		FixedColumnWidth:         layout.FixedColumnWidth,
		PresetColumnWidths:       presetSizes(layout.PresetColumnWidths),
		PresetWindowHeights:      presetSizes(layout.PresetWindowHeights),
		DefaultColumnDisplay:     columnDisplayPtr(layout.DefaultColumnDisplay),
		CenterFocusedColumn:      centerFocusedColumnPtr(layout.CenterFocusedColumn),
		AlwaysCenterSingleColumn: layout.AlwaysCenterSingleColumn,
		Struts:                   strutsPtr(layout.Struts),
	}
}

// workspaceDecorations keys each named workspace's decoration parts by its
// folded name.
func workspaceDecorations(workspaces []config.WorkspaceConfig) map[string]config.Decorations {
	parts := make(map[string]config.Decorations)
	for _, workspace := range workspaces {
		if workspace.Layout != nil {
			parts[config.WorkspaceKey(workspace.Name)] = workspace.Layout.Decorations
		}
	}
	return parts
}

// workspaceBackgroundColors keys the background-color of each named workspace
// that sets one by its folded name.
func workspaceBackgroundColors(workspaces []config.WorkspaceConfig) map[string]config.Color {
	colors := make(map[string]config.Color)
	for _, workspace := range workspaces {
		if workspace.Layout != nil && workspace.Layout.BackgroundColor != nil {
			colors[config.WorkspaceKey(workspace.Name)] = *workspace.Layout.BackgroundColor
		}
	}
	return colors
}

// presetSizes converts config preset sizes for the layout. A nil list stays
// nil so the model knows the workspace keeps the config's list.
func presetSizes(presets []config.PresetSize) []layout.PresetSize {
	if presets == nil {
		return nil
	}
	sizes := make([]layout.PresetSize, 0, len(presets))
	for _, preset := range presets {
		sizes = append(sizes, layout.PresetSize{Fixed: preset.Fixed, Value: preset.Value})
	}
	return sizes
}

// columnDisplay converts a config column display mode for the layout.
func configColumnDisplay(display config.ColumnDisplay) layout.ColumnDisplay {
	if display == config.ColumnDisplayTabbed {
		return layout.ColumnDisplayTabbed
	}
	return layout.ColumnDisplayNormal
}

// columnDisplayPtr converts an optional config column display mode for the
// layout, keeping nil for a workspace that keeps the config's mode.
func columnDisplayPtr(display *config.ColumnDisplay) *layout.ColumnDisplay {
	if display == nil {
		return nil
	}
	value := configColumnDisplay(*display)
	return &value
}

// centerFocusedColumn converts a config centering policy for the layout.
func centerFocusedColumn(mode config.CenterFocusedColumn) layout.CenterFocusedColumn {
	switch mode {
	case config.CenterFocusedColumnAlways:
		return layout.CenterFocusedColumnAlways
	case config.CenterFocusedColumnOnOverflow:
		return layout.CenterFocusedColumnOnOverflow
	}
	return layout.CenterFocusedColumnNever
}

// centerFocusedColumnPtr converts an optional config centering policy for the
// layout, keeping nil for a workspace that keeps the config's policy.
func centerFocusedColumnPtr(mode *config.CenterFocusedColumn) *layout.CenterFocusedColumn {
	if mode == nil {
		return nil
	}
	value := centerFocusedColumn(*mode)
	return &value
}

// struts converts the config's reserved layout edges for the layout.
func struts(s config.Struts) layout.Struts {
	return layout.Struts{
		Left:   s.Left,
		Right:  s.Right,
		Top:    s.Top,
		Bottom: s.Bottom,
	}
}

// strutsPtr converts optional config struts for the layout, keeping nil for a
// workspace that keeps the config's struts.
// tabSpace is the room a tabbed column keeps for its tab indicator, Niri's
// extra_size: the width and the gap, when the indicator sits within it.
// workspaceTabSpace is a workspace's own tab space, nil when its layout
// leaves the tab indicator alone.
func workspaceTabSpace(part config.TabIndicatorPart, tabIndicator config.TabIndicator) *layout.TabSpace {
	if part == (config.TabIndicatorPart{}) {
		return nil
	}
	space := wm.TabSpaceFromConfig(tabIndicator.Merged(part))
	return &space
}

func strutsPtr(s *config.Struts) *layout.Struts {
	if s == nil {
		return nil
	}
	value := struts(*s)
	return &value
}
