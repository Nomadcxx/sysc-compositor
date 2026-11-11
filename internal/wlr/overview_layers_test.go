package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"testing"
)

func TestOverviewProjectsBackgroundLayersAndBackdropRules(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	checked := false
	pollThenStop(c, func() bool {
		var background *layerSurface
		for _, layer := range c.layerOrder {
			if _, _, mapped := layer.mappedSize(); mapped && layer.layer == layerBackground {
				background = layer
				break
			}
		}
		if background == nil {
			return false
		}
		c.OpenOverview()
		frame, ok := c.model.OverviewFrame(background.outputName, c.model.OverviewZoom(c.overviewConfig.Zoom))
		if !ok {
			t.Error("output missing")
			return true
		}
		count := func() int {
			n := 0
			for _, projection := range c.overviewProjections {
				if projection.layer == background {
					n++
				}
			}
			return n
		}
		if got := count(); got != len(frame.Workspaces) {
			t.Errorf("background copies=%d want=%d", got, len(frame.Workspaces))
		}
		background.rules.PlaceWithinBackdrop = true
		c.syncOverviewScene()
		if !background.withinBackdrop() || count() != 1 {
			t.Error("eligible backdrop layer was duplicated into workspaces")
		}
		background.layer = layerBottom
		c.syncOverviewScene()
		if background.withinBackdrop() || count() != len(frame.Workspaces) {
			t.Error("bottom layer incorrectly honored background-only backdrop rule")
		}
		checked = true
		return true
	})
	runClient(t, c, []string{"swaybg", "-c", "#00ff00"}, nil, nil)
	if !checked {
		t.Fatal("background layer was never projected")
	}
}
