package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestSpawnUsesReservedX11Display(t *testing.T) {
	t.Setenv("DISPLAY", ":host")
	t.Setenv("WAYLAND_SOCKET", "3")
	for _, tc := range []struct {
		display    string
		configured map[string]*string
		want       string
	}{
		{":23", nil, ":23"},
		{"", nil, ""},
		{":23", map[string]*string{"DISPLAY": nil}, ""},
		{":23", map[string]*string{"DISPLAY": func() *string { s := ":override"; return &s }()}, ":override"},
	} {
		s := newSpawner("wayland-test", "", tc.configured, config.Default().Cursor, tc.display)
		got := ""
		for _, item := range s.childEnvironment() {
			if strings.HasPrefix(item, "WAYLAND_SOCKET=") {
				t.Fatal("child inherited the host's Wayland descriptor")
			}
			if value, ok := strings.CutPrefix(item, "DISPLAY="); ok {
				got = value
			}
		}
		if got != tc.want {
			t.Errorf("DISPLAY = %q, want %q", got, tc.want)
		}
	}
}

func TestSpawnShSeesTheCompositorEnvironment(t *testing.T) {
	t.Setenv("DISPLAY", ":9")
	t.Setenv("NOTIFY_SOCKET", "/run/notify")
	t.Setenv("WAYLAND_DISPLAY", "host-wayland")
	t.Setenv("SYSC_KEEP", "kept")
	t.Setenv("SYSC_DROP", "dropped")
	out := filepath.Join(t.TempDir(), "env")
	val := func(v string) *string { return &v }
	s := newSpawner("wayland-9", "/run/user/1000/niri.wayland-9.1.sock",
		map[string]*string{"SYSC_ADDED": val("added"), "SYSC_DROP": nil}, config.Default().Cursor, "")
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
	if err := newSpawner("w", "", nil, config.Default().Cursor, "").spawn([]string{"/nonexistent/sysc-spawn-test"}); err == nil {
		t.Fatal("spawning a missing program returned no error")
	}
	if err := newSpawner("w", "", nil, config.Default().Cursor, "").spawn(nil); err != nil {
		t.Fatalf("an empty command is a no-op in Niri, got %v", err)
	}
}

func TestSpawnerPropagatesCursorConfigToChildren(t *testing.T) {
	val := func(v string) *string { return &v }
	configured := map[string]*string{"XCURSOR_THEME": val("ChildTheme")}
	s := newSpawner("wayland-9", "", configured, config.Cursor{Theme: "Adwaita", Size: 32}, "")
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
	s := newSpawner("wayland-old", "/old.sock", nil, config.Default().Cursor, ":23")
	handlerCopy := s
	value := "reloaded"
	s.updateEnvironment("wayland-new", "/new.sock", map[string]*string{"SYSC_RELOAD_TEST": &value}, config.Default().Cursor, ":24")
	got := make(map[string]string)
	for _, item := range handlerCopy.childEnvironment() {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			got[name] = value
		}
	}
	if got["WAYLAND_DISPLAY"] != "wayland-new" || got["NIRI_SOCKET"] != "/new.sock" || got["SYSC_RELOAD_TEST"] != "reloaded" || got["DISPLAY"] != ":24" {
		t.Fatalf("updated child environment = %v", got)
	}
}

// TestSpawnerFollowsTheSessionEnvironment: --session sets XDG_CURRENT_DESKTOP,
// XDG_SESSION_TYPE and SYSC_SOCKET after the spawner exists, and children
// must see them, as Niri's do. Without XDG_SESSION_TYPE, Firefox's WebRTC
// assumes X11 and finds nothing to share through the portal.
func TestSpawnerFollowsTheSessionEnvironment(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "")
	s := newSpawner("wayland-9", "", nil, config.Default().Cursor, "")
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("XDG_CURRENT_DESKTOP", "niri:sysc")
	got := make(map[string]string)
	for _, item := range s.childEnvironment() {
		if name, value, ok := strings.Cut(item, "="); ok {
			got[name] = value
		}
	}
	if got["XDG_SESSION_TYPE"] != "wayland" || got["XDG_CURRENT_DESKTOP"] != "niri:sysc" {
		t.Fatalf("child environment missed the session variables: XDG_SESSION_TYPE=%q XDG_CURRENT_DESKTOP=%q",
			got["XDG_SESSION_TYPE"], got["XDG_CURRENT_DESKTOP"])
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
