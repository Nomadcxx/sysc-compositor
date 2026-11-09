package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_output.h>

static struct wlr_buffer *sysc_probe_committed_buffer(void *data) {
	const struct wlr_output_event_commit *event = data;
	if (!(event->state->committed & WLR_OUTPUT_STATE_BUFFER)) {
		return NULL;
	}
	return event->state->buffer;
}
*/
import "C"

import (
	"image"
	"image/color"
	"unsafe"
)

// probeOutputFrames calls read with the centre pixel of each frame output
// commits. That is what reached the display, which a capture does not render:
// tests use it for drawing only the output's own repaint adds. The returned
// function stops the probe.
func (c *Compositor) probeOutputFrames(output unsafe.Pointer, read func(color.NRGBA)) func() {
	listener := AddListener(unsafe.Pointer(&(*C.struct_wlr_output)(output).events.commit), func(data unsafe.Pointer) {
		buffer := C.sysc_probe_committed_buffer(data)
		if buffer == nil {
			return
		}
		x, y := int(buffer.width)/2, int(buffer.height)/2
		if pixels, err := c.readBuffer((*C.struct_wlr_buffer)(unsafe.Pointer(buffer)), image.Rect(x, y, x+1, y+1)); err == nil {
			read(pixels.NRGBAAt(0, 0))
		}
	})
	c.keep(listener)
	return func() { c.drop(listener) }
}
