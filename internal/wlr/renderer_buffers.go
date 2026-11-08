package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_buffer.h>
*/
import "C"

import "unsafe"

type rendererBuffer struct {
	pixels  *C.struct_wlr_buffer
	destroy *Listener
}

type rendererSurface struct {
	pending                  map[uint32]*C.struct_wlr_buffer
	commit, applied, destroy *Listener
}

// Preserve submitted SHM contents, including cached subsurface commits, before
// wlroots uploads them and releases the client's buffer. Its client-buffer
// cache may reuse a texture without updating its weak source pointer.
func (c *Compositor) watchRendererBuffers() {
	c.rendererBuffers = map[*C.struct_wlr_client_buffer]*rendererBuffer{}
	c.rendererSurfaces = map[*C.struct_wlr_surface]*rendererSurface{}
	c.keep(AddListener(unsafe.Pointer(&c.comp.events.new_surface), func(data unsafe.Pointer) {
		surface := (*C.struct_wlr_surface)(data)
		watch := &rendererSurface{pending: map[uint32]*C.struct_wlr_buffer{}}
		c.rendererSurfaces[surface] = watch
		watch.commit = AddListener(unsafe.Pointer(&surface.events.client_commit), func(unsafe.Pointer) {
			if surface.pending.committed&C.WLR_SURFACE_STATE_BUFFER == 0 || surface.pending.buffer == nil {
				return
			}
			pixels := copyBufferPixels(surface.pending.buffer)
			if old := watch.pending[uint32(surface.pending.seq)]; old != nil {
				C.wlr_buffer_drop(old)
			}
			watch.pending[uint32(surface.pending.seq)] = pixels
		})
		watch.applied = AddListener(unsafe.Pointer(&surface.events.commit), func(unsafe.Pointer) {
			seq := uint32(surface.current.seq)
			pixels, present := watch.pending[seq]
			if !present {
				return
			}
			delete(watch.pending, seq)
			client := surface.buffer
			if pixels == nil {
				if backup := c.rendererBuffers[client]; backup != nil {
					C.wlr_buffer_drop(backup.pixels)
					c.drop(backup.destroy)
					delete(c.rendererBuffers, client)
				}
				return
			}
			if client == nil {
				if pixels != nil {
					C.wlr_buffer_drop(pixels)
				}
				return
			}
			if backup := c.rendererBuffers[client]; backup != nil {
				C.wlr_buffer_drop(backup.pixels)
				backup.pixels = pixels
				return
			}
			backup := &rendererBuffer{pixels: pixels}
			backup.destroy = AddListener(unsafe.Pointer(&client.base.events.destroy), func(unsafe.Pointer) {
				C.wlr_buffer_drop(backup.pixels)
				delete(c.rendererBuffers, client)
				c.drop(backup.destroy)
			})
			c.rendererBuffers[client] = backup
			c.keep(backup.destroy)
		})
		watch.destroy = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
			for _, pixels := range watch.pending {
				if pixels != nil {
					C.wlr_buffer_drop(pixels)
				}
			}
			delete(c.rendererSurfaces, surface)
			c.drop(watch.commit)
			c.drop(watch.applied)
			c.drop(watch.destroy)
		})
		c.keep(watch.commit, watch.applied, watch.destroy)
	}))
}

func (c *Compositor) finishRendererBuffers() {
	for _, backup := range c.rendererBuffers {
		C.wlr_buffer_drop(backup.pixels)
		c.drop(backup.destroy)
	}
	for _, watch := range c.rendererSurfaces {
		for _, pixels := range watch.pending {
			if pixels != nil {
				C.wlr_buffer_drop(pixels)
			}
		}
		c.drop(watch.commit)
		c.drop(watch.applied)
		c.drop(watch.destroy)
	}
	c.rendererBuffers = nil
	c.rendererSurfaces = nil
}
