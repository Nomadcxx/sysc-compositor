package wlr

import "testing"

func TestScrollTrackerMatchesNiri(t *testing.T) {
	tr := newScrollTracker(120)
	if got := tr.accumulate(60); got != 0 {
		t.Fatalf("half a detent = %d ticks, want 0", got)
	}
	if got := tr.accumulate(60); got != 1 {
		t.Fatalf("a full detent = %d ticks, want 1", got)
	}
	tr.accumulate(60)
	if got := tr.accumulate(-30); got != 0 {
		t.Fatalf("a direction change = %d ticks, want 0 (the accumulator resets)", got)
	}
	if got := tr.accumulate(-210); got != -2 {
		t.Fatalf("-240 in all = %d ticks, want -2", got)
	}
	if got := tr.accumulate(-120 * 200); got != -127 {
		t.Fatalf("a huge scroll = %d ticks, want the -127 clamp", got)
	}
	tr.reset()
	if got := tr.accumulate(119); got != 0 {
		t.Fatalf("after reset 119 = %d ticks, want 0", got)
	}
}
