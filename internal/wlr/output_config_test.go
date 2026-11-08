package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func outputSnapshotNamed(c *Compositor, name string) (OutputSnapshot, bool) {
	for _, output := range c.OutputSnapshot() {
		if output.Name == name {
			return output, true
		}
	}
	return OutputSnapshot{}, false
}

// TestOutputConfigAppliesEachOutputsSection: an output section sets the mode,
// scale, transform and position, as Niri's reload_output_config does. A
// custom mode reaches an output that lists none, VRR an output cannot do is
// dropped while the rest applies, a missing mode falls back to the preferred
// one, and a position that overlaps another output is placed automatically.
func TestOutputConfigAppliesEachOutputsSection(t *testing.T) {
	c := newNested(t)
	outputs := c.OutputSnapshot()
	if len(outputs) != 1 || outputs[0].Logical == nil {
		t.Fatalf("outputs = %+v, want one enabled output", outputs)
	}
	primary := outputs[0]
	// Configure a listed mode that is not the current one, so the test does
	// not depend on one backend's mode table.
	var target *OutputModeSnapshot
	for i := range primary.Modes {
		if primary.CurrentMode == nil || uint32(i) != *primary.CurrentMode {
			target = &primary.Modes[i]
			break
		}
	}
	two := 2.0
	sections := func(side config.OutputPosition, primaryMode *config.OutputMode) []config.OutputConfig {
		return []config.OutputConfig{
			{Name: primary.Name, Mode: primaryMode, Position: &config.OutputPosition{}},
			{Name: "HEADLESS-1", Mode: &config.OutputMode{Width: 1280, Height: 720, Refresh: 60, Custom: true},
				Scale: &two, Transform: config.Transform90, Position: &side, VRR: &config.VRR{}},
		}
	}
	var primaryMode *config.OutputMode
	if target != nil {
		primaryMode = &config.OutputMode{Width: int(target.Width), Height: int(target.Height), Refresh: float64(target.RefreshRate) / 1000}
	}
	if err := c.Reconfigure(Options{Outputs: sections(config.OutputPosition{X: 5000, Y: 50}, primaryMode)}); err != nil {
		t.Fatal(err)
	}
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	primary, _ = outputSnapshotNamed(c, primary.Name)
	if target != nil {
		if primary.CurrentMode == nil || primary.Modes[*primary.CurrentMode] != *target {
			t.Errorf("%s current mode = %v, want the configured %+v", primary.Name, primary.CurrentMode, *target)
		}
	}
	if primary.Logical == nil || primary.Logical.X != 0 || primary.Logical.Y != 0 {
		t.Errorf("%s logical = %+v, want it at 0,0", primary.Name, primary.Logical)
	}
	side, ok := outputSnapshotNamed(c, "HEADLESS-1")
	if !ok || side.Logical == nil {
		t.Fatalf("HEADLESS-1 = %+v, want an enabled output", side)
	}
	// 1280x720 turned 90 degrees at scale 2.
	if got := *side.Logical; got != (LogicalOutputSnapshot{X: 5000, Y: 50, Width: 360, Height: 640, Scale: 2, Transform: uint32(config.Transform90)}) {
		t.Errorf("HEADLESS-1 logical = %+v, want 360x640 at 5000,50, scale 2, turned 90", got)
	}
	if side.VRREnabled {
		t.Error("HEADLESS-1 has VRR on, which headless outputs cannot do")
	}

	// An overlapping position falls back to the right of the placed outputs;
	// a mode the output lacks falls back to its preferred mode.
	if err := c.Reconfigure(Options{Outputs: sections(config.OutputPosition{X: 10, Y: 10},
		&config.OutputMode{Width: 123, Height: 45})}); err != nil {
		t.Fatal(err)
	}
	primary, _ = outputSnapshotNamed(c, primary.Name)
	side, _ = outputSnapshotNamed(c, "HEADLESS-1")
	if side.Logical == nil || primary.Logical == nil || side.Logical.X != int32(primary.Logical.Width) || side.Logical.Y != 0 {
		t.Errorf("HEADLESS-1 logical = %+v next to %+v, want it placed right of %s", side.Logical, primary.Logical, primary.Name)
	}
	if index, _ := pickOutputMode(outputModeChoicesOf(primary), nil); index >= 0 &&
		(primary.CurrentMode == nil || int(*primary.CurrentMode) != index) {
		t.Errorf("%s current mode = %v, want the preferred mode %d after a missing one", primary.Name, primary.CurrentMode, index)
	}
}

func outputModeChoicesOf(output OutputSnapshot) []outputModeChoice {
	modes := make([]outputModeChoice, len(output.Modes))
	for i, mode := range output.Modes {
		modes[i] = outputModeChoice{int(mode.Width), int(mode.Height), int(mode.RefreshRate), mode.Preferred}
	}
	return modes
}

// TestOutputConfigOffTakesTheOutputAwayAndBack: off takes the output out of
// the layout as an unplug does, its workspace moves away and comes back when
// the output is turned on again, as Niri's do; the output stays listed with no
// logical box. focus-at-startup picks the focused output.
func TestOutputConfigOffTakesTheOutputAwayAndBack(t *testing.T) {
	c := newNested(t)
	workspaces := []wm.WorkspaceConfig{{Name: "side", OpenOnOutput: "HEADLESS-1"}}
	on := []config.OutputConfig{{Name: "HEADLESS-1", FocusAtStartup: true}}
	if err := c.Reconfigure(Options{Workspaces: workspaces, Outputs: on}); err != nil {
		t.Fatal(err)
	}
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	c.focusDefaultOutput()
	if got := c.FocusedOutputName(); got != "HEADLESS-1" {
		t.Errorf("focused output = %q, want the focus-at-startup HEADLESS-1", got)
	}
	sideOutput := func() string {
		for _, workspace := range c.WorkspaceSnapshot().Workspaces {
			if workspace.Name != nil && *workspace.Name == "side" {
				return workspace.Output
			}
		}
		return ""
	}
	if got := sideOutput(); got != "HEADLESS-1" {
		t.Fatalf("workspace side is on %q, want HEADLESS-1", got)
	}
	enabled := c.EnabledOutputs()

	if err := c.Reconfigure(Options{Workspaces: workspaces,
		Outputs: []config.OutputConfig{{Name: "HEADLESS-1", Off: true}}}); err != nil {
		t.Fatal(err)
	}
	if got := c.EnabledOutputs(); got != enabled-1 {
		t.Errorf("enabled outputs = %d after off, want %d", got, enabled-1)
	}
	if side, ok := outputSnapshotNamed(c, "HEADLESS-1"); !ok || side.Logical != nil || side.CurrentMode != nil {
		t.Errorf("HEADLESS-1 after off = %+v (listed %t), want it listed with no logical box or mode", side, ok)
	}
	if got := sideOutput(); got == "HEADLESS-1" || got == "" {
		t.Errorf("workspace side is on %q after off, want it moved to the other output", got)
	}

	if err := c.Reconfigure(Options{Workspaces: workspaces, Outputs: on}); err != nil {
		t.Fatal(err)
	}
	if side, ok := outputSnapshotNamed(c, "HEADLESS-1"); !ok || side.Logical == nil {
		t.Errorf("HEADLESS-1 after on = %+v, want it back in the layout", side)
	}
	if got := sideOutput(); got != "HEADLESS-1" {
		t.Errorf("workspace side is on %q after on, want it back on HEADLESS-1", got)
	}
}
