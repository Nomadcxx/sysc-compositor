package wm

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"math"
)

func (m *Model) SetOutputLayout(name string, override *WorkspaceLayout) {
	if output := m.outputs[name]; output != nil {
		output.layout = override
		for _, workspace := range output.workspaces {
			m.applyWorkspaceLayout(output, workspace)
		}
	}
}

func (base *WorkspaceLayout) merged(over *WorkspaceLayout) *WorkspaceLayout {
	if base == nil {
		return over
	}
	if over == nil {
		return base
	}
	result := *base
	if over.Gaps != nil {
		result.Gaps = over.Gaps
	}
	if over.FixedColumnWidth != nil || over.ColumnWidthProportion != nil {
		result.FixedColumnWidth, result.ColumnWidthProportion = over.FixedColumnWidth, over.ColumnWidthProportion
	}
	if over.PresetColumnWidths != nil {
		result.PresetColumnWidths = over.PresetColumnWidths
	}
	if over.PresetWindowHeights != nil {
		result.PresetWindowHeights = over.PresetWindowHeights
	}
	if over.DefaultColumnDisplay != nil {
		result.DefaultColumnDisplay = over.DefaultColumnDisplay
	}
	if over.CenterFocusedColumn != nil {
		result.CenterFocusedColumn = over.CenterFocusedColumn
	}
	if over.AlwaysCenterSingleColumn != nil {
		result.AlwaysCenterSingleColumn = over.AlwaysCenterSingleColumn
	}
	if over.Struts != nil {
		result.Struts = over.Struts
	}
	if over.TabSpace != nil {
		result.TabSpace = over.TabSpace
	}
	if base.ConfigTabIndicator != nil && over.ConfigTabIndicator != nil {
		effective := base.ConfigTabIndicator.Merged(over.TabIndicatorPart)
		space := TabSpaceFromConfig(effective)
		result.TabSpace = &space
		result.ConfigTabIndicator = &effective
	}
	return &result
}

func TabSpaceFromConfig(t config.TabIndicator) layout.TabSpace {
	if t.Off || !t.PlaceWithinColumn {
		return layout.TabSpace{}
	}
	side := map[config.TabIndicatorPosition]layout.TabSide{
		config.TabIndicatorLeft: layout.TabLeft, config.TabIndicatorRight: layout.TabRight,
		config.TabIndicatorTop: layout.TabTop, config.TabIndicatorBottom: layout.TabBottom,
	}[t.Position]
	return layout.TabSpace{Size: max(0, int(math.Round(t.Width))+int(math.Round(t.Gap))),
		Side: side, HideWhenSingle: t.HideWhenSingleTab}
}
