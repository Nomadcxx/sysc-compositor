package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/touchprobe"
)

// The exit codes `sysc-compositor touch-probe` ends with. They tell the
// laptop gate what happened without reading the output.
const (
	probeFingerArrived = 0
	probeNoFinger      = 1
	probeUsage         = 2
	probeNoTouch       = 3
	probeFailed        = 4
	probeNoLift        = 5
)

// touchProbe is `sysc-compositor touch-probe`: a client that asks the seat
// for touch and reports what it receives.
//
// It is the evidence route for the touch gate. The seat's capabilities are
// not exposed anywhere else, and wayland-info cannot say whether a finger
// reaches a client, so a client that binds wl_seat.get_touch and prints its
// events can.
func touchProbe(name string, args []string, env Env) int {
	display := ""
	timeout := 120 * time.Second
	window := true
	untilStroke := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--display":
			if i+1 >= len(args) {
				return usage(name, env, args[i]+" needs a display name")
			}
			i++
			display = args[i]
		case "--timeout":
			if i+1 >= len(args) {
				return usage(name, env, args[i]+" needs a duration")
			}
			i++
			parsed, err := time.ParseDuration(args[i])
			if err != nil || parsed < 0 {
				return usage(name, env, "unknown timeout "+args[i])
			}
			timeout = parsed
		case "--no-window":
			window = false
		case "--until-stroke":
			untilStroke = true
		default:
			return usage(name, env, "unknown touch-probe argument "+args[i])
		}
	}
	if display == "" {
		display = env.Getenv("WAYLAND_DISPLAY")
	}
	// With --until-stroke the probe has to outlive the wait for the finger, so
	// that the lift can still arrive. The wait itself is bounded below.
	probeTimeout := timeout
	if untilStroke {
		probeTimeout = 0
	}
	probe, err := touchprobe.Start(touchprobe.Options{
		Display: display,
		Window:  window,
		Timeout: probeTimeout,
		Out:     env.Stdout,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: %v\n", name, err)
		return probeFailed
	}
	defer probe.Close()
	// A finger ends the wait; the timeout ends it too. With --until-stroke the
	// wait ends when the finger lifts again, so the gate records the whole
	// stroke and not only the moment it landed.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	arrived := probe.Arrived()
	stroke := probe.Stroke()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-signals:
			probe.Stop()
		case <-finished:
		}
	}()
	// waitFor ends when the event arrives, when the limit runs out, or when the
	// connection dies.
	waitFor := func(limit time.Duration, event <-chan struct{}) bool {
		if limit > 0 {
			timer := time.NewTimer(limit)
			defer timer.Stop()
			select {
			case <-event:
				return true
			case <-probe.Done():
				return false
			case <-timer.C:
				return false
			}
		}
		select {
		case <-event:
			return true
		case <-probe.Done():
			return false
		}
	}
	if waitFor(timeout, arrived) && untilStroke {
		waitFor(timeout, stroke)
	}
	probe.Stop()
	if err := probe.Wait(); err != nil {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: %v\n", name, err)
		return probeFailed
	}
	result := probe.Result()
	fmt.Fprintln(env.Stdout, result.Summary())
	if !result.HasTouchCapability() {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: the seat does not advertise touch\n", name)
		return probeNoTouch
	}
	if result.Downs == 0 {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: no finger arrived\n", name)
		return probeNoFinger
	}
	if untilStroke && result.Ups == 0 {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: the finger arrived but never lifted\n", name)
		return probeNoLift
	}
	return probeFingerArrived
}
