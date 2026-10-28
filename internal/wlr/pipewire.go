package wlr

/*
#cgo pkg-config: libpipewire-0.3 wayland-server
#include <errno.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <pipewire/pipewire.h>
#if !PW_CHECK_VERSION(1,4,8)
#error PipeWire 1.4.8 or newer is required
#endif
#include <spa/param/video/format-utils.h>
#include <spa/buffer/meta.h>
#include <wayland-server-core.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);

struct sysc_pw_event { int kind; uint32_t node; int active; const char *error; struct pw_buffer *buffer; };
struct sysc_pw {
 struct pw_loop *loop;
 struct pw_context *context;
 struct pw_core *core;
 struct spa_hook core_hook;
 struct wl_event_source *source;
 uintptr_t token;
 int sync;
 bool entered, listening;
};
struct sysc_pw_stream {
 struct sysc_pw *owner;
 struct pw_stream *stream;
 struct spa_hook hook;
 struct spa_source *timer;
 uintptr_t token;
 uint32_t width, height, rate, negotiated_width, negotiated_height;
 uint64_t sequence;
 bool listening;
};

static void sysc_pw_core_done(void *data, uint32_t id, int seq) {
 struct sysc_pw *p = data;
 if (id == PW_ID_CORE && seq == p->sync) {
  struct sysc_pw_event event = { .kind = 1 };
  sysc_listener_fire(p->token, &event);
 }
}
static void sysc_pw_core_error(void *data, uint32_t id, int seq, int res, const char *message) {
 struct sysc_pw *p = data;
 if (id != PW_ID_CORE && res != -EPIPE && res != -ECONNRESET) return;
 struct sysc_pw_event event = { .kind = 2, .error = message };
 sysc_listener_fire(p->token, &event);
}
static const struct pw_core_events sysc_pw_core_events = {
 PW_VERSION_CORE_EVENTS, .done = sysc_pw_core_done, .error = sysc_pw_core_error,
};
static int sysc_pw_dispatch(int fd, uint32_t mask, void *data) {
 struct sysc_pw *p = data;
 int result = pw_loop_iterate(p->loop, 0);
 if (result < 0 || (mask & (WL_EVENT_HANGUP | WL_EVENT_ERROR))) {
  struct sysc_pw_event event = { .kind = 2, .error = "PipeWire loop disconnected" };
  sysc_listener_fire(p->token, &event);
 }
 return 0;
}
static void sysc_pw_free(struct sysc_pw *p) {
 if (p == NULL) return;
 if (p->source != NULL) wl_event_source_remove(p->source);
 if (p->listening) spa_hook_remove(&p->core_hook);
 if (p->core != NULL) pw_core_disconnect(p->core);
 if (p->context != NULL) pw_context_destroy(p->context);
 if (p->entered) pw_loop_leave(p->loop);
 if (p->loop != NULL) pw_loop_destroy(p->loop);
 free(p);
}
static struct sysc_pw *sysc_pw_new(struct wl_event_loop *owner, uintptr_t token) {
 struct sysc_pw *p = calloc(1, sizeof(*p));
 if (p == NULL) return NULL;
 p->token = token;
 p->loop = pw_loop_new(NULL);
 if (p->loop == NULL) goto failed;
 pw_loop_enter(p->loop);
 p->entered = true;
 p->context = pw_context_new(p->loop, pw_properties_new(PW_KEY_APP_NAME, "sysc-compositor", NULL), 0);
 if (p->context == NULL) goto failed;
 p->core = pw_context_connect(p->context, NULL, 0);
 if (p->core == NULL) goto failed;
 pw_core_add_listener(p->core, &p->core_hook, &sysc_pw_core_events, p);
 p->listening = true;
 p->source = wl_event_loop_add_fd(owner, pw_loop_get_fd(p->loop), WL_EVENT_READABLE,
   sysc_pw_dispatch, p);
 if (p->source == NULL) goto failed;
 p->sync = pw_core_sync(p->core, PW_ID_CORE, 0);
 if (p->sync < 0) goto failed;
 return p;
failed:
 sysc_pw_free(p);
 return NULL;
}

static const struct spa_pod *sysc_pw_format(struct sysc_pw_stream *s, struct spa_pod_builder *b) {
 struct spa_rectangle size = {s->width, s->height};
 struct spa_fraction fps = {s->rate, 1000};
 return spa_pod_builder_add_object(b, SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
  SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video),
  SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
  SPA_FORMAT_VIDEO_format, SPA_POD_Id(SPA_VIDEO_FORMAT_RGBA),
  SPA_FORMAT_VIDEO_size, SPA_POD_Rectangle(&size),
  SPA_FORMAT_VIDEO_framerate, SPA_POD_Fraction(&fps));
}
static void sysc_pw_stream_tick(void *data, uint64_t expirations) {
 struct sysc_pw_stream *s = data;
 pw_stream_trigger_process(s->stream);
}
static void sysc_pw_stream_timer(struct sysc_pw_stream *s, bool active) {
 struct timespec first = {0}, interval = {0};
 if (active) {
  uint64_t ns = UINT64_C(1000000000000) / s->rate;
  first.tv_nsec = 1;
  interval.tv_sec = ns / UINT64_C(1000000000);
  interval.tv_nsec = ns % UINT64_C(1000000000);
 }
 pw_loop_update_timer(s->owner->loop, s->timer, &first, &interval, false);
}
static void sysc_pw_stream_state(void *data, enum pw_stream_state old,
  enum pw_stream_state state, const char *error) {
 struct sysc_pw_stream *s = data;
 sysc_pw_stream_timer(s, state == PW_STREAM_STATE_STREAMING);
 struct sysc_pw_event event = { .kind = 3, .node = pw_stream_get_node_id(s->stream),
   .active = state == PW_STREAM_STATE_STREAMING,
   .error = state == PW_STREAM_STATE_ERROR ? error : NULL };
 sysc_listener_fire(s->token, &event);
}
static void sysc_pw_stream_format(void *data, uint32_t id, const struct spa_pod *param) {
 struct sysc_pw_stream *s = data;
 if (id != SPA_PARAM_Format || param == NULL) return;
 struct spa_video_info_raw info = {0};
 if (spa_format_video_raw_parse(param, &info) < 0 || info.format != SPA_VIDEO_FORMAT_RGBA ||
   info.size.width != s->width || info.size.height != s->height ||
   (uint64_t)info.size.width * info.size.height * 4 > UINT64_C(64) * 1024 * 1024) {
  pw_stream_set_error(s->stream, -EINVAL, "Unsupported screen-cast format");
  return;
 }
 s->negotiated_width = info.size.width;
 s->negotiated_height = info.size.height;
 uint8_t storage[512];
 struct spa_pod_builder b = SPA_POD_BUILDER_INIT(storage, sizeof(storage));
 const struct spa_pod *params[3];
 params[0] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
   SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int(4, 2, 4),
   SPA_PARAM_BUFFERS_blocks, SPA_POD_Int(1),
   SPA_PARAM_BUFFERS_size, SPA_POD_Int(s->width * s->height * 4),
   SPA_PARAM_BUFFERS_stride, SPA_POD_Int(s->width * 4),
   SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int(1 << SPA_DATA_MemFd));
 params[1] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
   SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Header),
   SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_header)));
 params[2] = spa_pod_builder_add_object(&b, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
   SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Cursor),
   SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_cursor) + sizeof(struct spa_meta_bitmap) + 384*384*4));
 pw_stream_update_params(s->stream, params, 3);
}
static void sysc_pw_stream_process(void *data) {
 struct sysc_pw_stream *s = data;
 struct pw_buffer *buffer = pw_stream_dequeue_buffer(s->stream);
 if (buffer == NULL) return;
 if (s->negotiated_width != s->width || s->negotiated_height != s->height) {
  pw_stream_return_buffer(s->stream, buffer);
  return;
 }
 struct sysc_pw_event event = { .kind = 4, .buffer = buffer };
 sysc_listener_fire(s->token, &event);
}
static const struct pw_stream_events sysc_pw_stream_events = {
 PW_VERSION_STREAM_EVENTS, .state_changed = sysc_pw_stream_state,
 .param_changed = sysc_pw_stream_format, .process = sysc_pw_stream_process,
};
static void sysc_pw_stream_free(struct sysc_pw_stream *s) {
 if (s == NULL) return;
 if (s->timer != NULL) pw_loop_destroy_source(s->owner->loop, s->timer);
 if (s->listening) spa_hook_remove(&s->hook);
 if (s->stream != NULL) pw_stream_destroy(s->stream);
 free(s);
}
static struct sysc_pw_stream *sysc_pw_stream_new(struct sysc_pw *owner,
  uintptr_t token, uint32_t width, uint32_t height, uint32_t rate) {
 struct sysc_pw_stream *s = calloc(1, sizeof(*s));
 if (s == NULL) return NULL;
 s->owner = owner; s->token = token; s->width = width; s->height = height; s->rate = rate;
 s->timer = pw_loop_add_timer(owner->loop, sysc_pw_stream_tick, s);
 if (s->timer == NULL) goto failed;
 s->stream = pw_stream_new(owner->core, "sysc screen cast", pw_properties_new(
  PW_KEY_MEDIA_TYPE, "Video", PW_KEY_MEDIA_CATEGORY, "Capture", PW_KEY_MEDIA_ROLE, "Screen",
  PW_KEY_NODE_NAME, "sysc-screen-cast", NULL));
 if (s->stream == NULL) goto failed;
 pw_stream_add_listener(s->stream, &s->hook, &sysc_pw_stream_events, s);
 s->listening = true;
 uint8_t storage[512];
 struct spa_pod_builder b = SPA_POD_BUILDER_INIT(storage, sizeof(storage));
 const struct spa_pod *format = sysc_pw_format(s, &b);
 if (pw_stream_connect(s->stream, PW_DIRECTION_OUTPUT, PW_ID_ANY,
   PW_STREAM_FLAG_DRIVER | PW_STREAM_FLAG_MAP_BUFFERS, &format, 1) < 0) goto failed;
 return s;
failed:
 sysc_pw_stream_free(s);
 return NULL;
}
static int sysc_pw_stream_reconfigure(struct sysc_pw_stream *s, uint32_t width, uint32_t height, uint32_t rate) {
 uint32_t old_width=s->width, old_height=s->height, old_rate=s->rate;
 s->width = width; s->height = height; s->rate=rate;
 uint8_t storage[512];
 struct spa_pod_builder b = SPA_POD_BUILDER_INIT(storage, sizeof(storage));
 const struct spa_pod *format = sysc_pw_format(s, &b);
 int result=pw_stream_update_params(s->stream, &format, 1);
 if(result<0) {s->width=old_width; s->height=old_height; s->rate=old_rate;}
 else sysc_pw_stream_timer(s,pw_stream_get_state(s->stream,NULL)==PW_STREAM_STATE_STREAMING);
 return result;
}
static bool sysc_pw_stream_write(struct sysc_pw_stream *s, struct pw_buffer *pwbuffer,
  const uint8_t *pixels, int source_stride, const uint8_t *pointer, int pointer_stride,
  uint32_t pointer_width, uint32_t pointer_height, int x, int y, int hotspot_x, int hotspot_y) {
 struct spa_buffer *buffer = pwbuffer->buffer;
 if (buffer->n_datas != 1) return false;
 struct spa_data *data = &buffer->datas[0];
 uint32_t stride = s->width * 4, size = stride * s->height;
 if (data->data == NULL || data->chunk == NULL || data->maxsize < size) return false;
 for (uint32_t y = 0; y < s->height; y++) {
  memcpy((uint8_t *)data->data + y * stride, pixels + y * source_stride, stride);
 }
 data->chunk->offset = 0;
 data->chunk->size = size;
 data->chunk->stride = stride;
 data->chunk->flags = 0;
 struct spa_meta_header *header = spa_buffer_find_meta_data(buffer, SPA_META_Header, sizeof(*header));
 if (header != NULL) {
  header->flags = 0; header->offset = 0;
  header->pts = pw_stream_get_nsec(s->stream);
  header->dts_offset = 0; header->seq = s->sequence++;
 }
 size_t cursor_size = sizeof(struct spa_meta_cursor) + sizeof(struct spa_meta_bitmap) + 384*384*4;
 struct spa_meta_cursor *cursor = spa_buffer_find_meta_data(buffer, SPA_META_Cursor, cursor_size);
 if (cursor != NULL) {
  memset(cursor, 0, cursor_size);
  cursor->id = 1;
  cursor->position = SPA_POINT(x, y);
  cursor->hotspot = SPA_POINT(hotspot_x, hotspot_y);
  cursor->bitmap_offset = sizeof(*cursor);
  struct spa_meta_bitmap *bitmap = SPA_PTROFF(cursor, cursor->bitmap_offset, struct spa_meta_bitmap);
  bitmap->format = SPA_VIDEO_FORMAT_RGBA;
  bitmap->size = SPA_RECTANGLE(pointer == NULL ? 1 : pointer_width, pointer == NULL ? 1 : pointer_height);
  bitmap->stride = bitmap->size.width * 4;
  bitmap->offset = sizeof(*bitmap);
  if (pointer != NULL) {
   uint8_t *destination = SPA_PTROFF(bitmap, bitmap->offset, uint8_t);
   for (uint32_t row = 0; row < pointer_height; row++) {
    memcpy(destination + row * bitmap->stride, pointer + row * pointer_stride, bitmap->stride);
   }
  }
 }
 return pw_stream_queue_buffer(s->stream, pwbuffer) >= 0;
}
static void sysc_pw_stream_return(struct sysc_pw_stream *s, struct pw_buffer *buffer) {
 pw_stream_return_buffer(s->stream, buffer);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"sync"
	"unsafe"
)

var pipeWireInit sync.Once

type pipeWire struct {
	c                           *Compositor
	native                      *C.struct_sysc_pw
	token                       uintptr
	ready, failed, closed       bool
	readyCallback, lostCallback func(error)
	streams                     map[*pipeWireStream]struct{}
}

type pipeWireStream struct {
	p              *pipeWire
	native         *C.struct_sysc_pw_stream
	token          uintptr
	width, height  int
	refreshMilliHz uint32
	closed         bool
	frame          func() (*image.NRGBA, error)
	changed        func(uint32, bool, error)
	pointer        *pipeWirePointer
}

type pipeWirePointer struct {
	Image             *image.RGBA
	Position, Hotspot image.Point
}

// PipeWire and its callbacks share the compositor event loop. In particular,
// process is not a realtime callback and may render through wlroots here.
func (c *Compositor) startPipeWire(ready, lost func(error)) error {
	if c.pipewire != nil {
		return errors.New("wlr: PipeWire is already started")
	}
	pipeWireInit.Do(func() { C.pw_init(nil, nil) })
	p := &pipeWire{c: c, readyCallback: ready, lostCallback: lost, streams: make(map[*pipeWireStream]struct{})}
	p.token = Register(func(data unsafe.Pointer) { p.event((*C.struct_sysc_pw_event)(data)) })
	p.native = C.sysc_pw_new(c.loop, C.uintptr_t(p.token))
	if p.native == nil {
		Unregister(p.token)
		return errors.New("wlr: connecting to PipeWire failed")
	}
	c.pipewire = p
	return nil
}

func (p *pipeWire) event(event *C.struct_sysc_pw_event) {
	if p.closed || p.failed {
		return
	}
	if event.kind == 1 {
		if p.ready {
			return
		}
		p.ready = true
		_ = p.c.Post(func() {
			if !p.closed && p.readyCallback != nil {
				p.readyCallback(nil)
			}
		})
		return
	}
	if event.kind != 2 {
		return
	}
	p.failed = true
	err := fmt.Errorf("wlr: PipeWire: %s", C.GoString(event.error))
	_ = p.c.Post(func() {
		if p.closed {
			return
		}
		p.c.stopPipeWire()
		if !p.ready && p.readyCallback != nil {
			p.readyCallback(err)
		} else if p.lostCallback != nil {
			p.lostCallback(err)
		}
	})
}

func (c *Compositor) stopPipeWire() {
	p := c.pipewire
	if p == nil {
		return
	}
	p.closed = true
	for stream := range p.streams {
		stream.close()
	}
	C.sysc_pw_free(p.native)
	p.native = nil
	Unregister(p.token)
	c.pipewire = nil
}

func (p *pipeWire) newStream(width, height int, refreshMilliHz uint32, frame func() (*image.NRGBA, error), changed func(uint32, bool, error)) (*pipeWireStream, error) {
	if p.closed || p.failed || !p.ready {
		return nil, errors.New("wlr: PipeWire is not ready")
	}
	if _, err := captureAllocation(width, height, width*4); err != nil {
		return nil, err
	}
	if refreshMilliHz == 0 || refreshMilliHz > 1_000_000 {
		return nil, errors.New("wlr: invalid screen-cast refresh rate")
	}
	if frame == nil {
		return nil, errors.New("wlr: a screen cast needs a frame handler")
	}
	s := &pipeWireStream{p: p, width: width, height: height, refreshMilliHz: refreshMilliHz, frame: frame, changed: changed}
	s.token = Register(func(data unsafe.Pointer) { s.event((*C.struct_sysc_pw_event)(data)) })
	s.native = C.sysc_pw_stream_new(p.native, C.uintptr_t(s.token), C.uint32_t(width), C.uint32_t(height), C.uint32_t(refreshMilliHz))
	if s.native == nil {
		Unregister(s.token)
		return nil, errors.New("wlr: creating PipeWire stream failed")
	}
	p.streams[s] = struct{}{}
	return s, nil
}

func (s *pipeWireStream) event(event *C.struct_sysc_pw_event) {
	if s.closed {
		return
	}
	if event.kind == 3 {
		node, active := uint32(event.node), bool(event.active != 0)
		var err error
		if event.error != nil {
			err = fmt.Errorf("wlr: PipeWire stream: %s", C.GoString(event.error))
		}
		_ = s.p.c.Post(func() {
			if !s.closed && s.changed != nil {
				s.changed(node, active, err)
			}
		})
		return
	}
	if event.kind != 4 {
		return
	}
	img, err := s.frame()
	if img == nil && err == nil {
		// The owner may be renegotiating dimensions; never queue an old image.
		C.sysc_pw_stream_return(s.native, event.buffer)
		return
	}
	if err == nil && (img == nil || img.Bounds().Dx() != s.width || img.Bounds().Dy() != s.height) {
		err = errors.New("wlr: screen-cast frame does not match negotiated size")
	}
	if err == nil {
		_, err = captureAllocation(s.width, s.height, img.Stride)
	}
	if err == nil {
		var pointerPixels *C.uint8_t
		var pointerStride, pointerWidth, pointerHeight, x, y, hotspotX, hotspotY C.int
		if pointer := s.pointer; pointer != nil && pointer.Image != nil {
			icon := pointer.Image
			width, height := min(icon.Bounds().Dx(), 384), min(icon.Bounds().Dy(), 384)
			if width > 0 && height > 0 {
				if _, e := captureAllocation(width, height, icon.Stride); e != nil || len(icon.Pix) < (height-1)*icon.Stride+width*4 {
					err = errors.New("wlr: invalid PipeWire pointer image")
				} else {
					pointerPixels = (*C.uint8_t)(unsafe.Pointer(&icon.Pix[0]))
					pointerStride, pointerWidth, pointerHeight = C.int(icon.Stride), C.int(width), C.int(height)
					x, y = C.int(pointer.Position.X), C.int(pointer.Position.Y)
					hotspotX, hotspotY = C.int(pointer.Hotspot.X), C.int(pointer.Hotspot.Y)
				}
			}
		}
		start := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
		if len(img.Pix)-start < (s.height-1)*img.Stride+s.width*4 {
			err = errors.New("wlr: short screen-cast image buffer")
		}
		if err == nil && !bool(C.sysc_pw_stream_write(s.native, event.buffer, (*C.uint8_t)(unsafe.Pointer(&img.Pix[start])), C.int(img.Stride),
			pointerPixels, pointerStride, C.uint32_t(pointerWidth), C.uint32_t(pointerHeight), x, y, hotspotX, hotspotY)) {
			err = errors.New("wlr: invalid PipeWire frame buffer")
		}
	}
	if err != nil {
		C.sysc_pw_stream_return(s.native, event.buffer)
		_ = s.p.c.Post(func() {
			if !s.closed && s.changed != nil {
				s.changed(^uint32(0), false, err)
			}
		})
	}
}

func (s *pipeWireStream) resize(width, height int) error {
	return s.reconfigure(width, height, s.refreshMilliHz)
}

func (s *pipeWireStream) reconfigure(width, height int, refreshMilliHz uint32) error {
	if s.closed {
		return ErrClosed
	}
	if _, err := captureAllocation(width, height, width*4); err != nil {
		return err
	}
	if refreshMilliHz == 0 || refreshMilliHz > 1_000_000 {
		return errors.New("wlr: invalid screen-cast refresh rate")
	}
	if width == s.width && height == s.height && refreshMilliHz == s.refreshMilliHz {
		return nil
	}
	if result := C.sysc_pw_stream_reconfigure(s.native, C.uint32_t(width), C.uint32_t(height), C.uint32_t(refreshMilliHz)); result < 0 {
		return fmt.Errorf("wlr: renegotiating PipeWire stream: %d", result)
	}
	s.width, s.height = width, height
	s.refreshMilliHz = refreshMilliHz
	return nil
}

func (s *pipeWireStream) close() {
	if s.closed {
		return
	}
	s.closed = true
	C.sysc_pw_stream_free(s.native)
	s.native = nil
	delete(s.p.streams, s)
	Unregister(s.token)
}
