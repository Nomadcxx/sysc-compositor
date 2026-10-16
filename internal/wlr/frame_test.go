package wlr

import (
	"regexp"
	"testing"
)

var (
	reFrameRequest = regexp.MustCompile(`-> wl_surface#\d+\.frame\(new id wl_callback#(\d+)\)`)
	reCallbackDone = regexp.MustCompile(`\} wl_callback#(\d+)\.done\(`)
)

// TestClientsGetFrameCallbacks is the kitty that never repainted: a client
// that draws only when its wl_surface.frame callback fires (weston-simple-shm
// redraws on every one) must keep getting them. wl_display.sync callbacks
// also end in done, so only ids a frame request created are counted.
func TestClientsGetFrameCallbacks(t *testing.T) {
	c := newNested(t)
	const want = 30
	frames := map[string]bool{}
	done := 0
	runClient(t, c, []string{"weston-simple-shm"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		if m := reFrameRequest.FindStringSubmatch(line); m != nil {
			frames[m[1]] = true
		}
		if m := reCallbackDone.FindStringSubmatch(line); m != nil && frames[m[1]] {
			delete(frames, m[1])
			done++
		}
		return done >= want
	})
	if done < want {
		t.Fatalf("weston-simple-shm got %d frame callbacks before the run ended, want %d", done, want)
	}
}
