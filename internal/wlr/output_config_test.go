package wlr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestOutputRequestsAreTransientAndValidateBeforeApplying(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	change := func(name, action string) (string, error) {
		return c.ConfigureOutput(ipc.OutputRequest{Name: name, Action: json.RawMessage(action)})
	}
	for _, action := range []string{`{"CustomMode":{"mode":{"width":1280,"height":720,"refresh":60}}}`, `{"Scale":{"scale":{"Specific":2}}}`, `{"Transform":{"transform":"90"}}`, `{"Position":{"position":{"Specific":{"x":5000,"y":50}}}}`} {
		if result, err := change(name, action); err != nil || result != "Applied" {
			t.Fatalf("%s: %s, %v", action, result, err)
		}
	}
	before, _ := outputSnapshotNamed(c, name)
	if before.Logical == nil || before.Logical.Scale != 2 || before.Logical.Transform != 1 || before.Logical.X != 5000 || before.Logical.Y != 50 || before.Logical.Width != 360 {
		t.Fatalf("output = %+v", before)
	}
	if _, err := change(name, `{"Mode":{"mode":{"Specific":{"width":65535,"height":65535,"refresh":60}}}}`); err == nil {
		t.Fatal("unsupported mode succeeded")
	}
	after, _ := outputSnapshotNamed(c, name)
	if after.Logical == nil || *after.Logical != *before.Logical {
		t.Fatalf("rejected change moved output: %+v", after)
	}
	if _, err := change(name, `"Off"`); err != nil {
		t.Fatal(err)
	}
	if output, _ := outputSnapshotNamed(c, name); output.Logical != nil {
		t.Fatal("off output still has a logical box")
	}
	if _, err := change(name, `"On"`); err != nil {
		t.Fatal(err)
	}
	if result, err := change("missing-connector", `"Off"`); err != nil || result != "OutputWasMissing" {
		t.Fatalf("missing output: %s, %v", result, err)
	}
	if err := c.Reconfigure(Options{}); err != nil {
		t.Fatal(err)
	}
	after, _ = outputSnapshotNamed(c, name)
	if after.Logical == nil || after.Logical.Scale != 1 || after.Logical.Transform != 0 || after.Logical.X == 5000 {
		t.Fatalf("reload kept transient change: %+v", after)
	}
}

func TestOutputRequestUsesADisconnectedOutputsIdentity(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	if err := c.Reconfigure(Options{Outputs: []config.OutputConfig{{Name: name}}}); err != nil {
		t.Fatal(err)
	}
	// Headless outputs have no EDID; model the cached identity of a physical panel.
	identity := c.knownOutputs[name]
	identity.Make, identity.Model, identity.Serial = "Acme", "Panel", "42"
	c.knownOutputs[name] = identity
	parts := []string{identity.Make, identity.Model, identity.Serial}
	for i := range parts {
		if parts[i] == "" {
			parts[i] = "Unknown"
		}
	}
	for pointer := range c.backendOutputs {
		if outputName(pointer) == name {
			destroyOutput(pointer)
			break
		}
	}
	result, err := c.ConfigureOutput(ipc.OutputRequest{Name: strings.Join(parts, " "), Action: json.RawMessage(`"Off"`)})
	if err != nil || result != "OutputWasMissing" || len(c.outputConfigs) != 1 || !c.outputConfigs[0].Off {
		t.Fatalf("disconnected edit: result=%s err=%v configs=%+v", result, err, c.outputConfigs)
	}
}

func TestOutputModelineAppliesOnNativeAndHeadlessOutputs(t *testing.T) {
	c := newNested(t)
	headless := addConfigurableTestOutput(t, c)
	names := []string{headless}
	for pointer := range c.backendOutputs {
		if outputName(pointer) != headless {
			names = append(names, outputName(pointer))
		}
	}
	for _, name := range names {
		_, err := c.ConfigureOutput(ipc.OutputRequest{Name: name, Action: json.RawMessage(`{"Modeline":{"clock":148.5,"hdisplay":1920,"hsync_start":2008,"hsync_end":2052,"htotal":2200,"vdisplay":1080,"vsync_start":1084,"vsync_end":1089,"vtotal":1125,"hsync_polarity":"PHSync","vsync_polarity":"PVSync"}}`)})
		if err != nil {
			t.Fatal(err)
		}
		output, _ := outputSnapshotNamed(c, name)
		if output.CurrentMode == nil || output.Modes[*output.CurrentMode].Width != 1920 || output.Modes[*output.CurrentMode].Height != 1080 || output.Modes[*output.CurrentMode].RefreshRate != 60000 {
			t.Fatalf("%s modeline mode = %+v", name, output)
		}
	}
}

