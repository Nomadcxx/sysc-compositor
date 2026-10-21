package animation

import (
	"fmt"
	"math"
	"time"
)

// Spring models a unit-mass damped spring from From to To.
type Spring struct {
	from, to, initialVelocity float64
	dampingRatio, stiffness   float64
	epsilon                   float64
	duration                  time.Duration
}

// NewSpring validates the Niri 26.04 spring parameters.
func NewSpring(from, to, initialVelocity, dampingRatio, stiffness, epsilon float64) (Spring, error) {
	for _, value := range []float64{from, to, initialVelocity, dampingRatio, stiffness, epsilon} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Spring{}, fmt.Errorf("spring values must be finite")
		}
	}
	if dampingRatio < 0.1 || dampingRatio > 10 {
		return Spring{}, fmt.Errorf("damping ratio must be between 0.1 and 10")
	}
	if stiffness < 1 || stiffness > math.MaxUint32 {
		return Spring{}, fmt.Errorf("stiffness must be between 1 and %d", uint64(math.MaxUint32))
	}
	if epsilon < 0.00001 || epsilon > 0.1 {
		return Spring{}, fmt.Errorf("epsilon must be between 0.00001 and 0.1")
	}
	spring := Spring{
		from: from, to: to, initialVelocity: initialVelocity,
		dampingRatio: dampingRatio, stiffness: stiffness, epsilon: epsilon,
	}
	spring.duration = spring.computeDuration()
	return spring, nil
}

// ValueAt returns the spring position at elapsed time, clamped to its Niri duration.
func (s Spring) ValueAt(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return s.from
	}
	if elapsed >= s.duration {
		return s.to
	}
	value := s.positionAt(elapsed.Seconds())
	span := math.Abs(s.to - s.from)
	if span > 0 {
		value = min(max(value, math.Min(s.from, s.to)-10*span), math.Max(s.from, s.to)+10*span)
	}
	return value
}

// VelocityAt returns the spring velocity at elapsed time.
func (s Spring) VelocityAt(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return s.initialVelocity
	}
	if elapsed >= s.duration {
		return 0
	}
	t := elapsed.Seconds()
	x0 := s.from - s.to
	omega0 := math.Sqrt(s.stiffness)
	beta := s.dampingRatio * omega0
	envelope := math.Exp(-beta * t)
	if math.Abs(beta-omega0) <= float64(math.Nextafter32(1, 2)-1) {
		coefficient := beta*x0 + s.initialVelocity
		return envelope * (s.initialVelocity - beta*coefficient*t)
	}
	if beta < omega0 {
		omega1 := math.Sqrt(omega0*omega0 - beta*beta)
		coefficient := omega0*omega0*x0 + beta*s.initialVelocity
		return envelope * (s.initialVelocity*math.Cos(omega1*t) - coefficient/omega1*math.Sin(omega1*t))
	}
	omega2 := math.Sqrt(beta*beta - omega0*omega0)
	lambda1, lambda2 := -beta+omega2, -beta-omega2
	a := (s.initialVelocity - lambda2*x0) / (lambda1 - lambda2)
	b := x0 - a
	return a*lambda1*math.Exp(lambda1*t) + b*lambda2*math.Exp(lambda2*t)
}

// Duration returns Niri's spring duration estimate.
func (s Spring) Duration() time.Duration {
	return s.duration
}

func (s Spring) computeDuration() time.Duration {
	if math.Abs(s.to-s.from) <= math.Nextafter(1, 2)-1 {
		return 0
	}
	omega0 := math.Sqrt(s.stiffness)
	beta := s.dampingRatio * omega0
	if math.Abs(beta-omega0) <= float64(math.Nextafter32(1, 2)-1) || beta < omega0 {
		return durationFromSeconds(-math.Log(s.epsilon) / beta)
	}
	return s.overdampedDuration(beta, omega0)
}

func (s Spring) overdampedDuration(beta, omega0 float64) time.Duration {
	high := -math.Log(s.epsilon) / max(beta-math.Sqrt(beta*beta-omega0*omega0), math.SmallestNonzeroFloat64)
	if high <= 0 || math.IsInf(high, 0) || math.IsNaN(high) {
		high = 1 / omega0
	}
	low := 0.0
	for math.Abs(s.positionAt(high)-s.to) > s.epsilon {
		low, high = high, high*2
		if math.IsInf(high, 0) || high > float64(math.MaxInt64)/float64(time.Second) {
			return time.Duration(math.MaxInt64)
		}
	}
	for i := 0; i < 64; i++ {
		middle := low + (high-low)/2
		if math.Abs(s.positionAt(middle)-s.to) <= s.epsilon {
			high = middle
		} else {
			low = middle
		}
	}
	return durationFromSeconds(high)
}

func (s Spring) positionAt(t float64) float64 {
	x0 := s.from - s.to
	omega0 := math.Sqrt(s.stiffness)
	beta := s.dampingRatio * omega0
	if math.Abs(beta-omega0) <= float64(math.Nextafter32(1, 2)-1) {
		coefficient := beta*x0 + s.initialVelocity
		return s.to + math.Exp(-beta*t)*(x0+coefficient*t)
	}
	if beta < omega0 {
		omega1 := math.Sqrt(omega0*omega0 - beta*beta)
		coefficient := (beta*x0 + s.initialVelocity) / omega1
		return s.to + math.Exp(-beta*t)*(x0*math.Cos(omega1*t)+coefficient*math.Sin(omega1*t))
	}
	omega2 := math.Sqrt(beta*beta - omega0*omega0)
	lambda1, lambda2 := -beta+omega2, -beta-omega2
	a := (s.initialVelocity - lambda2*x0) / (lambda1 - lambda2)
	b := x0 - a
	return s.to + a*math.Exp(lambda1*t) + b*math.Exp(lambda2*t)
}

func durationFromSeconds(seconds float64) time.Duration {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 ||
		seconds > float64(math.MaxInt64)/float64(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds * float64(time.Second))
}
