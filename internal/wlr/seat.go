package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server xkbcommon
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <stdlib.h>
#include <sys/socket.h>
#include <unistd.h>
#include <xkbcommon/xkbcommon.h>
#include <wlr/interfaces/wlr_keyboard.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_keyboard.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_shell.h>

static const struct wlr_keyboard_impl sysc_injected_keyboard_impl = {
	.name = "sysc-injected-keyboard",
};

// An injected keyboard is created in process: wlr_keyboard_init is public
// in 0.20, so no virtual-keyboard protocol global is advertised (D8.6). The
// keymap is fixed to US so injected keycodes mean the same thing on every
// host. The struct lives in C memory because wlroots keeps pointers to it.
static struct wlr_keyboard *sysc_injected_keyboard_create(void) {
	struct wlr_keyboard *kb = calloc(1, sizeof(*kb));
	if (kb == NULL) {
		return NULL;
	}
	wlr_keyboard_init(kb, &sysc_injected_keyboard_impl, "sysc-injected-keyboard");
	struct xkb_context *ctx = xkb_context_new(XKB_CONTEXT_NO_FLAGS);
	struct xkb_rule_names names = { .layout = "us" };
	struct xkb_keymap *keymap = ctx == NULL ? NULL :
		xkb_keymap_new_from_names(ctx, &names, XKB_KEYMAP_COMPILE_NO_FLAGS);
	bool ok = keymap != NULL && wlr_keyboard_set_keymap(kb, keymap);
	xkb_keymap_unref(keymap);
	xkb_context_unref(ctx);
	if (!ok) {
		wlr_keyboard_finish(kb);
		free(kb);
		return NULL;
	}
	return kb;
}

static void sysc_injected_keyboard_destroy(struct wlr_keyboard *kb) {
	wlr_keyboard_finish(kb);
	free(kb);
}

// The keyboard's own state is updated first so modifiers and held keys are
// right for the next focus enter; then the seat delivers the key.
static void sysc_injected_key(struct wlr_seat *seat, struct wlr_keyboard *kb,
		uint32_t time, uint32_t keycode, bool pressed) {
	struct wlr_keyboard_key_event event = {
		.time_msec = time,
		.keycode = keycode,
		.update_state = true,
		.state = pressed ? WL_KEYBOARD_KEY_STATE_PRESSED : WL_KEYBOARD_KEY_STATE_RELEASED,
	};
	wlr_keyboard_notify_key(kb, &event);
	wlr_seat_keyboard_notify_key(seat, time, keycode, event.state);
	wlr_seat_keyboard_notify_modifiers(seat, &kb->modifiers);
}

static void sysc_injected_keyboard_enter(struct wlr_seat *seat,
		struct wlr_surface *surface, struct wlr_keyboard *kb) {
	wlr_seat_keyboard_notify_enter(seat, wlr_surface_get_root_surface(surface),
		kb->keycodes, kb->num_keycodes, &kb->modifiers);
}

// The registry name a client would see a seat under. wl_global_get_name
// needs a client, so a throwaway in-process one is made over a socketpair and
// destroyed at once; it never dispatches.
static uint32_t sysc_seat_global_name(struct wl_display *display, struct wlr_seat *seat) {
	int fds[2];
	if (socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, fds) != 0) {
		return 0;
	}
	struct wl_client *client = wl_client_create(display, fds[0]);
	if (client == NULL) {
		close(fds[0]);
		close(fds[1]);
		return 0;
	}
	uint32_t name = wl_global_get_name(seat->global, client);
	wl_client_destroy(client);
	close(fds[1]);
	return name;
}
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"
)

// Seat capability bits, mirrored so tests can assert without cgo.
const (
	CKeyboard = 1 << iota
	CPointer
)

// userSeatName is the seat the user's own devices drive. It is created first,
// in New, so it is advertised before any injected seat; a client that binds
// the first wl_seat gets it (sysc-shell does, sysc-1254).
const userSeatName = "seat0"

// maxInjectedSeats is the owner's answer to D8.5: one seat for injected input.
// The table is a slice so a second seat is a row, not a refactor.
const maxInjectedSeats = 1

var (
	ErrSeatName    = errors.New("wlr: an injected seat cannot take the user seat's name")
	ErrSeatLimit   = errors.New("wlr: the injected seat limit is reached")
	ErrUnknownSeat = errors.New("wlr: no injected seat by that name")
	ErrSeatCreate  = errors.New("wlr: creating the injected seat failed")
)

// injectedSeat is a seat fed by the compositor itself rather than by the
// user's hardware. Its pointer is a logical position only: it is never
// attached to the drawn cursor (rule D2.4.2), and its keyboard focus is its
// own, so it never moves the user's focus or selection (rule D2.4.1).
type injectedSeat struct {
	name     string
	cname    *C.char
	seat     *C.struct_wlr_seat
	keyboard *C.struct_wlr_keyboard
	x, y     float64
}

