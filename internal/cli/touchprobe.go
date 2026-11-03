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
		default:
			return usage(name, env, "unknown touch-probe argument "+args[i])
		}
	}
	if display == "" {
		display = env.Getenv("WAYLAND_DISPLAY")
	}
	probe, err := touchprobe.Start(touchprobe.Options{
		Display: display,
		Window:  window,
		Timeout: timeout,
		Out:     env.Stdout,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: touch-probe: %v\n", name, err)
		return probeFailed
	}
	defer probe.Close()
	// A finger ends the wait; the timeout ends it too.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			probe.Stop()
		case <-probe.Arrived():
		}
	}()
	if timeout > 0 {
		select {
		case <-probe.Arrived():
		case <-time.After(timeout):
		}
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
	return probeFingerArrived
}
