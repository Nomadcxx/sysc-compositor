package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdint.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_ext_image_capture_source_v1.h>
#include <wlr/types/wlr_ext_image_copy_capture_v1.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/util/box.h>
bool sysc_capture_sources_create(struct wl_display *,uintptr_t);
struct wlr_ext_image_capture_source_v1 *sysc_capture_source_new(uintptr_t,uint32_t,uint32_t,bool);
struct wlr_ext_image_capture_source_v1_cursor *sysc_capture_source_cursor(struct wlr_ext_image_capture_source_v1 *);
void sysc_capture_cursor_update(struct wlr_ext_image_capture_source_v1 *,bool,int32_t,int32_t,int32_t,int32_t);
bool sysc_capture_source_resource(struct wlr_ext_image_capture_source_v1 *,struct wl_client *,uint32_t);
void sysc_capture_source_finish(struct wlr_ext_image_capture_source_v1 *);
void sysc_capture_source_size(struct wlr_ext_image_capture_source_v1 *,uint32_t,uint32_t);
void sysc_capture_source_damage(struct wlr_ext_image_capture_source_v1 *,bool);
bool sysc_capture_frame_valid(struct wlr_ext_image_copy_capture_frame_v1 *,bool);
void sysc_capture_frame_ready(struct wlr_ext_image_copy_capture_frame_v1 *,struct wlr_buffer *,struct wlr_renderer *,uint32_t);
void sysc_capture_frame_failed(struct wlr_ext_image_copy_capture_frame_v1 *,uint32_t);
void sysc_legacy_constraints(struct wl_resource *,int,int);
void sysc_legacy_failed(struct wl_resource *);
void sysc_legacy_invalid(struct wl_resource *,bool);
void sysc_legacy_ready(struct wl_resource *,int,int,bool);
bool sysc_legacy_valid(struct wlr_buffer *,int,int);
bool sysc_legacy_write(struct wlr_buffer *,const uint8_t *,int);
bool sysc_legacy_box(struct wlr_output *,int,int,int,int,struct wlr_box *);
*/
import "C"

import (
	"bytes"
	"image"
	"image/draw"
	"math"
	"runtime/cgo"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

type captureProtocol struct {
	handle              cgo.Handle
	sources             map[*C.struct_wlr_ext_image_capture_source_v1]*captureSource
	cursors             map[capturePointerTarget]*captureSource
	sessions            map[*C.struct_wlr_ext_image_copy_capture_session_v1]*captureSession
	frames              map[*C.struct_wl_resource]*legacyCapture
	clients             map[*C.struct_wl_resource]*legacyCaptureClient
	generation          uint64
	sessionNew          *Listener
	surfaceDamageQueued bool
}
type capturePointerTarget struct {
	view   *toplevelView
	output unsafe.Pointer
}
type captureSource struct {
	c                     *Compositor
	native                *C.struct_wlr_ext_image_capture_source_v1
	handle                cgo.Handle
	view                  *toplevelView
	output                unsafe.Pointer
	resource              bool
	started               int
	lastPaint             bool
	cursor                bool
	image                 *image.RGBA
	dirty, queued, closed bool
}
type captureSession struct {
	source    *captureSource
	paint     bool
	destroyed *Listener
}
type legacyCaptureClient struct{ generation map[unsafe.Pointer]uint64 }
type legacyCapture struct {
	c                                       *Compositor
	resource                                *C.struct_wl_resource
	client                                  *legacyCaptureClient
	output                                  unsafe.Pointer
	box                                     image.Rectangle
	buffer                                  *C.struct_wlr_buffer
	pointer, damage, used, finished, queued bool
}

// Protocol coordinates are signed 32-bit. Clip in 64-bit arithmetic before
// narrowing to output coordinates, including requests extending past INT_MAX.
func clipCaptureRegion(x, y, width, height int32, bounds image.Rectangle) (image.Rectangle, bool) {
	if width <= 0 || height <= 0 {
		return image.Rectangle{}, false
	}
	x0, y0 := max(int64(x), int64(bounds.Min.X)), max(int64(y), int64(bounds.Min.Y))
	x1 := min(int64(x)+int64(width), int64(bounds.Max.X))
	y1 := min(int64(y)+int64(height), int64(bounds.Max.Y))
	if x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}, false
	}
	return image.Rect(int(x0), int(y0), int(x1), int(y1)), true
}