// addConfigurableTestOutput adds a headless output and returns its name:
// wlroots numbers headless outputs across the whole test process.
func addConfigurableTestOutput(t *testing.T, c *Compositor) string {
	t.Helper()
	before := map[string]bool{}
	for _, output := range c.OutputSnapshot() {
		before[output.Name] = true
	}
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	for _, output := range c.OutputSnapshot() {
		if !before[output.Name] {
			return output.Name
		}
	}
	t.Fatal("the headless output did not appear")
	return ""
}

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
	name := addConfigurableTestOutput(t, c)
	sections := func(side config.OutputPosition, primaryMode *config.OutputMode) []config.OutputConfig {
		return []config.OutputConfig{
			{Name: primary.Name, Mode: primaryMode, Position: &config.OutputPosition{}},
			{Name: name, Mode: &config.OutputMode{Width: 1280, Height: 720, Refresh: 60, Custom: true},
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

	primary, _ = outputSnapshotNamed(c, primary.Name)
	if target != nil {
		if primary.CurrentMode == nil || primary.Modes[*primary.CurrentMode] != *target {
			t.Errorf("%s current mode = %v, want the configured %+v", primary.Name, primary.CurrentMode, *target)
		}
	}
	if primary.Logical == nil || primary.Logical.X != 0 || primary.Logical.Y != 0 {
		t.Errorf("%s logical = %+v, want it at 0,0", primary.Name, primary.Logical)
	}
	side, ok := outputSnapshotNamed(c, name)
	if !ok || side.Logical == nil {
		t.Fatalf("%s = %+v, want an enabled output", name, side)
	}
	// 1280x720 turned 90 degrees at scale 2.
	if got := *side.Logical; got != (LogicalOutputSnapshot{X: 5000, Y: 50, Width: 360, Height: 640, Scale: 2, Transform: uint32(config.Transform90)}) {
		t.Errorf("%s logical = %+v, want 360x640 at 5000,50, scale 2, turned 90", name, got)
	}
	if side.VRREnabled {
		t.Errorf("%s has VRR on, which headless outputs cannot do", name)
	}

	// Configured outputs are placed in name order, and the headless output
	// (it has a make) comes first, so the primary output's 0,0 now overlaps it
	// and the primary goes right of it. A mode the output lacks falls back to
	// its preferred mode.
	if err := c.Reconfigure(Options{Outputs: sections(config.OutputPosition{X: 10, Y: 10},
		&config.OutputMode{Width: 123, Height: 45})}); err != nil {
		t.Fatal(err)
	}
	primary, _ = outputSnapshotNamed(c, primary.Name)
	side, _ = outputSnapshotNamed(c, name)
	if side.Logical == nil || primary.Logical == nil || side.Logical.X != 10 || side.Logical.Y != 10 ||
		primary.Logical.X != 10+int32(side.Logical.Width) || primary.Logical.Y != 0 {
		t.Errorf("%s logical = %+v, %s logical = %+v; want %s at 10,10 and %s right of it",
			name, side.Logical, primary.Name, primary.Logical, name, primary.Name)
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
// the layout as an unplug does, and its named workspace comes back when the
// output is turned on again; the output stays listed with no logical box, as
// in Niri. focus-at-startup picks the focused output.
func TestOutputConfigOffTakesTheOutputAwayAndBack(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	workspaces := []wm.WorkspaceConfig{{Name: "side", OpenOnOutput: name}}
	on := []config.OutputConfig{{Name: name, FocusAtStartup: true}}
	if err := c.Reconfigure(Options{Workspaces: workspaces, Outputs: on}); err != nil {
		t.Fatal(err)
	}
	c.focusDefaultOutput()
	if got := c.FocusedOutputName(); got != name {
		t.Errorf("focused output = %q, want the focus-at-startup %s", got, name)
	}
	sideOutput := func() string {
		for _, workspace := range c.WorkspaceSnapshot().Workspaces {
			if workspace.Name != nil && *workspace.Name == "side" {
				return workspace.Output
			}
		}
		return ""
	}
	if got := sideOutput(); got != name {
		t.Fatalf("workspace side is on %q, want %s", got, name)
	}
	enabled := c.EnabledOutputs()

	if err := c.Reconfigure(Options{Workspaces: workspaces,
		Outputs: []config.OutputConfig{{Name: name, Off: true}}}); err != nil {
		t.Fatal(err)
	}
	if got := c.EnabledOutputs(); got != enabled-1 {
		t.Errorf("enabled outputs = %d after off, want %d", got, enabled-1)
	}
	if side, ok := outputSnapshotNamed(c, name); !ok || side.Logical != nil || side.CurrentMode != nil {
		t.Errorf("%s after off = %+v (listed %t), want it listed with no logical box or mode", name, side, ok)
	}
	// As on an unplug, move the workspace itself to the remaining output.
	if got := sideOutput(); got == name {
		t.Errorf("workspace side is still on %s after off", name)
	}

	if err := c.Reconfigure(Options{Workspaces: workspaces, Outputs: on}); err != nil {
		t.Fatal(err)
	}
	if side, ok := outputSnapshotNamed(c, name); !ok || side.Logical == nil {
		t.Errorf("%s after on = %+v, want it back in the layout", name, side)
	}
	if got := sideOutput(); got != name {
		t.Errorf("workspace side is on %q after on, want it back on %s", got, name)
	}
}
