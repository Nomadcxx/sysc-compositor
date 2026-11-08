package session

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/godbus/dbus/v5"
)

type MonitorNames struct{ Connector, Make, Model, Serial string }
type MonitorMode struct {
	ID                          string
	Width, Height               int32
	RefreshRate, PreferredScale float64
	SupportedScales             []float64
	Properties                  map[string]dbus.Variant
}
type Monitor struct {
	Names      MonitorNames
	Modes      []MonitorMode
	Properties map[string]dbus.Variant
}
type LogicalMonitor struct {
	X, Y       int32
	Scale      float64
	Transform  uint32
	Primary    bool
	Monitors   []MonitorNames
	Properties map[string]dbus.Variant
}
type MonitorConfiguration struct {
	Connector, Mode string
	Properties      map[string]dbus.Variant
}
type LogicalMonitorConfiguration struct {
	X, Y      int32
	Scale     float64
	Transform uint32
	Primary   bool
	Monitors  []MonitorConfiguration
}
type displayState struct {
	Serial      uint32
	Monitors    []Monitor
	Logical     []LogicalMonitor
	Geometry    map[string]ipc.LogicalOutput
	CustomModes map[string]string
}

type portalDisplayConfig struct{ p *Portal }

func (s portalDisplayConfig) GetCurrentState() (uint32, []Monitor, []LogicalMonitor, map[string]dbus.Variant, *dbus.Error) {
	s.p.mu.Lock()
	state := s.p.state
	s.p.mu.Unlock()
	return state.Serial, state.Monitors, state.Logical, map[string]dbus.Variant{"layout-mode": dbus.MakeVariant(uint32(1))}, nil
}
func (s portalDisplayConfig) ApplyMonitorsConfig(serial, method uint32, logical []LogicalMonitorConfiguration, properties map[string]dbus.Variant) *dbus.Error {
	if s.p.hooks.ConfigureOutputs == nil {
		return unsupported("Applying monitor configuration is not available")
	}
	s.p.mu.Lock()
	state := s.p.state
	s.p.mu.Unlock()
	invalid := func(message string) *dbus.Error {
		return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{message})
	}
	if serial != state.Serial {
		return invalid("stale monitor configuration serial")
	}
	if method > 2 {
		return invalid("unknown configuration method")
	}
	if len(logical) == 0 {
		return invalid("at least one output must be enabled")
	}
	if mode, ok := properties["layout-mode"]; ok && mode.Value() != uint32(1) {
		return invalid("only logical layout mode is supported")
	}
	seen := map[string]bool{}
	var changes []config.OutputConfig
	for _, group := range logical {
		if len(group.Monitors) != 1 {
			return invalid("each logical monitor must contain exactly one physical monitor")
		}
		if math.IsNaN(group.Scale) || math.IsInf(group.Scale, 0) || group.Scale <= 0 || group.Scale > 10 || group.Transform > 7 {
			return invalid("invalid monitor scale or transform")
		}
		monitor := group.Monitors[0]
		if seen[monitor.Connector] {
			return invalid("duplicate monitor")
		}
		seen[monitor.Connector] = true
		var mode *config.OutputMode
		for _, connected := range state.Monitors {
			if connected.Names.Connector != monitor.Connector {
				continue
			}
			for _, advertised := range connected.Modes {
				if advertised.ID == monitor.Mode {
					mode = &config.OutputMode{Width: int(advertised.Width), Height: int(advertised.Height), Refresh: advertised.RefreshRate, Custom: state.CustomModes[monitor.Connector] == monitor.Mode}
				}
			}
		}
		if mode == nil {
			return invalid(fmt.Sprintf("unknown monitor or mode: %s %s", monitor.Connector, monitor.Mode))
		}
		scale := group.Scale
		changes = append(changes, config.OutputConfig{Name: monitor.Connector, Mode: mode, Scale: &scale, Transform: config.Transform(group.Transform), Position: &config.OutputPosition{X: int(group.X), Y: int(group.Y)}})
	}
	for _, monitor := range state.Monitors {
		if !seen[monitor.Names.Connector] {
			changes = append(changes, config.OutputConfig{Name: monitor.Names.Connector, Off: true})
		}
	}
	if err := s.p.hooks.ConfigureOutputs(s.p.ctx, serial, changes, method == 0); err != nil {
		return dbus.NewError("org.freedesktop.DBus.Error.Failed", []any{err.Error()})
	}
	return nil
}

func (p *Portal) CurrentDisplaySerial() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state.Serial
}

