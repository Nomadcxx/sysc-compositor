package wlr

import "testing"

func TestOutputSnapshotHasNiriModeAndLogicalGeometry(t *testing.T) {
	c := newNested(t)
	outputs := c.OutputSnapshot()
	if len(outputs) != c.EnabledOutputs() || len(outputs) == 0 {
		t.Fatalf("OutputSnapshot() has %d outputs, EnabledOutputs() = %d", len(outputs), c.EnabledOutputs())
	}
	for _, output := range outputs {
		if output.Name == "" {
			t.Error("output name is empty")
		}
		if output.CurrentMode == nil || int(*output.CurrentMode) >= len(output.Modes) {
			t.Errorf("output %q has no valid current mode: %+v", output.Name, output)
		}
		if output.Logical == nil || output.Logical.Width == 0 || output.Logical.Height == 0 {
			t.Errorf("output %q has no logical geometry: %+v", output.Name, output.Logical)
		}
	}
	if got := c.FocusedOutputName(); got == "" {
		t.Error("FocusedOutputName() is empty while an output is enabled")
	}
}

func TestOutputSnapshotDoesNotEnableUnsupportedVRR(t *testing.T) {
	c := newNested(t)
	outputs := c.OutputSnapshot()
	if len(outputs) == 0 {
		t.Fatal("OutputSnapshot() has no outputs")
	}
	for _, output := range outputs {
		if !output.VRRSupported && output.VRREnabled {
			t.Errorf("output %q reports VRR enabled without support", output.Name)
		}
	}
}