// AddInjectedSeat creates the seat for injected input. It is created on
// demand, never at startup, so no client sees an empty extra seat (D2.1).
func (c *Compositor) AddInjectedSeat(name string) error {
	if name == userSeatName {
		return ErrSeatName
	}
	if c.injectedSeat(name) != nil || len(c.injected) >= maxInjectedSeats {
		return ErrSeatLimit
	}
	s := &injectedSeat{name: name, cname: C.CString(name)}
	if s.seat = C.wlr_seat_create(c.display, s.cname); s.seat == nil {
		C.free(unsafe.Pointer(s.cname))
		return ErrSeatCreate
	}
	if s.keyboard = C.sysc_injected_keyboard_create(); s.keyboard == nil {
		C.wlr_seat_destroy(s.seat)
		C.free(unsafe.Pointer(s.cname))
		return ErrSeatCreate
	}
	C.wlr_seat_set_keyboard(s.seat, s.keyboard)
	C.wlr_seat_set_capabilities(s.seat, C.WL_SEAT_CAPABILITY_KEYBOARD|C.WL_SEAT_CAPABILITY_POINTER)
	c.injected = append(c.injected, s)
	return nil
}

// RemoveInjectedSeat destroys the seat and its keyboard. Clients see the
// global go away; the user's seat is untouched.
func (c *Compositor) RemoveInjectedSeat(name string) error {
	for i, s := range c.injected {
		if s.name == name {
			c.injected = append(c.injected[:i], c.injected[i+1:]...)
			c.dismissGrabbedPopups(s.seat)
			s.destroy()
			return nil
		}
	}
	return ErrUnknownSeat
}

func (s *injectedSeat) destroy() {
	C.wlr_seat_set_keyboard(s.seat, nil)
	C.sysc_injected_keyboard_destroy(s.keyboard)
	C.wlr_seat_destroy(s.seat)
	C.free(unsafe.Pointer(s.cname))
}

// InjectKey presses or releases one evdev keycode on the injected seat. It is
// delivered to that seat's keyboard focus only.
func (c *Compositor) InjectKey(name string, keycode uint32, pressed bool) error {
	s := c.injectedSeat(name)
	if s == nil {
		return ErrUnknownSeat
	}
	C.sysc_injected_key(s.seat, s.keyboard, nowMsec(), C.uint32_t(keycode), C.bool(pressed))
	return nil
}

// InjectPointerMotion moves the injected seat's logical pointer to layout
// coordinates and updates that seat's pointer focus. The drawn cursor does not
// move.
func (c *Compositor) InjectPointerMotion(name string, x, y float64) error {
	s := c.injectedSeat(name)
	if s == nil {
		return ErrUnknownSeat
	}
	s.x, s.y = x, y
	surface, sx, sy := c.surfaceAt(x, y)
	if surface == nil {
		C.wlr_seat_pointer_notify_clear_focus(s.seat)
		return nil
	}
	if !bool(C.wlr_seat_pointer_surface_has_focus(s.seat, surface)) {
		C.wlr_seat_pointer_notify_enter(s.seat, surface, sx, sy)
	}
	C.wlr_seat_pointer_notify_motion(s.seat, nowMsec(), sx, sy)
	C.wlr_seat_pointer_notify_frame(s.seat)
	return nil
}

// InjectPointerButton presses or releases a button on the injected seat. A
// press moves that seat's keyboard focus to the surface under its pointer; the
// user's focused window, activation and selection are not touched.
func (c *Compositor) InjectPointerButton(name string, button uint32, pressed bool) error {
	s := c.injectedSeat(name)
	if s == nil {
		return ErrUnknownSeat
	}
	state := C.enum_wl_pointer_button_state(C.WL_POINTER_BUTTON_STATE_RELEASED)
	if pressed {
		state = C.WL_POINTER_BUTTON_STATE_PRESSED
		if surface, _, _ := c.surfaceAt(s.x, s.y); surface != nil {
			C.sysc_injected_keyboard_enter(s.seat, surface, s.keyboard)
			c.touch(c.viewForSurface(surface))
		}
	}
	C.wlr_seat_pointer_notify_button(s.seat, nowMsec(), C.uint32_t(button), state)
	C.wlr_seat_pointer_notify_frame(s.seat)
	return nil
}

func (c *Compositor) injectedSeat(name string) *injectedSeat {
	for _, s := range c.injected {
		if s.name == name {
			return s
		}
	}
	return nil
}

func (c *Compositor) destroyInjectedSeats() {
	for _, s := range c.injected {
		s.destroy()
	}
	c.injected = nil
}

// seatNames lists the seat table in creation order, user seat first.
func (c *Compositor) seatNames() []string {
	names := []string{userSeatName}
	for _, s := range c.injected {
		names = append(names, s.name)
	}
	return names
}

// seatGlobal is the wl_registry name a seat is advertised under, 0 if unknown.
func (c *Compositor) seatGlobal(name string) uint32 {
	if name == userSeatName {
		return uint32(C.sysc_seat_global_name(c.display, c.seat))
	}
	if s := c.injectedSeat(name); s != nil {
		return uint32(C.sysc_seat_global_name(c.display, s.seat))
	}
	return 0
}

func (c *Compositor) injectedCapabilities(name string) int {
	if s := c.injectedSeat(name); s != nil {
		return int(s.seat.capabilities)
	}
	return 0
}

func (c *Compositor) cursorPosition() (float64, float64) {
	return float64(c.cursor.x), float64(c.cursor.y)
}

// viewForSurface is the toplevel view a (sub)surface belongs to, or nil.
func (c *Compositor) viewForSurface(surface *C.struct_wlr_surface) *toplevelView {
	root := C.wlr_surface_get_root_surface(surface)
	toplevel := C.wlr_xdg_toplevel_try_from_wlr_surface(root)
	if toplevel == nil {
		return nil
	}
	return c.views[unsafe.Pointer(toplevel)]
}

func nowMsec() C.uint32_t { return C.uint32_t(time.Now().UnixMilli()) }
