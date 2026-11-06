package wlr

import (
	"cmp"
	"image"
	"math"
	"slices"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// tabIndicatorHit is one drawn tab: a click inside it activates its window.
type tabIndicatorHit struct {
	rect image.Rectangle
	id   uint64
}

// tabIndicatorRects places a column's tabs beside area, Niri's tab_rects at
// scale one with the open animation finished.
func tabIndicatorRects(t config.TabIndicator, area image.Rectangle, count int) []image.Rectangle {
	if count <= 0 {
		return nil
	}
	width := float64(decorationWidth(t.Width))
	gap := math.Copysign(float64(decorationWidth(math.Abs(t.Gap))), t.Gap)
	gapsBetween := float64(decorationWidth(t.GapsBetweenTabs))
	vertical := t.Position == config.TabIndicatorLeft || t.Position == config.TabIndicatorRight
	side := float64(area.Dx())
	if vertical {
		side = float64(area.Dy())
	}
	minLength := math.Round(side * min(max(t.Length, 0), 2))
	n := float64(count)
	length := max(minLength, n*(1+gapsBetween)-gapsBetween)
	pxPerTab := (length+gapsBetween)/n - gapsBetween
	gapsBetween = math.Round(t.GapsBetweenTabs)
	length = n*(pxPerTab+gapsBetween) - gapsBetween
	pxPerTab = max(1, math.Floor(pxPerTab))
	onesLeft := int(math.Round(length - (n*(pxPerTab+gapsBetween) - gapsBetween)))

	x, y := -gap-width, math.Round((side-length)/2)
	switch t.Position {
	case config.TabIndicatorRight:
		x = float64(area.Dx()) + gap
	case config.TabIndicatorTop:
		x, y = y, x
	case config.TabIndicatorBottom:
		x, y = y, float64(area.Dy())+gap
	}
	x += float64(area.Min.X)
	y += float64(area.Min.Y)

	rects := make([]image.Rectangle, 0, count)
	for range count {
		size := pxPerTab
		if onesLeft > 0 {
			onesLeft--
			size++
		}
		w, h := width, size
		if !vertical {
			w, h = size, width
		}
		rects = append(rects, image.Rect(int(x), int(y), int(x+w), int(y+h)))
		if vertical {
			y += size + gapsBetween
		} else {
			x += size + gapsBetween
		}
	}
	return rects
}

// tabIndicatorRadii rounds the tabs' corners; with no gaps between them the
// tabs share one rounded outline, as in Niri.
func tabIndicatorRadii(t config.TabIndicator, index, count int) config.CornerRadius {
	r := t.CornerRadius
	if t.GapsBetweenTabs != 0 || count == 1 {
		return config.CornerRadius{r, r, r, r}
	}
	vertical := t.Position == config.TabIndicatorLeft || t.Position == config.TabIndicatorRight
	switch {
	case index == 0 && vertical:
		return config.CornerRadius{r, r, 0, 0}
	case index == 0:
		return config.CornerRadius{r, 0, 0, r}
	case index == count-1 && vertical:
		return config.CornerRadius{0, 0, r, r}
	case index == count-1:
		return config.CornerRadius{0, r, r, 0}
	}
	return config.CornerRadius{}
}

// tabColor is Niri's TabInfo colour: the window rule's, then the tab
// indicator's, then the focus ring's, or the border's when the ring is off.
func tabColor(view *toplevelView, t config.TabIndicator, active bool) config.Color {
	rule := view.appearance.TabIndicator
	fallback := view.decorations.ring
	if fallback.Off {
		fallback = view.decorations.border
	}
	pick := func(urgentColor, activeColor, inactiveColor *config.Color) *config.Color {
		switch {
		case view.urgent:
			return urgentColor
		case active:
			return activeColor
		}
		return inactiveColor
	}
	if c := cmp.Or(pick(rule.UrgentColor, rule.ActiveColor, rule.InactiveColor),
		pick(t.UrgentColor, t.ActiveColor, t.InactiveColor)); c != nil {
		return *c
	}
	return decorationColor(fallback, active, view.urgent)
}

// syncTabIndicators draws the tab indicator of every visible tabbed column.
func (c *Compositor) syncTabIndicators() {
	if c.model == nil || c.tabIndicatorTree == nil {
		return
	}
	snapshot := c.model.Snapshot()
	names := workspaceNamesByID(snapshot.Workspaces)
	views := make(map[uint64]*toplevelView, len(c.views))
	for _, view := range c.views {
		if view != nil {
			views[view.id] = view
		}
	}
	type columnKey struct {
		workspace uint64
		column    uint32
	}
	columns := make(map[columnKey][]wm.WindowSnapshot)
	var order []columnKey
	for _, window := range snapshot.Windows {
		if window.Tile.Display != layout.ColumnDisplayTabbed || window.Tile.Floating {
			continue
		}
		key := columnKey{window.WorkspaceID, window.Column}
		if columns[key] == nil {
			order = append(order, key)
		}
		columns[key] = append(columns[key], window)
	}

	c.tabIndicatorHits = c.tabIndicatorHits[:0]
	used := 0
	for _, key := range order {
		tabs := columns[key]
		slices.SortFunc(tabs, func(a, b wm.WindowSnapshot) int { return cmp.Compare(a.Tile.Tile, b.Tile.Tile) })
		var activeView *toplevelView
		for _, tab := range tabs {
			if tab.Tile.Active {
				activeView = views[tab.ID]
			}
		}
		// Niri hides the indicator unless the column is in its normal sizing.
		if activeView == nil || !activeView.mapped || !activeView.presentationVisible ||
			activeView.fullscreen || activeView.maximized {
			continue
		}
		var workspace config.Decorations
		if name := names[key.workspace]; name != nil {
			workspace = c.layoutOptions.WorkspaceDecorations[config.WorkspaceKey(*name)]
		}
		indicator := c.layoutOptions.TabIndicator.Merged(workspace.TabIndicator)
		if indicator.Off || indicator.HideWhenSingleTab && len(tabs) == 1 {
			continue
		}
		// The tabs sit beside the active tile: the window and its border.
		x, y := activeView.treePosition()
		border := activeView.borderWidth()
		geometry := activeView.geometry()
		area := image.Rect(x-border, y-border, x+geometry.w+border, y+geometry.h+border)
		columnActive := activeView.workspaceActive && c.activeOutputName == c.outputNameForView(activeView)
		for i, rect := range tabIndicatorRects(indicator, area, len(tabs)) {
			view := views[tabs[i].ID]
			if view == nil {
				continue
			}
			rgba := premultipliedColor(tabColor(view, indicator, tabs[i].Tile.Active))
			if !columnActive {
				for channel := range rgba {
					rgba[channel] *= 0.5
				}
			}
			if used == len(c.tabIndicatorNodes) {
				node := sceneFXCreateRect(unsafe.Pointer(c.tabIndicatorTree), 1, 1, [4]float32{})
				if node == nil {
					return
				}
				c.tabIndicatorNodes = append(c.tabIndicatorNodes, node)
			}
			node := c.tabIndicatorNodes[used]
			used++
			radii := fitCornerRadii(tabIndicatorRadii(indicator, i, len(tabs)), float64(rect.Dx()), float64(rect.Dy()))
			sceneFXRectUpdateFill(node, rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), sceneFXRadii(radii), rgba)
			sceneFXSetNodeEnabled(node, true)
			c.tabIndicatorHits = append(c.tabIndicatorHits, tabIndicatorHit{rect: rect, id: view.id})
		}
	}
	for _, node := range c.tabIndicatorNodes[used:] {
		sceneFXSetNodeEnabled(node, false)
	}
}

// tabIndicatorButton activates the window whose tab is under the point.
func (c *Compositor) tabIndicatorButton(x, y float64) bool {
	point := image.Pt(int(math.Floor(x)), int(math.Floor(y)))
	for _, hit := range c.tabIndicatorHits {
		if point.In(hit.rect) {
			c.FocusWindow(hit.id)
			return true
		}
	}
	return false
}
