// Package server owns the lifetime of one running compositor and its sockets:
// the control socket and the niri-ipc socket. Every method here runs on the
// locked compositor thread; the only cross-thread call permitted is
// Terminate, which merely sets a flag on the display. Socket goroutines reach
// the compositor through Post.
package server

import (
	"errors"
	"net"
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/control"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/unixsock"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// Server wraps the wlroots object graph behind the lifetime the binary needs.
type Server struct {
	c        *wlr.Compositor
	cfg      *config.Config
	ctl      *control.Server
	ctlPath  string
	niri     *unixsock.Listener
	state    *niriState
	events   *eventHub
	spawn    spawner
	stopping chan struct{}
}

// New builds the compositor from cfg (nil is niri's defaults) and records
// whether loading the user's startup config failed. The caller must already
// hold the compositor thread.
func New(cfg *config.Config, configLoadFailed bool) (*Server, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	workspaces := make([]wm.WorkspaceConfig, len(cfg.Workspaces))
	for i, workspace := range cfg.Workspaces {
		workspaces[i] = wm.WorkspaceConfig{Name: workspace.Name, OpenOnOutput: workspace.OpenOnOutput}
	}
	k := cfg.Keyboard
	c, err := wlr.NewWith(wlr.Options{Keyboard: wlr.Keyboard{
		Rules: k.Rules, Model: k.Model, Layout: k.Layout, Variant: k.Variant, Options: k.Options,
		File: expandHome(k.File), RepeatDelay: k.RepeatDelay, RepeatRate: k.RepeatRate,
	}, ModKey: cfg.Input.ModKey, ModKeyNested: cfg.Input.ModKeyNested,
		Layout: &wlr.LayoutOptions{
			Gaps: cfg.Layout.Gaps, ColumnWidthProportion: cfg.Layout.ColumnWidthProportion,
			FixedColumnWidth: cfg.Layout.FixedColumnWidth,
		}, CursorTheme: cfg.Cursor.Theme, CursorSize: cfg.Cursor.Size, Binds: cfg.Binds,
		Workspaces: workspaces, WindowRules: cfg.WindowRules, LayerRules: cfg.LayerRules,
	})
	if err != nil {
		return nil, err
	}
	s := &Server{
		c: c, cfg: cfg, state: newNiriState(configLoadFailed), events: newEventHub(),
		stopping: make(chan struct{}),
	}
	c.SetStateChanged(s.refreshNiriState)
	if err := s.startControl(); err != nil {
		// The compositor still serves clients without the control socket;
		// injected input is simply unavailable.
		os.Stderr.WriteString("sysc-compositor: control socket: " + err.Error() + "\n")
	}
	if err := s.startNiri(); err != nil {
		// Without it, `niri msg` cannot reach this compositor; clients
		// still can.
		os.Stderr.WriteString("sysc-compositor: niri socket: " + err.Error() + "\n")
	}
	s.spawn = newSpawner(c.SocketName(), s.NiriSocket(), cfg.Environment, cfg.Cursor)
	c.SetSpawnHandlers(s.spawn.spawn, s.spawn.spawnSh)
	return s, nil
}

// refreshNiriState copies wlroots state on its owner thread before publishing
// it to IPC readers.
func (s *Server) refreshNiriState() {
	before := s.state.snapshot()
	outputs := make(map[string]ipc.Output)
	var focused *ipc.Output
	focusedName := s.c.FocusedOutputName()
	for _, output := range s.c.OutputSnapshot() {
		value := toNiriOutput(output)
		outputs[value.Name] = value
	}
	model := s.c.WorkspaceSnapshot()
	focusedName = model.FocusedOutput
	if _, connected := outputs[focusedName]; !connected {
		focusedName = s.c.FocusedOutputName()
	}
	if output, connected := outputs[focusedName]; connected {
		copy := output
		focused = &copy
	}
	workspaces := toNiriWorkspaces(model)
	layers := s.c.LayerSnapshot()
	wireLayers := make([]ipc.LayerSurface, len(layers))
	for i, layer := range layers {
		wireLayers[i] = toNiriLayer(layer)
	}
	var keyboardLayouts *ipc.KeyboardLayouts
	if layouts := s.c.KeyboardLayoutsSnapshot(); layouts != nil {
		keyboardLayouts = &ipc.KeyboardLayouts{Names: layouts.Names, CurrentIdx: layouts.CurrentIdx}
	}
	windowInfo := make(map[uint64]wlr.WindowInfo)
	for _, window := range s.c.Windows() {
		windowInfo[window.ID] = window
	}
	windows := make([]ipc.Window, 0, len(model.Windows))
	for _, tile := range model.Windows {
		window, exists := windowInfo[tile.ID]
		if !exists {
			continue
		}
		outputName := niriWindowOutputName(tile.Output, focusedName, outputs)
		output, connected := outputs[outputName]
		if !connected {
			continue
		}
		windows = append(windows, toNiriWindow(window, tile, output))
	}
	s.state.replace(niriSnapshot{
		outputs: outputs, windows: windows, layers: wireLayers,
		workspaces: workspaces, keyboardLayouts: keyboardLayouts, focusedOutput: focused,
	})
	s.events.publish(eventStreamChanges(before, s.state.snapshot()))
}

// niriWindowOutputName keeps mapped windows in IPC when their center moves
// outside every output. M4 has no independent workspace placement, so the
// focused output is the available fallback until M5 adds it.
func niriWindowOutputName(windowOutput, focusedOutput string, outputs map[string]ipc.Output) string {
	if _, connected := outputs[windowOutput]; connected {
		return windowOutput
	}
	if _, connected := outputs[focusedOutput]; connected {
		return focusedOutput
	}
	return ""
}

func (s *Server) startNiri() error {
	path, err := niriSocketPath(os.Getenv("XDG_RUNTIME_DIR"), s.c.SocketName(), os.Getpid())
	if err != nil {
		return err
	}
	if s.niri, err = unixsock.Listen(path); err != nil {
		return err
	}
	return nil
}

// NiriSocket is the niri-ipc socket, or empty when it did not start.
func (s *Server) NiriSocket() string {
	if s.niri == nil {
		return ""
	}
	return s.niri.Path()
}

func (s *Server) startControl() error {
	path, err := control.SocketPath(os.Getenv("XDG_RUNTIME_DIR"), s.c.SocketName())
	if err != nil {
		return err
	}
	seat := controlSeat{c: s.c, stopping: s.stopping}
	ctl, err := control.Listen(path, control.NewDispatcher(seat))
	if err != nil {
		return err
	}
	s.ctl, s.ctlPath = ctl, path
	return nil
}

// ControlPath is the control socket, or empty when it did not start.
func (s *Server) ControlPath() string { return s.ctlPath }

// SocketName is the nested Wayland socket clients connect to.
func (s *Server) SocketName() string { return s.c.SocketName() }

// Run blocks until Terminate is called.
func (s *Server) Run() error {
	if s.c == nil {
		return errors.New("server already destroyed")
	}
	if s.ctl != nil {
		go s.ctl.Serve()
	}
	if s.niri != nil {
		h := niriHandler{
			quit: s.c.Terminate, spawn: s.spawn, snapshot: s.state.snapshot,
			stream: func(conn net.Conn) { s.state.serveEventStream(s.events, conn) },
			postAction: func(action ipc.Action, workspaceID uint64) error {
				return s.c.Post(func() {
					switch action.Name {
					case "FocusWindow":
						if action.WindowID != nil {
							s.c.FocusWindow(*action.WindowID)
						}
					case "CloseWindow":
						s.c.CloseWindow(action.WindowID)
					case "FocusWorkspace":
						s.c.FocusWorkspaceID(workspaceID)
					case "FocusWindowInColumn":
						if action.ColumnIndex != nil {
							s.c.FocusWindowInColumn(*action.ColumnIndex)
						}
					case "FocusWindowDown":
						s.c.FocusWindowDown()
					case "FocusWindowUp":
						s.c.FocusWindowUp()
					case "MoveWindowDown":
						s.c.MoveWindowDown()
					case "MoveWindowUp":
						s.c.MoveWindowUp()
					case "FocusColumnLeft":
						s.c.FocusColumnLeft()
					case "FocusColumnRight":
						s.c.FocusColumnRight()
					case "FocusColumnFirst":
						s.c.FocusColumnFirst()
					case "FocusColumnLast":
						s.c.FocusColumnLast()
					case "MoveColumnLeft":
						s.c.MoveColumnLeft()
					case "MoveColumnRight":
						s.c.MoveColumnRight()
					case "MoveColumnToFirst":
						s.c.MoveColumnToFirst()
					case "MoveColumnToLast":
						s.c.MoveColumnToLast()
					case "ConsumeWindowIntoColumn":
						s.c.ConsumeWindowIntoColumn()
					case "ExpelWindowFromColumn":
						s.c.ExpelWindowFromColumn()
					case "ConsumeOrExpelWindowLeft":
						s.c.ConsumeOrExpelWindowLeft(action.WindowID)
					case "ConsumeOrExpelWindowRight":
						s.c.ConsumeOrExpelWindowRight(action.WindowID)
					case "ToggleColumnTabbedDisplay":
						s.c.ToggleColumnTabbedDisplay()
					case "SetColumnDisplay":
						display := layout.ColumnDisplayNormal
						if action.ColumnDisplay == "Tabbed" {
							display = layout.ColumnDisplayTabbed
						}
						s.c.SetColumnDisplay(display)
					case "FullscreenWindow":
						s.c.ToggleFullscreenWindow(action.WindowID)
					case "ToggleWindowedFullscreen":
						s.c.ToggleWindowedFullscreenWindow(action.WindowID)
					case "ToggleWindowFloating":
						s.c.ToggleWindowFloatingFor(action.WindowID)
					case "MoveWindowToFloating":
						s.c.SetWindowFloatingFor(action.WindowID, true)
					case "MoveWindowToTiling":
						s.c.SetWindowFloatingFor(action.WindowID, false)
					case "FocusFloating":
						s.c.FocusFloating()
					case "FocusTiling":
						s.c.FocusTiling()
					case "SwitchFocusBetweenFloatingAndTiling":
						s.c.SwitchFocusFloatingAndTiling()
					case "MoveFloatingWindow":
						s.c.MoveFloatingWindow(action.WindowID,
							layout.PositionChange{Kind: layout.PositionChangeKind(action.X.Kind), Value: action.X.Value},
							layout.PositionChange{Kind: layout.PositionChangeKind(action.Y.Kind), Value: action.Y.Value},
						)
					case "ToggleWindowUrgent":
						if action.WindowID != nil {
							s.c.ToggleWindowUrgent(*action.WindowID)
						}
					case "SetWindowUrgent", "UnsetWindowUrgent":
						if action.WindowID != nil {
							s.c.SetWindowUrgent(*action.WindowID, action.Name == "SetWindowUrgent")
						}
					case "SetWorkspaceName":
						if action.WorkspaceName != nil {
							s.c.SetWorkspaceName(workspaceID, *action.WorkspaceName)
						}
					case "UnsetWorkspaceName":
						s.c.UnsetWorkspaceName(workspaceID)
					case "MoveWindowToWorkspace":
						s.c.MoveWindowToWorkspace(action.WindowID, workspaceID, action.Focus != nil && *action.Focus)
					case "MoveColumnToWorkspace":
						s.c.MoveColumnToWorkspace(workspaceID, action.Focus != nil && *action.Focus)
					case "MoveWindowToWorkspaceDown":
						s.c.MoveWindowToWorkspaceDown(action.Focus != nil && *action.Focus)
					case "MoveWindowToWorkspaceUp":
						s.c.MoveWindowToWorkspaceUp(action.Focus != nil && *action.Focus)
					case "MoveColumnToWorkspaceDown":
						s.c.MoveColumnToWorkspaceDown(action.Focus != nil && *action.Focus)
					case "MoveColumnToWorkspaceUp":
						s.c.MoveColumnToWorkspaceUp(action.Focus != nil && *action.Focus)
					case "MoveWorkspaceToMonitor":
						s.c.MoveWorkspaceToOutput(workspaceID, action.Output)
					case "MoveWorkspaceToMonitorLeft":
						s.c.MoveWorkspaceToMonitorDirection("left")
					case "MoveWorkspaceToMonitorRight":
						s.c.MoveWorkspaceToMonitorDirection("right")
					case "MoveWorkspaceToMonitorUp":
						s.c.MoveWorkspaceToMonitorDirection("up")
					case "MoveWorkspaceToMonitorDown":
						s.c.MoveWorkspaceToMonitorDirection("down")
					case "MoveWorkspaceToMonitorPrevious":
						s.c.MoveWorkspaceToMonitorDirection("previous")
					case "MoveWorkspaceToMonitorNext":
						s.c.MoveWorkspaceToMonitorDirection("next")
					case "MoveWindowToMonitor":
						s.c.MoveWindowToMonitor(action.WindowID, action.Output)
					case "MoveColumnToMonitor":
						s.c.MoveColumnToMonitor(action.Output)
					case "MoveWindowToMonitorLeft":
						s.c.MoveWindowToMonitorDirection(nil, "left")
					case "MoveWindowToMonitorRight":
						s.c.MoveWindowToMonitorDirection(nil, "right")
					case "MoveWindowToMonitorUp":
						s.c.MoveWindowToMonitorDirection(nil, "up")
					case "MoveWindowToMonitorDown":
						s.c.MoveWindowToMonitorDirection(nil, "down")
					case "MoveWindowToMonitorPrevious":
						s.c.MoveWindowToMonitorDirection(nil, "previous")
					case "MoveWindowToMonitorNext":
						s.c.MoveWindowToMonitorDirection(nil, "next")
					case "MoveColumnToMonitorLeft":
						s.c.MoveColumnToMonitorDirection("left")
					case "MoveColumnToMonitorRight":
						s.c.MoveColumnToMonitorDirection("right")
					case "MoveColumnToMonitorUp":
						s.c.MoveColumnToMonitorDirection("up")
					case "MoveColumnToMonitorDown":
						s.c.MoveColumnToMonitorDirection("down")
					case "MoveColumnToMonitorPrevious":
						s.c.MoveColumnToMonitorDirection("previous")
					case "MoveColumnToMonitorNext":
						s.c.MoveColumnToMonitorDirection("next")
					case "FocusMonitor":
						s.c.FocusMonitor(action.Output)
					case "FocusMonitorLeft":
						s.c.FocusMonitorDirection("left")
					case "FocusMonitorRight":
						s.c.FocusMonitorDirection("right")
					case "FocusMonitorUp":
						s.c.FocusMonitorDirection("up")
					case "FocusMonitorDown":
						s.c.FocusMonitorDirection("down")
					case "FocusMonitorPrevious":
						s.c.FocusMonitorDirection("previous")
					case "FocusMonitorNext":
						s.c.FocusMonitorDirection("next")
					}
				})
			},
		}
		go s.niri.Serve(h.serve)
	}
	// Niri runs spawn-at-startup, then spawn-sh-at-startup (src/main.rs).
	for _, argv := range s.cfg.SpawnAtStartup {
		s.startup(s.spawn.spawn(argv))
	}
	for _, command := range s.cfg.SpawnShAtStartup {
		s.startup(s.spawn.spawnSh(command))
	}
	return s.c.Run()
}

func (s *Server) startup(err error) {
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: spawn at startup: " + err.Error() + "\n")
	}
}

// Terminate is the only method safe to call off the compositor thread.
func (s *Server) Terminate() { s.c.Terminate() }

// Destroy releases the compositor and every listener it owns.
func (s *Server) Destroy() {
	if s.events != nil {
		// EventStream handlers outlive the request half-close. Stop them before
		// the Unix listener waits for its accepted connections to return.
		s.events.close()
	}
	if s.niri != nil {
		_ = s.niri.Close()
		s.niri = nil
	}
	if s.ctl != nil {
		// Unblock connections waiting on posted work before closing them:
		// once the loop has stopped, that work never runs.
		close(s.stopping)
		_ = s.ctl.Close()
		s.ctl = nil
	}
	if s.c != nil {
		s.c.Destroy()
		s.c = nil
	}
}
