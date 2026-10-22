// Package ipc is the niri-ipc 26.4.0 wire: one JSON value per line, with
// serde's externally tagged enums. A unit variant is a string ("Version"); a
// variant with fields is a one-key object ({"Action":{"Quit":{...}}}).
package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
)

// ErrParse is Niri's reply to a request it cannot decode. Niri sends only this
// context, not serde's detail (src/ipc/server.rs).
var ErrParse = errors.New("error parsing request")

// Request is a decoded request. Action is set when Name is "Action".
type Request struct {
	Name   string
	Action *Action
	Output *OutputRequest
}

// OutputRequest is Niri's temporary per-output configuration request. The
// action remains raw until M15 owns applying output changes.
type OutputRequest struct {
	Name   string
	Action json.RawMessage
}

// The state responses below mirror niri-ipc 26.4.0. Keep the wire types here
// so the compositor and its server share one spelling of the protocol.
type Output struct {
	Name         string         `json:"name"`
	Make         string         `json:"make"`
	Model        string         `json:"model"`
	Serial       *string        `json:"serial"`
	PhysicalSize *[2]uint32     `json:"physical_size"`
	Modes        []Mode         `json:"modes"`
	CurrentMode  *uint32        `json:"current_mode"`
	IsCustomMode bool           `json:"is_custom_mode"`
	VrrSupported bool           `json:"vrr_supported"`
	VrrEnabled   bool           `json:"vrr_enabled"`
	Logical      *LogicalOutput `json:"logical"`
}

type Mode struct {
	Width       uint16 `json:"width"`
	Height      uint16 `json:"height"`
	RefreshRate uint32 `json:"refresh_rate"`
	IsPreferred bool   `json:"is_preferred"`
}

type LogicalOutput struct {
	X         int32     `json:"x"`
	Y         int32     `json:"y"`
	Width     uint32    `json:"width"`
	Height    uint32    `json:"height"`
	Scale     float64   `json:"scale"`
	Transform Transform `json:"transform"`
}

type Transform string

const (
	TransformNormal     Transform = "Normal"
	Transform90         Transform = "90"
	Transform180        Transform = "180"
	Transform270        Transform = "270"
	TransformFlipped    Transform = "Flipped"
	TransformFlipped90  Transform = "Flipped90"
	TransformFlipped180 Transform = "Flipped180"
	TransformFlipped270 Transform = "Flipped270"
)

type Workspace struct {
	ID             uint64  `json:"id"`
	Idx            uint8   `json:"idx"`
	Name           *string `json:"name"`
	Output         *string `json:"output"`
	IsUrgent       bool    `json:"is_urgent"`
	IsActive       bool    `json:"is_active"`
	IsFocused      bool    `json:"is_focused"`
	ActiveWindowID *uint64 `json:"active_window_id"`
}

type Window struct {
	ID             uint64       `json:"id"`
	Title          *string      `json:"title"`
	AppID          *string      `json:"app_id"`
	PID            *int32       `json:"pid"`
	WorkspaceID    *uint64      `json:"workspace_id"`
	IsFocused      bool         `json:"is_focused"`
	IsFloating     bool         `json:"is_floating"`
	IsUrgent       bool         `json:"is_urgent"`
	Layout         WindowLayout `json:"layout"`
	FocusTimestamp *Timestamp   `json:"focus_timestamp"`
}

type WindowLayout struct {
	PosInScrollingLayout   *[2]uint32  `json:"pos_in_scrolling_layout"`
	TileSize               [2]float64  `json:"tile_size"`
	WindowSize             [2]int32    `json:"window_size"`
	TilePosInWorkspaceView *[2]float64 `json:"tile_pos_in_workspace_view"`
	WindowOffsetInTile     [2]float64  `json:"window_offset_in_tile"`
}

type Timestamp struct {
	Secs  uint64 `json:"secs"`
	Nanos uint32 `json:"nanos"`
}

type Layer string

const (
	LayerBackground Layer = "Background"
	LayerBottom     Layer = "Bottom"
	LayerTop        Layer = "Top"
	LayerOverlay    Layer = "Overlay"
)

type LayerKeyboardInteractivity string

const (
	LayerKeyboardNone      LayerKeyboardInteractivity = "None"
	LayerKeyboardExclusive LayerKeyboardInteractivity = "Exclusive"
	LayerKeyboardOnDemand  LayerKeyboardInteractivity = "OnDemand"
)

