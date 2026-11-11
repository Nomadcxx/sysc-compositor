package animation

import (
	"math"
	"testing"
	"time"
)

func TestEasingCurvesAndBezierInvertTime(t *testing.T) {
	for _, tc := range []struct {
		curve string
		want  float64
	}{{"linear", .5}, {"ease-out-quad", .75}, {"ease-out-cubic", .875}, {"ease-out-expo", 1 - math.Exp2(-5)}} {
		ease, err := NewEasing(0, 1, time.Second, tc.curve, [4]float64{})
		if err != nil || math.Abs(ease.ValueAt(500*time.Millisecond)-tc.want) > 1e-9 {
			t.Fatalf("%s midpoint = %v, %v", tc.curve, ease.ValueAt(500*time.Millisecond), err)
		}
		if ease.ValueAt(0) != 0 || ease.ValueAt(time.Second) != 1 || ease.Offset(2).ValueAt(time.Second) != 3 {
			t.Fatal("easing endpoints/offset changed")
		}
	}
	// x(t)=t^3 and y(t)=1-(1-t)^3: x=.125 corresponds to t=.5.
	ease, err := NewEasing(0, 1, time.Second, "cubic-bezier", [4]float64{0, 1, 0, 1})
	if err != nil || math.Abs(ease.ValueAt(125*time.Millisecond)-.875) > 1e-7 {
		t.Fatalf("bezier midpoint = %v, %v", ease.ValueAt(125*time.Millisecond), err)
	}
	for _, curve := range []string{"bogus", "cubic-bezier"} {
		if _, err := NewEasing(0, 1, time.Second, curve, [4]float64{-1, 0, 1, 0}); err == nil {
			t.Fatalf("invalid %s accepted", curve)
		}
	}
}
