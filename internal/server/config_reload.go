package server

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

type reloadTarget struct {
	path     config.Path
	explicit bool
}

func compositorOptions(cfg *config.Config, locale *config.Keyboard) wlr.Options {
	workspaces := make([]wm.WorkspaceConfig, len(cfg.Workspaces))
	for i, workspace := range cfg.Workspaces {
		workspaces[i] = wm.WorkspaceConfig{Name: workspace.Name, OpenOnOutput: workspace.OpenOnOutput}
	}
	k := cfg.Keyboard
	if locale != nil {
		k = keyboardWithLocale(k, *locale)
	}
	return wlr.Options{
		Keyboard: wlr.Keyboard{
			Rules: k.Rules, Model: k.Model, Layout: k.Layout, Variant: k.Variant, Options: k.Options,
			File: expandHome(k.File), RepeatDelay: k.RepeatDelay, RepeatRate: k.RepeatRate,
		},
		ModKey: cfg.Input.ModKey, ModKeyNested: cfg.Input.ModKeyNested,
		Layout: &wlr.LayoutOptions{
			Gaps: cfg.Layout.Gaps, ColumnWidthProportion: cfg.Layout.ColumnWidthProportion,
			FixedColumnWidth: cfg.Layout.FixedColumnWidth, InsertHint: cfg.Layout.InsertHint,
			FocusRing: cfg.Layout.FocusRing, Shadow: cfg.Layout.Shadow,
		},
		CursorTheme: cfg.Cursor.Theme, CursorSize: cfg.Cursor.Size, Binds: cfg.Binds,
		Overview: cfg.Overview, Gestures: cfg.Gestures, Animations: cfg.Animations, Blur: cfg.Blur,
		Tablet:         cfg.Input.Tablet,
		ScreenshotPath: cfg.ScreenshotPath,
		OverlayBinds:   cfg.OverlayBinds, HotkeyOverlay: cfg.HotkeyOverlay,
		ShowHotkeyOverlayAtStartup: !cfg.HotkeyOverlay.SkipAtStartup,
		RecentWindows:              &cfg.RecentWindows,
		PreferNoCSD:                cfg.PreferNoCSD, Workspaces: workspaces,
		WindowRules: cfg.WindowRules, LayerRules: cfg.LayerRules,
	}
}

func keyboardWithLocale(configured, locale config.Keyboard) config.Keyboard {
	if configured.Rules != "" || configured.Model != "" || configured.Layout != "" ||
		configured.Variant != "" || configured.Options != "" || configured.File != "" {
		return configured
	}
	configured.Rules, configured.Model = locale.Rules, locale.Model
	configured.Layout, configured.Variant = locale.Layout, locale.Variant
	configured.Options, configured.File = locale.Options, locale.File
	return configured
}

func (s *Server) startConfigWatcher() {
	s.watchStart.Do(func() {
		if s.configWatcher == nil {
			return
		}
		stop := make(chan struct{})
		s.watchStop = stop
		s.watchWG.Add(1)
		go func() {
			defer s.watchWG.Done()
			for {
				select {
				case <-stop:
					return
				case <-s.configWatcher.Events():
					s.reloadWatchedConfigs()
				}
			}
		}()
	})
}

// stopConfigWatcher is safe on a Server whose watcher never started, and no
// watcher starts after it.
func (s *Server) stopConfigWatcher() {
	s.watchStart.Do(func() {})
	s.watchClose.Do(func() {
		if s.watchStop != nil {
			close(s.watchStop)
		}
		if s.configWatcher != nil {
			s.configWatcher.Close()
		}
		s.watchWG.Wait()
	})
}

func (s *Server) requestConfigReload(path *string) {
	s.reloadMu.Lock()
	target := reloadTarget{path: s.configSearch}
	if path != nil {
		target = reloadTarget{path: config.Path{Named: *path}, explicit: true}
	}
	targets := []reloadTarget{target}
	sequence, c, ok := s.beginReloadLocked()
	s.reloadMu.Unlock()
	if ok {
		s.launchReload(sequence, c, targets)
	}
}

