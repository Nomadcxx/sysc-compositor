package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Nomadcxx/sysc-compositor/internal/unixsock"
)

// ConnID names one accepted connection for the lifetime of the server.
type ConnID uint64

// Handler answers requests. Handle is called from the connection's goroutine;
// Disconnect is called once when the connection ends.
type Handler interface {
	Handle(id ConnID, r Request, c *Conn) Reply
	Disconnect(id ConnID)
}

// Conn is the write side of one connection. Replies and pushed facts share it.
type Conn struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// Send writes one JSON line.
func (c *Conn) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

// Server serves the control wire on one peer-verified socket.
type Server struct {
	l       *unixsock.Listener
	handler Handler
	next    atomic.Uint64
}

// SocketPath is the control socket for a compositor serving the Wayland
// socket named display, inside the user's runtime directory.
func SocketPath(runtimeDir, display string) (string, error) {
	if !filepath.IsAbs(runtimeDir) {
		return "", errors.New("control: XDG_RUNTIME_DIR must be an absolute path")
	}
	if display == "" || strings.ContainsRune(display, '/') || strings.HasPrefix(display, ".") {
		return "", fmt.Errorf("control: bad display socket name %q", display)
	}
	return filepath.Join(runtimeDir, display+"-control.sock"), nil
}

// Listen binds the socket with mode 0600. A stale socket left by a dead
// compositor is replaced; a live one is refused.
func Listen(path string, h Handler) (*Server, error) {
	l, err := unixsock.Listen(path)
	if err != nil {
		return nil, err
	}
	return &Server{l: l, handler: h}, nil
}

// Serve accepts until Close.
func (s *Server) Serve() error {
	return s.l.Serve(func(c *net.UnixConn) { s.serveConn(ConnID(s.next.Add(1)), c) })
}

// Close stops accepting, ends every connection and removes the socket.
func (s *Server) Close() error { return s.l.Close() }

func (s *Server) serveConn(id ConnID, c *net.UnixConn) {
	defer s.handler.Disconnect(id)
	out := &Conn{enc: json.NewEncoder(c)}
	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, 4096), MaxLine+1)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var r Request
		if err := json.Unmarshal(line, &r); err != nil {
			if out.Send(fail(0, CodeBadRequest, "malformed JSON")) != nil {
				return
			}
			continue
		}
		if out.Send(s.handler.Handle(id, r, out)) != nil {
			return
		}
	}
	if errors.Is(scanner.Err(), bufio.ErrTooLong) {
		_ = out.Send(fail(0, CodeBadRequest, fmt.Sprintf("line exceeds %d bytes", MaxLine)))
	}
}