func (c *Compositor) initCaptureProtocol(manager *C.struct_wlr_ext_image_copy_capture_manager_v1) bool {
	p := &captureProtocol{sources: map[*C.struct_wlr_ext_image_capture_source_v1]*captureSource{}, cursors: map[capturePointerTarget]*captureSource{}, sessions: map[*C.struct_wlr_ext_image_copy_capture_session_v1]*captureSession{}, frames: map[*C.struct_wl_resource]*legacyCapture{}, clients: map[*C.struct_wl_resource]*legacyCaptureClient{}, generation: 1}
	p.handle = cgo.NewHandle(c)
	c.capture = p
	if !bool(C.sysc_capture_sources_create(c.display, C.uintptr_t(p.handle))) {
		return false
	}
	p.sessionNew = AddListener(unsafe.Pointer(&manager.events.new_session), func(data unsafe.Pointer) {
		native := (*C.struct_wlr_ext_image_copy_capture_session_v1)(data)
		source := p.sources[native.source]
		if source == nil {
			return
		}
		session := &captureSession{source: source, paint: source.lastPaint}
		p.sessions[native] = session
		session.destroyed = AddListener(unsafe.Pointer(&native.events.destroy), func(unsafe.Pointer) { delete(p.sessions, native); c.drop(session.destroyed) })
		c.keep(session.destroyed)
	})
	c.keep(p.sessionNew, AddListener(unsafe.Pointer(&c.comp.events.new_surface), func(data unsafe.Pointer) {
		surface := (*C.struct_wlr_surface)(data)
		var listeners []*Listener
		damage := func(unsafe.Pointer) { c.queueCaptureSurfaceDamage() }
		listeners = []*Listener{
			AddListener(unsafe.Pointer(&surface.events.commit), damage),
			AddListener(unsafe.Pointer(&surface.events.unmap), damage),
			AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
				for _, listener := range listeners {
					c.drop(listener)
				}
				c.queueCaptureSurfaceDamage()
			}),
		}
		c.keep(listeners...)
	}))
	return true
}

// Observe every client surface, including desynchronized children of layers,
// locks and IME popups. Coalesce until role/scene listeners finish the dispatch;
// the compositor's new_surface notification precedes those scene listeners.
func (c *Compositor) queueCaptureSurfaceDamage() {
	p := c.capture
	if p == nil || p.surfaceDamageQueued {
		return
	}
	p.surfaceDamageQueued = true
	_ = c.Post(func() {
		p.surfaceDamageQueued = false
		if c.capture == p {
			c.damageCaptureSources()
		}
	})
}

// windowCaptureSize and renderWindowCapture use the same output scale and xdg geometry.
func (c *Compositor) windowCaptureSize(v *toplevelView) (int, int) {
	if v == nil || v.toplevel == nil {
		return 0, 0
	}
	scale := c.windowCaptureScale(v)
	return int(float64(v.toplevel.base.geometry.width) * scale), int(float64(v.toplevel.base.geometry.height) * scale)
}

func (c *Compositor) windowCaptureScale(v *toplevelView) float64 {
	for pointer := range c.sceneOutputs {
		output := (*C.struct_wlr_output)(pointer)
		if C.GoString(output.name) == c.outputNameForView(v) {
			return float64(output.scale)
		}
	}
	return 1
}

//export sysc_capture_get_pointer
func sysc_capture_get_pointer(handle C.uintptr_t, seat *C.struct_wlr_seat) *C.struct_wlr_ext_image_capture_source_v1_cursor {
	s := cgo.Handle(handle).Value().(*captureSource)
	if s.closed || s.cursor || seat != s.c.seat {
		return nil
	}
	if _, _, live := s.targetSize(); !live {
		return nil
	}
	key := capturePointerTarget{s.view, s.output}
	cursor := s.c.capture.cursors[key]
	if cursor == nil {
		cursor = &captureSource{c: s.c, view: s.view, output: s.output, cursor: true, image: image.NewRGBA(image.Rect(0, 0, 1, 1))}
		cursor.handle = cgo.NewHandle(cursor)
		cursor.native = C.sysc_capture_source_new(C.uintptr_t(cursor.handle), 1, 1, true)
		if cursor.native == nil {
			cursor.handle.Delete()
			return nil
		}
		s.c.capture.cursors[key] = cursor
		s.c.capture.sources[cursor.native] = cursor
	}
	cursor.syncPointer()
	if cursor.closed {
		return nil
	}
	return C.sysc_capture_source_cursor(cursor.native)
}

