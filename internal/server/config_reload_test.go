package server

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
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
	s.applyConfigReload(s.reloadSeq, reloadTarget{path: config.Path{Named: path}}, candidate, loadErr)

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
	s.applyConfigReload(s.reloadSeq, reloadTarget{path: config.Path{Named: path}}, candidate, nil)

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
	s.applyConfigReload(s.reloadSeq, reloadTarget{path: config.Path{Named: requestedPath}, explicit: true}, requested, nil)
	if s.configPath != filepath.Clean(requestedPath) || s.cfg == activeConfig {
		t.Fatalf("successful explicit reload kept old root/config: root=%q cfg=%p", s.configPath, s.cfg)
	}
	if !slices.Contains(s.configSources, filepath.Join(dir, "optional.kdl")) {
		t.Fatalf("optional include was not tracked: %q", s.configSources)
	}
	if want := (config.Path{Named: filepath.Clean(requestedPath)}); !reflect.DeepEqual(s.configSearch, want) {
		t.Fatalf("later reloads read %+v, want %+v", s.configSearch, want)
	}
}

// TestConfigCreatedAfterStartupLoads: a session started with no config file
// loads one created later, directory and all, as niri's watcher does; it does
// not keep reloading the defaults.
func TestConfigCreatedAfterStartupLoads(t *testing.T) {
	user := filepath.Join(t.TempDir(), "niri", "config.kdl")
	s := newConfigReloadTestServerAt(t, config.LoadResult{}, config.Path{Search: []string{user}})
	if s.configWatcher == nil {
		t.Fatal("a missing searched config is not watched")
	}

	gaps := make(chan int, 1)
	go func() {
		defer s.Terminate()
		if err := os.MkdirAll(filepath.Dir(user), 0o700); err != nil {
			t.Error(err)
			return
		}
		writeConfigFile(t, user, "layout {\n    gaps 21\n}\n")
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s.reloadMu.Lock()
			got := s.cfg.Layout.Gaps
			s.reloadMu.Unlock()
			if got == 21 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		s.reloadMu.Lock()
		gaps <- s.cfg.Layout.Gaps
		s.reloadMu.Unlock()
	}()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if got := <-gaps; got != 21 {
		t.Fatalf("gaps = %v after creating %s, want 21", got, user)
	}
	if s.configPath != user {
		t.Fatalf("config root = %q, want %q", s.configPath, user)
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

	s.applyConfigReload(s.reloadSeq, reloadTarget{path: config.Path{Named: requestedPath}, explicit: true}, requested, err)
	if s.configPath != filepath.Clean(activePath) || s.retryRoot != filepath.Clean(requestedPath) {
		t.Fatalf("failed explicit reload changed active root: root=%q retry=%q", s.configPath, s.retryRoot)
	}
	if !slices.Contains(s.configSources, filepath.Join(dir, "missing.kdl")) {
		t.Fatalf("failed explicit include was not tracked: %q", s.configSources)
	}
}

func TestReportConfigGapsNamesUnappliedActions(t *testing.T) {
	cfg := &config.Config{
		Warnings:         []string{"blur"},
		UnappliedActions: []string{"maximize-column", "set-column-width"},
		NiriCommandBinds: []string{"Mod+Z"},
	}
	var stderr bytes.Buffer
	reportConfigGaps(cfg, &stderr)
	for _, want := range []string{
		"config section `blur` is accepted but not applied yet",
		"bind actions are accepted but not applied yet, so these binds do nothing: maximize-column, set-column-width",
		"these run `niri`, which is not part of sysc-compositor",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("warning %q does not contain %q", stderr.String(), want)
		}
	}
}

