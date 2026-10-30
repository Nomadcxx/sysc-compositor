// Package x11 reserves X11 sockets and activates xwayland-satellite on demand.
// Satellite owns X11; its Wayland windows use the compositor's usual xdg path.
package x11

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ownedPath struct {
	path string
	info os.FileInfo
}

// Satellite owns the display number and listeners across child exits.
type Satellite struct {
	display             string
	wayland             string
	files               []*os.File
	paths               []ownedPath
	epoll               int
	wakeRead, wakeWrite *os.File
	cancel              context.CancelFunc
	done                chan struct{}
	once                sync.Once
	mu                  sync.Mutex
	path                string
}

// Open probes on-demand support before reserving or advertising a display.
func Open(path, wayland string) (*Satellite, error) { return open(path, wayland, "/tmp") }

func open(path, wayland, root string) (_ *Satellite, result error) {
	if wayland == "" {
		return nil, errors.New("x11: empty Wayland display")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	probe := command(ctx, path, wayland, ":0", "--test-listenfd-support")
	err := probe.Run()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("x11: satellite on-demand probe: %w", err)
	}
	dir := filepath.Join(root, ".X11-unix")
	if err := os.Mkdir(dir, os.ModeSticky|0777); err == nil {
		if err := os.Chmod(dir, os.ModeSticky|0777); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	parent, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	owner := info.Sys().(*syscall.Stat_t).Uid
	if !info.IsDir() || info.Mode()&os.ModeSticky == 0 || info.Mode().Perm()&0022 != 0022 ||
		(owner != uint32(os.Getuid()) && owner != parent.Sys().(*syscall.Stat_t).Uid) {
		return nil, fmt.Errorf("x11: unsafe directory %s", dir)
	}
	s := &Satellite{wayland: wayland, path: path, epoll: -1, done: make(chan struct{})}
	defer func() {
		if result != nil {
			s.release()
		}
	}()
	// ponytail: skip occupied locks, including stale ones, as Niri does.
	// After 50 displays fail, report failure; add stale-lock recovery only if needed.
	for n := 0; n < 50; n++ {
		lockPath := filepath.Join(root, fmt.Sprintf(".X%d-lock", n))
		lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0444)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		info, err := lock.Stat()
		if err != nil {
			lock.Close()
			return nil, err
		}
		s.paths = append(s.paths, ownedPath{lockPath, info})
		_, err = fmt.Fprintf(lock, "%10d\n", os.Getpid())
		closeErr := lock.Close()
		if err != nil || closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		socketPath := filepath.Join(dir, fmt.Sprintf("X%d", n))
		// Do not unlink an existing pathname: a lock alone cannot establish
		// ownership of somebody else's live or stale socket.
		var unixFile, abstractFile *os.File
		unixFile, err = listen(socketPath)
		if err == nil {
			info, err = os.Lstat(socketPath)
			if err == nil {
				s.paths = append(s.paths, ownedPath{socketPath, info})
			}
		}
		if err == nil {
			abstractFile, err = listen("@" + socketPath)
		}
		if err != nil {
			if unixFile != nil {
				unixFile.Close()
			}
			s.releasePaths()
			if !errors.Is(err, syscall.EADDRINUSE) {
				return nil, fmt.Errorf("x11: bind display %d: %w", n, err)
			}
			continue
		}
		s.files = []*os.File{unixFile, abstractFile}
		s.display = fmt.Sprintf(":%d", n)
		break
	}
	if s.display == "" {
		return nil, errors.New("x11: no free display after 50 attempts")
	}
	s.epoll, err = syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	s.wakeRead, s.wakeWrite, err = os.Pipe()
	if err != nil {
		return nil, err
	}
	for _, file := range append([]*os.File{s.wakeRead}, s.files...) {
		if err := syscall.EpollCtl(s.epoll, syscall.EPOLL_CTL_ADD, int(file.Fd()),
			&syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(file.Fd())}); err != nil {
			return nil, err
		}
	}
	lifetime, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.serve(lifetime)
	return s, nil
}

func listen(path string) (*os.File, error) {
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	file, err := ln.File()
	ln.SetUnlinkOnClose(err != nil)
	ln.Close()
	return file, err
}

func command(ctx context.Context, path, wayland string, args ...string) *exec.Cmd {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "DISPLAY", "WAYLAND_DISPLAY", "WAYLAND_SOCKET", "NOTIFY_SOCKET", "RUST_BACKTRACE", "RUST_LIB_BACKTRACE":
		default:
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "WAYLAND_DISPLAY="+wayland)
	return cmd
}

func (s *Satellite) serve(ctx context.Context) {
	defer close(s.done)
	events := make([]syscall.EpollEvent, 3)
	for ctx.Err() == nil {
		count, err := syscall.EpollWait(s.epoll, events, -1)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "sysc-compositor: X11 readiness:", err)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if count == 0 {
			continue
		}
		s.mu.Lock()
		path := s.path
		s.mu.Unlock()
		cmd := command(ctx, path, s.wayland, s.display, "-listenfd", "3", "-listenfd", "4")
		cmd.ExtraFiles = s.files
		if err := cmd.Run(); ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "sysc-compositor: satellite exited: %v\n", err)
		}
		if ctx.Err() != nil {
			return
		}
		// Niri drains queued connections after failure/exit. Otherwise even
		// a departed client's pending connection can cause a restart busy loop.
		for _, file := range s.files {
			fd := int(file.Fd())
			if err := syscall.SetNonblock(fd, true); err != nil {
				return
			}
			for ctx.Err() == nil {
				conn, _, err := syscall.Accept4(fd, syscall.SOCK_CLOEXEC)
				if errors.Is(err, syscall.EINTR) {
					continue
				}
				if err != nil {
					break
				}
				syscall.Close(conn)
			}
			if err := syscall.SetNonblock(fd, false); err != nil {
				return
			}
		}
	}
}

func (s *Satellite) Display() string { return s.display }

// SetPath changes the executable used on the next activation, as Niri's reload.
func (s *Satellite) SetPath(path string) { s.mu.Lock(); s.path = path; s.mu.Unlock() }

func (s *Satellite) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.cancel()
		_, _ = s.wakeWrite.Write([]byte{1})
		<-s.done
		s.release()
	})
}

func (s *Satellite) releasePaths() {
	for _, owned := range s.paths {
		if current, err := os.Lstat(owned.path); err == nil && os.SameFile(owned.info, current) {
			if err := os.Remove(owned.path); err != nil {
				fmt.Fprintln(os.Stderr, "sysc-compositor: X11 cleanup:", err)
			}
		}
	}
	s.paths = nil
}

func (s *Satellite) release() {
	for _, file := range s.files {
		file.Close()
	}
	if s.wakeRead != nil {
		s.wakeRead.Close()
	}
	if s.wakeWrite != nil {
		s.wakeWrite.Close()
	}
	if s.epoll >= 0 {
		syscall.Close(s.epoll)
	}
	s.releasePaths()
}
