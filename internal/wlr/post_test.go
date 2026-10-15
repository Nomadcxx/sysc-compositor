package wlr

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestPostRunsOnTheCompositorThread: work posted from any goroutine runs on
// the thread that runs the event loop, in each poster's order, and nothing
// runs once the compositor is gone.
func TestPostRunsOnTheCompositorThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	before := LiveHandles()
	c, err := New()
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	owner := syscall.Gettid()

	const posters, each = 4, 100
	var (
		mu        sync.Mutex
		ran       int
		offThread int
		last      = make([]int, posters)
		disorder  int
	)
	for p := range posters {
		last[p] = -1
		go func() {
			for i := range each {
				err := c.Post(func() {
					mu.Lock()
					defer mu.Unlock()
					if syscall.Gettid() != owner {
						offThread++
					}
					if i != last[p]+1 {
						disorder++
					}
					last[p] = i
					if ran++; ran == posters*each {
						c.Terminate()
					}
				})
				if err != nil {
					t.Errorf("Post: %v", err)
				}
			}
		}()
	}
	stop := time.AfterFunc(5*time.Second, func() { _ = c.Post(c.Terminate) })
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	stop.Stop()
	c.Destroy()

	if ran != posters*each || offThread != 0 || disorder != 0 {
		t.Fatalf("ran %d of %d, %d off the compositor thread, %d out of order",
			ran, posters*each, offThread, disorder)
	}
	if err := c.Post(func() { t.Error("posted work ran after Destroy") }); !errors.Is(err, ErrClosed) {
		t.Fatalf("Post after Destroy = %v, want ErrClosed", err)
	}
	if after := LiveHandles(); after != before {
		t.Fatalf("LiveHandles() = %d, want %d", after, before)
	}
}