func (s *captureSource) syncPointer() {
	w, h, live := s.targetSize()
	if !live {
		s.close()
		return
	}
	var box image.Rectangle
	var scale float64
	visible := true
	if s.view != nil {
		x, y := s.view.treePosition()
		g := s.view.geometry()
		box = image.Rect(x, y, x+g.w, y+g.h)
		scale = s.c.windowCaptureScale(s.view)
		visible = sceneFXNodeEnabled(unsafe.Pointer(&s.view.tree.node))
	} else {
		output := (*C.struct_wlr_output)(s.output)
		box, scale = s.c.outputLayoutBox(s.output), float64(output.scale)
		if output.transform&1 != 0 {
			w, h = h, w
		}
	}
	position := image.Pt(int(math.Round((float64(s.c.cursor.x)-float64(box.Min.X))*scale)), int(math.Round((float64(s.c.cursor.y)-float64(box.Min.Y))*scale)))
	img, hotspot, available := s.c.pointerImage(scale)
	entered := false
	if available && visible {
		entered = img.Bounds().Add(position.Sub(hotspot)).Overlaps(image.Rect(0, 0, w, h))
	}
	if entered {
		if s.image.Bounds() != img.Bounds() || !bytes.Equal(s.image.Pix, img.Pix) {
			s.image = img
			s.dirty = true
		}
		width, height := img.Bounds().Dx(), img.Bounds().Dy()
		if int(s.native.width) != width || int(s.native.height) != height {
			C.sysc_capture_source_size(s.native, C.uint32_t(width), C.uint32_t(height))
		}
	}
	// Outside the target, keep the last sprite and coordinates; changes over a
	// different window/output must not leak into this target's cursor session.
	C.sysc_capture_cursor_update(s.native, C.bool(entered), C.int32_t(position.X), C.int32_t(position.Y), C.int32_t(hotspot.X), C.int32_t(hotspot.Y))
}

func (s *captureSource) size() (int, int, bool) {
	w, h, ok := s.targetSize()
	if ok && s.cursor {
		w, h = s.image.Bounds().Dx(), s.image.Bounds().Dy()
	}
	return w, h, ok
}
func (s *captureSource) targetSize() (int, int, bool) {
	if s.closed {
		return 0, 0, false
	}
	var w, h int
	if s.view != nil {
		if !s.view.mapped || s.view.toplevel == nil || !bool(s.view.toplevel.base.surface.mapped) || s.c.viewBlocksCapture(s.view, config.CaptureScreen) {
			return 0, 0, false
		}
		w, h = s.c.windowCaptureSize(s.view)
	} else {
		if _, ok := s.c.sceneOutputs[s.output]; !ok {
			return 0, 0, false
		}
		output := (*C.struct_wlr_output)(s.output)
		if !bool(output.enabled) {
			return 0, 0, false
		}
		w, h = int(output.width), int(output.height)
	}
	_, err := captureAllocation(w, h, w*4)
	return w, h, err == nil
}
func (s *captureSource) close() {
	if s.closed {
		return
	}
	s.closed = true
	if s.cursor {
		delete(s.c.capture.cursors, capturePointerTarget{s.view, s.output})
	}
	delete(s.c.capture.sources, s.native)
	C.sysc_capture_source_finish(s.native)
	s.handle.Delete()
	s.native = nil
	s.image = nil
}
func (s *captureSource) releaseUnused() {
	// ponytail: native cursor metadata sessions expose no destruction observer.
	// Keep one cursor source per live target, as the native output source does;
	// mapping/output retirement bounds the pool and stops its sessions.
	if s.cursor {
		return
	}
	_ = s.c.Post(func() {
		if !s.closed && !s.resource && s.started == 0 {
			s.close()
		}
	})
}
func (s *captureSource) schedule() {
	if s.closed || s.queued || (s.started == 0 && !s.cursor) {
		return
	}
	s.queued = true
	_ = s.c.Post(func() {
		if s.closed {
			return
		}
		s.queued = false
		if s.cursor {
			s.syncPointer()
			if s.closed {
				return
			}
		}
		w, h, ok := s.size()
		if !ok {
			s.close()
			return
		}
		if uint32(s.native.width) != uint32(w) || uint32(s.native.height) != uint32(h) {
			C.sysc_capture_source_size(s.native, C.uint32_t(w), C.uint32_t(h))
			s.dirty = true
		}
		dirty := s.dirty
		s.dirty = false
		if s.started > 0 {
			C.sysc_capture_source_damage(s.native, C.bool(dirty))
		}
	})
}

