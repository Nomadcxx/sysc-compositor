// Package server owns the lifetime of one running compositor and its control
// socket. Every method here runs on the locked compositor thread; the only
// cross-thread call permitted is Terminate, which merely sets a flag on the
// display. The control socket's goroutines reach the compositor through Post.
package server

import (
	"errors"
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/control"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// Server wraps the wlroots object graph behind the lifetime the binary needs.
type Server struct {
	c        *wlr.Compositor
	ctl      *control.Server
	ctlPath  string
	stopping chan struct{}
}

// New builds the compositor. The caller must already hold the compositor thread.
func New() (*Server, error) {
	c, err := wlr.New()
	if err != nil {
		return nil, err
	}
	s := &Server{c: c, stopping: make(chan struct{})}
	if err := s.startControl(); err != nil {
		// The compositor still serves clients without the control socket;
		// injected input is simply unavailable.
		os.Stderr.WriteString("sysc-compositor: control socket: " + err.Error() + "\n")
	}
	return s, nil
}

func (s *Server) startControl() error {
	path, err := control.SocketPath(os.Getenv("XDG_RUNTIME_DIR"), s.c.SocketName())
	if err != nil {
		return err
	}
	seat := controlSeat{c: s.c, stopping: s.stopping}
	ctl, err := control.Listen(path, control.NewDispatcher(seat))
	if err != nil {
		return err
	}
	s.ctl, s.ctlPath = ctl, path
	return nil
}

// ControlPath is the control socket, or empty when it did not start.
func (s *Server) ControlPath() string { return s.ctlPath }

// SocketName is the nested Wayland socket clients connect to.
func (s *Server) SocketName() string { return s.c.SocketName() }

// Run blocks until Terminate is called.
func (s *Server) Run() error {
	if s.c == nil {
		return errors.New("server already destroyed")
	}
	if s.ctl != nil {
		go s.ctl.Serve()
	}
	return s.c.Run()
}

// Terminate is the only method safe to call off the compositor thread.
func (s *Server) Terminate() { s.c.Terminate() }

// Destroy releases the compositor and every listener it owns.
func (s *Server) Destroy() {
	if s.ctl != nil {
		// Unblock connections waiting on posted work before closing them:
		// once the loop has stopped, that work never runs.
		close(s.stopping)
		_ = s.ctl.Close()
		s.ctl = nil
	}
	if s.c != nil {
		s.c.Destroy()
		s.c = nil
	}
}
