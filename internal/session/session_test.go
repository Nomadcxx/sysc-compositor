package session

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSessionLifecyclePublishesEnvironmentBeforeStartingTarget(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "commands.log")
	t.Setenv("SYSC_SESSION_TEST_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"systemctl", "dbus-update-activation-environment"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s %s\\n' \"${0##*/}\" \"$*\" >> \"$SYSC_SESSION_TEST_LOG\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	lifecycle, err := Activate(Environment{
		WaylandDisplay: "wayland-sysc", NiriSocket: "/run/user/1000/niri.sock",
		SyscSocket: "/run/user/1000/sysc.sock", Display: ":3",
	})
	if err != nil {
		t.Fatal(err)
	}
	first := readCommandLog(t, logPath)
	wantStart := []string{
		"dbus-update-activation-environment WAYLAND_DISPLAY=wayland-sysc NIRI_SOCKET=/run/user/1000/niri.sock SYSC_SOCKET=/run/user/1000/sysc.sock XDG_CURRENT_DESKTOP=niri:sysc XDG_SESSION_TYPE=wayland DISPLAY=:3",
		"systemctl --user import-environment WAYLAND_DISPLAY NIRI_SOCKET SYSC_SOCKET XDG_CURRENT_DESKTOP XDG_SESSION_TYPE DISPLAY",
		"systemctl --user start sysc-session.target",
	}
	if !reflect.DeepEqual(first, wantStart) {
		t.Fatalf("session start commands = %q, want %q", first, wantStart)
	}
	lifecycle.Close()
	wantAll := append(wantStart,
		"systemctl --user --no-block stop graphical-session.target",
		"systemctl --user unset-environment WAYLAND_DISPLAY NIRI_SOCKET SYSC_SOCKET XDG_CURRENT_DESKTOP XDG_SESSION_TYPE DISPLAY",
		"dbus-update-activation-environment WAYLAND_DISPLAY= NIRI_SOCKET= SYSC_SOCKET= XDG_CURRENT_DESKTOP= XDG_SESSION_TYPE= DISPLAY=",
	)
	if got := readCommandLog(t, logPath); !reflect.DeepEqual(got, wantAll) {
		t.Fatalf("session cleanup commands = %q, want %q", got, wantAll)
	}
}

func TestSessionActivationRequiresBothSockets(t *testing.T) {
	if _, err := Activate(Environment{WaylandDisplay: "wayland-1", NiriSocket: "/tmp/niri.sock"}); err == nil {
		t.Fatal("Activate() succeeded without the sysc socket")
	}
}

func readCommandLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}
