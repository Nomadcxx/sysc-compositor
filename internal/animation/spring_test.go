package animation

import (
	"math"
	"testing"
	"time"
)

func TestCriticalSpringStartsAtFromAndSettlesAtTarget(t *testing.T) {
	spring, err := NewSpring(0, 1, 0, 1, 100, 0.0001)
	if err != nil {
		t.Fatal(err)
	}
	if got := spring.ValueAt(0); got != 0 {
		t.Fatalf("ValueAt(0) = %v, want 0", got)
	}
	if got := spring.VelocityAt(0); got != 0 {
		t.Fatalf("VelocityAt(0) = %v, want 0", got)
	}
	middle := spring.ValueAt(100 * time.Millisecond)
	if middle <= 0 || middle >= 1 {
		t.Fatalf("critical spring midpoint = %v, want between endpoints", middle)
	}
	duration := spring.Duration()
	if duration <= 0 || duration >= time.Minute {
		t.Fatalf("critical spring duration = %v, want a finite positive duration", duration)
	}
	if got := spring.ValueAt(duration); got != 1 {
		t.Fatalf("ValueAt(duration) = %v, want exact target", got)
	}
}

func TestUnderdampedSpringPreservesInitialVelocity(t *testing.T) {
	spring, err := NewSpring(0, 1, 20, 0.5, 100, 0.0001)
	if err != nil {
		t.Fatal(err)
	}
	if got := spring.VelocityAt(0); got != 20 {
		t.Fatalf("VelocityAt(0) = %v, want 20", got)
	}
	if got := spring.ValueAt(100 * time.Millisecond); got <= 1 {
		t.Fatalf("spring with positive initial velocity did not overshoot: %v", got)
	}
}

func TestNewSpringRejectsInvalidParameters(t *testing.T) {
	tests := []struct {
		name                                            string
		from, to, velocity, damping, stiffness, epsilon float64
	}{
		{name: "non-finite endpoint", from: math.NaN(), to: 1, damping: 1, stiffness: 100, epsilon: 0.001},
		{name: "damping below Niri range", to: 1, damping: 0.09, stiffness: 100, epsilon: 0.001},
		{name: "zero stiffness", to: 1, damping: 1, epsilon: 0.001},
		{name: "epsilon below Niri range", to: 1, damping: 1, stiffness: 100, epsilon: 0.000001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSpring(test.from, test.to, test.velocity, test.damping, test.stiffness, test.epsilon); err == nil {
				t.Fatal("NewSpring() accepted invalid parameters")
			}
		})
	}
}