type LayerSurface struct {
	Namespace             string                     `json:"namespace"`
	Output                string                     `json:"output"`
	Layer                 Layer                      `json:"layer"`
	KeyboardInteractivity LayerKeyboardInteractivity `json:"keyboard_interactivity"`
}

type KeyboardLayouts struct {
	Names      []string `json:"names"`
	CurrentIdx uint8    `json:"current_idx"`
}

type OverviewState struct {
	IsOpen bool `json:"is_open"`
}

// WorkspaceReference is Niri's externally tagged workspace argument.
type WorkspaceReference struct {
	ID    *uint64
	Index *uint8
	Name  *string
}

type PositionChangeKind string

const (
	PositionSetFixed         PositionChangeKind = "SetFixed"
	PositionSetProportion    PositionChangeKind = "SetProportion"
	PositionAdjustFixed      PositionChangeKind = "AdjustFixed"
	PositionAdjustProportion PositionChangeKind = "AdjustProportion"
)

type PositionChange struct {
	Kind  PositionChangeKind
	Value float64
}

// Action is a decoded action. Only the fields of the actions built so far
// are kept; every other action is known by Name alone.
type Action struct {
	Name             string
	SkipConfirmation bool     // Quit
	Command          []string // Spawn
	ShellCommand     string   // SpawnSh
	Layout           string   // SwitchLayout: Next, Prev, or Index
	LayoutIndex      *uint8   // SwitchLayout Index
	ConfigPath       *string  // LoadConfigFile
	WindowID         *uint64
	Workspace        *WorkspaceReference
	WorkspaceName    *string
	ColumnIndex      *uint8
	ColumnDisplay    string
	Output           string
	Focus            *bool
	X, Y             PositionChange
}

// DecodeRequest decodes one request line.
func DecodeRequest(line []byte) (Request, error) {
	name, body, err := variant(line)
	if err != nil || !slices.Contains(Requests, name) {
		return Request{}, ErrParse
	}
	r := Request{Name: name}
	if name == "Action" {
		a, err := decodeAction(body)
		if err != nil {
			return Request{}, ErrParse
		}
		r.Action = &a
	} else if name == "Output" {
		output, err := decodeOutputRequest(body)
		if err != nil {
			return Request{}, ErrParse
		}
		r.Output = output
	}
	return r, nil
}

func decodeOutputRequest(body json.RawMessage) (*OutputRequest, error) {
	var fields struct {
		Name   *string         `json:"output"`
		Action json.RawMessage `json:"action"`
	}
	if err := strict(body, &fields); err != nil || fields.Name == nil || len(fields.Action) == 0 {
		return nil, ErrParse
	}
	action, value, err := variant(fields.Action)
	if err != nil || !slices.Contains(OutputActions, action) {
		return nil, ErrParse
	}
	if !validOutputAction(action, value) {
		return nil, ErrParse
	}
	return &OutputRequest{Name: *fields.Name, Action: bytes.Clone(fields.Action)}, nil
}

