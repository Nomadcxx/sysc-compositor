package wlr

/*
#cgo pkg-config: wlroots-0.20
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_scene.h>

static bool sysc_scene_output_needs_frame(void *scene_output) {
	return wlr_scene_output_needs_frame(scene_output);
}

// The buffer a commit put on screen, locked, or NULL if it carried none.
static struct wlr_buffer *sysc_committed_buffer(struct wlr_output_event_commit *event) {
	if (!(event->state->committed & WLR_OUTPUT_STATE_BUFFER) || event->state->buffer == NULL) {
		return NULL;
	}
	return wlr_buffer_lock(event->state->buffer);
}
*/
import "C"

import (
	"errors"
	"image"
	"unsafe"
)

// keepCommitted holds a reference to the buffer an output's commit put on
// screen, while a test has turned committedBuffers on. Pixel tests that render
// the scene afresh cannot see stale pixels the output's own buffers keep;
// this buffer is what the output shows.
func (c *Compositor) keepCommitted(output, event unsafe.Pointer) {
	if c.committedBuffers == nil {
		return
	}
	buffer := C.sysc_committed_buffer((*C.struct_wlr_output_event_commit)(event))
	if buffer == nil {
		return
	}
	c.releaseCommitted(output)
	c.committedBuffers[output] = unsafe.Pointer(buffer)
}

func (c *Compositor) releaseCommitted(output unsafe.Pointer) {
	if buffer := c.committedBuffers[output]; buffer != nil {
		C.wlr_buffer_unlock((*C.struct_wlr_buffer)(buffer))
		delete(c.committedBuffers, output)
	}
}

// committedImage reads back the buffer output last committed.
func (c *Compositor) committedImage(output unsafe.Pointer) (*image.NRGBA, error) {
	buffer := (*C.struct_wlr_buffer)(c.committedBuffers[output])
	if buffer == nil {
		return nil, errors.New("wlr: the output committed no buffer")
	}
	return c.readBuffer(buffer, image.Rect(0, 0, int(buffer.width), int(buffer.height)))
}

// outputsSettled reports whether no animation runs and every output has
// committed all of its damage.
func (c *Compositor) outputsSettled() bool {
	if len(c.animations) > 0 {
		return false
	}
	for _, so := range c.sceneOutputs {
		if bool(C.sysc_scene_output_needs_frame(unsafe.Pointer(so))) {
			return false
		}
	}
	return true
}
