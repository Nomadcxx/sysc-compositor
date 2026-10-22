package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestSpawnShSeesTheCompositorEnvironment(t *testing.T) {
	t.Setenv("DISPLAY", ":9")
	t.Setenv("NOTIFY_SOCKET", "/run/notify")
	t.Setenv("WAYLAND_DISPLAY", "host-wayland")
	t.Setenv("SYSC_KEEP", "kept")
	t.Setenv("SYSC_DROP", "dropped")
	out := filepath.Join(t.TempDir(), "env")
	val := func(v string) *string { return &v }
	s := newSpawner("wayland-9", "/run/user/1000/niri.wayland-9.1.sock",
		map[string]*string{"SYSC_ADDED": val("added"), "SYSC_DROP": nil}, config.Default().Cursor)
	err := s.spawnSh(`printf '%s|%s|%s|%s|%s|%s|%s' "$WAYLAND_DISPLAY" "$NIRI_SOCKET" "${DISPLAY-unset}" ` +
		`"${NOTIFY_SOCKET-unset}" "$SYSC_KEEP" "$SYSC_ADDED" "${SYSC_DROP-unset}" > ` + out)
	if err != nil {
		t.Fatal(err)
	}
	want := "wayland-9|/run/user/1000/niri.wayland-9.1.sock|unset|unset|kept|added|unset"
	var got string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(out); err == nil && strings.Count(string(b), "|") == 6 {
			got = string(b)
			break
		}
	}
	if got != want {
		t.Fatalf("child saw %q, want %q", got, want)
	}
}

func TestSpawnReportsAMissingProgram(t *testing.T) {
	if err := newSpawner("w", "", nil, config.Default().Cursor).spawn([]string{"/nonexistent/sysc-spawn-test"}); err == nil {
		t.Fatal("spawning a missing program returned no error")
	}
	if err := newSpawner("w", "", nil, config.Default().Cursor).spawn(nil); err != nil {
		t.Fatalf("an empty command is a no-op in Niri, got %v", err)
	}
}

func TestSpawnerPropagatesCursorConfigToChildren(t *testing.T) {
	val := func(v string) *string { return &v }
	configured := map[string]*string{"XCURSOR_THEME": val("ChildTheme")}
	s := newSpawner("wayland-9", "", configured, config.Cursor{Theme: "Adwaita", Size: 32})
	got := make(map[string]string)
	for _, item := range s.childEnvironment() {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			got[name] = value
		}
	}
	if got["XCURSOR_THEME"] != "ChildTheme" || got["XCURSOR_SIZE"] != "32" {
		t.Fatalf("child cursor environment = %v, want explicit theme override and configured size", got)
	}
}

func TestSpawnerEnvironmentUpdateAffectsExistingHandlers(t *testing.T) {
	s := newSpawner("wayland-old", "/old.sock", nil, config.Default().Cursor)
	handlerCopy := s
	value := "reloaded"
	s.updateEnvironment("wayland-new", "/new.sock", map[string]*string{"SYSC_RELOAD_TEST": &value}, config.Default().Cursor)
	got := make(map[string]string)
	for _, item := range handlerCopy.childEnvironment() {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			got[name] = value
		}
	}
	if got["WAYLAND_DISPLAY"] != "wayland-new" || got["NIRI_SOCKET"] != "/new.sock" || got["SYSC_RELOAD_TEST"] != "reloaded" {
		t.Fatalf("updated child environment = %v", got)
	}
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	for in, want := range map[string]string{"~": "/home/test", "~/bin/x": "/home/test/bin/x", "kitty": "kitty", "~x": "~x"} {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
