package wlr

import (
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestOverviewProjectionSurvivesBufferCommitAndUnmap(t *testing.T) {
	c := newNested(t)

	var view *toplevelView
	var rootSurface, childSurface unsafe.Pointer
	var probe *projectionProbe
	var rootCommit, rootDestroy, childCommit, childDestroy *Listener
	var previousChild uintptr
	var rootBufferShared, childBufferShared bool
	var childProjected, childBufferUpdated, childRemoved, projectionDestroyed bool
	var childHasCurrentBuffer bool
	var rootSubsurfaceCount int
	var rootCommitCount, childCommitCount int
	var latestStats [4]uintptr
	clientReturned := make(chan struct{})
	rootDestroyed := make(chan struct{})
	abort := make(chan struct{})
	t.Cleanup(func() {
		close(abort)
		if probe != nil {
			probe.destroy()
			probe.free()
		}
	})

	c.viewMapped = func(mapped *toplevelView) {
		if view != nil {
			return
		}
		view = mapped
		rootSurface = viewSurface(view)
		probe = newProjectionProbe(c, view)
		if probe == nil {
			c.Terminate()
			return
		}
		first, _, buffers, lockDelta := probe.stats()
		latestStats = [4]uintptr{first, 0, uintptr(buffers), uintptr(lockDelta)}
		rootBufferShared = buffers > 0 && lockDelta > 0
		rootCommitCount = 1

		rootCommit = AddListener(surfaceCommitSignal(rootSurface), func(unsafe.Pointer) {
			_ = c.Post(func() {
				if probe == nil {
					return
				}
				probe.refreshSource()
				first, _, buffers, lockDelta := probe.stats()
				latestStats = [4]uintptr{first, 0, uintptr(buffers), uintptr(lockDelta)}
				rootCommitCount++
				rootBufferShared = rootBufferShared || (buffers > 0 && lockDelta > 0)
			})
		})
		c.keep(rootCommit)
		rootDestroy = AddListener(surfaceDestroySignal(rootSurface), func(unsafe.Pointer) {
			if rootCommit != nil {
				c.drop(rootCommit)
				rootCommit = nil
			}
			c.drop(rootDestroy)
			rootDestroy = nil
			_ = c.Post(func() {
				if probe != nil {
					projectionDestroyed = probe.destroy()
				}
				close(rootDestroyed)
			})
		})
		c.keep(rootDestroy)
	}

	surfaceListener := AddListener(c.surfaceCreatedSignal(), func(data unsafe.Pointer) {
		surface := data
		if view == nil || surface == rootSurface || childSurface != nil {
			return
		}
		childSurface = surface
		childCommit = AddListener(surfaceCommitSignal(surface), func(unsafe.Pointer) {
			// Capture the source state while the commit signal owns the current
			// surface state. The client can destroy the child before Post runs.
			currentBuffer, _ := surfaceSubsurfaceState(surface)
			childHasCurrentBuffer = childHasCurrentBuffer || currentBuffer != 0
			_ = c.Post(func() {
				if probe == nil {
					return
				}
				probe.refreshSource()
				first, last, buffers, lockDelta := probe.stats()
				_, children := surfaceSubsurfaceState(rootSurface)
				rootSubsurfaceCount = max(rootSubsurfaceCount, children)
				latestStats = [4]uintptr{first, last, uintptr(buffers), uintptr(lockDelta)}
				childCommitCount++
				childBufferShared = childBufferShared || (buffers >= 2 && lockDelta > 0)
				childProjected = childProjected || (buffers >= 2 && first != 0 && last != 0)
				if previousChild != 0 && last != previousChild {
					childBufferUpdated = true
				}
				previousChild = last
			})
		})
		c.keep(childCommit)
		childDestroy = AddListener(surfaceDestroySignal(surface), func(unsafe.Pointer) {
			if childCommit != nil {
				c.drop(childCommit)
				childCommit = nil
			}
			c.drop(childDestroy)
			childDestroy = nil
			_ = c.Post(func() {
				if probe == nil {
					return
				}
				probe.refreshSource()
				_, _, buffers, _ := probe.stats()
				childRemoved = buffers == 1
			})
		})
		c.keep(childDestroy)
	})
	c.keep(surfaceListener)

	var clientError error
	go func() {
		clientError = testclient.RunSubsurfaceSequence(c.SocketName())
		close(clientReturned)
		select {
		case <-rootDestroyed:
			c.Terminate()
		case <-abort:
		}
	}()
	timeout := time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	timeout.Stop()
	select {
	case <-clientReturned:
	case <-time.After(time.Second):
		t.Fatal("Wayland subsurface test client did not return")
	}

	if clientError != nil {
		t.Fatal(clientError)
	}
	if view == nil || rootSurface == nil || childSurface == nil {
		t.Fatal("test client did not map an xdg toplevel and create its subsurface")
	}
	if !rootBufferShared || !childBufferShared {
		t.Fatalf("projection buffer references: root=%v child=%v stats=%v commits=%d/%d projected=%v current-buffer=%v root-subsurfaces=%d, want both shared", rootBufferShared, childBufferShared, latestStats, rootCommitCount, childCommitCount, childProjected, childHasCurrentBuffer, rootSubsurfaceCount)
	}
	if rootCommitCount == 0 || childCommitCount < 2 {
		t.Fatalf("surface commits: root=%d child=%d, want root and at least 2 child commits", rootCommitCount, childCommitCount)
	}
	if !childProjected || !childBufferUpdated {
		t.Fatalf("subsurface projection: present=%v updated=%v, want a live child buffer and its next commit", childProjected, childBufferUpdated)
	}
	if !childHasCurrentBuffer || rootSubsurfaceCount != 1 {
		t.Fatalf("source surface state: child buffer=%v root subsurfaces=%d, want mapped child and one attached subsurface", childHasCurrentBuffer, rootSubsurfaceCount)
	}
	if !childRemoved {
		t.Fatal("projection retained the subsurface buffer after its wl_surface was destroyed")
	}
	if !projectionDestroyed {
		t.Fatal("projection nodes remained after the root wl_surface was destroyed")
	}
}
