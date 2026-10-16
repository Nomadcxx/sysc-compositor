// Package unixsock is the listener the compositor's sockets share: mode 0600,
// same-uid peers only, one goroutine per connection, and a Close that ends
// every connection and removes the socket.
package unixsock

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
)

// Listener accepts peer-verified connections on one socket.
type Listener struct {
	ln     *net.UnixListener
	path   string
	uid    uint32
	wg     sync.WaitGroup
	mu     sync.Mutex
	conns  map[*net.UnixConn]struct{}
	closed bool
}

// Listen binds path with mode 0600. A stale socket left by a dead process is
// replaced; a live one is refused.
func Listen(path string) (*Listener, error) {
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, fmt.Errorf("unixsock: %s is already served", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// The runtime directory is 0700, so no other user can reach the socket
	// between bind and chmod; the peer check is the per-connection rule.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		ln.Close()
		return nil, fmt.Errorf("unixsock: %s is not mode 0600", path)
	}
	return &Listener{ln: ln, path: path, uid: uint32(os.Getuid()),
		conns: map[*net.UnixConn]struct{}{}}, nil
}

// Path is the socket's path.
func (l *Listener) Path() string { return l.path }

// Serve accepts until Close, running serve on its own goroutine for each
// connection from the same uid. The connection is closed when serve returns.
func (l *Listener) Serve(serve func(*net.UnixConn)) error {
	for {
		c, err := l.ln.AcceptUnix()
		if err != nil {
			l.mu.Lock()
			closed := l.closed
			l.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		if !peerOK(c, l.uid) {
			c.Close()
			continue
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			c.Close()
			return nil
		}
		l.conns[c] = struct{}{}
		l.wg.Add(1)
		l.mu.Unlock()
		go func() {
			defer func() {
				c.Close()
				l.mu.Lock()
				delete(l.conns, c)
				l.mu.Unlock()
				l.wg.Done()
			}()
			serve(c)
		}()
	}
}

// Close stops accepting, ends every connection, waits for their goroutines
// and removes the socket.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	err := l.ln.Close()
	for c := range l.conns {
		c.Close()
	}
	l.mu.Unlock()
	l.wg.Wait()
	_ = os.Remove(l.path)
	return err
}

func peerOK(c *net.UnixConn, own uint32) bool {
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
	return acceptPeer(peer, own, credErr)
}

// acceptPeer is the identity rule: only the compositor's own uid. This is a
// process-identity boundary, not an authorisation (design D3.2).
func acceptPeer(peer, own uint32, err error) bool {
	return err == nil && peer == own
}
