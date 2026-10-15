package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#include <stdint.h>
#include <sys/eventfd.h>
#include <wayland-server-core.h>

extern void sysc_post_fire(uintptr_t token);

static int sysc_post_trampoline(int fd, uint32_t mask, void *data) {
	(void)fd;
	(void)mask;
	sysc_post_fire((uintptr_t)data);
	return 0;
}

static struct wl_event_source *sysc_post_source(struct wl_event_loop *loop, int fd,
		uintptr_t token) {
	return wl_event_loop_add_fd(loop, fd, WL_EVENT_READABLE, sysc_post_trampoline,
		(void *)token);
}

static int sysc_post_eventfd(void) {
	return eventfd(0, EFD_NONBLOCK | EFD_CLOEXEC);
}
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"sync"
	"syscall"
	"unsafe"
)

// ErrClosed is returned by Post once the compositor is destroyed.
var ErrClosed = errors.New("wlr: compositor is closed")

// postQueue carries work from other goroutines to the compositor thread. The
// eventfd wakes the event loop; the queue itself is plain Go under a mutex.
type postQueue struct {
	mu     sync.Mutex
	fns    []func()
	fd     int
	closed bool
	source *C.struct_wl_event_source
	token  uintptr
}

// Post runs fn on the compositor thread, in the order Post was called from
// any one goroutine. It is the only way code off that thread may touch
// wlroots.
func (c *Compositor) Post(fn func()) error {
	q := &c.post
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.source == nil {
		return ErrClosed
	}
	q.fns = append(q.fns, fn)
	var one [8]byte
	binary.NativeEndian.PutUint64(one[:], 1)
	// EAGAIN means the counter is already non-zero: the loop is woken anyway.
	if _, err := syscall.Write(q.fd, one[:]); err != nil && err != syscall.EAGAIN {
		return err
	}
	return nil
}

func (c *Compositor) startPost() error {
	q := &c.post
	fd := int(C.sysc_post_eventfd())
	if fd < 0 {
		return errors.New("wlr: eventfd failed")
	}
	q.fd = fd
	q.token = Register(func(unsafe.Pointer) { c.drainPosted() })
	if q.source = C.sysc_post_source(c.loop, C.int(fd), C.uintptr_t(q.token)); q.source == nil {
		Unregister(q.token)
		syscall.Close(fd)
		return errors.New("wlr: wl_event_loop_add_fd failed")
	}
	return nil
}

func (c *Compositor) drainPosted() {
	q := &c.post
	var buf [8]byte
	q.mu.Lock()
	_, _ = syscall.Read(q.fd, buf[:])
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// stopPost refuses further work and drops anything queued. It runs before the
// display, and with it the event loop, is destroyed.
func (c *Compositor) stopPost() {
	q := &c.post
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.fns = nil
	if q.source != nil {
		C.wl_event_source_remove(q.source)
		q.source = nil
		Unregister(q.token)
		syscall.Close(q.fd)
	}
}
