package wlr

import (
	"regexp"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
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

// TestHiddenWorkspaceKeepsFrameCallbacks: a client that draws only on frame
// callbacks keeps making progress after its window moves to a workspace no
// output shows. Niri answers such surfaces from a one-second fallback timer;
// the answers must not wake the output either.
func TestHiddenWorkspaceKeepsFrameCallbacks(t *testing.T) {
	c := newNested(t)
	watchdog := time.AfterFunc(15*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	views := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case views <- view:
		default:
		}
	}
	go func() {
		defer c.Terminate()
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		var view *toplevelView
		select {
		case view = <-views:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		hidden := false
		onCompositor(t, c, func() {
			snapshot := c.WorkspaceSnapshot()
			output := ""
			for _, window := range snapshot.Windows {
				if window.ID == view.id {
					output = window.Output
				}
			}
			for _, workspace := range snapshot.Workspaces {
				if workspace.Output == output && !workspace.IsActive &&
					c.model.MoveWindowToWorkspace(view.id, workspace.ID, false) {
					c.applyWorkspaceModel()
					hidden = !bool(view.tree.node.enabled)
					return
				}
			}
		})
		if !hidden {
			t.Error("could not move the window to a hidden workspace")
			return
		}
		// Let the move's own frames finish before counting.
		if _, err := client.Next(time.Second); err != nil {
			t.Error(err)
			return
		}
		drawn := func() (n int) {
			onCompositor(t, c, func() { n = c.scenesDrawn() })
			return n
		}
		before := drawn()
		client.PaceFrames()
		if _, err := client.Next(2500 * time.Millisecond); err != nil {
			t.Error(err)
			return
		}
		if got := client.FrameDones(); got < 2 || got > 4 {
			t.Errorf("the hidden client got %d frame callbacks in 2.5 s, want 2-4 from a one-second fallback", got)
		}
		if after := drawn(); after != before {
			t.Errorf("the hidden client's commits drew %d output frames, want none", after-before)
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}
