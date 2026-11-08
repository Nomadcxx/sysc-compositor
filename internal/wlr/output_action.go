package wlr

import (
	"encoding/json"
	"fmt"
	"math"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// editOutputConfig is Niri's apply_transient_output_config. Decode the wire at
// the boundary before touching the live list; invalid edits leave it alone.
func editOutputConfig(section config.OutputConfig, raw json.RawMessage) (config.OutputConfig, error) {
	request := ipc.Request{Name: "Output", Output: &ipc.OutputRequest{Name: section.Name, Action: raw}}
	if _, err := ipc.DecodeRequest(ipc.EncodeRequest(request)); err != nil {
		return section, err
	}
	var unit string
	if json.Unmarshal(raw, &unit) == nil {
		section.Off = unit == "Off"
		return section, nil
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(raw, &object)
	for name, body := range object {
		var fields struct {
			Mode, Scale, Position json.RawMessage
			Transform             string
			Vrr                   bool
			OnDemand              bool `json:"on_demand"`
		}
		_ = json.Unmarshal(body, &fields)
		specific := func(raw json.RawMessage, target any) bool {
			var choice map[string]json.RawMessage
			if json.Unmarshal(raw, &choice) != nil {
				return false
			}
			value, ok := choice["Specific"]
			return ok && json.Unmarshal(value, target) == nil
		}
		switch name {
		case "Mode", "CustomMode":
			var mode config.OutputMode
			selected := false
			if name == "CustomMode" {
				selected = json.Unmarshal(fields.Mode, &mode) == nil
				mode.Custom = true
			} else {
				selected = specific(fields.Mode, &mode)
			}
			section.Mode, section.Modeline = nil, nil
			if selected {
				if mode.Width <= 0 || mode.Height <= 0 || mode.Refresh < 0 || mode.Refresh > float64(math.MaxInt32)/1000 || (mode.Custom && mode.Refresh <= 0) {
					return section, fmt.Errorf("invalid output mode or refresh rate")
				}
				section.Mode = &mode
			}
		case "Modeline":
			var m struct {
				Clock                              float64
				HDisplay, HTotal, VDisplay, VTotal int
				HSyncStart                         int    `json:"hsync_start"`
				HSyncEnd                           int    `json:"hsync_end"`
				VSyncStart                         int    `json:"vsync_start"`
				VSyncEnd                           int    `json:"vsync_end"`
				HSyncPolarity                      string `json:"hsync_polarity"`
				VSyncPolarity                      string `json:"vsync_polarity"`
			}
			_ = json.Unmarshal(body, &m)
			if m.Clock <= 0 || m.Clock > float64(math.MaxUint32)/1000 || m.HDisplay <= 0 || !(m.HDisplay < m.HSyncStart && m.HSyncStart < m.HSyncEnd && m.HSyncEnd < m.HTotal) || m.VDisplay <= 0 || !(m.VDisplay < m.VSyncStart && m.VSyncStart < m.VSyncEnd && m.VSyncEnd < m.VTotal) {
				return section, fmt.Errorf("invalid modeline clock or timings")
			}
			section.Modeline = &config.Modeline{Clock: m.Clock, HDisplay: m.HDisplay, HSyncStart: m.HSyncStart, HSyncEnd: m.HSyncEnd, HTotal: m.HTotal, VDisplay: m.VDisplay, VSyncStart: m.VSyncStart, VSyncEnd: m.VSyncEnd, VTotal: m.VTotal, HSyncPositive: m.HSyncPolarity == "PHSync", VSyncPositive: m.VSyncPolarity == "PVSync"}
		case "Scale":
			var scale float64
			section.Scale = nil
			if specific(fields.Scale, &scale) {
				section.Scale = &scale
			}
		case "Transform":
			for i, transform := range []ipc.Transform{ipc.TransformNormal, ipc.Transform90, ipc.Transform180, ipc.Transform270, ipc.TransformFlipped, ipc.TransformFlipped90, ipc.TransformFlipped180, ipc.TransformFlipped270} {
				if fields.Transform == string(transform) {
					section.Transform = config.Transform(i)
				}
			}
		case "Position":
			var position config.OutputPosition
			section.Position = nil
			if specific(fields.Position, &position) {
				section.Position = &position
			}
		case "Vrr":
			section.VRR = nil
			if fields.Vrr {
				section.VRR = &config.VRR{OnDemand: fields.OnDemand}
			}
		}
	}
	return section, nil
}

// ConfigureOutput changes only the in-memory section, as Niri's Output request
// does. Reconfigure replaces the list and drops these transient changes.
// It must run on the compositor thread.
func (c *Compositor) ConfigureOutput(request ipc.OutputRequest) (string, error) {
	identity := config.OutputName{Connector: request.Name}
	result := "OutputWasMissing"
	for pointer := range c.backendOutputs {
		if outputIdentity(pointer).Matches(request.Name) {
			identity = outputIdentity(pointer)
			result = "Applied"
			break
		}
	}
	index := -1
	if result == "OutputWasMissing" {
		for _, known := range c.knownOutputs {
			if known.Matches(request.Name) {
				identity = known
				break
			}
		}
	}
	for i := range c.outputConfigs {
		if identity.Matches(c.outputConfigs[i].Name) {
			index = i
			break
		}
	}
	section := config.OutputConfig{Name: request.Name}
	if index >= 0 {
		section = c.outputConfigs[index]
	}
	edited, err := editOutputConfig(section, request.Action)
	if err != nil {
		return "", err
	}
	previous := c.outputConfigs
	c.outputConfigs = append([]config.OutputConfig(nil), previous...)
	if index >= 0 {
		c.outputConfigs[index] = edited
	} else {
		index = len(c.outputConfigs)
		c.outputConfigs = append(c.outputConfigs, edited)
	}
	changed := map[unsafe.Pointer]bool{}
	for pointer := range c.backendOutputs {
		if c.outputConfig(pointer) == &c.outputConfigs[index] {
			changed[pointer] = true
		}
	}
	if err = c.applyOutputConfigsMode(changed); err != nil {
		c.outputConfigs = previous
		return "", err
	}
	c.refreshState()
	return result, nil
}

// ConfigureOutputs applies a complete hardware configuration from a display
// client. Appearance overrides remain owned by the user's config. A verify
// request tests without changing either the live list or the layout.
func (c *Compositor) ConfigureOutputs(changes []config.OutputConfig, testOnly bool) error {
	previous := c.outputConfigs
	c.outputConfigs = append([]config.OutputConfig(nil), previous...)
	defer func() {
		if testOnly {
			c.outputConfigs = previous
		}
	}()
	seen := map[string]bool{}
	changed := map[unsafe.Pointer]bool{}
	for _, change := range changes {
		var identity config.OutputName
		found := false
		for pointer := range c.backendOutputs {
			if outputIdentity(pointer).Matches(change.Name) {
				identity = outputIdentity(pointer)
				changed[pointer] = true
				change.Transform = c.outputRelativeTransform(pointer, change.Transform)
				found = true
				break
			}
		}
		if !found || seen[identity.Connector] {
			c.outputConfigs = previous
			return fmt.Errorf("unknown or duplicate output %q", change.Name)
		}
		seen[identity.Connector] = true
		index := -1
		for i := range c.outputConfigs {
			if identity.Matches(c.outputConfigs[i].Name) {
				index = i
				break
			}
		}
		if index < 0 {
			index = len(c.outputConfigs)
			c.outputConfigs = append(c.outputConfigs, config.OutputConfig{Name: change.Name})
		}
		section := &c.outputConfigs[index]
		if change.Off {
			section.Off = true
			continue
		}
		section.Off, section.Mode, section.Modeline = change.Off, change.Mode, change.Modeline
		section.Scale, section.Transform, section.Position, section.VRR = change.Scale, change.Transform, change.Position, change.VRR
	}
	var err error
	if testOnly {
		err = c.testOutputConfigs(changed)
	} else {
		err = c.applyOutputConfigsMode(changed)
	}
	if err != nil {
		c.outputConfigs = previous
		return err
	}
	if !testOnly {
		c.refreshState()
	}
	return nil
}
