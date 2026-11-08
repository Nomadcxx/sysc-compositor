package session

import (
	"context"
	"errors"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/godbus/dbus/v5"
)

func TestDisplayConfigAppliesAndVerifiesTransientChanges(t *testing.T) {
	calls := 0
	fail := false
	p, client := testPortal(t, PortalHooks{ConfigureOutputs: func(_ context.Context, serial uint32, changes []config.OutputConfig, verify bool) error {
		calls++
		if serial == 0 || len(changes) != 2 {
			t.Errorf("changes = %+v, serial %d", changes, serial)
		}
		var enabled, disabled bool
		for _, change := range changes {
			if change.Name == "DP-1" {
				enabled = !change.Off && change.Mode != nil && change.Mode.Width == 1920 && change.Scale != nil && *change.Scale == 1.5 && change.Transform == config.Transform90 && change.Position.X == -100
			}
			if change.Name == "DP-2" {
				disabled = change.Off
			}
		}
		if !enabled || !disabled || verify != (calls == 1) {
			t.Errorf("changes = %+v, verify %t", changes, verify)
		}
		if fail {
			return errors.New("refused")
		}
		return nil
	}})
	p.Update(map[string]ipc.Output{"DP-1": {Name: "DP-1", Modes: []ipc.Mode{{Width: 1920, Height: 1080, RefreshRate: 60000}}}, "DP-2": {Name: "DP-2"}}, nil)
	serial := p.CurrentDisplaySerial()
	logical := []LogicalMonitorConfiguration{{X: -100, Y: 20, Scale: 1.5, Transform: 1, Monitors: []MonitorConfiguration{{Connector: "DP-1", Mode: "1920x1080@60.000"}}}}
	object := client.Object(displayConfigName, displayConfigPath)
	for _, method := range []uint32{0, 1, 2} {
		if err := object.Call(displayConfigName+".ApplyMonitorsConfig", 0, serial, method, logical, map[string]dbus.Variant{}).Err; err != nil {
			t.Fatal(err)
		}
	}
	fail = true
	if err := object.Call(displayConfigName+".ApplyMonitorsConfig", 0, serial, uint32(1), logical, map[string]dbus.Variant{}).Err; err == nil {
		t.Fatal("backend failure succeeded")
	}
	for _, stale := range []uint32{0, serial + 1} {
		if err := object.Call(displayConfigName+".ApplyMonitorsConfig", 0, stale, uint32(1), logical, map[string]dbus.Variant{}).Err; err == nil {
			t.Fatal("stale serial succeeded")
		}
	}
	if calls != 4 {
		t.Fatalf("backend called %d times", calls)
	}
}