func (c *Compositor) syncCaptureSources() {
	if c.capture == nil {
		return
	}
	for _, source := range c.capture.sources {
		w, h, ok := source.size()
		if !ok {
			source.close()
			continue
		}
		if int(source.native.width) != w || int(source.native.height) != h {
			C.sysc_capture_source_size(source.native, C.uint32_t(w), C.uint32_t(h))
			source.dirty = true
			source.schedule()
		}
	}
	for _, frame := range c.capture.frames {
		if _, live := c.sceneOutputs[frame.output]; !frame.finished && (!live || !bool((*C.struct_wlr_output)(frame.output).enabled)) {
			frame.fail()
		}
	}
}

// ponytail: output changes send full damage to every output source; add per-output
// regions if capture profiling requires it. Window sources wake only for their own tree.
func (c *Compositor) damageCaptureSources(views ...*toplevelView) {
	if c.capture == nil {
		return
	}
	c.syncCaptureSources()
	c.capture.generation++
	for _, source := range c.capture.sources {
		if source.cursor {
			source.schedule()
			continue
		}
		dirty := source.view == nil
		for _, v := range views {
			if source.view == v {
				dirty = true
			}
		}
		if dirty {
			source.dirty = true
			source.schedule()
		}
	}
	for _, frame := range c.capture.frames {
		if frame.used && !frame.finished {
			frame.schedule()
		}
	}
}

// damageCapturePointers wakes only active sessions that requested pointer painting.
func (c *Compositor) damageCapturePointers() {
	if c.capture == nil {
		return
	}
	c.capture.generation++
	for _, source := range c.capture.cursors {
		source.schedule()
	}
	for _, session := range c.capture.sessions {
		if session.paint {
			session.source.dirty = true
			session.source.schedule()
		}
	}
	for _, frame := range c.capture.frames {
		if frame.pointer && frame.used && !frame.finished {
			frame.schedule()
		}
	}
}

func (c *Compositor) closeWindowCaptureSources(v *toplevelView) {
	if c.capture == nil {
		return
	}
	for _, s := range c.capture.sources {
		if s.view == v {
			s.close()
		}
	}
}
func (c *Compositor) destroyCaptureSources() {
	if c.capture == nil {
		return
	}
	p := c.capture
	for _, s := range p.sources {
		s.close()
	}
	for resource, frame := range p.frames {
		frame.fail()
		C.wl_resource_destroy(resource)
	}
	c.drop(p.sessionNew)
}

// finishCaptureProtocol follows wl_display_destroy_clients: bound managers may
// still invoke their resource destructors after their frames/sources stop.
func (c *Compositor) finishCaptureProtocol() {
	if c.capture != nil {
		c.capture.handle.Delete()
		c.capture = nil
	}
}

//export sysc_capture_create
func sysc_capture_create(handle C.uintptr_t, client *C.struct_wl_client, id C.uint32_t, target *C.struct_wl_resource, window C.int) {
	c := cgo.Handle(handle).Value().(*Compositor)
	source := &captureSource{c: c, resource: true}
	if window != 0 {
		if c.foreign != nil {
			if f := c.foreign.handles[target]; f != nil {
				source.view = f.view
			}
		}
		if source.view == nil {
			C.sysc_capture_source_resource(nil, client, id)
			return
		}
	} else {
		source.output = unsafe.Pointer(C.wlr_output_from_resource(target))
	}
	w, h, ok := source.size()
	if !ok {
		C.sysc_capture_source_resource(nil, client, id)
		return
	}
	source.handle = cgo.NewHandle(source)
	source.native = C.sysc_capture_source_new(C.uintptr_t(source.handle), C.uint32_t(w), C.uint32_t(h), false)
	if source.native == nil {
		source.handle.Delete()
		C.wl_client_post_no_memory(client)
		return
	}
	c.capture.sources[source.native] = source
	if !bool(C.sysc_capture_source_resource(source.native, client, id)) {
		source.close()
	}
}

