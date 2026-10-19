package server

import (
	"errors"

	"github.com/Nomadcxx/sysc-compositor/internal/control"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// injectedSeatName is the wl_seat name clients see for the injected seat.
// The display name a driving client gives on attach is a fact, not this.
const injectedSeatName = "agent"

var errStopping = errors.New("server: compositor is stopping")

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// controlSeat implements control.Seat over the compositor. Each call is
// posted onto the compositor thread and waited for; it gives up once the
// server begins to stop, because posted work then never runs.
type controlSeat struct {
	c        *wlr.Compositor
	stopping <-chan struct{}
}

func (s controlSeat) call(fn func() error) error {
	done := make(chan error, 1)
	if err := s.c.Post(func() { done <- fn() }); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-s.stopping:
		return errStopping
	}
}

func (s controlSeat) Attach(string) error {
	return s.call(func() error { return s.c.AddInjectedSeat(injectedSeatName) })
}

func (s controlSeat) Detach() error {
	return s.call(func() error { return s.c.RemoveInjectedSeat(injectedSeatName) })
}

func (s controlSeat) Windows() ([]control.Window, error) {
	var out []control.Window
	err := s.call(func() error {
		for _, w := range s.c.Windows() {
			out = append(out, control.Window{ID: w.ID, AppID: stringValue(w.AppID), Title: stringValue(w.Title), PID: w.PID,
				X: w.X, Y: w.Y, Width: w.Width, Height: w.Height})
		}
		return nil
	})
	return out, err
}

func (s controlSeat) Point(x, y float64) error {
	return s.call(func() error { return s.c.InjectPointerMotion(injectedSeatName, x, y) })
}

func (s controlSeat) Button(code uint32, pressed bool) error {
	return s.call(func() error { return s.c.InjectPointerButton(injectedSeatName, code, pressed) })
}

func (s controlSeat) Key(code uint32, pressed bool) error {
	return s.call(func() error { return s.c.InjectKey(injectedSeatName, code, pressed) })
}
