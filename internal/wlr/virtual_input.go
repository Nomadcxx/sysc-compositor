package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/interfaces/wlr_keyboard.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_method_v2.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_virtual_keyboard_v1.h>
#include <wlr/types/wlr_virtual_pointer_v1.h>
#include <wlr/types/wlr_output.h>
#include <stdlib.h>
#include <string.h>

// Native virtual-pointer callbacks collapse an explicit inert seat/output to
// NULL, also used for a valid default request. Preserve that distinction at
// Wayland's synchronous pre-dispatch hook; never record protocol contents.
struct sysc_virtual_request_guard {
	struct wl_protocol_logger *logger;
	struct wl_listener client_destroy;
	struct wl_client *client;
	uint32_t id;
	bool valid;
};
// The native creation callback runs before another request dispatches, so one
// pending record also bounds failed-allocation/client teardown bookkeeping.
static void sysc_virtual_request_clear(struct sysc_virtual_request_guard *guard) {
	if (guard->client != NULL) wl_list_remove(&guard->client_destroy.link);
	guard->client = NULL;
}
static void sysc_virtual_request_client_gone(struct wl_listener *listener, void *data) {
	struct sysc_virtual_request_guard *guard = wl_container_of(listener, guard, client_destroy);
	sysc_virtual_request_clear(guard);
}
static void sysc_virtual_request_check(void *data, enum wl_protocol_logger_type direction,
		const struct wl_protocol_logger_message *message) {
	if (direction != WL_PROTOCOL_LOGGER_REQUEST ||
			strcmp(wl_resource_get_class(message->resource), "zwlr_virtual_pointer_manager_v1") != 0) return;
	int count = message->message_opcode == 0 ? 2 : message->message_opcode == 2 ? 3 : 0;
	if (count == 0 || message->arguments_count != count) return;
	struct sysc_virtual_request_guard *guard = data;
	sysc_virtual_request_clear(guard);
	guard->client = wl_resource_get_client(message->resource);
	guard->id = message->arguments[count - 1].n;
	struct wl_resource *seat = (struct wl_resource *)message->arguments[0].o;
	struct wl_resource *output = count == 3 ? (struct wl_resource *)message->arguments[1].o : NULL;
	guard->valid = (seat == NULL || wlr_seat_client_from_resource(seat) != NULL) &&
		(output == NULL || wlr_output_from_resource(output) != NULL);
	guard->client_destroy.notify = sysc_virtual_request_client_gone;
	wl_client_add_destroy_listener(guard->client, &guard->client_destroy);
}
static struct sysc_virtual_request_guard *sysc_virtual_request_guard_create(struct wl_display *display) {
	struct sysc_virtual_request_guard *guard = calloc(1, sizeof(*guard));
	if (guard == NULL) return NULL;
	guard->logger = wl_display_add_protocol_logger(display, sysc_virtual_request_check, guard);
	if (guard->logger == NULL) { free(guard); return NULL; }
	return guard;
}
static bool sysc_virtual_request_take(struct sysc_virtual_request_guard *guard, struct wl_resource *resource) {
	bool valid = guard != NULL && guard->client == wl_resource_get_client(resource) &&
		guard->id == wl_resource_get_id(resource) && guard->valid;
	if (guard != NULL) sysc_virtual_request_clear(guard);
	return valid;
}
static void sysc_virtual_request_guard_destroy(struct sysc_virtual_request_guard *guard) {
	if (guard == NULL) return;
	sysc_virtual_request_clear(guard);
	wl_protocol_logger_destroy(guard->logger);
	free(guard);
}

static void sysc_virtual_reject(struct wl_resource *resource) {
	wl_resource_post_error(resource, 0, "virtual input requires the user seat and a live output");
}

