package wlr

import (
	"regexp"
	"testing"
	"time"
	"unsafe"
)

var (
	rePointerRelease = regexp.MustCompile(`wl_pointer#\d+\.button\(\d+, \d+, 272, 0\)`)
	rePointerLeave   = regexp.MustCompile(`wl_pointer#\d+\.leave\(`)
)

func TestClickingEmptyOtherOutputClearsKeyboardFocusFromPreviousOutput(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	var (
		view                                      *toplevelView
		focusedSurfaceBefore, focusedSurfaceAfter unsafe.Pointer
		focusedViewBefore                         *toplevelView
		targetOutput                              string
		windowX, windowY, clickX, clickY          float64
	)
	pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		view = v
		windowX, windowY = c.viewCenter(v)
		windowOutput := c.outputNameAt(windowX, windowY)
		for _, output := range c.OutputSnapshot() {
			if output.Name == windowOutput || output.Logical == nil {
				continue
			}
			for _, xFraction := range []float64{0.5, 0.9, 0.1} {
				for _, yFraction := range []float64{0.5, 0.9, 0.1} {
					x := float64(output.Logical.X) + float64(output.Logical.Width)*xFraction
					y := float64(output.Logical.Y) + float64(output.Logical.Height)*yFraction
					if c.outputNameAt(x, y) == output.Name {
						if surface, _, _ := c.surfaceAt(x, y); surface == nil {
							targetOutput, clickX, clickY = output.Name, x, y
							return []func(){
								func() {
									p.moveTo(windowX, windowY)
									p.button(btnLeft, true)
									focusedSurfaceBefore, focusedViewBefore = c.userFocus()
									p.button(btnLeft, false)
								},
								func() {
									p.moveTo(clickX, clickY)
									p.button(btnLeft, true)
									focusedSurfaceAfter, _ = c.userFocus()
									p.button(btnLeft, false)
								},
							}
						}
					}
				}
			}
		}
		t.Errorf("no empty point found on an output other than %q", windowOutput)
		return nil
	})
	if focusedViewBefore != view || focusedSurfaceBefore == nil {
		t.Fatalf("keyboard focus before moving outputs = (%p, %p), want the mapped view %p", focusedSurfaceBefore, focusedViewBefore, view)
	}
	if targetOutput == "" {
		t.Fatal("test found no second output to click")
	}
	if focusedSurfaceAfter != nil {
		t.Errorf("keyboard focus after clicking empty space on %q = %p, want no surface on that output", targetOutput, focusedSurfaceAfter)
	}
	if got := c.FocusedOutputName(); got != targetOutput {
		t.Errorf("focused output after click = %q, want %q", got, targetOutput)
	}
}

// pointerScene maps one weston-terminal in the layout's top-left corner,
// gives the user seat an in-process pointer, and runs the steps scene returns
// on the compositor thread with a pause before each and after the last, so the
// client can answer each one. It returns the client's WAYLAND_DEBUG lines.
func pointerScene(t *testing.T, c *Compositor, scene func(v *toplevelView, p *fakePointer) []func()) []string {
	t.Helper()
	var pointer *fakePointer
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	mapped := false
	c.viewMapped = func(v *toplevelView) {
		if mapped {
			return
		}
		mapped = true
		box := c.layoutBox()
		c.placeView(v, box.x, box.y)
		if pointer = c.addFakePointer(); pointer == nil {
			t.Error("could not attach the test pointer")
			c.Terminate()
			return
		}
		steps := scene(v, pointer)
		go func() {
			for _, step := range steps {
				time.Sleep(150 * time.Millisecond)
				done := make(chan struct{})
				if c.Post(func() { step(); close(done) }) != nil {
					return
				}
				<-done
			}
			// The client still has to read what the last step sent.
			time.Sleep(150 * time.Millisecond)
			_ = c.Post(c.Terminate)
		}()
	}
	lines := runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, nil)
	if !mapped {
		t.Fatal("weston-terminal never mapped a window")
	}
	return lines
}

// emptyPoint is a layout point with no surface under it: the corner opposite
// the window pointerScene placed.
func emptyPoint(t *testing.T, c *Compositor) (float64, float64) {
	box := c.layoutBox()
	x, y := float64(box.x+box.w-10), float64(box.y+box.h-10)
	if surface, _, _ := c.surfaceAt(x, y); surface != nil {
		t.Errorf("(%v,%v) is covered by a surface; the output is too small for this test", x, y)
	}
	return x, y
}

// TestPointerFocusStaysOnPressedSurface is report P1: a selection dragged out
// of a window keeps its pointer focus until the button is released, and the
// release reaches that window before it is left.
func TestPointerFocusStaysOnPressedSurface(t *testing.T) {
	c := newNested(t)
	var surface, before, during, after unsafe.Pointer
	lines := pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		surface = unsafe.Pointer(v.toplevel.base.surface)
		return []func(){
			func() { p.moveTo(c.viewCenter(v)); before = c.userPointerFocus() },
			func() { p.button(btnLeft, true) },
			func() { p.moveTo(emptyPoint(t, c)); during = c.userPointerFocus() },
			func() { p.button(btnLeft, false); after = c.userPointerFocus() },
		}
	})
	if before != surface {
		t.Fatalf("pointer focus over the window = %p, want its surface %p", before, surface)
	}
	if during != surface {
		t.Errorf("pointer focus while dragging out with the button held = %p, want the pressed surface %p", during, surface)
	}
	if after != nil {
		t.Errorf("pointer focus after the release over empty space = %p, want none", after)
	}
	release, leave := -1, -1
	for i, line := range lines {
		if release < 0 && rePointerRelease.MatchString(line) {
			release = i
		}
		if leave < 0 && rePointerLeave.MatchString(line) {
			leave = i
		}
	}
	if release < 0 {
		t.Error("the client never received the button release")
	}
	if leave < 0 || leave < release {
		t.Errorf("client saw leave at line %d and the release at %d; want the release first, then leave", leave, release)
	}
}