//export sysc_capture_source_event
func sysc_capture_source_event(handle C.uintptr_t, kind, value C.int) {
	s := cgo.Handle(handle).Value().(*captureSource)
	if s.closed {
		return
	}
	switch kind {
	case 0:
		s.started++
		s.lastPaint = value != 0
	case 1:
		s.started--
		s.releaseUnused()
	case 2:
		if value != 0 || s.dirty {
			s.schedule()
		}
	case 3:
		s.resource = false
		s.releaseUnused()
	}
}

//export sysc_capture_copy
func sysc_capture_copy(handle C.uintptr_t, frame *C.struct_wlr_ext_image_copy_capture_frame_v1) {
	s := cgo.Handle(handle).Value().(*captureSource)
	if _, _, ok := s.size(); !ok {
		C.sysc_capture_frame_failed(frame, 2)
		return
	}
	if !bool(C.sysc_capture_frame_valid(frame, C.bool(s.cursor))) {
		C.sysc_capture_frame_failed(frame, 1)
		return
	}
	paint := false
	for native, session := range s.c.capture.sessions {
		if native.frame == frame {
			paint = session.paint
			break
		}
	}
	var buffer *C.struct_wlr_buffer
	if s.cursor {
		buffer = imageBuffer(s.image)
		if buffer != nil {
			defer C.wlr_buffer_drop(buffer)
		}
	} else if s.view != nil {
		img, err := s.c.renderWindowCapture(s.view, config.CaptureScreen)
		if err != nil {
			C.sysc_capture_frame_failed(frame, 0)
			return
		}
		if paint {
			x, y := s.view.treePosition()
			g := s.view.geometry()
			scale := float64(img.Bounds().Dx()) / float64(g.w)
			s.c.drawPointer(img, image.Rect(x, y, x+g.w, y+g.h), scale)
		}
		buffer = imageBuffer(img)
		if buffer != nil {
			defer C.wlr_buffer_drop(buffer)
		}
	} else if paint {
		img, err := s.c.captureOutput(s.output)
		if err != nil {
			C.sysc_capture_frame_failed(frame, 0)
			return
		}
		s.c.drawOutputPointer(img, s.output)
		rgba := image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		buffer = imageBuffer(rgba)
		if buffer != nil {
			defer C.wlr_buffer_drop(buffer)
		}
	} else {
		var err error
		buffer, err = s.c.renderOutput(s.output)
		if err != nil {
			C.sysc_capture_frame_failed(frame, 0)
			return
		}
		defer C.wlr_buffer_unlock(buffer)
	}
	if buffer == nil {
		C.sysc_capture_frame_failed(frame, 0)
		return
	}
	transform := C.uint32_t(C.WL_OUTPUT_TRANSFORM_NORMAL)
	if s.view == nil && !s.cursor {
		transform = C.uint32_t((*C.struct_wlr_output)(s.output).transform)
	}
	C.sysc_capture_frame_ready(frame, buffer, s.c.renderer, transform)
}

