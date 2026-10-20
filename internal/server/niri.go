package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// niriSocketPath is where Niri puts its socket (src/ipc/server.rs:81).
func niriSocketPath(runtimeDir, display string, pid int) (string, error) {
	if !filepath.IsAbs(runtimeDir) {
		return "", errors.New("server: XDG_RUNTIME_DIR must be an absolute path")
	}
	return filepath.Join(runtimeDir, fmt.Sprintf("niri.%s.%d.sock", display, pid)), nil
}

// Version is what the Version request answers: ours, and the wire it speaks.
func Version() string {
	v := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			v = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				v += " " + s.Value[:7]
			}
		}
	}
	return "sysc-compositor " + v + " (niri-ipc 26.4.0)"
}

// niriHandler answers the niri-ipc requests M2 builds; every other request
// and action decodes and replies that it is not built yet.
type niriHandler struct {
	quit       func()
	spawn      spawner
	snapshot   func() niriSnapshot
	stream     func(net.Conn)
	postAction func(ipc.Action, uint64) error
}

// niriSnapshot contains only plain Go wire data, never wlroots objects.
type niriSnapshot struct {
	outputs          map[string]ipc.Output
	workspaces       []ipc.Workspace
	windows          []ipc.Window
	layers           []ipc.LayerSurface
	keyboardLayouts  *ipc.KeyboardLayouts
	focusedOutput    *ipc.Output
	configLoadFailed bool
}

// niriState publishes immutable Go snapshots to IPC goroutines. replace is
// called on the compositor thread; readers never touch wlroots objects.
type niriState struct {
	mu      sync.RWMutex
	current niriSnapshot
}

func newNiriState(configLoadFailed bool) *niriState {
	current := emptyNiriSnapshot()
	current.configLoadFailed = configLoadFailed
	return &niriState{current: current}
}

func emptyNiriSnapshot() niriSnapshot {
	return niriSnapshot{
		outputs:    map[string]ipc.Output{},
		workspaces: []ipc.Workspace{},
		windows:    []ipc.Window{},
		layers:     []ipc.LayerSurface{},
	}
}

