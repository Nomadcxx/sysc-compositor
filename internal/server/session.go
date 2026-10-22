package server

import (
	"errors"
	"fmt"
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/session"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// StartSessionDBus starts the locale1 and login1 clients for --session only.
func (s *Server) StartSessionDBus() {
	if s.sessionDBus != nil || s.c == nil {
		return
	}
	watcher, err := session.StartDBus(func(keyboard config.Keyboard) {
		s.postSessionUpdate(func() { s.applyLocaleXKB(keyboard) })
	}, func(closed bool) {
		s.postSessionUpdate(func() { s.setLidClosed(closed) })
	})
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: session D-Bus: " + err.Error() + "\n")
		return
	}
	s.sessionDBus = watcher
}

func (s *Server) postSessionUpdate(update func()) {
	if err := s.c.Post(update); err != nil && !errors.Is(err, wlr.ErrClosed) {
		fmt.Fprintf(os.Stderr, "sysc-compositor: session update: %v\n", err)
	}
}

func (s *Server) applyLocaleXKB(keyboard config.Keyboard) {
	s.localeXKB = &keyboard
	if !hasExplicitXKB(s.cfg.Keyboard) {
		if err := s.c.Reconfigure(compositorOptions(s.cfg, s.localeXKB)); err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: apply locale1 keyboard: %v\n", err)
		}
	}
}

func hasExplicitXKB(keyboard config.Keyboard) bool {
	return keyboard.Rules != "" || keyboard.Model != "" || keyboard.Layout != "" ||
		keyboard.Variant != "" || keyboard.Options != "" || keyboard.File != ""
}