// TestEmptySpaceShowsDefaultImage is report P2: the client's own image (the
// terminal's I-beam) does not linger once the pointer is over empty space.
func TestEmptySpaceShowsDefaultImage(t *testing.T) {
	c := newNested(t)
	var over, empty string
	pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		return []func(){
			func() { p.moveTo(c.viewCenter(v)) },
			func() { over = c.drawnImage },
			func() { p.moveTo(emptyPoint(t, c)); empty = c.drawnImage },
		}
	})
	if over != "" {
		t.Fatalf("image over the terminal = %q, want the client's surface; it never set one", over)
	}
	if empty != "default" {
		t.Errorf("image over empty space = %q, want %q", empty, "default")
	}
}

// TestPopupResolvesToItsWindow is report P3: the pointer over a window's popup
// menu is over that window, so a click or a Mod+drag there acts on it.
func TestPopupResolvesToItsWindow(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	var (
		view, hit *toplevelView
		popup     []popupPlacement
	)
	pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		view = v
		// The injected seat opens the menu, so its grab is not on the
		// user seat the test then points with.
		x, y := c.viewCenter(v)
		return []func(){
			func() {
				if err := c.InjectPointerMotion("agent", x, y); err != nil {
					t.Error(err)
				}
				if err := c.InjectPointerButton("agent", btnRight, true); err != nil {
					t.Error(err)
				}
			},
			func() {},
			func() {
				if popup = c.popupPlacements(); len(popup) == 0 {
					return
				}
				b := popup[0].box
				p.moveTo(float64(b.x)+float64(b.w)/2, float64(b.y)+float64(b.h)/2)
				hit = c.viewAtCursor()
			},
		}
	})
	if len(popup) == 0 {
		t.Fatal("the right click opened no mapped popup")
	}
	if hit != view {
		t.Errorf("view under the pointer on the popup = %p, want its window %p", hit, view)
	}
}

// TestUnmapReleasesFocusAndGrab is report P4: a window that unmaps and stays
// alive keeps no seat's focus and no compositor grab.
func TestUnmapReleasesFocusAndGrab(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	var (
		surface                      unsafe.Pointer
		userKB, injKB, injPtr        unsafe.Pointer
		focused, grabbed             *toplevelView
		preUser, preInjKB, preInjPtr unsafe.Pointer
		preGrab                      *toplevelView
	)
	pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		surface = unsafe.Pointer(v.toplevel.base.surface)
		x, y := c.viewCenter(v)
		return []func(){
			func() {
				c.focusView(v)
				if err := c.InjectPointerMotion("agent", x, y); err != nil {
					t.Error(err)
				}
				if err := c.InjectPointerButton("agent", btnLeft, true); err != nil {
					t.Error(err)
				}
				c.startPointerGrab(v, grabMove, 0, btnLeft)
				preUser, _ = c.userFocus()
				preInjKB, preInjPtr = c.injectedFocus("agent"), c.injectedPointerFocus("agent")
				preGrab = c.grabView
			},
			func() {
				c.unmapView(v)
				userKB, focused = c.userFocus()
				injKB, injPtr = c.injectedFocus("agent"), c.injectedPointerFocus("agent")
				grabbed = c.grabView
				// The client may now commit a buffer to a surface that is no
				// longer configured; end before it does.
				c.Terminate()
			},
		}
	})
	if preUser != surface || preInjKB != surface || preInjPtr != surface || preGrab == nil {
		t.Fatalf("setup: user keyboard %p, injected keyboard %p and pointer %p, grab %p; want the window %p and a grab",
			preUser, preInjKB, preInjPtr, preGrab, surface)
	}
	if userKB != nil || focused != nil {
		t.Errorf("user keyboard focus %p and focused view %p after unmap, want none", userKB, focused)
	}
	if injKB != nil || injPtr != nil {
		t.Errorf("injected seat keyboard %p and pointer %p after unmap, want none", injKB, injPtr)
	}
	if grabbed != nil {
		t.Errorf("compositor grab on %p after unmap, want none", grabbed)
	}
}

// TestCompositorGrabClearsPointerFocus is report M4: Mod+drag leaves the
// window under the pointer rather than freezing it mid-enter, and the window
// gets the pointer back once the button is released.
func TestCompositorGrabClearsPointerFocus(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	var (
		surface, before, during, after unsafe.Pointer
		grabbing                       bool
	)
	pointerScene(t, c, func(v *toplevelView, p *fakePointer) []func() {
		surface = unsafe.Pointer(v.toplevel.base.surface)
		return []func(){
			func() {
				if keyboard = c.addFakeKeyboard(); keyboard == nil {
					t.Error("could not attach the test keyboard")
				}
				p.moveTo(c.viewCenter(v))
				before = c.userPointerFocus()
			},
			func() {
				keyboard.key(c.modKeycode(), true)
				p.button(btnLeft, true)
				grabbing = c.pointerGrab != nil
				during = c.userPointerFocus()
			},
			func() {
				p.button(btnLeft, false)
				keyboard.key(c.modKeycode(), false)
				after = c.userPointerFocus()
			},
		}
	})
	if before != surface {
		t.Fatalf("pointer focus over the window = %p, want its surface %p", before, surface)
	}
	if !grabbing {
		t.Fatal("Mod+press started no compositor grab")
	}
	if during != nil {
		t.Errorf("pointer focus during the grab = %p, want none", during)
	}
	if after != surface {
		t.Errorf("pointer focus after the grab ended = %p, want the window under the pointer %p", after, surface)
	}
}
