package server

import (
	"encoding/json"
	"io"
	"net"
	"reflect"
	"slices"
	"sync"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

const eventStreamBufferSize = 64

type eventHub struct {
	mu      sync.Mutex
	clients map[*eventStreamClient]struct{}
	closed  bool
}

type eventStreamClient struct {
	conn       net.Conn
	queue      chan []byte
	disconnect chan struct{}
	stopOnce   sync.Once
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[*eventStreamClient]struct{})}
}

func (s *niriState) serveEventStream(h *eventHub, conn net.Conn) {
	// Hold the state read lock through registration. A state update either
	// lands before this initial snapshot or is queued after the client joins.
	s.mu.RLock()
	client := h.register(conn, initialEventLines(cloneNiriSnapshot(s.current)))
	s.mu.RUnlock()
	h.serve(client)
}

func (h *eventHub) register(conn net.Conn, initial [][]byte) *eventStreamClient {
	client := &eventStreamClient{
		conn: conn, queue: make(chan []byte, eventStreamBufferSize),
		disconnect: make(chan struct{}),
	}
	h.mu.Lock()
	if h.closed {
		client.stop()
	} else {
		for _, line := range initial {
			client.queue <- line
		}
		h.clients[client] = struct{}{}
	}
	h.mu.Unlock()
	return client
}