// Update consumes immutable IPC snapshots on the compositor thread. The D-Bus
// representation owns its maps/slices, so neither readers nor callers share a
// mutable compositor object. Signals follow the new snapshot.
func (p *Portal) Update(outputs map[string]ipc.Output, windows []ipc.Window) {
	state := displayState{Monitors: []Monitor{}, Logical: []LogicalMonitor{}, Geometry: map[string]ipc.LogicalOutput{}, CustomModes: map[string]string{}}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		output := outputs[name]
		serial := name
		if output.Serial != nil {
			serial = *output.Serial
		}
		ids := MonitorNames{name, output.Make, output.Model, serial}
		builtin := strings.HasPrefix(name, "eDP-") || strings.HasPrefix(name, "LVDS-") || strings.HasPrefix(name, "DSI-")
		label := output.Make
		if builtin {
			label = "Built-in display"
		} else if output.PhysicalSize != nil {
			label += " " + displayDiagonal(*output.PhysicalSize)
		} else if output.Model != "Unknown" {
			label += " " + output.Model
		}
		monitor := Monitor{Names: ids, Modes: []MonitorMode{}, Properties: map[string]dbus.Variant{"display-name": dbus.MakeVariant(label), "is-builtin": dbus.MakeVariant(builtin)}}
		for i, mode := range output.Modes {
			hz := float64(mode.RefreshRate) / 1000
			properties := map[string]dbus.Variant{"is-preferred": dbus.MakeVariant(mode.IsPreferred)}
			if output.CurrentMode != nil && uint32(i) == *output.CurrentMode {
				properties["is-current"] = dbus.MakeVariant(true)
			}
			monitor.Modes = append(monitor.Modes, MonitorMode{ID: strconv.Itoa(int(mode.Width)) + "x" + strconv.Itoa(int(mode.Height)) + "@" + strconv.FormatFloat(hz, 'f', 3, 64), Width: int32(mode.Width), Height: int32(mode.Height), RefreshRate: hz, PreferredScale: 1, SupportedScales: supportedScales(mode.Width, mode.Height), Properties: properties})
		}
		state.Monitors = append(state.Monitors, monitor)
		if output.IsCustomMode && output.CurrentMode != nil && int(*output.CurrentMode) < len(monitor.Modes) {
			state.CustomModes[name] = monitor.Modes[*output.CurrentMode].ID
		}
		if logical := output.Logical; logical != nil {
			state.Geometry[name] = *logical
			transform := uint32(0)
			for i, value := range []ipc.Transform{ipc.TransformNormal, ipc.Transform90, ipc.Transform180, ipc.Transform270, ipc.TransformFlipped, ipc.TransformFlipped90, ipc.TransformFlipped180, ipc.TransformFlipped270} {
				if value == logical.Transform {
					transform = uint32(i)
				}
			}
			state.Logical = append(state.Logical, LogicalMonitor{X: logical.X, Y: logical.Y, Scale: logical.Scale, Transform: transform, Monitors: []MonitorNames{ids}, Properties: map[string]dbus.Variant{}})
		}
	}
	introspected := make(map[uint64]map[string]dbus.Variant, len(windows))
	for _, window := range windows {
		title, app := "", ""
		if window.Title != nil {
			title = *window.Title
		}
		if window.AppID != nil {
			app = *window.AppID + ".desktop"
		}
		introspected[window.ID] = map[string]dbus.Variant{"title": dbus.MakeVariant(title), "app-id": dbus.MakeVariant(app)}
	}
	p.mu.Lock()
	if p.screenCast != nil {
		introspected[p.screenCast.dynamicID] = dynamicCastWindow()
	}
	state.Serial = p.state.Serial
	monitorsChanged := !reflect.DeepEqual(state, p.state)
	if monitorsChanged {
		state.Serial++
		if state.Serial == 0 {
			state.Serial = 1
		}
		p.state = state
	}
	windowsChanged := !reflect.DeepEqual(introspected, p.windows)
	p.windows = introspected
	stopped := p.ctx.Err() != nil
	p.mu.Unlock()
	if stopped {
		return
	}
	if monitorsChanged {
		_ = p.conn.Emit(displayConfigPath, displayConfigName+".MonitorsChanged")
	}
	if windowsChanged {
		_ = p.conn.Emit(introspectPath, introspectName+".WindowsChanged")
	}
}

func displayDiagonal(size [2]uint32) string {
	inches := math.Hypot(float64(size[0]), float64(size[1])) / 25.4
	for _, known := range []float64{12.1, 13.3, 15.6} {
		if math.Abs(inches-known) < .1 {
			return strconv.FormatFloat(known, 'f', 1, 64) + "″"
		}
	}
	return strconv.FormatFloat(math.Round(inches), 'f', 0, 64) + "″"
}

// Niri 26.04 advertises quarter steps from 1 through 4 whose rounded
// logical resolution has at least 800*480 pixels.
func supportedScales(width, height uint16) []float64 {
	scales := []float64{}
	for quarter := 4; quarter <= 16; quarter++ {
		scale := float64(quarter) / 4
		if math.Round(float64(width)/scale)*math.Round(float64(height)/scale) >= 800*480 {
			scales = append(scales, scale)
		}
	}
	return scales
}
