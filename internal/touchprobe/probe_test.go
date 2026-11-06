package touchprobe

import "testing"

// TestCapabilityNames: the seat's capability bits are spelled out in the
// order the protocol numbers them.
func TestCapabilityNames(t *testing.T) {
	cases := map[uint32]string{
		0: "none",
		1: "pointer",
		2: "keyboard",
		3: "pointer|keyboard",
		4: "touch",
		5: "pointer|touch",
		7: "pointer|keyboard|touch",
	}
	for caps, want := range cases {
		if got := CapabilityNames(caps); got != want {
			t.Errorf("CapabilityNames(%d) = %q, want %q", caps, got, want)
		}
	}
}

// TestSummaryCountsTheFinger: the summary is the one line that says what
// the probe saw.
func TestSummaryCountsTheFinger(t *testing.T) {
	result := Result{Downs: 1, Ups: 1, Motions: 2, Frames: 3, OnSurface: true}
	want := "touch-probe: summary downs=1 ups=1 motions=2 frames=3 cancels=0 on_surface=1"
	if got := result.Summary(); got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

// TestHasTouchCapability: the touch bit is the seat's fourth.
func TestHasTouchCapability(t *testing.T) {
	if (Result{Capabilities: 3}).HasTouchCapability() {
		t.Error("a seat without touch reported touch")
	}
	if !(Result{Capabilities: 4}).HasTouchCapability() {
		t.Error("a seat with touch reported no touch")
	}
}