func (h *eventHub) publish(lines [][]byte) {
	if len(lines) == 0 {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	for client := range h.clients {
		slow := false
		for _, line := range lines {
			select {
			case client.queue <- line:
			default:
				slow = true
			}
			if slow {
				break
			}
		}
		if slow {
			delete(h.clients, client)
			client.stop()
		}
	}
	h.mu.Unlock()
}

func (h *eventHub) close() {
	h.mu.Lock()
	h.closed = true
	for client := range h.clients {
		delete(h.clients, client)
		client.stop()
	}
	h.mu.Unlock()
}

func (h *eventHub) serve(client *eventStreamClient) {
	done := make(chan struct{})
	go func() {
		select {
		case <-client.disconnect:
			_ = client.conn.Close()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		client.stop()
		h.mu.Lock()
		delete(h.clients, client)
		h.mu.Unlock()
	}()
	for {
		select {
		case <-client.disconnect:
			return
		case line := <-client.queue:
			if err := writeEventLine(client.conn, line); err != nil {
				return
			}
		}
	}
}

func (client *eventStreamClient) stop() {
	client.stopOnce.Do(func() { close(client.disconnect) })
}

func (h *eventHub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func writeEventLine(conn net.Conn, line []byte) error {
	for len(line) != 0 {
		n, err := conn.Write(line)
		line = line[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func initialEventLines(state niriSnapshot) [][]byte {
	lines := [][]byte{
		eventLine("WorkspacesChanged", struct {
			Workspaces []ipc.Workspace `json:"workspaces"`
		}{Workspaces: nonNilWorkspaces(state.workspaces)}),
		eventLine("WindowsChanged", struct {
			Windows []ipc.Window `json:"windows"`
		}{Windows: nonNilWindows(state.windows)}),
	}
	if state.keyboardLayouts != nil {
		lines = append(lines, eventLine("KeyboardLayoutsChanged", struct {
			KeyboardLayouts ipc.KeyboardLayouts `json:"keyboard_layouts"`
		}{KeyboardLayouts: *state.keyboardLayouts}))
	}
	lines = append(lines,
		eventLine("OverviewOpenedOrClosed", struct {
			IsOpen bool `json:"is_open"`
		}{IsOpen: state.overviewOpen}),
		eventLine("ConfigLoaded", struct {
			Failed bool `json:"failed"`
		}{Failed: state.configLoadFailed}),
		eventLine("CastsChanged", struct {
			Casts []any `json:"casts"`
		}{Casts: []any{}}),
	)
	return lines
}

func nonNilWorkspaces(workspaces []ipc.Workspace) []ipc.Workspace {
	if workspaces == nil {
		return []ipc.Workspace{}
	}
	return workspaces
}

func nonNilWindows(windows []ipc.Window) []ipc.Window {
	if windows == nil {
		return []ipc.Window{}
	}
	return windows
}

func eventStreamChanges(before, after niriSnapshot) [][]byte {
	var lines [][]byte
	if workspaceDetailsChanged(before.workspaces, after.workspaces) {
		lines = append(lines, eventLine("WorkspacesChanged", struct {
			Workspaces []ipc.Workspace `json:"workspaces"`
		}{Workspaces: nonNilWorkspaces(after.workspaces)}))
	} else {
		for i, workspace := range after.workspaces {
			previous := before.workspaces[i]
			if previous.IsUrgent != workspace.IsUrgent {
				lines = append(lines, eventLine("WorkspaceUrgencyChanged", struct {
					ID     uint64 `json:"id"`
					Urgent bool   `json:"urgent"`
				}{ID: workspace.ID, Urgent: workspace.IsUrgent}))
			}
			if previous.ActiveWindowID == nil && workspace.ActiveWindowID != nil ||
				previous.ActiveWindowID != nil && workspace.ActiveWindowID == nil ||
				previous.ActiveWindowID != nil && workspace.ActiveWindowID != nil &&
					*previous.ActiveWindowID != *workspace.ActiveWindowID {
				lines = append(lines, eventLine("WorkspaceActiveWindowChanged", struct {
					WorkspaceID    uint64  `json:"workspace_id"`
					ActiveWindowID *uint64 `json:"active_window_id"`
				}{WorkspaceID: workspace.ID, ActiveWindowID: workspace.ActiveWindowID}))
			}
			if workspace.IsFocused && !previous.IsFocused {
				lines = append(lines, workspaceActivatedLine(workspace.ID, true))
				continue
			}
			if workspace.IsActive && !previous.IsActive {
				lines = append(lines, workspaceActivatedLine(workspace.ID, false))
			}
		}
	}
	lines = append(lines, eventStreamWindowChanges(before.windows, after.windows)...)
	lines = append(lines, eventStreamWindowLayoutChanges(before.windows, after.windows)...)
	if after.keyboardLayouts != nil {
		switch {
		case before.keyboardLayouts == nil || !slices.Equal(before.keyboardLayouts.Names, after.keyboardLayouts.Names):
			lines = append(lines, eventLine("KeyboardLayoutsChanged", struct {
				KeyboardLayouts ipc.KeyboardLayouts `json:"keyboard_layouts"`
			}{KeyboardLayouts: *after.keyboardLayouts}))
		case before.keyboardLayouts.CurrentIdx != after.keyboardLayouts.CurrentIdx:
			lines = append(lines, eventLine("KeyboardLayoutSwitched", struct {
				Idx uint8 `json:"idx"`
			}{Idx: after.keyboardLayouts.CurrentIdx}))
		}
	}
	if before.overviewOpen != after.overviewOpen {
		lines = append(lines, eventLine("OverviewOpenedOrClosed", struct {
			IsOpen bool `json:"is_open"`
		}{IsOpen: after.overviewOpen}))
	}
	return lines
}

func eventStreamWindowLayoutChanges(before, after []ipc.Window) [][]byte {
	previous := make(map[uint64]ipc.Window, len(before))
	for _, window := range before {
		previous[window.ID] = window
	}
	changes := make([][2]any, 0)
	for _, window := range after {
		old, exists := previous[window.ID]
		if exists && !reflect.DeepEqual(old.Layout, window.Layout) {
			changes = append(changes, [2]any{window.ID, window.Layout})
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return [][]byte{eventLine("WindowLayoutsChanged", struct {
		Changes [][2]any `json:"changes"`
	}{Changes: changes})}
}

func eventStreamWindowChanges(before, after []ipc.Window) [][]byte {
	previous := make(map[uint64]ipc.Window, len(before))
	for _, window := range before {
		previous[window.ID] = window
	}
	seen := make(map[uint64]struct{}, len(after))
	var lines [][]byte
	hadFocusedWindow := focusedWindowID(before) != nil
	for _, window := range after {
		seen[window.ID] = struct{}{}
		old, exists := previous[window.ID]
		if !exists || windowDetailsChanged(old, window) {
			lines = append(lines, eventLine("WindowOpenedOrChanged", struct {
				Window ipc.Window `json:"window"`
			}{Window: window}))
			continue
		}
		if window.IsFocused && !old.IsFocused {
			id := window.ID
			lines = append(lines, eventLine("WindowFocusChanged", struct {
				ID *uint64 `json:"id"`
			}{ID: &id}))
		}
		if !reflect.DeepEqual(old.FocusTimestamp, window.FocusTimestamp) {
			lines = append(lines, eventLine("WindowFocusTimestampChanged", struct {
				ID             uint64         `json:"id"`
				FocusTimestamp *ipc.Timestamp `json:"focus_timestamp"`
			}{ID: window.ID, FocusTimestamp: window.FocusTimestamp}))
		}
		if old.IsUrgent != window.IsUrgent {
			lines = append(lines, eventLine("WindowUrgencyChanged", struct {
				ID     uint64 `json:"id"`
				Urgent bool   `json:"urgent"`
			}{ID: window.ID, Urgent: window.IsUrgent}))
		}
	}
	for _, window := range before {
		if _, exists := seen[window.ID]; !exists {
			lines = append(lines, eventLine("WindowClosed", struct {
				ID uint64 `json:"id"`
			}{ID: window.ID}))
		}
	}
	if hadFocusedWindow && focusedWindowID(after) == nil {
		lines = append(lines, eventLine("WindowFocusChanged", struct {
			ID *uint64 `json:"id"`
		}{}))
	}
	return lines
}

func focusedWindowID(windows []ipc.Window) *uint64 {
	for _, window := range windows {
		if window.IsFocused {
			id := window.ID
			return &id
		}
	}
	return nil
}

func windowDetailsChanged(before, after ipc.Window) bool {
	return before.WorkspaceID == nil && after.WorkspaceID != nil ||
		before.WorkspaceID != nil && after.WorkspaceID == nil ||
		before.WorkspaceID != nil && after.WorkspaceID != nil && *before.WorkspaceID != *after.WorkspaceID ||
		!reflect.DeepEqual(before.Title, after.Title) || !reflect.DeepEqual(before.AppID, after.AppID) ||
		before.IsFloating != after.IsFloating
}

func workspaceDetailsChanged(before, after []ipc.Workspace) bool {
	if len(before) != len(after) {
		return true
	}
	for i := range before {
		previous, current := before[i], after[i]
		previous.IsUrgent, previous.IsActive, previous.IsFocused, previous.ActiveWindowID = false, false, false, nil
		current.IsUrgent, current.IsActive, current.IsFocused, current.ActiveWindowID = false, false, false, nil
		if !reflect.DeepEqual(previous, current) {
			return true
		}
	}
	return false
}

func workspaceActivatedLine(id uint64, focused bool) []byte {
	return eventLine("WorkspaceActivated", struct {
		ID      uint64 `json:"id"`
		Focused bool   `json:"focused"`
	}{ID: id, Focused: focused})
}

func eventLine(name string, value any) []byte {
	line, err := json.Marshal(map[string]any{name: value})
	if err != nil {
		panic("sysc-compositor: an internal event payload was not JSON encodable: " + err.Error())
	}
	return append(line, '\n')
}