func validOutputAction(name string, body json.RawMessage) bool {
	unit := body == nil
	switch name {
	case "Off", "On":
		return unit
	case "Mode":
		var fields struct {
			Mode json.RawMessage `json:"mode"`
		}
		return body != nil && strict(body, &fields) == nil && validModeToSet(fields.Mode)
	case "CustomMode":
		var fields struct {
			Mode json.RawMessage `json:"mode"`
		}
		return body != nil && strict(body, &fields) == nil && validConfiguredMode(fields.Mode)
	case "Modeline":
		var fields struct {
			Clock         *float64 `json:"clock"`
			HDisplay      *uint16  `json:"hdisplay"`
			HSyncStart    *uint16  `json:"hsync_start"`
			HSyncEnd      *uint16  `json:"hsync_end"`
			HTotal        *uint16  `json:"htotal"`
			VDisplay      *uint16  `json:"vdisplay"`
			VSyncStart    *uint16  `json:"vsync_start"`
			VSyncEnd      *uint16  `json:"vsync_end"`
			VTotal        *uint16  `json:"vtotal"`
			HSyncPolarity *string  `json:"hsync_polarity"`
			VSyncPolarity *string  `json:"vsync_polarity"`
		}
		if body == nil || strict(body, &fields) != nil || fields.Clock == nil || fields.HDisplay == nil ||
			fields.HSyncStart == nil || fields.HSyncEnd == nil || fields.HTotal == nil || fields.VDisplay == nil ||
			fields.VSyncStart == nil || fields.VSyncEnd == nil || fields.VTotal == nil ||
			fields.HSyncPolarity == nil || fields.VSyncPolarity == nil {
			return false
		}
		return (*fields.HSyncPolarity == "PHSync" || *fields.HSyncPolarity == "NHSync") &&
			(*fields.VSyncPolarity == "PVSync" || *fields.VSyncPolarity == "NVSync")
	case "Scale":
		var fields struct {
			Scale json.RawMessage `json:"scale"`
		}
		return body != nil && strict(body, &fields) == nil && validScaleToSet(fields.Scale)
	case "Transform":
		var fields struct {
			Transform json.RawMessage `json:"transform"`
		}
		return body != nil && strict(body, &fields) == nil &&
			validUnitVariant(fields.Transform, "Normal", "90", "180", "270", "Flipped", "Flipped90", "Flipped180", "Flipped270")
	case "Position":
		var fields struct {
			Position json.RawMessage `json:"position"`
		}
		return body != nil && strict(body, &fields) == nil && validPositionToSet(fields.Position)
	case "Vrr":
		var fields struct {
			Vrr      *bool `json:"vrr"`
			OnDemand *bool `json:"on_demand"`
		}
		return body != nil && strict(body, &fields) == nil && fields.Vrr != nil && fields.OnDemand != nil
	default:
		return false
	}
}

func validModeToSet(raw json.RawMessage) bool {
	name, body, err := variant(raw)
	if err != nil {
		return false
	}
	switch name {
	case "Automatic":
		return body == nil
	case "Specific":
		return validConfiguredMode(body)
	default:
		return false
	}
}

func validConfiguredMode(raw json.RawMessage) bool {
	var fields struct {
		Width   *uint16         `json:"width"`
		Height  *uint16         `json:"height"`
		Refresh json.RawMessage `json:"refresh"`
	}
	if raw == nil || strict(raw, &fields) != nil || fields.Width == nil || fields.Height == nil {
		return false
	}
	if len(fields.Refresh) == 0 || bytes.Equal(bytes.TrimSpace(fields.Refresh), []byte("null")) {
		return true
	}
	var refresh float64
	return json.Unmarshal(fields.Refresh, &refresh) == nil
}

func validScaleToSet(raw json.RawMessage) bool {
	name, body, err := variant(raw)
	if err != nil {
		return false
	}
	switch name {
	case "Automatic":
		return body == nil
	case "Specific":
		var scale *float64
		return body != nil && json.Unmarshal(body, &scale) == nil && scale != nil
	default:
		return false
	}
}

func validPositionToSet(raw json.RawMessage) bool {
	name, body, err := variant(raw)
	if err != nil {
		return false
	}
	switch name {
	case "Automatic":
		return body == nil
	case "Specific":
		var position struct {
			X *int32 `json:"x"`
			Y *int32 `json:"y"`
		}
		return body != nil && strict(body, &position) == nil && position.X != nil && position.Y != nil
	default:
		return false
	}
}

func validUnitVariant(raw json.RawMessage, names ...string) bool {
	name, body, err := variant(raw)
	return err == nil && body == nil && slices.Contains(names, name)
}