func (s *Server) reloadWatchedConfigs() {
	s.reloadMu.Lock()
	targets := []reloadTarget{{path: s.configSearch}}
	if s.retryRoot != "" && s.retryRoot != s.configPath {
		targets = append(targets, reloadTarget{path: config.Path{Named: s.retryRoot}, explicit: true})
	}
	sequence, c, ok := s.beginReloadLocked()
	s.reloadMu.Unlock()
	if ok {
		s.launchReload(sequence, c, targets)
	}
}

func (s *Server) beginReloadLocked() (uint64, *wlr.Compositor, bool) {
	if s.destroying {
		return 0, nil, false
	}
	s.reloadSeq++
	sequence := s.reloadSeq
	s.reloadWG.Add(1)
	return sequence, s.c, true
}

func (s *Server) launchReload(sequence uint64, c *wlr.Compositor, targets []reloadTarget) {
	go func() {
		defer s.reloadWG.Done()
		for _, target := range targets {
			load, err := config.LoadWithSources(target.path.Resolve())
			if postErr := c.Post(func() { s.applyConfigReload(sequence, target, load, err) }); postErr != nil {
				if !errors.Is(postErr, wlr.ErrClosed) {
					os.Stderr.WriteString("sysc-compositor: config reload post: " + postErr.Error() + "\n")
				}
				return
			}
		}
	}()
}

func (s *Server) applyConfigReload(sequence uint64, target reloadTarget, load config.LoadResult, loadErr error) {
	s.reloadMu.Lock()
	if s.destroying || sequence != s.reloadSeq {
		s.reloadMu.Unlock()
		return
	}
	if loadErr == nil && load.Root == "" {
		// No searched config exists: as niri, keep the active one. The
		// watches still follow any searched directory created since, and a
		// config written there before its watch was added loads now.
		watched := s.watchedConfigs(s.configSources)
		search := s.configSearch
		s.reloadMu.Unlock()
		s.updateConfigWatches(watched)
		if search.Resolve() != "" {
			s.reloadWatchedConfigs()
		}
		return
	}
	if loadErr == nil {
		if load.Config == nil {
			load.Config = config.Default()
		}
		loadErr = s.c.Reconfigure(compositorOptions(load.Config, s.localeXKB))
	}
	if loadErr != nil {
		s.configSources = mergeSources(s.configSources, load.Sources)
		if target.explicit && load.Root != s.configPath {
			s.retryRoot = load.Root
		}
		watched := s.watchedConfigs(s.configSources)
		s.reloadMu.Unlock()
		s.updateConfigWatches(watched)
		os.Stderr.WriteString("sysc-compositor: config reload: " + loadErr.Error() + "\n")
		if s.announcer != nil {
			s.announcer.Announce(configErrorAnnouncement)
		}
		s.state.publishConfigLoaded(s.events, true)
		return
	}

	s.cfg = load.Config
	s.configPath = load.Root
	if target.explicit {
		s.configSearch = config.Path{Named: load.Root}
	}
	s.configSources = slices.Clone(load.Sources)
	s.retryRoot = ""
	s.configureX11()
	s.spawn.updateEnvironment(s.c.SocketName(), s.NiriSocket(), load.Config.Environment, load.Config.Cursor, s.Display())
	watched := s.watchedConfigs(s.configSources)
	s.reloadMu.Unlock()
	s.updateConfigWatches(watched)
	s.state.publishConfigLoaded(s.events, false)
}

func (s *Server) updateConfigWatches(watched []string) {
	if s.configWatcher == nil {
		return
	}
	if err := s.configWatcher.Update(watched); err != nil {
		os.Stderr.WriteString("sysc-compositor: config watcher update: " + err.Error() + "\n")
	}
}

// watchedConfigs adds the searched config paths to sources, so creating or
// removing a user or system config reloads, as niri's watcher does. The
// caller holds reloadMu.
func (s *Server) watchedConfigs(sources []string) []string {
	return mergeSources(sources, s.configSearch.Search)
}

func mergeSources(first, second []string) []string {
	seen := make(map[string]struct{}, len(first)+len(second))
	merged := make([]string, 0, len(first)+len(second))
	for _, source := range append(slices.Clone(first), second...) {
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		merged = append(merged, source)
	}
	return merged
}

func validateConfigPath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute: " + path)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("path does not point to a file: " + path)
	}
	return nil
}
