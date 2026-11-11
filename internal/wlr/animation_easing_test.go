package wlr

import (
	"math"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestAnimationEasingHonorsSlowdownAndRetargets(t *testing.T) {
	slowdown := 2.0
	c := &Compositor{animationConfig: config.Animations{Slowdown: &slowdown}, animations: map[animationKey]*runningAnimation{}, animationTimer: time.NewTimer(time.Hour)}
	defer c.animationTimer.Stop()
	key := animationKey{property: "ease"}
	kind := config.Animation{Easing: &config.EasingConfig{DurationMS: 1000, Curve: "linear"}}
	c.animateValue(key, kind, 0, 1, 0, func(float64) {}, nil)
	running := c.animations[key]
	if running == nil || running.duration() != 2*time.Second || math.Abs(running.valueAt(time.Second)-.5) > 1e-9 {
		t.Fatalf("slow easing = %+v", running)
	}
	running.started = time.Now().Add(-time.Second)
	c.animateValue(key, kind, 0, 2, 0, func(float64) {}, nil)
	retarget := c.animations[key]
	if retarget == running || math.Abs(retarget.valueAt(0)-.5) > .01 {
		t.Fatal("easing retarget jumped to its original start")
	}
}