func decodeAction(raw json.RawMessage) (Action, error) {
	name, body, err := variant(raw)
	if err != nil || !slices.Contains(Actions, name) {
		return Action{}, ErrParse
	}
	a := Action{Name: name}
	switch name {
	case "Quit":
		var f struct {
			SkipConfirmation *bool `json:"skip_confirmation"`
		}
		if err := strict(body, &f); err != nil || f.SkipConfirmation == nil {
			return Action{}, ErrParse
		}
		a.SkipConfirmation = *f.SkipConfirmation
	case "Spawn":
		var f struct {
			Command *[]string `json:"command"`
		}
		if err := strict(body, &f); err != nil || f.Command == nil {
			return Action{}, ErrParse
		}
		a.Command = *f.Command
	case "SpawnSh":
		var f struct {
			Command *string `json:"command"`
		}
		if err := strict(body, &f); err != nil || f.Command == nil {
			return Action{}, ErrParse
		}
		a.ShellCommand = *f.Command
	case "SwitchLayout":
		var f struct {
			Layout json.RawMessage `json:"layout"`
		}
		if err := strict(body, &f); err != nil || len(f.Layout) == 0 {
			return Action{}, ErrParse
		}
		name, value, err := variant(f.Layout)
		if err != nil {
			return Action{}, ErrParse
		}
		switch name {
		case "Next", "Prev":
			if value != nil {
				return Action{}, ErrParse
			}
			a.Layout = name
		case "Index":
			var index uint8
			if value == nil || json.Unmarshal(value, &index) != nil {
				return Action{}, ErrParse
			}
			a.Layout, a.LayoutIndex = name, &index
		default:
			return Action{}, ErrParse
		}
	case "LoadConfigFile":
		var f struct {
			Path *string `json:"path"`
		}
		if err := strict(body, &f); err != nil {
			return Action{}, ErrParse
		}
		a.ConfigPath = f.Path
	case "FocusWindow":
		var f struct {
			ID *uint64 `json:"id"`
		}
		if err := strict(body, &f); err != nil || f.ID == nil {
			return Action{}, ErrParse
		}
		a.WindowID = f.ID
	case "CloseWindow":
		var f struct {
			ID *uint64 `json:"id"`
		}
		if err := strict(body, &f); err != nil {
			return Action{}, ErrParse
		}
		a.WindowID = f.ID
	case "FocusWorkspace":
		var f struct {
			Reference *json.RawMessage `json:"reference"`
		}
		if err := strict(body, &f); err != nil || f.Reference == nil {
			return Action{}, ErrParse
		}
		a.Workspace, err = decodeWorkspaceReference(*f.Reference)
		if err != nil {
			return Action{}, ErrParse
		}
	case "FocusWindowInColumn":
		var f struct {
			Index *uint8 `json:"index"`
		}
		if err := strict(body, &f); err != nil || f.Index == nil {
			return Action{}, ErrParse
		}
		a.ColumnIndex = f.Index
	case "SetColumnDisplay":
		var f struct {
			Display *string `json:"display"`
		}
		if err := strict(body, &f); err != nil || f.Display == nil || (*f.Display != "Normal" && *f.Display != "Tabbed") {
			return Action{}, ErrParse
		}
		a.ColumnDisplay = *f.Display
	case "FullscreenWindow", "ToggleWindowedFullscreen", "ToggleWindowFloating", "MoveWindowToFloating", "MoveWindowToTiling",
		"ConsumeOrExpelWindowLeft", "ConsumeOrExpelWindowRight":
		var f struct {
			ID *uint64 `json:"id"`
		}
		if err := strict(body, &f); err != nil {
			return Action{}, ErrParse
		}
		a.WindowID = f.ID
	case "ToggleWindowUrgent", "SetWindowUrgent", "UnsetWindowUrgent":
		var f struct {
			ID *uint64 `json:"id"`
		}
		if err := strict(body, &f); err != nil || f.ID == nil {
			return Action{}, ErrParse
		}
		a.WindowID = f.ID
	case "FocusFloating", "FocusTiling", "SwitchFocusBetweenFloatingAndTiling",
		"ConsumeWindowIntoColumn", "ExpelWindowFromColumn", "ToggleColumnTabbedDisplay",
		"ToggleOverview", "OpenOverview", "CloseOverview", "ToggleKeyboardShortcutsInhibit",
		"PowerOffMonitors", "PowerOnMonitors":
		if err := strict(body, &struct{}{}); err != nil {
			return Action{}, ErrParse
		}
	case "SetWorkspaceName":
		var f struct {
			Name      *string          `json:"name"`
			Workspace *json.RawMessage `json:"workspace"`
		}
		if err := strict(body, &f); err != nil || f.Name == nil {
			return Action{}, ErrParse
		}
		a.WorkspaceName = f.Name
		if f.Workspace != nil && string(*f.Workspace) != "null" {
			a.Workspace, err = decodeWorkspaceReference(*f.Workspace)
			if err != nil {
				return Action{}, ErrParse
			}
		}
	case "UnsetWorkspaceName":
		var f struct {
			Reference *json.RawMessage `json:"reference"`
		}
		if err := strict(body, &f); err != nil {
			return Action{}, ErrParse
		}
		if f.Reference != nil && string(*f.Reference) != "null" {
			a.Workspace, err = decodeWorkspaceReference(*f.Reference)
			if err != nil {
				return Action{}, ErrParse
			}
		}
	case "MoveWindowToWorkspace":
		var f struct {
			WindowID  *uint64          `json:"window_id"`
			Reference *json.RawMessage `json:"reference"`
			Focus     *bool            `json:"focus"`
		}
		if err := strict(body, &f); err != nil || f.Reference == nil || f.Focus == nil {
			return Action{}, ErrParse
		}
		a.Workspace, err = decodeWorkspaceReference(*f.Reference)
		if err != nil {
			return Action{}, ErrParse
		}
		a.WindowID, a.Focus = f.WindowID, f.Focus
	case "MoveColumnToWorkspace":
		var f struct {
			Reference *json.RawMessage `json:"reference"`
			Focus     *bool            `json:"focus"`
		}
		if err := strict(body, &f); err != nil || f.Reference == nil || f.Focus == nil {
			return Action{}, ErrParse
		}
		a.Workspace, err = decodeWorkspaceReference(*f.Reference)
		if err != nil {
			return Action{}, ErrParse
		}
		a.Focus = f.Focus
	case "MoveWindowToWorkspaceDown", "MoveWindowToWorkspaceUp",
		"MoveColumnToWorkspaceDown", "MoveColumnToWorkspaceUp":
		var f struct {
			Focus *bool `json:"focus"`
		}
		if err := strict(body, &f); err != nil || f.Focus == nil {
			return Action{}, ErrParse
		}
		a.Focus = f.Focus
	case "MoveWindowToMonitor":
		var f struct {
			ID     *uint64 `json:"id"`
			Output *string `json:"output"`
		}
		if err := strict(body, &f); err != nil || f.Output == nil {
			return Action{}, ErrParse
		}
		a.WindowID, a.Output = f.ID, *f.Output
	case "MoveColumnToMonitor":
		var f struct {
			Output *string `json:"output"`
		}
		if err := strict(body, &f); err != nil || f.Output == nil {
			return Action{}, ErrParse
		}
		a.Output = *f.Output
	case "MoveWorkspaceToMonitor":
		var f struct {
			Output    *string          `json:"output"`
			Reference *json.RawMessage `json:"reference"`
		}
		if err := strict(body, &f); err != nil || f.Output == nil {
			return Action{}, ErrParse
		}
		a.Output = *f.Output
		if f.Reference != nil && string(*f.Reference) != "null" {
			a.Workspace, err = decodeWorkspaceReference(*f.Reference)
			if err != nil {
				return Action{}, ErrParse
			}
		}
	case "FocusMonitor":
		var f struct {
			Output *string `json:"output"`
		}
		if err := strict(body, &f); err != nil || f.Output == nil {
			return Action{}, ErrParse
		}
		a.Output = *f.Output
	case "MoveFloatingWindow":
		var f struct {
			ID *uint64          `json:"id"`
			X  *json.RawMessage `json:"x"`
			Y  *json.RawMessage `json:"y"`
		}
		if err := strict(body, &f); err != nil || f.X == nil || f.Y == nil {
			return Action{}, ErrParse
		}
		a.WindowID = f.ID
		a.X, err = decodePositionChange(*f.X)
		if err != nil {
			return Action{}, ErrParse
		}
		a.Y, err = decodePositionChange(*f.Y)
		if err != nil {
			return Action{}, ErrParse
		}
	}
	return a, nil
}