//export sysc_legacy_create
func sysc_legacy_create(handle C.uintptr_t, manager, resource, outputResource *C.struct_wl_resource, pointer, region, x, y, width, height C.int) {
	c := cgo.Handle(handle).Value().(*Compositor)
	p := c.capture
	client := p.clients[manager]
	if client == nil {
		client = &legacyCaptureClient{generation: map[unsafe.Pointer]uint64{}}
		p.clients[manager] = client
	}
	frame := &legacyCapture{c: c, resource: resource, client: client, output: unsafe.Pointer(C.wlr_output_from_resource(outputResource)), pointer: pointer != 0}
	p.frames[resource] = frame
	if _, ok := c.sceneOutputs[frame.output]; !ok {
		frame.fail()
		return
	}
	out := (*C.struct_wlr_output)(frame.output)
	frame.box = image.Rect(0, 0, int(out.width), int(out.height))
	if !bool(out.enabled) {
		frame.fail()
		return
	}
	if region != 0 {
		var ow, oh C.int
		C.wlr_output_effective_resolution(out, &ow, &oh)
		clipped, valid := clipCaptureRegion(int32(x), int32(y), int32(width), int32(height), image.Rect(0, 0, int(ow), int(oh)))
		if !valid {
			frame.fail()
			return
		}
		var box C.struct_wlr_box
		if !bool(C.sysc_legacy_box(out, C.int(clipped.Min.X), C.int(clipped.Min.Y), C.int(clipped.Dx()), C.int(clipped.Dy()), &box)) {
			frame.fail()
			return
		}
		frame.box = image.Rect(int(box.x), int(box.y), int(box.x+box.width), int(box.y+box.height))
	}
	w, h := frame.box.Dx(), frame.box.Dy()
	if _, err := captureAllocation(w, h, w*4); err != nil {
		frame.fail()
		return
	}
	C.sysc_legacy_constraints(resource, C.int(w), C.int(h))
}

//export sysc_legacy_copy
func sysc_legacy_copy(handle C.uintptr_t, resource, bufferResource *C.struct_wl_resource, damage C.int) {
	c := cgo.Handle(handle).Value().(*Compositor)
	frame := c.capture.frames[resource]
	if frame == nil || frame.finished {
		return
	}
	if frame.used {
		C.sysc_legacy_invalid(resource, true)
		return
	}
	frame.used = true
	buffer := C.wlr_buffer_try_from_resource(bufferResource)
	if !bool(C.sysc_legacy_valid(buffer, C.int(frame.box.Dx()), C.int(frame.box.Dy()))) {
		if buffer != nil {
			C.wlr_buffer_unlock(buffer)
		}
		C.sysc_legacy_invalid(resource, false)
		return
	}
	frame.buffer = buffer
	frame.damage = damage != 0
	frame.deliver()
}
func (f *legacyCapture) fail() {
	if f.finished {
		return
	}
	f.finished = true
	if f.buffer != nil {
		C.wlr_buffer_unlock(f.buffer)
		f.buffer = nil
	}
	C.sysc_legacy_failed(f.resource)
}

// Render after the scene update completes, as ext-copy sources do. A policy
// rematch can close frozen UI while another capture is rendering.
func (f *legacyCapture) schedule() {
	if f.finished || f.queued {
		return
	}
	f.queued = true
	_ = f.c.Post(func() {
		f.queued = false
		if f.c.capture != nil && f.c.capture.frames[f.resource] == f {
			f.deliver()
		}
	})
}

func (f *legacyCapture) deliver() {
	if f.finished || f.buffer == nil {
		return
	}
	if _, live := f.c.sceneOutputs[f.output]; !live || !bool((*C.struct_wlr_output)(f.output).enabled) {
		f.fail()
		return
	}
	if f.damage && f.client.generation[f.output] == f.c.capture.generation {
		return
	}
	img, err := f.c.captureOutput(f.output)
	if err != nil || !f.box.In(img.Bounds()) {
		f.fail()
		return
	}
	if f.pointer {
		f.c.drawOutputPointer(img, f.output)
	}
	pixels := img.Pix[img.PixOffset(f.box.Min.X, f.box.Min.Y):]
	if !bool(C.sysc_legacy_write(f.buffer, (*C.uint8_t)(unsafe.Pointer(&pixels[0])), C.int(img.Stride))) {
		f.fail()
		return
	}
	C.wlr_buffer_unlock(f.buffer)
	f.buffer = nil
	f.finished = true
	f.client.generation[f.output] = f.c.capture.generation
	C.sysc_legacy_ready(f.resource, C.int(f.box.Dx()), C.int(f.box.Dy()), C.bool(f.damage))
}

//export sysc_legacy_destroyed
func sysc_legacy_destroyed(handle C.uintptr_t, resource *C.struct_wl_resource, manager C.int) {
	c := cgo.Handle(handle).Value().(*Compositor)
	if manager != 0 {
		delete(c.capture.clients, resource)
		return
	}
	frame := c.capture.frames[resource]
	if frame != nil && frame.buffer != nil {
		C.wlr_buffer_unlock(frame.buffer)
	}
	delete(c.capture.frames, resource)
}
