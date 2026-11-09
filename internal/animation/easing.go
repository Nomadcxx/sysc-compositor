package animation

import (
	"fmt"
	"math"
	"time"
)

// Easing evaluates Niri's five easing curves over a fixed duration.
type Easing struct {
	from, to float64
	duration time.Duration
	curve    string
	bezier   [4]float64
}

func NewEasing(from, to float64, duration time.Duration, curve string, bezier [4]float64) (Easing, error) {
	for _, value := range append([]float64{from, to}, bezier[:]...) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Easing{}, fmt.Errorf("easing values must be finite")
		}
	}
	if duration < 0 {
		return Easing{}, fmt.Errorf("easing duration must not be negative")
	}
	switch curve {
	case "linear", "ease-out-quad", "ease-out-cubic", "ease-out-expo":
	case "cubic-bezier":
		if bezier[0] < 0 || bezier[0] > 1 || bezier[2] < 0 || bezier[2] > 1 {
			return Easing{}, fmt.Errorf("cubic-bezier time coordinates must be between 0 and 1")
		}
	default:
		return Easing{}, fmt.Errorf("unknown easing curve %q", curve)
	}
	return Easing{from, to, duration, curve, bezier}, nil
}

func (e Easing) Duration() time.Duration { return e.duration }

func (e Easing) Offset(delta float64) Easing {
	e.from += delta
	e.to += delta
	return e
}

func (e Easing) ValueAt(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return e.from
	}
	if elapsed >= e.duration {
		return e.to
	}
	y, _ := e.curveAt(float64(elapsed) / float64(e.duration))
	return e.from + (e.to-e.from)*y
}

func (e Easing) VelocityAt(elapsed time.Duration) float64 {
	if elapsed >= e.duration || e.duration == 0 {
		return 0
	}
	_, derivative := e.curveAt(math.Max(0, float64(elapsed)/float64(e.duration)))
	return (e.to - e.from) * derivative / e.duration.Seconds()
}

func (e Easing) curveAt(x float64) (float64, float64) {
	switch e.curve {
	case "linear":
		return x, 1
	case "ease-out-quad":
		return 1 - (1-x)*(1-x), 2 * (1 - x)
	case "ease-out-cubic":
		return 1 - math.Pow(1-x, 3), 3 * (1 - x) * (1 - x)
	case "ease-out-expo":
		power := math.Exp2(-10 * x)
		return 1 - power, 10 * math.Ln2 * power
	}
	// A cubic's parameter is not elapsed time. Bisection handles even a
	// zero x derivative at an endpoint without Newton's division by zero.
	value := func(t, a, b float64) float64 { return 3*(1-t)*(1-t)*t*a + 3*(1-t)*t*t*b + t*t*t }
	derivative := func(t, a, b float64) float64 { return 3*(1-t)*(1-t)*a + 6*(1-t)*t*(b-a) + 3*t*t*(1-b) }
	lo, hi := 0.0, 1.0
	for range 48 {
		mid := (lo + hi) / 2
		if value(mid, e.bezier[0], e.bezier[2]) < x {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := (lo + hi) / 2
	dx := derivative(t, e.bezier[0], e.bezier[2])
	return value(t, e.bezier[1], e.bezier[3]), derivative(t, e.bezier[1], e.bezier[3]) / math.Max(dx, 1e-12)
}
