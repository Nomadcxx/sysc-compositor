package session

import (
	"bufio"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/godbus/dbus/v5"
)

type testProperties struct{ values map[string]dbus.Variant }

func (p testProperties) GetAll(string) (map[string]dbus.Variant, *dbus.Error) {
	return p.values, nil
}

func TestDBusReadsAndTracksLocaleAndLidProperties(t *testing.T) {
	address := startPrivateBus(t)
	service, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	for _, item := range []struct {
		name       string
		path       dbus.ObjectPath
		properties testProperties
	}{
		{localeBus, localePath, testProperties{values: map[string]dbus.Variant{
			"X11Model": dbus.MakeVariant("pc105"), "X11Layout": dbus.MakeVariant("us,de"),
			"X11Variant": dbus.MakeVariant(""), "X11Options": dbus.MakeVariant("grp:alt_shift_toggle"),
		}}},
		{loginBus, loginPath, testProperties{values: map[string]dbus.Variant{"LidClosed": dbus.MakeVariant(false)}}},
	} {
		if err := service.Export(item.properties, item.path, propertiesIface); err != nil {
			t.Fatal(err)
		}
		if _, err := service.RequestName(item.name, dbus.NameFlagDoNotQueue); err != nil {
			t.Fatal(err)
		}
	}
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	xkbUpdates := make(chan config.Keyboard, 4)
	lidUpdates := make(chan bool, 4)
	watcher, err := startDBus(client, func(k config.Keyboard) { xkbUpdates <- k }, func(closed bool) { lidUpdates <- closed })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(watcher.Close)
	if got := receive(t, xkbUpdates); got.Layout != "us,de" || got.Model != "pc105" || got.Options != "grp:alt_shift_toggle" {
		t.Fatalf("initial locale1 keyboard = %+v", got)
	}
	if got := receive(t, lidUpdates); got {
		t.Fatal("initial login1 LidClosed = true, want false")
	}
	if err := service.Emit(localePath, propertiesIface+".PropertiesChanged", localeIface,
		map[string]dbus.Variant{"X11Layout": dbus.MakeVariant("fr")}, []string{}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, xkbUpdates); got.Layout != "fr" || got.Model != "pc105" {
		t.Fatalf("changed locale1 keyboard = %+v", got)
	}
	if err := service.Emit(loginPath, propertiesIface+".PropertiesChanged", loginIface,
		map[string]dbus.Variant{"LidClosed": dbus.MakeVariant(true)}, []string{}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, lidUpdates); !got {
		t.Fatal("changed login1 LidClosed = false, want true")
	}
}

func startPrivateBus(t *testing.T) string {
	t.Helper()
	command := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Skipf("dbus-daemon unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatalf("dbus-daemon did not print an address: %v", scanner.Err())
	}
	return scanner.Text()
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case value := <-ch:
		return value
	case <-ctx.Done():
		var zero T
		t.Fatal("timed out waiting for D-Bus property update")
		return zero
	}
}
