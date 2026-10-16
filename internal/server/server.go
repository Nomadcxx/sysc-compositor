// Package server owns the lifetime of one running compositor and its sockets:
// the control socket and the niri-ipc socket. Every method here runs on the
// locked compositor thread; the only cross-thread call permitted is
// Terminate, which merely sets a flag on the display. Socket goroutines reach
// the compositor through Post.
package server

import (
	"errors"
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/control"
	"github.com/Nomadcxx/sysc-compositor/internal/unixsock"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// Server wraps the wlroots object graph behind the lifetime the binary needs.
type Server struct {
	c        *wlr.Compositor
	cfg      *config.Config
	ctl      *control.Server
	ctlPath  string
	niri     *unixsock.Listener
	spawn    spawner
	stopping chan struct{}
}

// New builds the compositor from cfg (nil is niri's defaults). The caller must
// already hold the compositor thread.
func New(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	k := cfg.Keyboard
	c, err := wlr.NewWith(wlr.Options{Keyboard: wlr.Keyboard{
		Rules: k.Rules, Model: k.Model, Layout: k.Layout, Variant: k.Variant, Options: k.Options,
		File: expandHome(k.File), RepeatDelay: k.RepeatDelay, RepeatRate: k.RepeatRate,
	}})
	if err != nil {
		return nil, err
	}
	s := &Server{c: c, cfg: cfg, stopping: make(chan struct{})}
	if err := s.startControl(); err != nil {
		// The compositor still serves clients without the control socket;
		// injected input is simply unavailable.
		os.Stderr.WriteString("sysc-compositor: control socket: " + err.Error() + "\n")
	}
	if err := s.startNiri(); err != nil {
		// Without it, `niri msg` cannot reach this compositor; clients
		// still can.
		os.Stderr.WriteString("sysc-compositor: niri socket: " + err.Error() + "\n")
	}
	s.spawn = newSpawner(c.SocketName(), s.NiriSocket(), cfg.Environment)
	return s, nil
}

func (s *Server) startNiri() error {
	path, err := niriSocketPath(os.Getenv("XDG_RUNTIME_DIR"), s.c.SocketName(), os.Getpid())
	if err != nil {
		return err
	}
	if s.niri, err = unixsock.Listen(path); err != nil {
		return err
	}
	return nil
}

// NiriSocket is the niri-ipc socket, or empty when it did not start.
func (s *Server) NiriSocket() string {
	if s.niri == nil {
		return ""
	}
	return s.niri.Path()
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
	if s.niri != nil {
		h := niriHandler{quit: s.c.Terminate, spawn: s.spawn}
		go s.niri.Serve(h.serve)
	}
	// Niri runs spawn-at-startup, then spawn-sh-at-startup (src/main.rs).
	for _, argv := range s.cfg.SpawnAtStartup {
		s.startup(s.spawn.spawn(argv))
	}
	for _, command := range s.cfg.SpawnShAtStartup {
		s.startup(s.spawn.spawnSh(command))
	}
	return s.c.Run()
}

func (s *Server) startup(err error) {
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: spawn at startup: " + err.Error() + "\n")
	}
}

// Terminate is the only method safe to call off the compositor thread.
func (s *Server) Terminate() { s.c.Terminate() }

// Destroy releases the compositor and every listener it owns.
func (s *Server) Destroy() {
	if s.niri != nil {
		_ = s.niri.Close()
		s.niri = nil
	}
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
