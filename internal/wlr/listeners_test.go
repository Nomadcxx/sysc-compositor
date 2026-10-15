package wlr

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestListenersDoNotGrowWithClients: windows that open and close leave the
// compositor holding no more listeners than before they came.
func TestListenersDoNotGrowWithClients(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	mapped := make(chan struct{}, 1)
	c.viewMapped = func(*toplevelView) { mapped <- struct{}{} }
	onLoop := func(fn func()) {
		done := make(chan struct{})
		if err := c.Post(func() { fn(); close(done) }); err != nil {
			t.Error(err)
			return
		}
		<-done
	}

	var counts []int
	result := make(chan error, 1)
	go func() {
		defer c.Terminate()
		var baseline int
		onLoop(func() { baseline = c.liveListeners() })
		for i := range 3 {
			cmd := exec.Command(terminal)
			cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := cmd.Start(); err != nil {
				result <- err
				return
			}
			select {
			case <-mapped:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				result <- fmt.Errorf("client %d never mapped", i)
				return
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(20 * time.Millisecond) {
				var views, live int
				onLoop(func() { views, live = len(c.views), c.liveListeners() })
				if views == 0 {
					counts = append(counts, live)
					break
				}
				if time.Now().After(deadline) {
					result <- fmt.Errorf("client %d's window never went away", i)
					return
				}
			}
		}
		for _, n := range counts {
			if n != baseline {
				result <- fmt.Errorf("listeners after each client = %v, want %d every time", counts, baseline)
				return
			}
		}
		result <- nil
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
