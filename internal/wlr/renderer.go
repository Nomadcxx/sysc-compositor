package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server egl
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <EGL/egl.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/render/wlr_texture.h>
#include <wlr/types/wlr_compositor.h>
#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_linux_dmabuf_v1.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_damage_ring.h>
static bool sysc_renderer_check_dmabuf(struct wlr_dmabuf_attributes *attributes, void *data) {
	struct wlr_texture *texture = wlr_texture_from_dmabuf(data,attributes);
	if (texture == NULL) return false;
	wlr_texture_destroy(texture);
	return true;
}
// sysc_renderer_pin_context leaves the renderer's EGL context current on the
// compositor thread. SceneFX restores the previous binding after every pass
// and texture import; with nothing bound, that releases the context each time,
// and NVIDIA's driver spins in eglMakeCurrent on every release. A pass on a
// scratch buffer exposes the context SceneFX keeps private.
static bool sysc_renderer_pin_context(struct wlr_renderer *renderer, struct wlr_buffer *scratch) {
	struct wlr_render_pass *pass = wlr_renderer_begin_buffer_pass(renderer, scratch, NULL);
	if (pass == NULL) {
		return false;
	}
	EGLDisplay display = eglGetCurrentDisplay();
	EGLContext context = eglGetCurrentContext();
	if (!wlr_render_pass_submit(pass) || context == EGL_NO_CONTEXT) {
		return false;
	}
	return eglMakeCurrent(display, EGL_NO_SURFACE, EGL_NO_SURFACE, context) &&
		eglGetCurrentContext() == context;
}

static void sysc_renderer_rebind_dmabuf(struct wlr_scene *scene, struct wlr_renderer *renderer) {
	if (scene->linux_dmabuf_v1 != NULL)
		wlr_linux_dmabuf_v1_set_check_dmabuf_callback(scene->linux_dmabuf_v1,sysc_renderer_check_dmabuf,renderer);
}
static void sysc_renderer_damage_output(struct wlr_scene_output *scene) {
	wlr_damage_ring_add_whole(&scene->damage_ring);
	wlr_output_schedule_frame(scene->output);
}
static bool sysc_renderer_restore_client(struct wlr_client_buffer *client, struct wlr_buffer *source, struct wlr_renderer *renderer) {
	if (client->texture != NULL) return true;
	if (source == NULL) return false;
	client->texture = wlr_texture_from_buffer(renderer,source);
	if (client->texture == NULL) return false;
	// Reuse wlroots' texture lifetime listener after its old renderer died.
	wl_signal_add(&renderer->events.destroy,&client->WLR_PRIVATE.renderer_destroy);
	return true;
}
static bool sysc_renderer_source_dmabuf(struct wlr_buffer *source) {
	struct wlr_dmabuf_attributes attributes;
	return source != NULL && wlr_buffer_get_dmabuf(source,&attributes);
}

*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

func (c *Compositor) watchRendererLoss() {
	c.rendererLost = AddListener(unsafe.Pointer(&c.renderer.events.lost), func(unsafe.Pointer) {
		if c.rendererRecoveryQueued {
			return
		}
		c.rendererRecoveryQueued = true
		// Leave the renderer's signal stack before replacing its object graph.
		if err := c.Post(func() {
			c.rendererRecoveryQueued = false
			if err := c.recoverRenderer(); err != nil {
				fmt.Fprintf(os.Stderr, "sysc-compositor: renderer recovery: %v\n", err)
				c.Terminate()
			}
		}); err != nil {
			c.rendererRecoveryQueued = false
		}
	})
	c.keep(c.rendererLost)
}

// pinRendererContext keeps the renderer's context bound between frames, as
// Smithay and KWin do, so SceneFX's per-pass restore is a no-op rather than a
// release. Without it the compositor still draws, only more slowly on NVIDIA.
func (c *Compositor) pinRendererContext() {
	if c.animationShaders == nil || c.animationShaders.scratch == nil {
		return
	}
	if !bool(C.sysc_renderer_pin_context(c.renderer, c.animationShaders.scratch)) {
		fmt.Fprintln(os.Stderr, "sysc-compositor: could not keep the EGL context current")
	}
}

