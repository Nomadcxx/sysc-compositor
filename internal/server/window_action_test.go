package server

import (
	"bufio"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

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
		s, err := New(nil, false)
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
