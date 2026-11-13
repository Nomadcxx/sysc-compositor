package wlr

import (
	"encoding/json"
	"testing"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// TestLidPolicyTurnsLaptopPanelOffOnlyWhileExternalOutputExists: Niri's
// laptop-panel policy. The nested WL-1 output is the external connector.
func TestLidPolicyTurnsLaptopPanelOffOnlyWhileExternalOutputExists(t *testing.T) {
	c := newNested(t)
	var external unsafe.Pointer
	for pointer := range c.sceneOutputs {
		external = pointer
	}
	externalName := outputName(external)
	laid := func(name string) bool {
		output, ok := outputSnapshotNamed(c, name)
		return ok && output.Logical != nil
	}
	lit := func(name string) bool {
		for pointer := range c.sceneOutputs {
			if outputName(pointer) == name {
				return c.outputIsEnabled(pointer)
			}
		}
		return false
	}

	// A panel that connects while the lid is closed stays off.
	c.SetLidClosed(true)
	headless, err := addNamedHeadlessTestOutput(c, 800, 600, "eDP-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	const panel = "eDP-1"
	if _, known := outputSnapshotNamed(c, panel); !known || laid(panel) || !laid(externalName) {
		t.Fatalf("lid closed at hotplug: panel known=%t laid=%t, external laid=%t", known, laid(panel), laid(externalName))
	}
	c.SetLidClosed(false)
	if !laid(panel) || !lit(panel) {
		t.Fatal("opening the lid did not restore the panel")
	}
	c.SetLidClosed(true)
	if laid(panel) {
		t.Fatal("closing the lid left the panel on beside an external output")
	}

	// The debug flag keeps it on; reload derives the policy again and never
	// writes it into the output config.
	if err := c.Reconfigure(Options{Debug: config.Debug{KeepLaptopPanelOnWhenLidIsClosed: true}}); err != nil {
		t.Fatal(err)
	}
	if !laid(panel) {
		t.Fatal("keep-laptop-panel-on-when-lid-is-closed did not keep the panel on")
	}
	if err := c.Reconfigure(Options{}); err != nil {
		t.Fatal(err)
	}
	if laid(panel) || len(c.outputConfigs) != 0 {
		t.Fatalf("reload: panel laid=%t, configs=%+v", laid(panel), c.outputConfigs)
	}

	// Strict IPC and output-management requests cannot turn the panel on
	// against the lid, in either preparation or apply.
	if result, err := c.ConfigureOutput(ipc.OutputRequest{Name: panel, Action: json.RawMessage(`"On"`)}); err != nil || result != "Applied" {
		t.Fatalf("IPC On: %s, %v", result, err)
	}
	if laid(panel) {
		t.Fatal("IPC On overrode the lid")
	}
	scale := 1.0
	for _, testOnly := range []bool{true, false} {
		if err := c.ConfigureOutputs([]config.OutputConfig{{Name: panel, Scale: &scale}}, testOnly); err != nil {
			t.Fatalf("output management testOnly=%t: %v", testOnly, err)
		}
		if laid(panel) {
			t.Fatalf("output management testOnly=%t overrode the lid", testOnly)
		}
	}

	// Explicit config off survives opening the lid.
	if err := c.Reconfigure(Options{Outputs: []config.OutputConfig{{Name: panel, Off: true}}}); err != nil {
		t.Fatal(err)
	}
	c.SetLidClosed(false)
	if laid(panel) {
		t.Fatal("opening the lid overrode the panel's config off")
	}
	if err := c.Reconfigure(Options{}); err != nil {
		t.Fatal(err)
	}
	if !laid(panel) {
		t.Fatal("removing config off did not restore the panel")
	}

	// A lid cycle while monitors are powered off keeps the panel dark, and
	// PowerOnMonitors still restores it.
	if err := c.PowerOffMonitors(); err != nil {
		t.Fatal(err)
	}
	c.SetLidClosed(true)
	c.SetLidClosed(false)
	if !laid(panel) || lit(panel) {
		t.Fatalf("lid cycle while powered off: panel laid=%t lit=%t", laid(panel), lit(panel))
	}
	if err := c.PowerOnMonitors(); err != nil {
		t.Fatal(err)
	}
	if !lit(panel) || !lit(externalName) {
		t.Fatal("PowerOnMonitors did not restore both outputs")
	}

	// Unplugging the external output restores the panel before the nested
	// backend would terminate for want of outputs.
	c.SetLidClosed(true)
	if laid(panel) {
		t.Fatal("panel on with the lid closed")
	}
	destroyOutput(external)
	if !laid(panel) || !lit(panel) || len(c.sceneOutputs) != 1 {
		t.Fatal("losing the external output did not restore the sole panel")
	}
}