static void sysc_virtual_keyboard_reset(struct wlr_keyboard *keyboard) {
	while (keyboard->num_keycodes > 0) {
		struct wlr_keyboard_key_event event = {
			.keycode = keyboard->keycodes[keyboard->num_keycodes - 1],
			.state = WL_KEYBOARD_KEY_STATE_RELEASED,
			.update_state = false,
		};
		wlr_keyboard_notify_key(keyboard, &event);
	}
	wlr_keyboard_notify_modifiers(keyboard, 0, 0, 0, 0);
}
*/
import "C"

import "unsafe"

// Virtual keyboards remain outside the physical group: merging them erases
// the client identity needed to return IME keys without recapturing them.
type virtualKeyboard struct {
	keyboard   *C.struct_wlr_keyboard
	suppressed map[uint32]bool
	held       map[uint32]bool
	resetting  bool
	listeners  []*Listener
}

type virtualPointer struct {
	pointer    *C.struct_wlr_pointer
	stopped    bool
	outputGone *Listener
}

func (c *Compositor) createVirtualInput() bool {
	keyboards := C.wlr_virtual_keyboard_manager_v1_create(c.display)
	pointers := C.wlr_virtual_pointer_manager_v1_create(c.display)
	if keyboards == nil || pointers == nil {
		return false
	}
	c.virtualPointerGuard = unsafe.Pointer(C.sysc_virtual_request_guard_create(c.display))
	if c.virtualPointerGuard == nil {
		return false
	}
	c.virtualKeyboards = map[*C.struct_wlr_keyboard]*virtualKeyboard{}
	c.virtualPointers = map[*C.struct_wlr_pointer]*virtualPointer{}
	c.keep(AddListener(unsafe.Pointer(&keyboards.events.new_virtual_keyboard), c.handleVirtualKeyboard),
		AddListener(unsafe.Pointer(&pointers.events.new_virtual_pointer), c.handleVirtualPointer))
	return true
}

func (c *Compositor) destroyVirtualInputGuard() {
	C.sysc_virtual_request_guard_destroy((*C.struct_sysc_virtual_request_guard)(c.virtualPointerGuard))
	c.virtualPointerGuard = nil
}

func (c *Compositor) handleVirtualKeyboard(data unsafe.Pointer) {
	device := (*C.struct_wlr_virtual_keyboard_v1)(data)
	if device.seat != c.seat {
		C.sysc_virtual_reject(device.resource)
		return
	}
	keyboard := &device.keyboard
	v := &virtualKeyboard{keyboard: keyboard, suppressed: map[uint32]bool{},
		held: map[uint32]bool{}}
	c.virtualKeyboards[keyboard] = v
	C.wlr_keyboard_set_repeat_info(keyboard, c.keyboard.repeat_info.rate, c.keyboard.repeat_info.delay)
	v.listeners = []*Listener{
		AddListener(unsafe.Pointer(&keyboard.events.key), func(data unsafe.Pointer) {
			if v.resetting || c.injectedInputBlocked() {
				return
			}
			event := (*C.struct_wlr_keyboard_key_event)(data)
			key := uint32(event.keycode)
			pressed := event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED
			if pressed == v.held[key] {
				return
			}
			if pressed {
				v.held[key] = true
			} else {
				delete(v.held, key)
			}
			c.handleKeyboardKeyFrom(keyboard, v.suppressed, data)
		}),
		AddListener(unsafe.Pointer(&keyboard.events.keymap), func(unsafe.Pointer) { c.refreshActiveKeyboard(keyboard) }),
		AddListener(unsafe.Pointer(&keyboard.events.repeat_info), func(unsafe.Pointer) { c.refreshActiveKeyboard(keyboard) }),
		AddListener(unsafe.Pointer(&keyboard.events.modifiers), func(unsafe.Pointer) {
			if !v.resetting && !c.injectedInputBlocked() {
				c.handleKeyboardModifiersFrom(keyboard)
			}
		}),
		AddListener(unsafe.Pointer(&keyboard.base.events.destroy), func(unsafe.Pointer) {
			c.releaseVirtualKeyboard(v)
			for _, listener := range v.listeners {
				c.drop(listener)
			}
			delete(c.virtualKeyboards, keyboard)
			if c.activeKeyboard == keyboard {
				c.activateKeyboard(c.keyboard)
			}
			c.refreshCapabilities()
		}),
	}
	c.keep(v.listeners...)
	c.refreshCapabilities()
}

