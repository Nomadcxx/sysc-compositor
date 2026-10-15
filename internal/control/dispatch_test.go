package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSeat records every effect the dispatcher asks for.
type fakeSeat struct {
	mu       sync.Mutex
	calls    []string
	windows  []Window
	attached bool
}

func (f *fakeSeat) log(format string, args ...any) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	f.mu.Unlock()
}

func (f *fakeSeat) Attach(name string) error { f.attached = true; f.log("attach %s", name); return nil }
func (f *fakeSeat) Detach() error            { f.attached = false; f.log("detach"); return nil }
func (f *fakeSeat) Windows() ([]Window, error) {
	return append([]Window(nil), f.windows...), nil
}
func (f *fakeSeat) Point(x, y float64) error { f.log("point %v %v", x, y); return nil }
func (f *fakeSeat) Button(code uint32, pressed bool) error {
	f.log("button %d %v", code, pressed)
	return nil
}
func (f *fakeSeat) Key(code uint32, pressed bool) error {
	f.log("key %d %v", code, pressed)
	return nil
}

func (f *fakeSeat) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

var testWindows = []Window{
	{ID: 7, AppID: "foot", PID: 40, X: 100, Y: 50, Width: 400, Height: 300},
	{ID: 3, AppID: "foot", PID: 41, X: 600, Y: 50, Width: 400, Height: 300},
	{ID: 9, AppID: "firefox", PID: 42, X: 0, Y: 400, Width: 800, Height: 600},
}

// validRequest is a well-formed request for every verb, so the Stop table can
// be generated from the registry rather than written by hand.
func validRequest(verb string) Request {
	r := Request{ID: 1, Verb: verb, Name: "assistant"}
	switch verb {
	case "point":
		r.Target, r.X, r.Y = "id:7", 10, 10
	case "button":
		r.Button, r.Pressed = "left", true
	case "key":
		r.Key, r.Pressed = "a", true
	case "type":
		r.Text = "hi"
	}
	return r
}

func newTestDispatcher() (*Dispatcher, *fakeSeat) {
	seat := &fakeSeat{windows: testWindows}
	return NewDispatcher(seat), seat
}

// TestEveryMutatingVerbRefusesAfterStop is design D3.8's enforcement table,
// generated from the dispatcher's own verb registry: a verb added later
// without the Stop gate fails here.
func TestEveryMutatingVerbRefusesAfterStop(t *testing.T) {
	t.Parallel()
	names := make([]string, 0, len(verbs))
	for name := range verbs {
		names = append(names, name)
	}
	sort.Strings(names)
	mutating := 0
	for _, name := range names {
		d, seat := newTestDispatcher()
		if rep := d.Handle(1, validRequest("attach"), nil); !rep.OK {
			t.Fatalf("attach: %+v", rep)
		}
		if rep := d.Handle(2, Request{ID: 1, Verb: "stop"}, nil); !rep.OK {
			t.Fatalf("stop: %+v", rep)
		}
		seat.take()
		rep := d.Handle(1, validRequest(name), nil)
		effects := seat.take()
		if verbs[name].mutating {
			mutating++
			if rep.Error != CodeStopped {
				t.Errorf("%s after Stop = %+v, want %s", name, rep, CodeStopped)
			}
			if len(effects) != 0 {
				t.Errorf("%s after Stop reached the seat: %v", name, effects)
			}
			continue
		}
		if rep.Error == CodeStopped {
			t.Errorf("%s is not mutating but answered %s", name, CodeStopped)
		}
	}
	if mutating < 5 {
		t.Fatalf("only %d mutating verbs in the registry; the table is not exercising the gate", mutating)
	}
	// Nothing on the socket lifts Stop.
	d, _ := newTestDispatcher()
	d.Handle(1, Request{Verb: "stop"}, nil)
	for _, name := range names {
		d.Handle(1, validRequest(name), nil)
	}
	if rep := d.Handle(1, validRequest("key"), nil); rep.Error != CodeStopped {
		t.Fatalf("Stop was lifted by a socket verb: %+v", rep)
	}
}

func TestStopReleasesHeldInput(t *testing.T) {
	t.Parallel()
	d, seat := newTestDispatcher()
	d.Handle(1, validRequest("attach"), nil)
	d.Handle(1, Request{Verb: "key", Key: "Shift", Pressed: true}, nil)
	d.Handle(1, Request{Verb: "button", Button: "left", Pressed: true}, nil)
	seat.take()
	d.Handle(5, Request{Verb: "stop"}, nil) // any connection, e.g. the shell
	got := strings.Join(seat.take(), ";")
	if !strings.Contains(got, "key 42 false") || !strings.Contains(got, "button 272 false") {
		t.Fatalf("Stop released %q, want Shift and the left button released", got)
	}
}

func TestAttachOwnership(t *testing.T) {
	t.Parallel()
	d, seat := newTestDispatcher()
	if rep := d.Handle(1, validRequest("key"), nil); rep.Error != CodeNotAttached {
		t.Fatalf("key before attach = %+v, want not-attached", rep)
	}
	d.Handle(1, validRequest("attach"), nil)
	if rep := d.Handle(2, validRequest("attach"), nil); rep.Error != CodeBusy {
		t.Fatalf("second attach = %+v, want busy", rep)
	}
	if rep := d.Handle(2, validRequest("key"), nil); rep.Error != CodeNotAttached {
		t.Fatalf("key from a non-owner = %+v, want not-attached", rep)
	}
	d.Handle(1, Request{Verb: "key", Key: "a", Pressed: true}, nil)
	seat.take()
	d.Disconnect(1)
	if got := strings.Join(seat.take(), ";"); got != "key 30 false;detach" {
		t.Fatalf("owner disconnect = %q, want the held key released then detach", got)
	}
	if rep := d.Handle(2, validRequest("attach"), nil); !rep.OK {
		t.Fatalf("attach after the owner left = %+v", rep)
	}
}