func decodePositionChange(raw json.RawMessage) (PositionChange, error) {
	name, value, err := variant(raw)
	if err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return PositionChange{}, ErrParse
	}
	var change PositionChange
	switch name {
	case string(PositionSetFixed):
		change.Kind = PositionSetFixed
	case string(PositionSetProportion):
		change.Kind = PositionSetProportion
	case string(PositionAdjustFixed):
		change.Kind = PositionAdjustFixed
	case string(PositionAdjustProportion):
		change.Kind = PositionAdjustProportion
	default:
		return PositionChange{}, ErrParse
	}
	if err := json.Unmarshal(value, &change.Value); err != nil || math.IsNaN(change.Value) || math.IsInf(change.Value, 0) {
		return PositionChange{}, ErrParse
	}
	return change, nil
}

func decodeWorkspaceReference(raw json.RawMessage) (*WorkspaceReference, error) {
	name, value, err := variant(raw)
	if err != nil {
		return nil, ErrParse
	}
	reference := &WorkspaceReference{}
	switch name {
	case "Id":
		var id uint64
		if err := json.Unmarshal(value, &id); err != nil {
			return nil, ErrParse
		}
		reference.ID = &id
	case "Index":
		var index uint8
		if err := json.Unmarshal(value, &index); err != nil {
			return nil, ErrParse
		}
		reference.Index = &index
	case "Name":
		var workspaceName string
		if err := json.Unmarshal(value, &workspaceName); err != nil {
			return nil, ErrParse
		}
		reference.Name = &workspaceName
	default:
		return nil, ErrParse
	}
	return reference, nil
}

