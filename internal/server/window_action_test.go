package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

func TestNiriWindowActionsOverSocket(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	type started struct {
		server *Server
		err    error
	}
	ready := make(chan started, 1)
	done := make(chan struct{})
	runErr := make(chan error, 1)
	mapped := make(chan uint64, 4)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		s, err := New(config.LoadResult{}, config.Path{}, false)
		if err != nil {
			ready <- started{err: err}
			close(done)
			return
		}
		seen := map[uint64]bool{}
		s.c.SetStateChanged(func() {
			s.refreshNiriState()
			for _, window := range s.c.Windows() {
				if !seen[window.ID] {
					seen[window.ID] = true
					mapped <- window.ID
				}
			}
		})
		ready <- started{server: s}
		runErr <- s.Run()
		s.Destroy()
		close(done)
	}()
	startedServer := <-ready
	if startedServer.err != nil {
		t.Skipf("nested compositor unavailable: %v", startedServer.err)
	}
	s := startedServer.server
	var clients []*exec.Cmd
	t.Cleanup(func() {
		for _, client := range clients {
			if client.Process != nil {
				_ = client.Process.Kill()
			}
		}
		select {
		case <-done:
		default:
			s.Terminate()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("nested compositor did not stop")
			}
		}
	})

	startClient := func() <-chan error {
		cmd := exec.Command(terminal)
		cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+s.c.SocketName())
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if err := cmd.Start(); err != nil {
			t.Fatalf("start weston-terminal: %v", err)
		}
		clients = append(clients, cmd)
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		return exited
	}
	readID := func() uint64 {
		t.Helper()
		select {
		case id := <-mapped:
			return id
		case <-time.After(5 * time.Second):
			t.Fatal("nested client did not map a toplevel")
			return 0
		}
	}
	firstExited := startClient()
	firstID := readID()
	secondExited := startClient()
	secondID := readID()
	if firstID == secondID || firstID == 0 || secondID == 0 {
		t.Fatalf("window IDs = %d, %d; want unique nonzero IDs", firstID, secondID)
	}
	requestAction := func(action ipc.Action) {
		t.Helper()
		conn, err := net.DialTimeout("unix", s.NiriSocket(), time.Second)
		if err != nil {
			t.Fatalf("connect Niri socket: %v", err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		line := append(ipc.EncodeRequest(ipc.Request{Name: "Action", Action: &action}), '\n')
		if _, err := conn.Write(line); err != nil {
			t.Fatalf("write Niri action: %v", err)
		}
		reply, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || reply != `{"Ok":"Handled"}`+"\n" {
			t.Fatalf("Niri action reply = %q, %v; want Handled", reply, err)
		}
	}
	requestWireAction := func(line string) {
		t.Helper()
		conn, err := net.DialTimeout("unix", s.NiriSocket(), time.Second)
		if err != nil {
			t.Fatalf("connect Niri socket: %v", err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write(append([]byte(line), '\n')); err != nil {
			t.Fatalf("write Niri action: %v", err)
		}
		reply, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || reply != `{"Ok":"Handled"}`+"\n" {
			t.Fatalf("Niri action reply = %q, %v; want Handled", reply, err)
		}
	}
	waitForState := func(check func(niriSnapshot) bool, what string) niriSnapshot {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			state := s.state.snapshot()
			if check(state) {
				return state
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s; current state: %+v", what, s.state.snapshot())
		return niriSnapshot{}
	}
	containsFocused := func(state niriSnapshot, id uint64) bool {
		for _, window := range state.windows {
			if window.ID == id && window.IsFocused {
				return true
			}
		}
		return false
	}

	requestAction(ipc.Action{Name: "FocusWindow", WindowID: &firstID})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, firstID) }, "FocusWindow target")
	state := s.state.snapshot()
	if len(state.workspaces) == 0 {
		t.Fatal("Niri snapshot has no current workspace")
	}
	workspaceID := state.workspaces[0].ID
	requestAction(ipc.Action{Name: "FocusWorkspace", Workspace: &ipc.WorkspaceReference{ID: &workspaceID}})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, firstID) }, "FocusWorkspace target output")
	workspaceName := "Work"
	workspaceLookupName := "work"
	requestAction(ipc.Action{Name: "SetWorkspaceName", WorkspaceName: &workspaceName})
	waitForState(func(state niriSnapshot) bool {
		for _, workspace := range state.workspaces {
			if workspace.ID == workspaceID {
				return workspace.Name != nil && *workspace.Name == workspaceName
			}
		}
		return false
	}, "SetWorkspaceName")
	requestAction(ipc.Action{Name: "FocusWorkspace", Workspace: &ipc.WorkspaceReference{Name: &workspaceLookupName}})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, firstID) }, "case-insensitive named workspace focus")
	state = s.state.snapshot()
	var targetWorkspaceID uint64
	for _, workspace := range state.workspaces {
		if workspace.Idx == 2 && workspace.Output != nil && state.focusedOutput != nil && *workspace.Output == state.focusedOutput.Name {
			targetWorkspaceID = workspace.ID
			break
		}
	}
	if targetWorkspaceID == 0 {
		t.Fatalf("missing trailing workspace for move: %+v", state.workspaces)
	}
	follow := true
	requestAction(ipc.Action{
		Name: "MoveWindowToWorkspace", WindowID: &firstID,
		Workspace: &ipc.WorkspaceReference{ID: &targetWorkspaceID}, Focus: &follow,
	})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.WorkspaceID != nil && *window.WorkspaceID == targetWorkspaceID && window.IsFocused
			}
		}
		return false
	}, "MoveWindowToWorkspace with focus follow")
	requestAction(ipc.Action{
		Name: "MoveWindowToWorkspace", WindowID: &firstID,
		Workspace: &ipc.WorkspaceReference{Name: &workspaceLookupName}, Focus: &follow,
	})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.WorkspaceID != nil && *window.WorkspaceID == workspaceID && window.IsFocused
			}
		}
		return false
	}, "move window back to named workspace")
	state = s.state.snapshot()
	var nextWorkspaceID uint64
	for _, workspace := range state.workspaces {
		if workspace.Idx == 2 && workspace.Output != nil && state.focusedOutput != nil && *workspace.Output == state.focusedOutput.Name {
			nextWorkspaceID = workspace.ID
			break
		}
	}
	if nextWorkspaceID == 0 {
		t.Fatalf("missing adjacent workspace for relative move: %+v", state.workspaces)
	}
	requestAction(ipc.Action{Name: "MoveWindowToWorkspaceDown", Focus: &follow})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.WorkspaceID != nil && *window.WorkspaceID == nextWorkspaceID && window.IsFocused
			}
		}
		return false
	}, "move window to workspace down with focus follow")
	requestAction(ipc.Action{Name: "MoveWindowToWorkspaceUp", Focus: &follow})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.WorkspaceID != nil && *window.WorkspaceID == workspaceID && window.IsFocused
			}
		}
		return false
	}, "move window back up with focus follow")
	requestAction(ipc.Action{Name: "MoveWindowToFloating", WindowID: &firstID})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.IsFloating
			}
		}
		return false
	}, "MoveWindowToFloating")
	state = s.state.snapshot()
	var floatingPosition [2]float64
	for _, window := range state.windows {
		if window.ID == firstID && window.Layout.TilePosInWorkspaceView != nil {
			floatingPosition = *window.Layout.TilePosInWorkspaceView
		}
	}
	requestWireAction(fmt.Sprintf(`{"Action":{"MoveFloatingWindow":{"id":%d,"x":{"AdjustFixed":40},"y":{"AdjustFixed":-25}}}}`, firstID))
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID && window.Layout.TilePosInWorkspaceView != nil {
				position := *window.Layout.TilePosInWorkspaceView
				return position == [2]float64{floatingPosition[0] + 40, floatingPosition[1] - 25}
			}
		}
		return false
	}, "MoveFloatingWindow relative position changes")
	requestAction(ipc.Action{Name: "MoveFloatingWindow",
		X: ipc.PositionChange{Kind: ipc.PositionAdjustFixed, Value: 10},
		Y: ipc.PositionChange{Kind: ipc.PositionAdjustFixed, Value: 5},
	})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID && window.Layout.TilePosInWorkspaceView != nil {
				position := *window.Layout.TilePosInWorkspaceView
				return position == [2]float64{floatingPosition[0] + 50, floatingPosition[1] - 20}
			}
		}
		return false
	}, "MoveFloatingWindow focused window")
	requestAction(ipc.Action{Name: "MoveWindowToTiling", WindowID: &firstID})
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return !window.IsFloating
			}
		}
		return false
	}, "MoveWindowToTiling")
	requestAction(ipc.Action{Name: "FocusWindow", WindowID: &firstID})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, firstID) }, "FocusWindow before consume")
	requestAction(ipc.Action{Name: "MoveColumnToFirst"})
	// Handled precedes posting the action. Separate socket connections do not
	// order dependent actions, so observe each prerequisite before proceeding.
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return window.WorkspaceID != nil && *window.WorkspaceID == workspaceID &&
					window.Layout.PosInScrollingLayout != nil && window.Layout.PosInScrollingLayout[0] == 1
			}
		}
		return false
	}, "MoveColumnToFirst before consume")
	requestAction(ipc.Action{Name: "ConsumeWindowIntoColumn"})
	state = waitForState(func(state niriSnapshot) bool {
		var grouped int
		for _, window := range state.windows {
			if (window.ID == firstID || window.ID == secondID) &&
				window.WorkspaceID != nil && *window.WorkspaceID == workspaceID &&
				window.Layout.PosInScrollingLayout != nil && window.Layout.PosInScrollingLayout[0] == 1 {
				grouped++
			}
		}
		return grouped == 2
	}, "ConsumeWindowIntoColumn before workspace move")
	nextWorkspaceID = 0
	for _, workspace := range state.workspaces {
		if workspace.Idx == 2 && workspace.Output != nil && state.focusedOutput != nil && *workspace.Output == state.focusedOutput.Name {
			nextWorkspaceID = workspace.ID
			break
		}
	}
	if nextWorkspaceID == 0 {
		t.Fatalf("missing adjacent workspace for column move: %+v", state.workspaces)
	}
	requestAction(ipc.Action{Name: "MoveColumnToWorkspaceDown", Focus: &follow})
	waitForState(func(state niriSnapshot) bool {
		var moved int
		for _, window := range state.windows {
			if (window.ID == firstID || window.ID == secondID) && window.WorkspaceID != nil && *window.WorkspaceID == nextWorkspaceID {
				moved++
			}
		}
		return moved == 2
	}, "move tabbed column to workspace down")
	requestAction(ipc.Action{Name: "MoveColumnToWorkspaceUp", Focus: &follow})
	waitForState(func(state niriSnapshot) bool {
		var returned int
		for _, window := range state.windows {
			if (window.ID == firstID || window.ID == secondID) && window.WorkspaceID != nil && *window.WorkspaceID == workspaceID {
				returned++
			}
		}
		return returned == 2
	}, "move tabbed column back to workspace up")
	secondIndex := uint8(2)
	requestAction(ipc.Action{Name: "FocusWindowInColumn", ColumnIndex: &secondIndex})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, secondID) }, "FocusWindowInColumn after consume")
	requestAction(ipc.Action{Name: "FocusWindow", WindowID: &firstID})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, firstID) }, "restore focus before close")
	requestAction(ipc.Action{Name: "ConsumeOrExpelWindowRight", WindowID: &secondID})
	state = waitForState(func(state niriSnapshot) bool {
		columns := make(map[uint64]uint32)
		for _, window := range state.windows {
			if window.ID == firstID || window.ID == secondID {
				columns[window.ID] = window.Layout.PosInScrollingLayout[0]
			}
		}
		return len(columns) == 2 && columns[firstID] != columns[secondID]
	}, "ConsumeOrExpelWindowRight targeting an unfocused window")
	if !containsFocused(state, firstID) {
		t.Fatal("ConsumeOrExpelWindowRight changed focus from the unrelated window")
	}
	requestAction(ipc.Action{Name: "ConsumeOrExpelWindowLeft", WindowID: &secondID})
	state = waitForState(func(state niriSnapshot) bool {
		columns := make(map[uint64]uint32)
		for _, window := range state.windows {
			if window.ID == firstID || window.ID == secondID {
				columns[window.ID] = window.Layout.PosInScrollingLayout[0]
			}
		}
		return len(columns) == 2 && columns[firstID] == columns[secondID]
	}, "ConsumeOrExpelWindowLeft targeting an unfocused window")
	requestAction(ipc.Action{Name: "CloseWindow", WindowID: &firstID})
	select {
	case <-firstExited:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseWindow did not close its selected xdg client")
	}
	waitForState(func(state niriSnapshot) bool {
		for _, window := range state.windows {
			if window.ID == firstID {
				return false
			}
		}
		return true
	}, "closed window removal")
	requestAction(ipc.Action{Name: "FocusWindow", WindowID: &secondID})
	waitForState(func(state niriSnapshot) bool { return containsFocused(state, secondID) }, "second FocusWindow target")
	requestAction(ipc.Action{Name: "CloseWindow", WindowID: &secondID})
	select {
	case <-secondExited:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseWindow did not close the second xdg client")
	}
	waitForState(func(state niriSnapshot) bool { return len(state.windows) == 0 }, "all windows to close")
	s.Terminate()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("nested compositor did not stop after the test")
	}
	if err := <-runErr; err != nil {
		t.Fatalf("nested compositor Run: %v", err)
	}
}