func TestTargetResolution(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		target string
		want   uint64
		code   string
	}{
		{"id:3", 3, ""},
		{"pid:42", 9, ""},
		{"app-id:foot", 7, ""}, // ambiguous: the most recently focused (first) wins
		{"app-id:nothing", 0, CodeUnknownTarget},
		{"id:99", 0, CodeUnknownTarget},
		{"id:x", 0, CodeBadRequest},
		{"window:3", 0, CodeBadRequest},
		{"", 0, CodeBadRequest},
	} {
		w, code := resolveTarget(tc.target, testWindows)
		if code != tc.code || (code == "" && w.ID != tc.want) {
			t.Errorf("resolveTarget(%q) = %d/%q, want %d/%q", tc.target, w.ID, code, tc.want, tc.code)
		}
	}
	d, seat := newTestDispatcher()
	d.Handle(1, validRequest("attach"), nil)
	seat.take()
	rep := d.Handle(1, Request{Verb: "point", Target: "app-id:nothing"}, nil)
	if rep.Error != CodeUnknownTarget || len(seat.take()) != 0 {
		t.Fatalf("unknown target = %+v, and must reach no seat", rep)
	}
	rep = d.Handle(1, Request{Verb: "point", Target: "pid:42", X: 5, Y: 6}, nil)
	if !rep.OK || rep.Window == nil || rep.Window.ID != 9 {
		t.Fatalf("point = %+v", rep)
	}
	if got := seat.take(); len(got) != 1 || got[0] != "point 5 406" {
		t.Fatalf("point effects = %v, want window-local mapped to layout (5,406)", got)
	}
}

func TestTypeUSText(t *testing.T) {
	t.Parallel()
	d, seat := newTestDispatcher()
	d.Handle(1, validRequest("attach"), nil)
	seat.take()
	if rep := d.Handle(1, Request{Verb: "type", Text: "A!"}, nil); !rep.OK {
		t.Fatalf("type = %+v", rep)
	}
	want := "key 42 true;key 30 true;key 30 false;key 42 false;key 42 true;key 2 true;key 2 false;key 42 false"
	if got := strings.Join(seat.take(), ";"); got != want {
		t.Fatalf("type A! = %q, want %q", got, want)
	}
	if rep := d.Handle(1, Request{Verb: "type", Text: "ok é"}, nil); rep.Error != CodeUnsupported {
		t.Fatalf("non-US text = %+v, want unsupported", rep)
	}
	if got := seat.take(); len(got) != 0 {
		t.Fatalf("unsupported text typed %v before refusing", got)
	}
}

// blockingWriter holds every write until released, standing in for a slow
// subscriber.
type blockingWriter struct {
	release chan struct{}
	mu      sync.Mutex
	buf     bytes.Buffer
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func TestFactsCoalesce(t *testing.T) {
	t.Parallel()
	d, _ := newTestDispatcher()
	w := &blockingWriter{release: make(chan struct{})}
	d.Handle(5, Request{Verb: "subscribe"}, &Conn{enc: json.NewEncoder(w)})
	d.Handle(1, validRequest("attach"), nil)

	start := time.Now()
	for i := range 1000 {
		d.Handle(1, Request{Verb: "point", Target: "id:7", X: float64(i), Y: 0}, nil)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("1000 points took %v with a stuck subscriber: publishing blocked", elapsed)
	}
	close(w.release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.mu.Lock()
		out := w.buf.String()
		w.mu.Unlock()
		if strings.Contains(out, `{"fact":"agent.0.x","value":1099}`) {
			if n := strings.Count(out, `"agent.0.x"`); n > 10 {
				t.Fatalf("%d agent.0.x facts delivered; a slow subscriber should get the latest, not every value", n)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the latest agent.0.x never arrived:\n%s", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFactsFollowInput(t *testing.T) {
	t.Parallel()
	d, _ := newTestDispatcher()
	r, w := io.Pipe()
	defer r.Close()
	d.Handle(5, Request{Verb: "subscribe"}, &Conn{enc: json.NewEncoder(w)})
	dec := json.NewDecoder(r)
	seen := map[string]any{}
	await := func(name string, value any) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			if v, ok := seen[name]; ok && fmt.Sprint(v) == fmt.Sprint(value) {
				return
			}
			got := make(chan Fact, 1)
			go func() {
				var f Fact
				_ = dec.Decode(&f)
				got <- f
			}()
			select {
			case f := <-got:
				seen[f.Fact] = f.Value
			case <-deadline:
				t.Fatalf("fact %s=%v never arrived; seen %v", name, value, seen)
			}
		}
	}
	await("agent.active", false)
	d.Handle(1, validRequest("attach"), nil)
	await("agent.active", true)
	await("agent.name", "assistant")
	d.Handle(1, Request{Verb: "point", Target: "id:7", X: 1, Y: 2}, nil)
	await("agent.0.x", 101)
	await("agent.0.y", 52)
	await("agent.target", 7)
	d.Handle(1, Request{Verb: "button", Button: "left", Pressed: true}, nil)
	await("agent.0.down", true)
	d.Handle(9, Request{Verb: "stop"}, nil)
	await("agent.stopped", true)
	await("agent.0.down", false)
}
