package server

import (
	"context"
	"net"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"golang.org/x/sys/unix"
)

// Observe peer shutdown without consuming bytes owned by the IPC scanner.
// The duplicate keeps poll from observing a reused descriptor after Close.
func unixConnectionContext(connection *net.UnixConn) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	file, err := connection.File()
	if err != nil {
		cancel()
		return ctx, cancel
	}
	go func() {
		defer file.Close()
		defer cancel()
		descriptors := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLRDHUP}}
		for ctx.Err() == nil {
			_, err := unix.Poll(descriptors, 100)
			if err == unix.EINTR {
				continue
			}
			if err != nil || descriptors[0].Revents != 0 {
				return
			}
		}
	}()
	return ctx, cancel
}

func (s *Server) ipcPickWindow(ctx context.Context) (*ipc.Window, error) {
	type result struct {
		window *ipc.Window
		err    error
	}
	reply := make(chan result, 1)
	if err := s.c.Post(func() {
		if ctx.Err() != nil {
			return
		}
		var stop func() bool
		cancel, err := s.c.PickWindow(func(id uint64, err error) {
			if stop != nil {
				stop()
			}
			var window *ipc.Window
			if err == nil && id != 0 {
				for _, candidate := range s.state.snapshot().windows {
					if candidate.ID == id {
						copy := candidate
						window = &copy
						break
					}
				}
			}
			reply <- result{window, err}
		})
		if err != nil {
			reply <- result{err: err}
			return
		}
		stop = context.AfterFunc(ctx, func() { _ = s.c.Post(cancel) })
	}); err != nil {
		return nil, err
	}
	select {
	case got := <-reply:
		return got.window, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stopping:
		return nil, errStopping
	}
}

func (s *Server) ipcPickColor(ctx context.Context) ([3]float64, error) {
	// The portal and IPC share one correlated color-pick implementation.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-s.stopping:
			cancel()
		case <-stopped:
		}
	}()
	return s.portalPickColor(ctx)
}