// recoverRenderer follows Mango's do_renderer_recreate. wlroots and the scene
// release cached textures when the old renderer dies, then import retained
// buffers on the next frame.
func (c *Compositor) recoverRenderer() error {
	if c.renderer == nil {
		return ErrClosed
	}
	renderer := (*C.struct_wlr_renderer)(c.createConfiguredRenderer())
	if renderer == nil {
		return fmt.Errorf("recreating renderer failed")
	}
	allocator := C.wlr_allocator_autocreate(c.backend, renderer)
	if allocator == nil {
		C.wlr_renderer_destroy(renderer)
		return fmt.Errorf("recreating allocator failed")
	}
	retained := map[*C.struct_wlr_client_buffer]*C.struct_wlr_buffer{}
	walkCaptureScene(&c.scene.tree.node, func(node *C.struct_wlr_scene_node) {
		if node._type != C.WLR_SCENE_NODE_BUFFER {
			return
		}
		sceneBuffer := C.wlr_scene_buffer_from_node(node)
		if sceneBuffer.buffer == nil {
			return
		}
		client := C.wlr_client_buffer_get(sceneBuffer.buffer)
		if client == nil || c.rendererBuffers[client] != nil || retained[client] != nil {
			return
		}
		if bool(C.sysc_renderer_source_dmabuf(client.source)) {
			retained[client] = C.wlr_buffer_lock(client.source)
		}
	})
	defer func() {
		for _, source := range retained {
			C.wlr_buffer_unlock(source)
		}
	}()
	c.finishAnimationShaders()
	oldRenderer, oldAllocator := c.renderer, c.alloc
	c.drop(c.rendererLost)
	c.renderer, c.alloc = renderer, allocator
	c.configureAnimationShaders()
	C.wlr_compositor_set_renderer(c.comp, renderer)
	var failed string
	for pointer := range c.backendOutputs {
		if !bool(C.wlr_output_init_render((*C.struct_wlr_output)(pointer), allocator, renderer)) {
			failed = outputName(pointer)
			break
		}
	}
	c.watchRendererLoss()
	C.wlr_allocator_destroy(oldAllocator)
	// Destroying the old EGL context releases this thread's binding.
	C.wlr_renderer_destroy(oldRenderer)
	c.pinRendererContext()
	C.sysc_renderer_rebind_dmabuf(c.scene, renderer)
	if failed != "" {
		return fmt.Errorf("reinitializing output %s failed", failed)
	}
	for client, backup := range c.rendererBuffers {
		if !bool(C.sysc_renderer_restore_client(client, backup.pixels, renderer)) {
			return fmt.Errorf("restoring a SHM texture failed")
		}
	}
	for client, source := range retained {
		if !bool(C.sysc_renderer_restore_client(client, source, renderer)) {
			return fmt.Errorf("restoring a DMA-BUF texture failed")
		}
	}
	var unavailable bool
	walkCaptureScene(&c.scene.tree.node, func(node *C.struct_wlr_scene_node) {
		if node._type == C.WLR_SCENE_NODE_BUFFER {
			buffer := C.wlr_scene_buffer_from_node(node)
			if buffer.buffer != nil {
				client := C.wlr_client_buffer_get(buffer.buffer)
				if client != nil && client.texture == nil {
					unavailable = true
				}
			}
		}
	})
	if unavailable {
		return fmt.Errorf("a client buffer has no recoverable contents")
	}

	for pointer, sceneOutput := range c.sceneOutputs {
		c.invalidateBackdropBlur(pointer)
		C.sysc_renderer_damage_output(sceneOutput)
	}
	c.damageCaptureSources()
	return nil
}
