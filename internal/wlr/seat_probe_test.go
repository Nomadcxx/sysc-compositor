package wlr

import "testing"

// TestSeatTable answers the one question the Tier 3 designs cannot proceed
// without: can wlroots 0.20 hold more than one seat, and is the state on each
// seat independent?
//
// It does. The plan this grew from predicted a single interesting seat with
// everything else decoration. That prediction is now a fact rather than an
// assumption, which is why the probe exists: the answer decides whether the
// agent seat in gh#262 is buildable as designed or is a parity exclusion.
//
// Two extra seats is the row that matters, because that is what the design
// asks for: a seat for the agent and a seat for the agent's own dialogs.
func TestSeatTable(t *testing.T) {
	c := newNested(t)

	result := ProbeSeats(c.Display(), 2)

	if result.Seats != 3 {
		t.Fatalf("held %d seats, want 3; wlroots 0.20 will not carry the two extra "+
			"seats the agent design needs", result.Seats)
	}
	if !result.Distinct {
		t.Error("two seat creations returned the same object; seat names are not " +
			"producing independent seats")
	}

	want := []uint32{
		uint32(CKeyboard),
		uint32(CPointer),
		uint32(CKeyboard | CPointer),
	}
	if len(result.Capabilities) != len(want) {
		t.Fatalf("read back %d capability sets, want %d", len(result.Capabilities), len(want))
	}
	for i := range want {
		if result.Capabilities[i] != want[i] {
			t.Errorf("seat %d advertised capabilities %d, want %d", i,
				result.Capabilities[i], want[i])
		}
	}

	if !result.Isolated {
		t.Error("clearing the injected seats changed the user seat; an agent seat " +
			"could take the user's input away, so the seats are not a boundary")
	}
}
