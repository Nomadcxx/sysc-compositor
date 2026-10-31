package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/session"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// StartSessionDBus starts locale1/login1 clients, ScreenSaver inhibition,
// GNOME portal services, the a11y KeyboardMonitor and the AT-SPI announcer
// for --session only. Failures leave the session running.
func (s *Server) StartSessionDBus() {
	if s.c == nil {
		return
	}
	if s.portal == nil {
		portal, err := session.StartPortal(session.PortalHooks{Screenshot: s.portalScreenshot, PickColor: s.portalPickColor, OpenWayland: s.portalOpenWayland})
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: portal services: %v\n", err)
		} else {
			s.portal = portal
			snapshot := s.state.snapshot()
			portal.Update(snapshot.outputs, snapshot.windows)
			s.startPortalScreenCast(portal)
		}
	}
	if s.screenSaver == nil {
		screenSaver, err := session.StartScreenSaver(func(inhibited bool) {
			s.postSessionUpdate(func() { s.c.SetIdleInhibitedByDBus(inhibited) })
		})
		if err != nil {
			os.Stderr.WriteString("sysc-compositor: org.freedesktop.ScreenSaver: " + err.Error() + "\n")
		} else {
			s.screenSaver = screenSaver
		}
	}
	if s.keyMonitor == nil {
		// Orca's key grabs, as Niri serves them to session instances only.
		monitor, err := session.StartKeyboardMonitor(a11y.NewMonitor())
		if err != nil {
			os.Stderr.WriteString("sysc-compositor: org.freedesktop.a11y.Manager: " + err.Error() + "\n")
		} else {
			s.keyMonitor = monitor
			s.c.SetKeyboardMonitor(monitor.ProcessKey)
		}
	}
	if s.announcer == nil {
		// Compositor UI for screen readers, as Niri's AccessKit adapter.
		announcer, err := a11y.StartAnnouncer()
		if err != nil {
			os.Stderr.WriteString("sysc-compositor: AT-SPI announcer: " + err.Error() + "\n")
		} else {
			s.announcer = announcer
			s.c.SetAnnouncer(announcer)
		}
	}
	if s.sessionDBus != nil {
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

func (s *Server) startPortalScreenCast(portal *session.Portal) {
	c := s.c
	hooks := session.ScreenCastHooks{
		Start: func(ctx context.Context, request session.CastRequest, node func(uint32), closed func()) error {
			reply := make(chan error, 1)
			if err := c.Post(func() {
				if ctx.Err() != nil {
					reply <- ctx.Err()
					return
				}
				reply <- c.StartCast(wlr.CastOptions{
					SessionID: request.SessionID, StreamID: request.StreamID,
					Target: request.Target, CursorMode: request.CursorMode,
					NodeAdded: func(id uint32) { go node(id) },
					Closed:    func() { go closed() },
				})
			}); err != nil {
				return err
			}
			select {
			case err := <-reply:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Stop: func(id uint64) { _ = c.Post(func() { c.StopCast(id) }) },
	}
	failed := func(err error) { fmt.Fprintf(os.Stderr, "sysc-compositor: ScreenCast unavailable: %v\n", err) }
	lifetime, cancel := context.WithCancel(context.Background())
	if err := c.StartScreenCast(func(err error) {
		if err != nil {
			cancel()
			failed(err)
			return
		}
		id := c.DynamicCastWindowID()
		go func() {
			if err := portal.EnableScreenCast(lifetime, hooks, id); err != nil && lifetime.Err() == nil {
				failed(err)
			}
		}()
	}, func(err error) {
		cancel()
		failed(err)
	}); err != nil {
		cancel()
		failed(err)
	}
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

func (s *Server) portalScreenshot(ctx context.Context, pointer bool) (string, error) {
	c := s.c
	type result struct {
		path string
		err  error
	}
	reply := make(chan result, 1)
	if err := c.Post(func() {
		if ctx.Err() != nil {
			return
		}
		if err := c.ScreenshotScreenResult(ctx, pointer, func(path string, err error) { reply <- result{path, err} }); err != nil {
			reply <- result{err: err}
		}
	}); err != nil {
		return "", err
	}
	select {
	case result := <-reply:
		return result.path, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *Server) portalPickColor(ctx context.Context) ([3]float64, error) {
	c := s.c
	type result struct {
		rgb [3]float64
		err error
	}
	reply := make(chan result, 1)
	if err := c.Post(func() {
		if ctx.Err() != nil {
			return
		}
		var stop func() bool
		cancel, err := c.PickColor(func(rgb [3]float64, err error) {
			if stop != nil {
				stop()
			}
			reply <- result{rgb, err}
		})
		if err != nil {
			reply <- result{err: err}
			return
		}
		stop = context.AfterFunc(ctx, func() { _ = c.Post(cancel) })
	}); err != nil {
		return [3]float64{}, err
	}
	select {
	case result := <-reply:
		return result.rgb, result.err
	case <-ctx.Done():
		return [3]float64{}, ctx.Err()
	}
}

func (s *Server) portalOpenWayland(ctx context.Context, fd int) error {
	c := s.c
	// A cancellation may return before posted work runs. Serialize the short
	// native duplicate with ending this borrowed-fd call, so neither a queued
	// request nor shutdown can use a descriptor after its owner closes it.
	var mu sync.Mutex
	active := true
	defer func() { mu.Lock(); active = false; mu.Unlock() }()
	reply := make(chan error, 1)
	if err := c.Post(func() {
		mu.Lock()
		defer mu.Unlock()
		if !active || ctx.Err() != nil {
			return
		}
		reply <- c.AddServiceClient(fd)
	}); err != nil {
		return err
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
