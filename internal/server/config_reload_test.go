package server

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestFailedConfigReloadPreservesActiveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	writeConfigFile(t, path, "layout {\n    gaps 19\n}\n")
	loaded, err := config.LoadWithSources(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigReloadTestServer(t, loaded)
	active := s.cfg

	writeConfigFile(t, path, "include \"missing.kdl\"")
	candidate, loadErr := config.LoadWithSources(path)
	if loadErr == nil {
		t.Fatal("malformed candidate unexpectedly loaded")
	}
	s.applyConfigReload(s.reloadSeq, reloadTarget{root: path}, candidate, loadErr)

	if s.cfg != active || s.configPath != filepath.Clean(path) {
		t.Fatalf("failed reload changed active config: cfg=%p root=%q", s.cfg, s.configPath)
	}
	if !slices.Contains(s.configSources, filepath.Join(filepath.Dir(path), "missing.kdl")) {
		t.Fatalf("failed include source was not watched: %q", s.configSources)
	}
	if !s.state.snapshot().configLoadFailed {
		t.Fatal("failed reload state was not published")
	}
}

func TestSuccessfulConfigReloadAppliesSettingsAndSpawnEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	writeConfigFile(t, path, "layout {\n    gaps 18\n}\n")
	loaded, err := config.LoadWithSources(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigReloadTestServer(t, loaded)

	writeConfigFile(t, path, "layout {\n    gaps 26\n}\nenvironment {\n    SYSC_M9_RELOAD_TEST \"reloaded\"\n}\n")
	candidate, err := config.LoadWithSources(path)
	if err != nil {
		t.Fatal(err)
	}
	s.applyConfigReload(s.reloadSeq, reloadTarget{root: path}, candidate, nil)

	if s.cfg != candidate.Config || s.configPath != filepath.Clean(path) {
		t.Fatalf("successful reload not applied: cfg=%p root=%q", s.cfg, s.configPath)
	}
	if !slices.Contains(s.spawn.childEnvironment(), "SYSC_M9_RELOAD_TEST=reloaded") {
		t.Fatal("future spawns did not receive the reloaded environment")
	}
	if s.state.snapshot().configLoadFailed {
		t.Fatal("successful reload left the failed config state set")
	}
}

func TestExplicitConfigReloadChangesRootOnlyAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.kdl")
	requestedPath := filepath.Join(dir, "requested.kdl")
	writeConfigFile(t, activePath, "layout {\n    gaps 18\n}\n")
	writeConfigFile(t, requestedPath, "include \"optional.kdl\" optional=true\nlayout {\n    gaps 22\n}\n")
	active, err := config.LoadWithSources(activePath)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigReloadTestServer(t, active)
	activeConfig := s.cfg

	requested, err := config.LoadWithSources(requestedPath)
	if err != nil {
		t.Fatal(err)
	}
	s.applyConfigReload(s.reloadSeq, reloadTarget{root: requestedPath, explicit: true}, requested, nil)
	if s.configPath != filepath.Clean(requestedPath) || s.cfg == activeConfig {
		t.Fatalf("successful explicit reload kept old root/config: root=%q cfg=%p", s.configPath, s.cfg)
	}
	if !slices.Contains(s.configSources, filepath.Join(dir, "optional.kdl")) {
		t.Fatalf("optional include was not tracked: %q", s.configSources)
	}
}

func TestFailedExplicitConfigReloadKeepsActiveRoot(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.kdl")
	requestedPath := filepath.Join(dir, "requested.kdl")
	writeConfigFile(t, activePath, "layout {\n    gaps 18\n}\n")
	writeConfigFile(t, requestedPath, "include \"missing.kdl\"\n")
	active, err := config.LoadWithSources(activePath)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigReloadTestServer(t, active)
	requested, err := config.LoadWithSources(requestedPath)
	if err == nil {
		t.Fatal("explicit config with a missing required include unexpectedly loaded")
	}

	s.applyConfigReload(s.reloadSeq, reloadTarget{root: requestedPath, explicit: true}, requested, err)
	if s.configPath != filepath.Clean(activePath) || s.retryRoot != filepath.Clean(requestedPath) {
		t.Fatalf("failed explicit reload changed active root: root=%q retry=%q", s.configPath, s.retryRoot)
	}
	if !slices.Contains(s.configSources, filepath.Join(dir, "missing.kdl")) {
		t.Fatalf("failed explicit include was not tracked: %q", s.configSources)
	}
}

func newConfigReloadTestServer(t *testing.T, loaded config.LoadResult) *Server {
	t.Helper()
	runtime.LockOSThread()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	s, err := New(loaded, false)
	if err != nil {
		runtime.UnlockOSThread()
		t.Skipf("no nested wayland session available: %v", err)
	}
	t.Cleanup(func() {
		s.Destroy()
		runtime.UnlockOSThread()
	})
	return s
}

func writeConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
