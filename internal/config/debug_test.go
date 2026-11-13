package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDebugConfigurationRejectsUnknownAndMalformed(t *testing.T) {
	for _, text := range []string{
		`debug 1 { disable-direct-scanout; }`,
		`debug key=true { disable-direct-scanout; }`,
		`debug { unknown; }`,
		`debug { preview-render "other"; }`,
		`debug { preview-render "screencast" "screen-capture"; }`,
		`debug { preview-render "screencast" extra=true; }`,
		`debug { render-drm-device 7; }`,
		`debug { render-drm-device "/dev/dri/card1" { bad; }; }`,
		`debug { ignore-drm-device "/dev/dri/card1" extra=true; }`,
		`debug { disable-direct-scanout "false"; }`,
		`debug { disable-cursor-plane 1; }`,
		`debug { disable-transactions extra=true; }`,
		`debug { enable-overlay-planes { bad; }; }`,
		`debug { strict-new-window-focus-policy; strict-new-window-focus-policy false; }`,
		`debug { preview-render "screencast"; preview-render "screen-capture"; }`,
		`config-notification 1`,
		`config-notification extra=true {}`,
		`config-notification { unknown; }`,
		`config-notification { disable-failed 1; }`,
		`config-notification { disable-failed; disable-failed false; }`,
		`input { workspace-auto-back-and-forth 1; }`,
		`input { workspace-auto-back-and-forth; workspace-auto-back-and-forth false; }`,
	} {
		t.Run(text, func(t *testing.T) {
			if _, err := Parse("bad.kdl", []byte(text)); err == nil {
				t.Fatalf("accepted %s", text)
			}
		})
	}
}

func TestDebugConfigurationMergesAcrossIncludes(t *testing.T) {
	dir := t.TempDir()
	base := `debug {
 preview-render "screencast"
 dbus-interfaces-in-non-session-instances
 wait-for-frame-completion-before-queueing
 enable-overlay-planes
 disable-cursor-plane
 disable-direct-scanout
 keep-max-bpc-unchanged
 restrict-primary-scanout-to-matching-format
 force-disable-connectors-on-resume
 render-drm-device "/dev/dri/card1"
 ignore-drm-device "/dev/dri/card2"
 force-pipewire-invalid-modifier
 emulate-zero-presentation-time
 disable-resize-throttling
 disable-transactions
 keep-laptop-panel-on-when-lid-is-closed
 disable-monitor-names
 strict-new-window-focus-policy
 honor-xdg-activation-with-invalid-serial
 deactivate-unfocused-windows
 skip-cursor-only-updates-during-vrr
}
config-notification { disable-failed; }
input { workspace-auto-back-and-forth; }
`
	if err := os.WriteFile(filepath.Join(dir, "base.kdl"), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(filepath.Join(dir, "config.kdl"), []byte(`include "base.kdl"
debug {
 preview-render "screen-capture"
 disable-direct-scanout false
 render-drm-device "/dev/dri/card3"
 ignore-drm-device "/dev/dri/card4"
 ignore-drm-device "/dev/dri/card5"
}
config-notification { disable-failed false; }
input { workspace-auto-back-and-forth false; }
`))
	if err != nil {
		t.Fatal(err)
	}
	target, device := CaptureScreen, "/dev/dri/card3"
	want := Debug{
		PreviewRender: &target, DBusInterfacesInNonSessionInstances: true, WaitForFrameCompletionBeforeQueueing: true,
		EnableOverlayPlanes: true, DisableCursorPlane: true, DisableDirectScanout: false, KeepMaxBPCUnchanged: true,
		RestrictPrimaryScanoutToMatchingFormat: true, ForceDisableConnectorsOnResume: true, RenderDRMDevice: &device,
		IgnoredDRMDevices:            []string{"/dev/dri/card2", "/dev/dri/card4", "/dev/dri/card5"},
		ForcePipeWireInvalidModifier: true, EmulateZeroPresentationTime: true, DisableResizeThrottling: true,
		DisableTransactions: true, KeepLaptopPanelOnWhenLidIsClosed: true, DisableMonitorNames: true,
		StrictNewWindowFocusPolicy: true, HonorXDGActivationWithInvalidSerial: true, DeactivateUnfocusedWindows: true,
		SkipCursorOnlyUpdatesDuringVRR: true,
	}
	if !reflect.DeepEqual(cfg.Debug, want) {
		t.Fatalf("debug=%+v, want %+v", cfg.Debug, want)
	}
	if cfg.ConfigNotification.DisableFailed || cfg.Input.WorkspaceAutoBackAndForth {
		t.Fatalf("explicit false did not override flags: %+v %+v", cfg.ConfigNotification, cfg.Input)
	}
	// Every debug setting is applied; Niri documents keep-max-bpc-unchanged
	// as a no-op since 25.11, so it does not warn either.
	for _, warning := range cfg.Warnings {
		if strings.HasPrefix(warning, "debug") {
			t.Errorf("unexpected debug warning %q", warning)
		}
	}
}

func TestCloseoutConfigFlagsDefaultAndRemoval(t *testing.T) {
	cfg, err := Parse("flags.kdl", []byte(`debug { preview-render "screencast"; disable-direct-scanout; }
config-notification { disable-failed; }
input { workspace-auto-back-and-forth; }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Debug.PreviewRender == nil || *cfg.Debug.PreviewRender != CaptureCast || !cfg.Debug.DisableDirectScanout || !cfg.ConfigNotification.DisableFailed || !cfg.Input.WorkspaceAutoBackAndForth {
		t.Fatalf("flags were not applied: %+v", cfg)
	}
	cfg, err = Parse("removed.kdl", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Debug, Debug{}) || cfg.ConfigNotification.DisableFailed || cfg.Input.WorkspaceAutoBackAndForth {
		t.Fatalf("removed settings survived: %+v", cfg)
	}
}
