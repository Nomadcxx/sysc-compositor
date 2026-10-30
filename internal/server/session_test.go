package server

import (
	"bufio"
	"os/exec"
	"runtime"

	"github.com/godbus/dbus/v5"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestLocaleKeyboardFillsOnlyUnsetXKBSettings(t *testing.T) {
	locale := config.Keyboard{Model: "pc105", Layout: "us,de", Variant: "", Options: "grp:alt_shift_toggle"}
	configured := config.Keyboard{RepeatDelay: 400, RepeatRate: 40}
	got := keyboardWithLocale(configured, locale)
	if got.Model != locale.Model || got.Layout != locale.Layout || got.Options != locale.Options ||
		got.RepeatDelay != configured.RepeatDelay || got.RepeatRate != configured.RepeatRate {
		t.Fatalf("locale keyboard overlay = %+v, want XKB defaults and configured repeat settings", got)
	}
	configured.Layout = "de"
	got = keyboardWithLocale(configured, locale)
	if got.Layout != "de" || got.Model != "" || got.Options != "" {
		t.Fatalf("explicit KDL XKB settings were overridden: %+v", got)
	}
}

func TestNestedServerDoesNotAcquirePortalNames(t *testing.T) {
	command := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Skipf("dbus-daemon unavailable: %v", err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("private bus did not print its address")
	}
	address := scanner.Text()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	server, err := New(config.LoadResult{}, config.Path{}, false)
	if err != nil {
		t.Skipf("nested compositor unavailable: %v", err)
	}
	defer server.Destroy()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, name := range []string{"org.gnome.Mutter.DisplayConfig", "org.gnome.Mutter.ServiceChannel", "org.gnome.Shell.Introspect", "org.gnome.Shell.Screenshot", "org.gnome.Mutter.ScreenCast", "org.freedesktop.ScreenSaver"} {
		var owned bool
		if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned); err != nil {
			t.Fatal(err)
		}
		if owned {
			t.Errorf("nested startup acquired session name %s", name)
		}
	}
}