func (c *Compositor) releaseVirtualKeyboard(v *virtualKeyboard) {
	for key := range c.keyboardRoutes[v.keyboard] {
		event := C.struct_wlr_keyboard_key_event{keycode: C.uint32_t(key),
			time_msec: nowMsec(), state: C.WL_KEYBOARD_KEY_STATE_RELEASED}
		c.forwardKeyboardKey(v.keyboard, &event)
	}
	clear(v.held)
	clear(v.suppressed)
	delete(c.keyboardRoutes, v.keyboard)
	if c.keyboardRepeatKeyboard == v.keyboard {
		c.stopKeyboardRepeat()
	}
}

func (c *Compositor) handleVirtualPointer(data unsafe.Pointer) {
	event := (*C.struct_wlr_virtual_pointer_v1_new_pointer_event)(data)
	device := event.new_pointer
	if !bool(C.sysc_virtual_request_take((*C.struct_sysc_virtual_request_guard)(c.virtualPointerGuard), device.resource)) {
		C.sysc_virtual_reject(device.resource)
		return
	}
	if event.suggested_seat != nil && event.suggested_seat != c.seat {
		C.sysc_virtual_reject(device.resource)
		return
	}
	if event.suggested_output != nil {
		if _, live := c.sceneOutputs[unsafe.Pointer(event.suggested_output)]; !live {
			C.sysc_virtual_reject(device.resource)
			return
		}
	}
	pointer := &device.pointer
	v := &virtualPointer{pointer: pointer}
	c.virtualPointers[pointer] = v
	c.handleNewInput(unsafe.Pointer(&pointer.base))
	if event.suggested_output != nil {
		C.wlr_cursor_map_input_to_output(c.cursor, &pointer.base, event.suggested_output)
		v.outputGone = AddListener(unsafe.Pointer(&event.suggested_output.events.destroy), func(unsafe.Pointer) {
			c.releaseVirtualPointer(v)
			v.stopped = true
			c.drop(v.outputGone)
			v.outputGone = nil
		})
		c.keep(v.outputGone)
	}
}

func (c *Compositor) releaseVirtualPointer(v *virtualPointer) { c.releasePointerButtons(v.pointer) }

func (c *Compositor) releasePointerButtons(pointer *C.struct_wlr_pointer) {
	for button := range c.pointerButtons[pointer] {
		event := C.struct_wlr_pointer_button_event{pointer: pointer,
			time_msec: nowMsec(), button: C.uint32_t(button), state: C.WL_POINTER_BUTTON_STATE_RELEASED}
		c.deliverPointerButton(&event)
	}
}

// Only physical input wakes the session. Virtual protocols are user-seat
// devices; injected seats keep their separate Stop-gated control path.
func (c *Compositor) pointerInputAllowed(pointer *C.struct_wlr_pointer) bool {
	if v := c.virtualPointers[pointer]; v != nil {
		return !v.stopped && !c.injectedInputBlocked()
	}
	c.userActivity()
	return true
}

func (c *Compositor) updateVirtualInputGate() {
	blocked := c.injectedInputBlocked()
	if blocked == c.virtualInputGated {
		return
	}
	for _, v := range c.virtualKeyboards {
		c.releaseVirtualKeyboard(v)
		// wlroots accepts key state while our delivery gate is closed. Reset
		// on reopening too, so a later focus enter cannot replay those keys.
		v.resetting = true
		C.sysc_virtual_keyboard_reset(v.keyboard)
		v.resetting = false
	}
	for _, v := range c.virtualPointers {
		c.releaseVirtualPointer(v)
	}
	if c.seat != nil && c.keyboard != nil {
		c.activateKeyboard(c.keyboard)
	}
	c.virtualInputGated = blocked
}
