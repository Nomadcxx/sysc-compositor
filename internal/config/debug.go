package config

import (
	"fmt"
	"github.com/sblinch/kdl-go/document"
)

// Debug is Niri 26.04's backend and policy tuning section.
type Debug struct {
	PreviewRender                        *CaptureTarget
	DBusInterfacesInNonSessionInstances  bool
	WaitForFrameCompletionBeforeQueueing bool
	EnableOverlayPlanes                  bool
	DisableCursorPlane                   bool
	DisableDirectScanout                 bool
	// Niri deprecated this setting in 25.11 and no longer changes max bpc.
	KeepMaxBPCUnchanged                    bool
	RestrictPrimaryScanoutToMatchingFormat bool
	ForceDisableConnectorsOnResume         bool
	RenderDRMDevice                        *string
	IgnoredDRMDevices                      []string
	// MemFd casts have no DRM modifiers, so this DMA-BUF tuning flag is a no-op.
	ForcePipeWireInvalidModifier        bool
	EmulateZeroPresentationTime         bool
	DisableResizeThrottling             bool
	DisableTransactions                 bool
	KeepLaptopPanelOnWhenLidIsClosed    bool
	DisableMonitorNames                 bool
	StrictNewWindowFocusPolicy          bool
	HonorXDGActivationWithInvalidSerial bool
	DeactivateUnfocusedWindows          bool
	SkipCursorOnlyUpdatesDuringVRR      bool
}

type ConfigNotification struct{ DisableFailed bool }

// debug merges fields across includes; only ignore-drm-device repeats inside
// one block, and its paths append rather than replacing earlier paths.
func (c *Config) debug(node *document.Node) error {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return fmt.Errorf("takes child nodes only")
	}
	flags := map[string]*bool{
		"dbus-interfaces-in-non-session-instances":    &c.Debug.DBusInterfacesInNonSessionInstances,
		"wait-for-frame-completion-before-queueing":   &c.Debug.WaitForFrameCompletionBeforeQueueing,
		"enable-overlay-planes":                       &c.Debug.EnableOverlayPlanes,
		"disable-cursor-plane":                        &c.Debug.DisableCursorPlane,
		"disable-direct-scanout":                      &c.Debug.DisableDirectScanout,
		"keep-max-bpc-unchanged":                      &c.Debug.KeepMaxBPCUnchanged,
		"restrict-primary-scanout-to-matching-format": &c.Debug.RestrictPrimaryScanoutToMatchingFormat,
		"force-disable-connectors-on-resume":          &c.Debug.ForceDisableConnectorsOnResume,
		"force-pipewire-invalid-modifier":             &c.Debug.ForcePipeWireInvalidModifier,
		"emulate-zero-presentation-time":              &c.Debug.EmulateZeroPresentationTime,
		"disable-resize-throttling":                   &c.Debug.DisableResizeThrottling,
		"disable-transactions":                        &c.Debug.DisableTransactions,
		"keep-laptop-panel-on-when-lid-is-closed":     &c.Debug.KeepLaptopPanelOnWhenLidIsClosed,
		"disable-monitor-names":                       &c.Debug.DisableMonitorNames,
		"strict-new-window-focus-policy":              &c.Debug.StrictNewWindowFocusPolicy,
		"honor-xdg-activation-with-invalid-serial":    &c.Debug.HonorXDGActivationWithInvalidSerial,
		"deactivate-unfocused-windows":                &c.Debug.DeactivateUnfocusedWindows,
		"skip-cursor-only-updates-during-vrr":         &c.Debug.SkipCursorOnlyUpdatesDuringVRR,
	}
	seen := map[string]bool{}
	for _, child := range node.Children {
		name := nodeName(child)
		if name != "ignore-drm-device" && seen[name] {
			return fmt.Errorf("duplicate setting %q", name)
		}
		seen[name] = true
		if len(child.Properties) != 0 || len(child.Children) != 0 {
			return fmt.Errorf("%s takes arguments only", name)
		}
		switch name {
		case "preview-render", "render-drm-device", "ignore-drm-device":
			values, err := stringArgs(child, 1, 1)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			value := values[0]
			switch name {
			case "preview-render":
				var target CaptureTarget
				switch value {
				case "screencast":
					target = CaptureCast
				case "screen-capture":
					target = CaptureScreen
				default:
					return fmt.Errorf("preview-render must be screencast or screen-capture")
				}
				c.Debug.PreviewRender = &target
			case "render-drm-device":
				c.Debug.RenderDRMDevice = &value
			case "ignore-drm-device":
				c.Debug.IgnoredDRMDevices = append(c.Debug.IgnoredDRMDevices, value)
			}
		default:
			field := flags[name]
			if field == nil {
				return fmt.Errorf("unexpected node `%s`", name)
			}
			value, err := flagArg(child)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			*field = value
		}
	}
	return nil
}

func (c *Config) configNotification(node *document.Node) error {
	if len(node.Arguments) != 0 || len(node.Properties) != 0 {
		return fmt.Errorf("takes child nodes only")
	}
	seen := false
	for _, child := range node.Children {
		if name := nodeName(child); name != "disable-failed" {
			return fmt.Errorf("unexpected node `%s`", name)
		}
		if seen {
			return fmt.Errorf("duplicate setting disable-failed")
		}
		seen = true
		value, err := flagArg(child)
		if err != nil {
			return err
		}
		c.ConfigNotification.DisableFailed = value
	}
	return nil
}
