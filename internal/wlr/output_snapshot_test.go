package wlr

import "testing"

func TestFocusedOutputTracksFocusInsteadOfPointer(t *testing.T) {
	outputs := []string{"HDMI-A-1", "DP-1"}
	active := chooseActiveOutputName("", "", outputs)
	if active != "DP-1" {
		t.Fatalf("initial active output = %q, want first sorted output DP-1", active)
	}

	// Pointer motion has no effect until a focus transition names another output.
	active = chooseActiveOutputName(active, "", outputs)
	if active != "DP-1" {
		t.Fatalf("active output after pointer-only motion = %q, want DP-1", active)
	}

	active = chooseActiveOutputName(active, "HDMI-A-1", outputs)
	if active != "HDMI-A-1" {
		t.Fatalf("active output after focus moves = %q, want HDMI-A-1", active)
	}
}

func TestFocusedOutputFallsBackWhenDisconnected(t *testing.T) {
	if got := chooseActiveOutputName("HDMI-A-1", "", []string{"DP-1"}); got != "DP-1" {
		t.Fatalf("active output after disconnect = %q, want DP-1", got)
	}
}

func TestClickUpdatesActiveOutputOverEmptySpace(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	outputs := c.OutputSnapshot()
	if len(outputs) != 2 {
		t.Fatalf("got %d outputs after adding a headless test output, want 2", len(outputs))
	}

	active := c.FocusedOutputName()
	var clicked OutputSnapshot
	for _, output := range outputs {
		if output.Name != active {
			clicked = output
			break
		}
	}
	if clicked.Logical == nil {
		t.Fatalf("no second output with logical geometry: %+v", outputs)
	}
	x := float64(clicked.Logical.X) + float64(clicked.Logical.Width)/2
	y := float64(clicked.Logical.Y) + float64(clicked.Logical.Height)/2
	if surface, _, _ := c.surfaceAt(x, y); surface != nil {
		t.Fatalf("center of output %q is covered by a surface", clicked.Name)
	}

	pointer := c.addFakePointer()
	if pointer == nil {
		t.Fatal("could not attach the test pointer")
	}
	t.Cleanup(pointer.remove)

	pointer.moveTo(x, y)
	if got := c.outputNameAt(x, y); got != clicked.Name {
		t.Fatalf("click point output = %q, want %q", got, clicked.Name)
	}
	// A click must select its output even when it hits no focusable surface.
	pointer.button(btnLeft, true)
	if got := c.activeOutputName; got != clicked.Name {
		t.Fatalf("active output after click over empty space = %q, want %q", got, clicked.Name)
	}
	pointer.button(btnLeft, false)
}

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
