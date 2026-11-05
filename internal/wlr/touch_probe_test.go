package wlr

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/touchprobe"
)

// probeLog collects the probe's lines, and lets a test wait for one of them.
type probeLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *probeLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, string(p))
	return len(p), nil
}

func (l *probeLog) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "")
}

// waitFor waits for a line the probe wrote.
func (l *probeLog) waitFor(t *testing.T, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(l.text(), want) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("the probe never wrote %q; it wrote:\n%s", want, l.text())
	return false
}

// TestTouchProbeSeesTheFinger: the probe client sees the finger the
// compositor sends.
//
// This is the evidence route the touch gate needs. The seat's capabilities
// are nowhere else to be seen, and wayland-info cannot say whether a finger
// reaches a client, so a client that asks for touch and reports its events
// can.
func TestTouchProbeSeesTheFinger(t *testing.T) {
	c := newNested(t)
	watchdog := time.AfterFunc(20*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	mapped := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case mapped <- view:
		default:
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		var screen *fakeTouch
		onCompositor(t, c, func() { screen = c.addFakeTouch() })
		if screen == nil {
			t.Error("the fake touch screen did not appear")
			return
		}
		log := &probeLog{}
		probe, err := touchprobe.Start(touchprobe.Options{
			Display: c.SocketName(),
			Window:  true,
			Out:     log,
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer probe.Close()
		if !log.waitFor(t, "touch-probe: touch bound", 10*time.Second) {
			return
		}
		var view *toplevelView
		select {
		case view = <-mapped:
		case <-time.After(10 * time.Second):
			t.Error("the probe's window never mapped")
			return
		}
		var dx, dy float64
		onCompositor(t, c, func() {
			lx, ly := viewCenter(view)
			dx, dy = touchDevicePoint(c, lx, ly)
		})
		onCompositor(t, c, func() {
			screen.down(1, dx, dy)
			screen.frame()
			screen.up(1)
			screen.frame()
		})
		if !log.waitFor(t, "touch-probe: down id=1", 10*time.Second) {
			return
		}
		result := probe.Result()
		if !result.HasTouchCapability() {
			t.Errorf("the seat advertised no touch: %d", result.Capabilities)
		}
		if result.Downs != 1 {
			t.Errorf("the probe saw %d fingers go down, want 1", result.Downs)
		}
		if result.Ups != 1 {
			t.Errorf("the probe saw %d fingers lift, want 1", result.Ups)
		}
		if !result.OnSurface {
			t.Error("the finger did not land on the probe's own window")
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
