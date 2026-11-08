package wlr

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestOutputManagementPublishesTestsAndApplies(t *testing.T) {
	c := newNested(t)
	name := addConfigurableTestOutput(t, c)
	if err := c.Reconfigure(Options{Outputs: []config.OutputConfig{{Name: name, VRR: &config.VRR{OnDemand: true}}}}); err != nil {
		t.Fatal(err)
	}
	for _, verify := range []bool{true, false} {
		runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
			heads, applied, err := testclient.ConfigureOutputScale(c.SocketName(), name, 2, verify)
			if err != nil || !applied || heads != 2 {
				t.Fatalf("verify=%t: heads %d, applied %t, err %v", verify, heads, applied, err)
			}
		})
		output, _ := outputSnapshotNamed(c, name)
		want := 2.0
		if verify {
			want = 1
		}
		if output.Logical == nil || output.Logical.Scale != want {
			t.Fatalf("verify=%t: output %+v", verify, output)
		}
		if section := c.outputConfigByName(name); section == nil || section.VRR == nil || !section.VRR.OnDemand {
			t.Fatal("scale request erased on-demand VRR")
		}
	}
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		_, ok, err := testclient.ConfigureOutputScale(c.SocketName(), name, 0, false)
		if err != nil || !ok {
			t.Errorf("disable: %t %v", ok, err)
		}
	})
	if _, err := c.ConfigureOutput(ipc.OutputRequest{Name: name, Action: json.RawMessage(`"On"`)}); err != nil {
		t.Fatal(err)
	}
	if output, _ := outputSnapshotNamed(c, name); output.Logical == nil || output.Logical.Scale != 2 {
		t.Fatalf("protocol off lost the scale: %+v", output)
	}
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		if err := testclient.DisableOutputVRR(c.SocketName(), name); err != nil {
			t.Error(err)
		}
	})
	if section := c.outputConfigByName(name); section.VRR != nil {
		t.Fatal("explicit adaptive-sync off retained on-demand VRR")
	}
}

func TestOutputPowerProtocolKeepsTheOutputLayout(t *testing.T) {
	c := newNested(t)
	addConfigurableTestOutput(t, c)
	for _, enabled := range []bool{false, true} {
		runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
			if err := testclient.SetOutputPower(c.SocketName(), 1, enabled); err != nil {
				t.Error(err)
			}
		})
		want := 1
		if enabled {
			want = 2
		}
		if c.EnabledOutputs() != want || len(c.sceneOutputs) != 2 {
			t.Fatalf("power=%t: enabled=%d layout=%d", enabled, c.EnabledOutputs(), len(c.sceneOutputs))
		}
	}
}
