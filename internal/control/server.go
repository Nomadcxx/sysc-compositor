package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

// Server accepts peer-verified connections on one socket.
type Server struct {
	ln      *net.UnixListener
	path    string
	handler Handler
	uid     uint32
	next    atomic.Uint64
	wg      sync.WaitGroup
	mu      sync.Mutex
	conns   map[*net.UnixConn]struct{}
	closed  bool
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
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, fmt.Errorf("control: %s is already served", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// The runtime directory is 0700, so no other user can reach the socket
	// between bind and chmod; the peer check below is the per-connection rule.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		ln.Close()
		return nil, fmt.Errorf("control: %s is not mode 0600", path)
	}
	return &Server{ln: ln, path: path, handler: h, uid: uint32(os.Getuid()),
		conns: map[*net.UnixConn]struct{}{}}, nil
}

// Serve accepts until Close.
func (s *Server) Serve() error {
	for {
		c, err := s.ln.AcceptUnix()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		if !s.peerOK(c) {
			c.Close()
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return nil
		}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go s.serveConn(ConnID(s.next.Add(1)), c)
	}
}

// Close stops accepting, ends every connection and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	err := s.ln.Close()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	_ = os.Remove(s.path)
	return err
}

func (s *Server) peerOK(c *net.UnixConn) bool {
	raw, err := c.SyscallConn()
	if err != nil {
		return false
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		credErr = err
	}
	var peer uint32
	if cred != nil {
		peer = cred.Uid
	}
	return acceptPeer(peer, s.uid, credErr)
}

// acceptPeer is the identity rule: only the compositor's own uid. This is a
// process-identity boundary, not an authorisation (design D3.2).
func acceptPeer(peer, own uint32, err error) bool {
	return err == nil && peer == own
}

func (s *Server) serveConn(id ConnID, c *net.UnixConn) {
	defer func() {
		s.handler.Disconnect(id)
		c.Close()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		s.wg.Done()
	}()
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
