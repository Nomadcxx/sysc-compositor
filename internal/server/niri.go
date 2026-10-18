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

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
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
	quit     func()
	spawn    spawner
	snapshot func() niriSnapshot
	stream   func(net.Conn)
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
	mu              sync.RWMutex
	current         niriSnapshot
	workspaceIDs    map[string]uint64
	nextWorkspaceID uint64
}

func newNiriState(configLoadFailed bool) *niriState {
	current := emptyNiriSnapshot()
	current.configLoadFailed = configLoadFailed
	return &niriState{
		current:         current,
		workspaceIDs:    map[string]uint64{},
		nextWorkspaceID: 1,
	}
}

func emptyNiriSnapshot() niriSnapshot {
	return niriSnapshot{
		outputs:    map[string]ipc.Output{},
		workspaces: []ipc.Workspace{},
		windows:    []ipc.Window{},
		layers:     []ipc.LayerSurface{},
	}
}

// replace commits a complete output/layer view and preserves each output's
// single active, empty Niri workspace for that output's connected lifetime.
func (s *niriState) replace(next niriSnapshot) {
	if next.outputs == nil {
		next.outputs = map[string]ipc.Output{}
	}
	if next.windows == nil {
		next.windows = []ipc.Window{}
	}
	if next.layers == nil {
		next.layers = []ipc.LayerSurface{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next.configLoadFailed = s.current.configLoadFailed
	for name := range s.workspaceIDs {
		if _, connected := next.outputs[name]; !connected {
			delete(s.workspaceIDs, name)
		}
	}

	names := make([]string, 0, len(next.outputs))
	for name := range next.outputs {
		names = append(names, name)
	}
	slices.Sort(names)

	focusedName := ""
	if next.focusedOutput != nil {
		focusedName = next.focusedOutput.Name
	}
	next.workspaces = make([]ipc.Workspace, 0, len(names))
	for _, name := range names {
		id, ok := s.workspaceIDs[name]
		if !ok {
			id = s.nextWorkspaceID
			s.nextWorkspaceID++
			s.workspaceIDs[name] = id
		}
		outputName := name
		next.workspaces = append(next.workspaces, ipc.Workspace{
			ID: id, Idx: 1, Output: &outputName,
			IsActive: true, IsFocused: name == focusedName,
		})
	}
	if _, connected := next.outputs[focusedName]; !connected {
		next.focusedOutput = nil
	}
	s.current = next
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
		state.outputs[name] = output
	}
	state.workspaces = slices.Clone(current.workspaces)
	state.windows = slices.Clone(current.windows)
	state.layers = slices.Clone(current.layers)
	if current.keyboardLayouts != nil {
		layouts := *current.keyboardLayouts
		layouts.Names = slices.Clone(current.keyboardLayouts.Names)
		state.keyboardLayouts = &layouts
	}
	if current.focusedOutput != nil {
		focused := *current.focusedOutput
		state.focusedOutput = &focused
	}
	return state
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

func (h niriHandler) serve(c *net.UnixConn) {
	ipc.ServeConnWithStream(c, h.handle, h.stream)
}
