package wlr

import (
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestOutputLayoutAndBackdropReachTheirOutput(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	gaps, width := 20, 400
	red := config.Color{R: 1, A: 1}
	if err := c.Reconfigure(Options{Animations: config.Animations{Off: true}, Outputs: []config.OutputConfig{{Name: name, Layout: &config.WorkspaceLayout{Gaps: &gaps, FixedColumnWidth: &width}, BackdropColor: &red}}, Workspaces: []wm.WorkspaceConfig{{Name: "side", OpenOnOutput: name}}}); err != nil {
		t.Fatal(err)
	}
	id, ok := c.model.WorkspaceIDByName("side")
	if !ok {
		t.Fatal("workspace missing")
	}
	c.model.MapWindowToWorkspace(42, id)
	for _, window := range c.model.Snapshot().Windows {
		if window.ID == 42 && window.Tile.Width != width {
			t.Fatalf("output width ignored: %+v", window.Tile)
		}
	}
	c.model.DestroyWindow(42)
	c.OpenOverview()
	found := false
	for _, backdrop := range c.overviewBackdrops {
		if backdrop.output == name {
			found = true
			if backdrop.Color != red {
				t.Fatalf("backdrop = %+v", backdrop.Color)
			}
		}
	}
	if !found {
		t.Fatal("output backdrop missing")
	}
}

func TestOutputOnDemandVRRFollowsVisibleSurfaces(t *testing.T) {
	c := newNested(t)
	var output unsafe.Pointer
	for pointer := range c.sceneOutputs {
		output = pointer
		break
	}
	on := true
	if err := c.Reconfigure(Options{Animations: config.Animations{Off: true}, Outputs: []config.OutputConfig{{Name: outputName(output), VRR: &config.VRR{OnDemand: true}}}, WindowRules: []config.WindowRule{{VariableRefreshRate: &on}}}); err != nil {
		t.Fatal(err)
	}
	if c.outputWantsVRR(output) {
		t.Fatal("empty output wants VRR")
	}
	c.viewMapped = func(view *toplevelView) {
		time.AfterFunc(200*time.Millisecond, func() {
			_ = c.Post(func() {
				if !c.outputWantsVRR(output) {
					t.Error("visible window did not request VRR")
				}
				c.model.FocusWorkspaceDown()
				c.applyWorkspaceModel()
				if c.outputWantsVRR(output) {
					t.Error("hidden window still requests VRR")
				}
				c.Terminate()
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}
