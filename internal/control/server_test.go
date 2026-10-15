package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPeerDecision(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		peer, own uint32
		err       error
		want      bool
	}{
		{"same uid", 1000, 1000, nil, true},
		{"other uid", 1001, 1000, nil, false},
		{"root is not the user", 0, 1000, nil, false},
		{"credentials unreadable", 1000, 1000, errors.New("EINVAL"), false},
	} {
		if got := acceptPeer(tc.peer, tc.own, tc.err); got != tc.want {
			t.Errorf("%s: acceptPeer = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSocketPath(t *testing.T) {
	t.Parallel()
	if got, err := SocketPath("/run/user/1000", "wayland-2"); err != nil || got != "/run/user/1000/wayland-2-control.sock" {
		t.Fatalf("SocketPath = %q, %v", got, err)
	}
	for _, bad := range [][2]string{{"", "wayland-2"}, {"relative", "wayland-2"}, {"/run/user/1000", ""}, {"/run/user/1000", "../x"}} {
		if _, err := SocketPath(bad[0], bad[1]); err == nil {
			t.Errorf("SocketPath(%q, %q) accepted", bad[0], bad[1])
		}
	}
}

// echoHandler answers every request with its own verb, so framing can be
// tested apart from the dispatcher.
type echoHandler struct {
	mu   sync.Mutex
	gone []ConnID
}

func (*echoHandler) Handle(_ ConnID, r Request, _ *Conn) Reply {
	return Reply{ID: r.ID, OK: true, Message: r.Verb}
}

func (h *echoHandler) Disconnect(c ConnID) {
	h.mu.Lock()
	h.gone = append(h.gone, c)
	h.mu.Unlock()
}

func serve(t *testing.T, h Handler) (string, *Server) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "w-control.sock")
	s, err := Listen(path, h)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(func() { _ = s.Close() })
	return path, s
}

func TestSocketModeAndRoundTrip(t *testing.T) {
	t.Parallel()
	h := &echoHandler{}
	path, _ := serve(t, h)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket mode = %v, want a 0600 socket", info.Mode())
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	exchange := func(line string) Reply {
		t.Helper()
		if _, err := conn.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		out, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var rep Reply
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatalf("reply %q: %v", out, err)
		}
		return rep
	}

	if rep := exchange(`{"id":7,"verb":"windows"}`); rep.ID != 7 || !rep.OK || rep.Message != "windows" {
		t.Fatalf("round trip = %+v", rep)
	}
	if rep := exchange(`{"id":8,"verb":`); rep.Error != CodeBadRequest {
		t.Fatalf("malformed JSON = %+v, want bad-request", rep)
	}
	if rep := exchange(`{"id":9,"verb":"state"}`); rep.ID != 9 || !rep.OK {
		t.Fatalf("connection did not survive malformed JSON: %+v", rep)
	}
	// An oversize line is refused and the connection closed.
	if _, err := conn.Write([]byte(`{"id":10,"verb":"type","text":"` + strings.Repeat("x", MaxLine) + "\"}\n")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	out, _ := r.ReadBytes('\n')
	var rep Reply
	_ = json.Unmarshal(out, &rep)
	if rep.Error != CodeBadRequest {
		t.Fatalf("oversize line = %q, want bad-request", out)
	}
	if _, err := r.ReadBytes('\n'); err == nil {
		t.Fatal("connection stayed open after an oversize line")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		n := len(h.gone)
		h.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Disconnect was not called for the closed connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListenRefusesALiveSocket(t *testing.T) {
	t.Parallel()
	path, _ := serve(t, &echoHandler{})
	if _, err := Listen(path, &echoHandler{}); err == nil {
		t.Fatal("a second Listen took over a live socket")
	}
}
