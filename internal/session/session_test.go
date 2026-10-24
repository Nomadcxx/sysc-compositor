package session

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionLifecyclePublishesAndClearsEnvironment(t *testing.T) {
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
	}
	if !reflect.DeepEqual(first, wantStart) {
		t.Fatalf("session start commands = %q, want %q", first, wantStart)
	}
	lifecycle.Close()
	wantAll := append(wantStart,
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

func TestNotifyReadySendsReadyToNotifySocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	t.Setenv("NOTIFY_SOCKET", path)
	if err := NotifyReady(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("notification = %q, want READY=1", got)
	}
}

func TestNotifyReadyWithoutNotifySocketDoesNothing(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	if err := NotifyReady(); err != nil {
		t.Fatalf("NotifyReady() = %v, want nil outside a notify service", err)
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
