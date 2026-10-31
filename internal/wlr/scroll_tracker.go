package wlr

import "math"

// scrollTracker is Niri's ScrollTracker (src/input/scroll_tracker.rs): it
// turns scroll amounts into whole ticks of size tick, dropping the remainder
// when the direction changes.
type scrollTracker struct {
	tick, last, acc float64
}

func newScrollTracker(tick float64) scrollTracker { return scrollTracker{tick: tick} }

func (s *scrollTracker) accumulate(amount float64) int8 {
	if (s.last > 0 && amount < 0) || (s.last < 0 && amount > 0) {
		s.acc = 0
	}
	s.last = amount
	s.acc += amount
	if math.Abs(s.acc) < s.tick {
		return 0
	}
	clamped := math.Max(-127*s.tick, math.Min(127*s.tick, s.acc))
	ticks := int8(int16(clamped) / int16(s.tick))
	s.acc = math.Mod(s.acc, s.tick)
	return ticks
}

func (s *scrollTracker) reset() { s.last, s.acc = 0, 0 }
