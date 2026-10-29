package wlr

/*
#cgo pkg-config: wlroots-0.20
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_seat.h>
*/
import "C"

import (
	"cmp"
	"errors"
	"image"
	"image/draw"
	"slices"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// CastOptions belongs to one session stream. Callbacks run on the compositor
// thread and may only publish state or schedule work; they must not block.
type CastOptions struct {
	SessionID, StreamID uint64
	Target              ipc.CastTarget
	CursorMode          uint32 // Mutter: hidden, embedded, metadata
	NodeAdded           func(uint32)
	Closed              func()
}

type screenCast struct {
	c       *Compositor
	options CastOptions
	state   ipc.Cast
	stream  *pipeWireStream
}

func (c *Compositor) StartScreenCast(ready, lost func(error)) error {
	return c.startPipeWire(func(err error) {
		if err == nil {
			c.nextViewID++
			c.dynamicCastID = c.nextViewID
		}
		if ready != nil {
			ready(err)
		}
	}, lost)
}

// DynamicCastWindowID is reserved from the mapping ID allocator, but never
// inserted into Windows IPC or the layout. Only portal Introspect exposes it.
func (c *Compositor) DynamicCastWindowID() uint64 { return c.dynamicCastID }

func (c *Compositor) StartCast(options CastOptions) error {
	if c.pipewire == nil || !c.pipewire.ready || c.pipewire.failed || options.CursorMode > 2 || options.SessionID == 0 || options.StreamID == 0 {
		return errors.New("wlr: invalid cast or PipeWire unavailable")
	}
	if _, exists := c.casts[options.StreamID]; exists {
		return errors.New("wlr: cast stream already exists")
	}
	if options.Target.Output != nil && options.Target.WindowID != nil {
		return errors.New("wlr: ambiguous cast target")
	}
	s := &screenCast{c: c, options: options, state: ipc.Cast{SessionID: options.SessionID, StreamID: options.StreamID, Kind: ipc.CastKindPipeWire}}
	if options.Target.WindowID != nil && *options.Target.WindowID == c.dynamicCastID {
		s.state.IsDynamicTarget = true
	} else {
		s.state.Target = cloneCastTarget(options.Target)
		if _, _, _, _, _, ok := s.target(); !ok {
			return errors.New("wlr: cast target is missing")
		}
		if err := s.connect(); err != nil {
			return err
		}
	}
	if c.casts == nil {
		c.casts = make(map[uint64]*screenCast)
	}
	c.casts[options.StreamID] = s
	c.castStateChanged()
	return nil
}

func (s *screenCast) connect() error {
	w, h, rate, _, _, ok := s.target()
	if !ok {
		return errors.New("wlr: cast target is missing")
	}
	stream, err := s.c.pipewire.newStream(w, h, rate, s.frame, func(node uint32, active bool, err error) {
		if err != nil {
			s.c.StopCast(s.state.SessionID)
			return
		}
		first := node != ^uint32(0) && s.state.PWNodeID == nil
		if node != ^uint32(0) {
			s.state.PWNodeID = &node
		}
		s.state.IsActive = active
		s.c.castStateChanged()
		if first && s.options.NodeAdded != nil {
			s.options.NodeAdded(node)
		}
	})
	if err == nil {
		s.stream = stream
	}
	return err
}

// target returns upright physical dimensions and the current source geometry.
func (s *screenCast) target() (w, h int, rate uint32, view *toplevelView, output unsafe.Pointer, ok bool) {
	if id := s.state.Target.WindowID; id != nil {
		for _, v := range s.c.views {
			if v.id == *id && v.mapped && v.toplevel != nil {
				view = v
				w, h = s.c.windowCaptureSize(v)
				output = s.c.outputByName(s.c.outputNameForView(v))
				break
			}
		}
		if view == nil {
			return
		}
	} else if name := s.state.Target.Output; name != nil {
		output = s.c.outputByName(*name)
		if output == nil || !s.c.outputIsEnabled(output) {
			return
		}
		out := (*C.struct_wlr_output)(output)
		w, h = int(out.width), int(out.height)
		if out.transform&1 != 0 {
			w, h = h, w
		}
	} else {
		return
	}
	rate = 60000
	if output != nil && (*C.struct_wlr_output)(output).refresh > 0 {
		rate = uint32((*C.struct_wlr_output)(output).refresh)
	}
	_, err := captureAllocation(w, h, w*4)
	ok = err == nil
	return
}

func (s *screenCast) frame() (*image.NRGBA, error) {
	s.stream.pointer = nil
	w, h, rate, view, output, live := s.target()
	if !live {
		if s.state.Target.Output != nil || s.state.Target.WindowID != nil {
			if s.state.IsDynamicTarget {
				s.c.ClearDynamicCastTarget()
				return blackCastFrame(s.stream.width, s.stream.height), nil
			}
			// Native process still owns a dequeued buffer: teardown must wait
			// until its deferred error callback, never close the stream here.
			return nil, errors.New("wlr: cast target disappeared")
		}
		return blackCastFrame(s.stream.width, s.stream.height), nil
	}
	if w != s.stream.width || h != s.stream.height || rate != s.stream.refreshMilliHz {
		if err := s.stream.reconfigure(w, h, rate); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if s.c.sessionLocked {
		return blackCastFrame(w, h), nil
	}
	var img *image.NRGBA
	var box image.Rectangle
	var scale float64
	if view != nil {
		rgba, err := s.c.renderWindowCapture(view, config.CaptureCast)
		if err != nil {
			return nil, err
		}
		img = image.NewNRGBA(rgba.Bounds())
		draw.Draw(img, img.Bounds(), rgba, rgba.Bounds().Min, draw.Src)
		x, y := view.treePosition()
		g := view.geometry()
		box, scale = image.Rect(x, y, x+g.w, y+g.h), s.c.windowCaptureScale(view)
		if s.c.viewBlocksCapture(view, config.CaptureCast) {
			return img, nil
		}
		// Match both the hit and the focus after grabs. An overlapping client
		// owns its pointer image; it must not enter this window's stream.
		if s.c.viewAtCursor() != view || (s.c.dndScroll == nil && s.c.viewForSurface(s.c.seat.pointer_state.focused_surface) != view) {
			return img, nil
		}
	} else {
		buffer, err := s.c.captureOutputForTarget(output, config.CaptureCast)
		if err != nil {
			return nil, err
		}
		out := (*C.struct_wlr_output)(output)
		img = uprightCastFrame(buffer, uint32(out.transform))
		box, scale = s.c.outputLayoutBox(output), float64(out.scale)
	}
	if s.options.CursorMode == 1 {
		s.c.drawPointer(img, box, scale)
	} else if s.options.CursorMode == 2 {
		icon, hotspot, visible := s.c.pointerImage(scale)
		if visible && (view == nil || sceneFXNodeEnabled(unsafe.Pointer(&view.tree.node))) {
			position := s.c.pointerOrigin(box, scale, image.Point{})
			if icon.Bounds().Add(position.Sub(hotspot)).Overlaps(img.Bounds()) {
				s.stream.pointer = &pipeWirePointer{Image: icon, Position: position, Hotspot: hotspot}
			}
		}
	}
	return img, nil
}

func blackCastFrame(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func uprightCastFrame(buffer *image.NRGBA, transform uint32) *image.NRGBA {
	if transform == 0 {
		return buffer
	}
	w, h := buffer.Bounds().Dx(), buffer.Bounds().Dy()
	if transform&1 != 0 {
		w, h = h, w
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := outputPointToBuffer(image.Pt(x, y), buffer.Bounds().Size(), transform).Add(buffer.Bounds().Min)
			img.SetNRGBA(x, y, buffer.NRGBAAt(p.X, p.Y))
		}
	}
	return img
}

func cloneCastTarget(target ipc.CastTarget) ipc.CastTarget {
	if target.WindowID != nil {
		id := *target.WindowID
		target.WindowID = &id
	}
	if target.Output != nil {
		name := *target.Output
		target.Output = &name
	}
	return target
}

func (c *Compositor) CastSnapshot() []ipc.Cast {
	casts := make([]ipc.Cast, 0, len(c.casts))
	for _, s := range c.casts {
		state := s.state
		state.Target = cloneCastTarget(state.Target)
		if state.PWNodeID != nil {
			node := *state.PWNodeID
			state.PWNodeID = &node
		}
		casts = append(casts, state)
	}
	slices.SortFunc(casts, func(a, b ipc.Cast) int { return cmp.Compare(a.StreamID, b.StreamID) })
	return casts
}

func (c *Compositor) StopCast(sessionID uint64) {
	var closed func()
	for id, s := range c.casts {
		if s.state.SessionID != sessionID {
			continue
		}
		delete(c.casts, id)
		if s.stream != nil {
			s.stream.close()
		}
		if s.options.Closed != nil {
			closed = s.options.Closed
		}
	}
	c.castStateChanged()
	if closed != nil {
		closed()
	}
}

func (c *Compositor) SetDynamicCastWindow(id *uint64) {
	windowID, ok := c.windowActionID(id)
	if !ok {
		return
	}
	for _, view := range c.views {
		if view.id == windowID && view.mapped && view.toplevel != nil {
			c.setDynamicCastTarget(ipc.CastTarget{WindowID: &windowID})
			return
		}
	}
}

func (c *Compositor) SetDynamicCastMonitor(name *string) {
	output := c.FocusedOutputName()
	if name != nil {
		output = *name
	}
	if pointer := c.outputByName(output); pointer != nil && c.outputIsEnabled(pointer) {
		c.setDynamicCastTarget(ipc.CastTarget{Output: &output})
	}
}

func (c *Compositor) ClearDynamicCastTarget() { c.setDynamicCastTarget(ipc.CastTarget{}) }

func (c *Compositor) setDynamicCastTarget(target ipc.CastTarget) {
	for _, s := range c.casts {
		if !s.state.IsDynamicTarget {
			continue
		}
		s.state.Target = cloneCastTarget(target)
		if s.stream == nil && (target.Output != nil || target.WindowID != nil) {
			if err := s.connect(); err != nil {
				c.StopCast(s.state.SessionID)
			}
		}
	}
	c.castStateChanged()
}

func (c *Compositor) syncCasts() {
	for _, s := range c.casts {
		if s.state.Target.Output == nil && s.state.Target.WindowID == nil {
			continue
		}
		if _, _, _, _, _, live := s.target(); !live {
			if s.state.IsDynamicTarget {
				c.ClearDynamicCastTarget()
			} else {
				c.StopCast(s.state.SessionID)
			}
		}
	}
}

func (c *Compositor) castStateChanged() {
	if c.stateChanged != nil {
		c.stateChanged()
	}
}