func TestWorkspaceLayoutReachesTheModelOptions(t *testing.T) {
	gaps := 8
	proportion := 0.75
	display := config.ColumnDisplayNormal
	centerNever := config.CenterFocusedColumnNever
	alwaysCenterOff := false
	cfg := &config.Config{
		Layout: config.Layout{
			PresetColumnWidths:       []config.PresetSize{{Value: 0.25}},
			PresetWindowHeights:      []config.PresetSize{{Fixed: true, Value: 700}},
			DefaultColumnDisplay:     config.ColumnDisplayTabbed,
			CenterFocusedColumn:      config.CenterFocusedColumnOnOverflow,
			AlwaysCenterSingleColumn: true,
			EmptyWorkspaceAboveFirst: true,
			Struts:                   config.Struts{Left: 10, Right: 10, Bottom: 32},
		},
		Workspaces: []config.WorkspaceConfig{{
			Name: "wide",
			Layout: &config.WorkspaceLayout{
				Gaps: &gaps, ColumnWidthProportion: &proportion,
				PresetColumnWidths:       []config.PresetSize{{Fixed: true, Value: 600}},
				DefaultColumnDisplay:     &display,
				CenterFocusedColumn:      &centerNever,
				AlwaysCenterSingleColumn: &alwaysCenterOff,
				Struts:                   &config.Struts{Top: 44},
			},
		}},
	}
	options := compositorOptions(cfg, nil)
	if len(options.Layout.PresetColumnWidths) != 1 || options.Layout.PresetColumnWidths[0] != (layout.PresetSize{Value: 0.25}) {
		t.Errorf("the preset column widths = %+v, want 0.25", options.Layout.PresetColumnWidths)
	}
	if len(options.Layout.PresetWindowHeights) != 1 ||
		options.Layout.PresetWindowHeights[0] != (layout.PresetSize{Fixed: true, Value: 700}) {
		t.Errorf("the preset window heights = %+v, want fixed 700", options.Layout.PresetWindowHeights)
	}
	if options.Layout.DefaultColumnDisplay != layout.ColumnDisplayTabbed {
		t.Errorf("the default column display = %v, want tabbed", options.Layout.DefaultColumnDisplay)
	}
	if options.Layout.CenterFocusedColumn != layout.CenterFocusedColumnOnOverflow {
		t.Errorf("the centering policy = %v, want on-overflow", options.Layout.CenterFocusedColumn)
	}
	if !options.Layout.AlwaysCenterSingleColumn || !options.Layout.EmptyWorkspaceAboveFirst {
		t.Errorf("the layout flags = %t %t, want both set",
			options.Layout.AlwaysCenterSingleColumn, options.Layout.EmptyWorkspaceAboveFirst)
	}
	if options.Layout.Struts != (layout.Struts{Left: 10, Right: 10, Bottom: 32}) {
		t.Errorf("the config struts = %+v, want left and right 10 and bottom 32", options.Layout.Struts)
	}
	if len(options.Workspaces) != 1 {
		t.Fatalf("workspaces = %+v, want one", options.Workspaces)
	}
	workspace := options.Workspaces[0]
	if workspace.Name != "wide" {
		t.Fatalf("the workspace name = %q, want wide", workspace.Name)
	}
	if workspace.Layout == nil {
		t.Fatal("the workspace has no layout")
	}
	if flag := workspace.Layout.AlwaysCenterSingleColumn; flag == nil || *flag {
		t.Errorf("the workspace's always-center-single-column = %v, want false", flag)
	}
	if workspace.Layout.Gaps == nil || *workspace.Layout.Gaps != 8 {
		t.Errorf("the gaps = %+v, want 8", workspace.Layout.Gaps)
	}
	if workspace.Layout.ColumnWidthProportion == nil || *workspace.Layout.ColumnWidthProportion != 0.75 {
		t.Errorf("the column width proportion = %+v, want 0.75", workspace.Layout.ColumnWidthProportion)
	}
	if len(workspace.Layout.PresetColumnWidths) != 1 ||
		workspace.Layout.PresetColumnWidths[0] != (layout.PresetSize{Fixed: true, Value: 600}) {
		t.Errorf("the preset column widths of the workspace = %+v, want fixed 600", workspace.Layout.PresetColumnWidths)
	}
	if workspace.Layout.DefaultColumnDisplay == nil || *workspace.Layout.DefaultColumnDisplay != layout.ColumnDisplayNormal {
		t.Errorf("the default column display of the workspace = %+v, want normal", workspace.Layout.DefaultColumnDisplay)
	}
	if workspace.Layout.CenterFocusedColumn == nil || *workspace.Layout.CenterFocusedColumn != layout.CenterFocusedColumnNever {
		t.Errorf("the centering policy of the workspace = %+v, want its own never", workspace.Layout.CenterFocusedColumn)
	}
	if workspace.Layout.Struts == nil || *workspace.Layout.Struts != (layout.Struts{Top: 44}) {
		t.Errorf("the struts of the workspace = %+v, want its own top 44", workspace.Layout.Struts)
	}
}

func newConfigReloadTestServer(t *testing.T, loaded config.LoadResult) *Server {
	t.Helper()
	return newConfigReloadTestServerAt(t, loaded, config.Path{Named: loaded.Root})
}

func newConfigReloadTestServerAt(t *testing.T, loaded config.LoadResult, path config.Path) *Server {
	t.Helper()
	// AF_UNIX paths include the test runtime directory and have a 108-byte
	// ceiling. A long test name or TMPDIR must not prevent IPC startup.
	dir, err := os.MkdirTemp("/var/tmp", "sysc-reload-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	runtime.LockOSThread()
	s, err := New(loaded, path, false)
	if err != nil {
		runtime.UnlockOSThread()
		t.Skipf("no nested wayland session available: %v", err)
	}
	t.Cleanup(func() {
		s.Destroy()
		runtime.UnlockOSThread()
	})
	if s.NiriSocket() == "" {
		t.Fatal("test compositor did not start its IPC socket")
	}
	return s
}

func writeConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
