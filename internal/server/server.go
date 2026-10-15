// Package server owns the lifetime of one running compositor. Every method here
// runs on the locked compositor thread; the only cross-thread call permitted is
// Terminate, which merely sets a flag on the display.
package server

import (
	"errors"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// Server wraps the wlroots object graph behind the lifetime the binary needs.
type Server struct {
	c *wlr.Compositor
}

// New builds the compositor. The caller must already hold the compositor thread.
func New() (*Server, error) {
	c, err := wlr.New()
	if err != nil {
		return nil, err
	}
	return &Server{c: c}, nil
}

// SocketName is the nested Wayland socket clients connect to.
func (s *Server) SocketName() string { return s.c.SocketName() }

// Run blocks until Terminate is called.
func (s *Server) Run() error {
	if s.c == nil {
		return errors.New("server already destroyed")
	}
	return s.c.Run()
}

// Terminate is the only method safe to call off the compositor thread.
func (s *Server) Terminate() { s.c.Terminate() }

// Destroy releases the compositor and every listener it owns.
func (s *Server) Destroy() {
	if s.c != nil {
		s.c.Destroy()
		s.c = nil
	}
}