// variant splits an externally tagged enum value into its name and body. A
// unit variant has no body.
func variant(raw []byte) (string, json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return name, nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) != 1 {
		return "", nil, ErrParse
	}
	for k, v := range obj {
		return k, v, nil
	}
	return "", nil, ErrParse
}

func strict(body json.RawMessage, v any) error {
	if body == nil {
		return ErrParse
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// EncodeRequest is the line a client sends, without its newline.
func EncodeRequest(r Request) []byte {
	if r.Name == "Output" && r.Output != nil {
		return marshal(map[string]any{"Output": map[string]any{
			"output": r.Output.Name, "action": r.Output.Action,
		}})
	}
	if r.Name != "Action" || r.Action == nil {
		return marshal(r.Name)
	}
	a := r.Action
	var body any = struct{}{}
	switch a.Name {
	case "Quit":
		body = struct {
			SkipConfirmation bool `json:"skip_confirmation"`
		}{a.SkipConfirmation}
	case "Spawn":
		command := a.Command
		if command == nil {
			command = []string{}
		}
		body = struct {
			Command []string `json:"command"`
		}{command}
	case "SpawnSh":
		body = struct {
			Command string `json:"command"`
		}{a.ShellCommand}
	case "SwitchLayout":
		var layout any
		switch a.Layout {
		case "Next", "Prev":
			layout = a.Layout
		case "Index":
			index := uint8(0)
			if a.LayoutIndex != nil {
				index = *a.LayoutIndex
			}
			layout = map[string]uint8{"Index": index}
		}
		body = struct {
			Layout any `json:"layout"`
		}{layout}
	case "LoadConfigFile":
		body = struct {
			Path *string `json:"path"`
		}{a.ConfigPath}
	case "FocusWindow":
		id := uint64(0)
		if a.WindowID != nil {
			id = *a.WindowID
		}
		body = struct {
			ID uint64 `json:"id"`
		}{id}
	case "CloseWindow":
		body = struct {
			ID *uint64 `json:"id"`
		}{a.WindowID}
	case "FocusWorkspace":
		body = struct {
			Reference any `json:"reference"`
		}{workspaceReferenceValue(a.Workspace)}
	case "FocusWindowInColumn":
		body = struct {
			Index uint8 `json:"index"`
		}{}
		if a.ColumnIndex != nil {
			body = struct {
				Index uint8 `json:"index"`
			}{*a.ColumnIndex}
		}
	case "SetColumnDisplay":
		body = struct {
			Display string `json:"display"`
		}{a.ColumnDisplay}
	case "FullscreenWindow", "ToggleWindowedFullscreen", "ToggleWindowFloating", "MoveWindowToFloating", "MoveWindowToTiling",
		"ConsumeOrExpelWindowLeft", "ConsumeOrExpelWindowRight",
		"ToggleWindowUrgent", "SetWindowUrgent", "UnsetWindowUrgent":
		body = struct {
			ID *uint64 `json:"id"`
		}{a.WindowID}
	case "SetWorkspaceName":
		name := ""
		if a.WorkspaceName != nil {
			name = *a.WorkspaceName
		}
		body = struct {
			Name      string `json:"name"`
			Workspace any    `json:"workspace"`
		}{name, workspaceReferenceValue(a.Workspace)}
	case "UnsetWorkspaceName":
		body = struct {
			Reference any `json:"reference"`
		}{workspaceReferenceValue(a.Workspace)}
	case "MoveWindowToWorkspace", "MoveColumnToWorkspace":
		focus := false
		if a.Focus != nil {
			focus = *a.Focus
		}
		if a.Name == "MoveWindowToWorkspace" {
			body = struct {
				WindowID  *uint64 `json:"window_id"`
				Reference any     `json:"reference"`
				Focus     bool    `json:"focus"`
			}{a.WindowID, workspaceReferenceValue(a.Workspace), focus}
		} else {
			body = struct {
				Reference any  `json:"reference"`
				Focus     bool `json:"focus"`
			}{workspaceReferenceValue(a.Workspace), focus}
		}
	case "MoveWindowToWorkspaceDown", "MoveWindowToWorkspaceUp",
		"MoveColumnToWorkspaceDown", "MoveColumnToWorkspaceUp":
		focus := true
		if a.Focus != nil {
			focus = *a.Focus
		}
		body = struct {
			Focus bool `json:"focus"`
		}{focus}
	case "MoveWindowToMonitor":
		body = struct {
			ID     *uint64 `json:"id"`
			Output string  `json:"output"`
		}{a.WindowID, a.Output}
	case "MoveColumnToMonitor":
		body = struct {
			Output string `json:"output"`
		}{a.Output}
	case "MoveWorkspaceToMonitor":
		body = struct {
			Output    string `json:"output"`
			Reference any    `json:"reference"`
		}{a.Output, workspaceReferenceValue(a.Workspace)}
	case "FocusMonitor":
		body = struct {
			Output string `json:"output"`
		}{a.Output}
	case "MoveFloatingWindow":
		body = struct {
			ID *uint64 `json:"id"`
			X  any     `json:"x"`
			Y  any     `json:"y"`
		}{a.WindowID, positionChangeValue(a.X), positionChangeValue(a.Y)}
	}
	return marshal(map[string]any{"Action": map[string]any{a.Name: body}})
}

func positionChangeValue(change PositionChange) any {
	kind := change.Kind
	if kind == "" {
		kind = PositionAdjustFixed
	}
	return map[string]float64{string(kind): change.Value}
}

func workspaceReferenceValue(reference *WorkspaceReference) any {
	if reference == nil {
		return nil
	}
	switch {
	case reference.ID != nil:
		return map[string]any{"Id": *reference.ID}
	case reference.Index != nil:
		return map[string]any{"Index": *reference.Index}
	case reference.Name != nil:
		return map[string]any{"Name": *reference.Name}
	default:
		return nil
	}
}

// marshal writes JSON as serde_json does: '<', '>' and '&' are not escaped.
func marshal(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// Reply is Result<Response, String>: exactly one of Ok and Err is meaningful.
type Reply struct {
	Ok  json.RawMessage
	Err string
}

// Handled is the response to an action.
var Handled = json.RawMessage(`"Handled"`)

// VersionResponse is the response to Version.
func VersionResponse(v string) json.RawMessage {
	return marshal(map[string]string{"Version": v})
}

// SuccessValue wraps a response payload in niri-ipc's externally tagged enum.
func SuccessValue(name string, value any) json.RawMessage {
	return marshal(map[string]any{name: value})
}

// Unimplemented is the error for a request or action that decodes but is not
// built yet (design §5).
func Unimplemented(name string) Reply { return Reply{Err: "unimplemented: " + name} }

// MarshalJSON writes {"Ok":…} or {"Err":"…"}.
func (r Reply) MarshalJSON() ([]byte, error) {
	if r.Ok == nil {
		return marshal(map[string]string{"Err": r.Err}), nil
	}
	return marshal(map[string]json.RawMessage{"Ok": r.Ok}), nil
}

// UnmarshalJSON reads {"Ok":…} or {"Err":"…"}.
func (r *Reply) UnmarshalJSON(b []byte) error {
	name, body, err := variant(b)
	if err != nil || body == nil {
		return fmt.Errorf("ipc: reply is not Ok or Err: %s", b)
	}
	switch name {
	case "Ok":
		r.Ok, r.Err = body, ""
		return nil
	case "Err":
		r.Ok = nil
		return json.Unmarshal(body, &r.Err)
	}
	return fmt.Errorf("ipc: reply is not Ok or Err: %s", b)
}