// replace commits a complete compositor snapshot. Workspace identity and
// lifetime belong to the window manager model, not this IPC copy.
func (s *niriState) replace(next niriSnapshot) {
	if next.outputs == nil {
		next.outputs = map[string]ipc.Output{}
	}
	if next.windows == nil {
		next.windows = []ipc.Window{}
	}
	if next.workspaces == nil {
		next.workspaces = []ipc.Workspace{}
	}
	if next.layers == nil {
		next.layers = []ipc.LayerSurface{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next.configLoadFailed = s.current.configLoadFailed
	focusedName := ""
	if next.focusedOutput != nil {
		focusedName = next.focusedOutput.Name
	}
	if _, connected := next.outputs[focusedName]; !connected {
		next.focusedOutput = nil
	}
	s.current = cloneNiriSnapshot(next)
}

func (s *niriState) snapshot() niriSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneNiriSnapshot(s.current)
}

func cloneNiriSnapshot(current niriSnapshot) niriSnapshot {
	state := current
	state.outputs = make(map[string]ipc.Output, len(current.outputs))
	for name, output := range current.outputs {
		state.outputs[name] = cloneNiriOutput(output)
	}
	state.workspaces = slices.Clone(current.workspaces)
	for i := range state.workspaces {
		state.workspaces[i].Name = clonePointer(state.workspaces[i].Name)
		state.workspaces[i].Output = clonePointer(state.workspaces[i].Output)
		state.workspaces[i].ActiveWindowID = clonePointer(state.workspaces[i].ActiveWindowID)
	}
	state.windows = slices.Clone(current.windows)
	for i := range state.windows {
		window := &state.windows[i]
		window.Title = clonePointer(window.Title)
		window.AppID = clonePointer(window.AppID)
		window.PID = clonePointer(window.PID)
		window.WorkspaceID = clonePointer(window.WorkspaceID)
		window.Layout.PosInScrollingLayout = clonePointer(window.Layout.PosInScrollingLayout)
		window.Layout.TilePosInWorkspaceView = clonePointer(window.Layout.TilePosInWorkspaceView)
		window.FocusTimestamp = clonePointer(window.FocusTimestamp)
	}
	state.layers = slices.Clone(current.layers)
	if current.keyboardLayouts != nil {
		layouts := *current.keyboardLayouts
		layouts.Names = slices.Clone(current.keyboardLayouts.Names)
		state.keyboardLayouts = &layouts
	}
	if current.focusedOutput != nil {
		focused := cloneNiriOutput(*current.focusedOutput)
		state.focusedOutput = &focused
	}
	return state
}

func cloneNiriOutput(output ipc.Output) ipc.Output {
	output.Serial = clonePointer(output.Serial)
	output.PhysicalSize = clonePointer(output.PhysicalSize)
	output.Modes = slices.Clone(output.Modes)
	output.CurrentMode = clonePointer(output.CurrentMode)
	output.Logical = clonePointer(output.Logical)
	return output
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (h niriHandler) currentState() niriSnapshot {
	if h.snapshot != nil {
		return h.snapshot()
	}
	return emptyNiriSnapshot()
}

func toNiriOutput(output wlr.OutputSnapshot) ipc.Output {
	modes := make([]ipc.Mode, len(output.Modes))
	for i, mode := range output.Modes {
		modes[i] = ipc.Mode{
			Width: mode.Width, Height: mode.Height, RefreshRate: mode.RefreshRate,
			IsPreferred: mode.Preferred,
		}
	}
	var logical *ipc.LogicalOutput
	if output.Logical != nil {
		logical = &ipc.LogicalOutput{
			X: output.Logical.X, Y: output.Logical.Y,
			Width: output.Logical.Width, Height: output.Logical.Height,
			Scale: output.Logical.Scale, Transform: niriTransform(output.Logical.Transform),
		}
	}
	return ipc.Output{
		Name: output.Name, Make: output.Make, Model: output.Model,
		Serial: output.Serial, PhysicalSize: output.PhysicalSize,
		Modes: modes, CurrentMode: output.CurrentMode, IsCustomMode: output.IsCustomMode,
		VrrSupported: output.VRRSupported, VrrEnabled: output.VRREnabled, Logical: logical,
	}
}

func toNiriWindow(window wlr.WindowInfo, tile wm.WindowSnapshot, output ipc.Output) ipc.Window {
	workspaceID := tile.WorkspaceID
	result := ipc.Window{
		ID: window.ID, Title: clonePointer(window.Title), AppID: clonePointer(window.AppID),
		WorkspaceID: &workspaceID, IsFocused: window.IsFocused,
		Layout: ipc.WindowLayout{
			PosInScrollingLayout: &([2]uint32{tile.Column + 1, 1}),
			TileSize:             [2]float64{float64(tile.Tile.Width), float64(tile.Tile.Height)},
			WindowSize:           [2]int32{int32(window.Width), int32(window.Height)},
		},
	}
	if window.PID > 0 {
		pid := int32(window.PID)
		result.PID = &pid
	}
	position := [2]float64{float64(tile.Tile.X), float64(tile.Tile.Y)}
	if output.Logical != nil {
		position[0] -= float64(output.Logical.X)
		position[1] -= float64(output.Logical.Y)
	}
	result.Layout.TilePosInWorkspaceView = &position
	result.Layout.WindowOffsetInTile = [2]float64{
		float64(window.X - tile.Tile.X), float64(window.Y - tile.Tile.Y),
	}
	if window.HasFocusTimestamp {
		result.FocusTimestamp = &ipc.Timestamp{
			Secs:  uint64(window.FocusTimestamp / time.Second),
			Nanos: uint32(window.FocusTimestamp % time.Second),
		}
	}
	return result
}

func toNiriWorkspaces(model wm.Snapshot) []ipc.Workspace {
	workspaces := make([]ipc.Workspace, 0, len(model.Workspaces))
	for _, workspace := range model.Workspaces {
		output := workspace.Output
		workspaces = append(workspaces, ipc.Workspace{
			ID: workspace.ID, Idx: workspace.Index, Output: &output,
			IsActive: workspace.IsActive, IsFocused: workspace.IsFocused,
			ActiveWindowID: clonePointer(workspace.ActiveWindowID),
		})
	}
	return workspaces
}

func niriTransform(transform uint32) ipc.Transform {
	switch transform {
	case 1:
		return ipc.Transform90
	case 2:
		return ipc.Transform180
	case 3:
		return ipc.Transform270
	case 4:
		return ipc.TransformFlipped
	case 5:
		return ipc.TransformFlipped90
	case 6:
		return ipc.TransformFlipped180
	case 7:
		return ipc.TransformFlipped270
	default:
		return ipc.TransformNormal
	}
}

func toNiriLayer(layer wlr.LayerSnapshot) ipc.LayerSurface {
	kind := ipc.LayerOverlay
	switch layer.Layer {
	case wlr.LayerBackgroundKind:
		kind = ipc.LayerBackground
	case wlr.LayerBottomKind:
		kind = ipc.LayerBottom
	case wlr.LayerTopKind:
		kind = ipc.LayerTop
	}
	keyboard := ipc.LayerKeyboardNone
	switch layer.KeyboardInteractivity {
	case wlr.LayerKeyboardExclusiveMode:
		keyboard = ipc.LayerKeyboardExclusive
	case wlr.LayerKeyboardOnDemandMode:
		keyboard = ipc.LayerKeyboardOnDemand
	}
	return ipc.LayerSurface{
		Namespace: layer.Namespace, Output: layer.Output, Layer: kind,
		KeyboardInteractivity: keyboard,
	}
}

func (h niriHandler) handle(r ipc.Request) (ipc.Reply, func()) {
	switch r.Name {
	case "Version":
		return ipc.Reply{Ok: ipc.VersionResponse(Version())}, nil
	case "ReturnError":
		// Niri's own string (src/ipc/server.rs:273).
		return ipc.Reply{Err: "example compositor error"}, nil
	case "Outputs":
		outputs := h.currentState().outputs
		if outputs == nil {
			outputs = map[string]ipc.Output{}
		}
		return ipc.Reply{Ok: ipc.SuccessValue("Outputs", outputs)}, nil
	case "Workspaces":
		workspaces := h.currentState().workspaces
		if workspaces == nil {
			workspaces = []ipc.Workspace{}
		}
		return ipc.Reply{Ok: ipc.SuccessValue("Workspaces", workspaces)}, nil
	case "Windows":
		windows := h.currentState().windows
		if windows == nil {
			windows = []ipc.Window{}
		}
		return ipc.Reply{Ok: ipc.SuccessValue("Windows", windows)}, nil
	case "Layers":
		layers := h.currentState().layers
		if layers == nil {
			layers = []ipc.LayerSurface{}
		}
		return ipc.Reply{Ok: ipc.SuccessValue("Layers", layers)}, nil
	case "FocusedWindow":
		var focused *ipc.Window
		for _, window := range h.currentState().windows {
			if window.IsFocused {
				copy := window
				focused = &copy
				break
			}
		}
		return ipc.Reply{Ok: ipc.SuccessValue("FocusedWindow", focused)}, nil
	case "FocusedOutput":
		return ipc.Reply{Ok: ipc.SuccessValue("FocusedOutput", h.currentState().focusedOutput)}, nil
	case "EventStream":
		return ipc.Reply{Ok: ipc.Handled}, nil
	case "Action":
		return h.action(*r.Action)
	}
	return ipc.Unimplemented(r.Name), nil
}

func (h niriHandler) action(a ipc.Action) (ipc.Reply, func()) {
	var err error
	switch a.Name {
	case "Quit":
		// There is no confirmation dialog yet, so skip_confirmation changes
		// nothing. The reply goes out before the shutdown starts.
		return ipc.Reply{Ok: ipc.Handled}, h.quit
	case "Spawn":
		err = h.spawn.spawn(a.Command)
	case "SpawnSh":
		err = h.spawn.spawnSh(a.ShellCommand)
	case "FocusWindow", "CloseWindow":
		return h.windowAction(a, 0)
	case "FocusWorkspace":
		target, ok := focusWorkspaceTarget(h.currentState(), a.Workspace)
		if !ok {
			return ipc.Reply{Ok: ipc.Handled}, nil
		}
		return h.windowAction(a, target)
	default:
		return ipc.Unimplemented(a.Name), nil
	}
	// Niri spawns on a thread and only logs a failure; the request is
	// handled either way.
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: spawn: " + err.Error() + "\n")
	}
	return ipc.Reply{Ok: ipc.Handled}, nil
}

func (h niriHandler) windowAction(action ipc.Action, workspaceID uint64) (ipc.Reply, func()) {
	if h.postAction == nil {
		return ipc.Unimplemented(action.Name), nil
	}
	return ipc.Reply{Ok: ipc.Handled}, func() {
		if err := h.postAction(action, workspaceID); err != nil {
			os.Stderr.WriteString("sysc-compositor: action " + action.Name + ": " + err.Error() + "\n")
		}
	}
}

func focusWorkspaceTarget(state niriSnapshot, reference *ipc.WorkspaceReference) (uint64, bool) {
	if reference == nil {
		return 0, false
	}
	for _, workspace := range state.workspaces {
		matches := false
		switch {
		case reference.ID != nil:
			matches = workspace.ID == *reference.ID
		case reference.Index != nil:
			matches = workspace.Idx == *reference.Index && state.focusedOutput != nil &&
				workspace.Output != nil && *workspace.Output == state.focusedOutput.Name
		case reference.Name != nil:
			matches = workspace.Name != nil && *workspace.Name == *reference.Name
		}
		if matches && workspace.Output != nil {
			return workspace.ID, true
		}
	}
	return 0, false
}

func (h niriHandler) serve(c *net.UnixConn) {
	ipc.ServeConnWithStream(c, h.handle, h.stream)
}
