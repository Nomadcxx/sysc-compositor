package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_input_method_v2.h>
#include <wlr/types/wlr_scene.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_text_input_v3.h>
#include <wlr/types/wlr_virtual_keyboard_v1.h>

// The text input's state, as the input method sees it, then done.
static void sysc_im_send_state(struct wlr_input_method_v2 *im, struct wlr_text_input_v3 *ti) {
	if (ti->active_features & WLR_TEXT_INPUT_V3_FEATURE_SURROUNDING_TEXT) {
		wlr_input_method_v2_send_surrounding_text(im,
			ti->current.surrounding.text != NULL ? ti->current.surrounding.text : "",
			ti->current.surrounding.cursor, ti->current.surrounding.anchor);
	}
	wlr_input_method_v2_send_text_change_cause(im, ti->current.text_change_cause);
	if (ti->active_features & WLR_TEXT_INPUT_V3_FEATURE_CONTENT_TYPE) {
		wlr_input_method_v2_send_content_type(im,
			ti->current.content_type.hint, ti->current.content_type.purpose);
	}
	wlr_input_method_v2_send_done(im);
}

// The input method's committed text, as the text input sees it, then done.
static void sysc_ti_send_im_state(struct wlr_text_input_v3 *ti, struct wlr_input_method_v2 *im) {
	struct wlr_input_method_v2_state *s = &im->current;
	if (s->preedit.text != NULL) {
		wlr_text_input_v3_send_preedit_string(ti, s->preedit.text,
			s->preedit.cursor_begin, s->preedit.cursor_end);
	}
	if (s->commit_text != NULL) {
		wlr_text_input_v3_send_commit_string(ti, s->commit_text);
	}
	if (s->delete.before_length != 0 || s->delete.after_length != 0) {
		wlr_text_input_v3_send_delete_surrounding_text(ti,
			s->delete.before_length, s->delete.after_length);
	}
	wlr_text_input_v3_send_done(ti);
}

static bool sysc_same_client(struct wlr_text_input_v3 *ti, struct wlr_surface *surface) {
	return wl_resource_get_client(ti->resource) == wl_resource_get_client(surface->resource);
}

static bool sysc_keyboard_from_input_method(struct wlr_keyboard *keyboard,
		struct wlr_input_method_v2 *im) {
	struct wlr_virtual_keyboard_v1 *virtual = wlr_input_device_get_virtual_keyboard(&keyboard->base);
	return virtual != NULL && wl_resource_get_client(virtual->resource) == wl_resource_get_client(im->resource);
}

struct sysc_find_buffer {
	struct wlr_surface *surface;
	int x, y;
	bool found;
};

static void sysc_find_surface_buffer(struct wlr_scene_buffer *buffer, int x, int y, void *data) {
	struct sysc_find_buffer *find = data;
	struct wlr_scene_surface *scene_surface = wlr_scene_surface_try_from_buffer(buffer);
	if (!find->found && scene_surface != NULL && scene_surface->surface == find->surface) {
		find->x = x;
		find->y = y;
		find->found = true;
	}
}

// Where a surface is drawn, in layout coordinates.
static bool sysc_surface_layout_position(struct wlr_scene *scene, struct wlr_surface *surface,
		int *x, int *y) {
	struct sysc_find_buffer find = { .surface = surface };
	wlr_scene_node_for_each_buffer(&scene->tree.node, sysc_find_surface_buffer, &find);
	*x = find.x;
	*y = find.y;
	return find.found;
}
*/
import "C"

import "unsafe"

// Text input (text-input-v3) relayed to one input method (input-method-v2)
// on the user seat. Text inputs of the client with user keyboard focus get
// enter and leave; the enabled one is relayed to the input method, and the
// input method's preedit, commit and delete come back to it. Input-method
// popups are drawn in the overlay layer under the text cursor rectangle.
//
// An IME keyboard grab receives unhandled keys and modifiers. Only a virtual
// keyboard belonging to that IME's Wayland client bypasses its grab.

type textInput struct {
	input     *C.struct_wlr_text_input_v3
	listeners []*Listener
}

type inputPopup struct {
	popup     *C.struct_wlr_input_popup_surface_v2
	tree      *C.struct_wlr_scene_tree
	listeners []*Listener
}

func (c *Compositor) createTextInput() bool {
	textInputs := C.wlr_text_input_manager_v3_create(c.display)
	inputMethods := C.wlr_input_method_manager_v2_create(c.display)
	if textInputs == nil || inputMethods == nil {
		return false
	}
	c.textInputs = map[*C.struct_wlr_text_input_v3]*textInput{}
	c.inputPopups = map[*C.struct_wlr_input_popup_surface_v2]*inputPopup{}
	c.keep(AddListener(unsafe.Pointer(&textInputs.events.new_text_input), c.handleNewTextInput),
		AddListener(unsafe.Pointer(&inputMethods.events.new_input_method), c.handleNewInputMethod))
	return true
}

func (c *Compositor) handleNewTextInput(data unsafe.Pointer) {
	input := (*C.struct_wlr_text_input_v3)(data)
	t := &textInput{input: input}
	c.textInputs[input] = t
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		l := AddListener(signal, fn)
		t.listeners = append(t.listeners, l)
		c.keep(l)
	}
	add(unsafe.Pointer(&input.events.enable), func(unsafe.Pointer) {
		c.activeTextInput = input
		if c.inputMethod != nil {
			C.wlr_input_method_v2_send_activate(c.inputMethod)
			C.sysc_im_send_state(c.inputMethod, input)
		}
		c.placeInputPopups()
	})
	add(unsafe.Pointer(&input.events.commit), func(unsafe.Pointer) {
		if c.activeTextInput == input && c.inputMethod != nil {
			C.sysc_im_send_state(c.inputMethod, input)
		}
		c.placeInputPopups()
	})
	add(unsafe.Pointer(&input.events.disable), func(unsafe.Pointer) {
		c.deactivateTextInput(input)
	})
	add(unsafe.Pointer(&input.events.destroy), func(unsafe.Pointer) {
		c.deactivateTextInput(input)
		for _, l := range t.listeners {
			c.drop(l)
		}
		delete(c.textInputs, input)
	})
	c.updateTextInputFocus()
}

func (c *Compositor) deactivateTextInput(input *C.struct_wlr_text_input_v3) {
	if c.activeTextInput != input {
		return
	}
	c.activeTextInput = nil
	if c.inputMethod != nil {
		C.wlr_input_method_v2_send_deactivate(c.inputMethod)
		C.wlr_input_method_v2_send_done(c.inputMethod)
	}
	c.placeInputPopups()
}

// updateTextInputFocus runs after every user keyboard focus change: the
// focused client's text inputs get enter, all others leave.
func (c *Compositor) updateTextInputFocus() {
	focused := c.seat.keyboard_state.focused_surface
	for input := range c.textInputs {
		if input.seat != c.seat {
			continue
		}
		if input.focused_surface != nil && input.focused_surface != focused {
			c.deactivateTextInput(input)
			C.wlr_text_input_v3_send_leave(input)
		}
		if focused != nil && input.focused_surface == nil && bool(C.sysc_same_client(input, focused)) {
			C.wlr_text_input_v3_send_enter(input, focused)
		}
	}
}

type keyboardRoute struct {
	grab      *C.struct_wlr_input_method_keyboard_grab_v2
	announced bool
}

func (c *Compositor) handleNewInputMethod(data unsafe.Pointer) {
	im := (*C.struct_wlr_input_method_v2)(data)
	if im.seat != c.seat || c.inputMethod != nil {
		C.wlr_input_method_v2_send_unavailable(im)
		return
	}
	c.inputMethod = im
	var listeners []*Listener
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		l := AddListener(signal, fn)
		listeners = append(listeners, l)
		c.keep(l)
	}
	add(unsafe.Pointer(&im.events.commit), func(unsafe.Pointer) {
		if c.activeTextInput != nil {
			C.sysc_ti_send_im_state(c.activeTextInput, im)
		}
	})
	add(unsafe.Pointer(&im.events.grab_keyboard), func(data unsafe.Pointer) {
		grab := (*C.struct_wlr_input_method_keyboard_grab_v2)(data)
		C.wlr_input_method_keyboard_grab_v2_set_keyboard(grab, c.keyboard)
		for source, routes := range c.keyboardRoutes {
			for key, route := range routes {
				if !route.announced && route.grab == nil && c.inputMethodGrab(source) == grab {
					route.grab = grab
					routes[key] = route
				}
			}
		}
		var destroy *Listener
		destroy = AddListener(unsafe.Pointer(&grab.events.destroy), func(unsafe.Pointer) {
			c.drop(destroy)
			// Retire only presses delivered to this grab. Client-owned keys
			// keep their release route, and a reused native address owns none.
			for keyboard, routes := range c.keyboardRoutes {
				for key, destination := range routes {
					if destination.grab == grab {
						delete(routes, key)
					}
				}
				if len(routes) == 0 {
					delete(c.keyboardRoutes, keyboard)
				}
			}
			c.activateKeyboard(c.keyboard)
		})
		c.keep(destroy)
	})
	add(unsafe.Pointer(&im.events.new_popup_surface), c.handleNewInputPopup)
	add(unsafe.Pointer(&im.events.destroy), func(unsafe.Pointer) {
		for _, l := range listeners {
			c.drop(l)
		}
		c.inputMethod = nil
	})
	if c.activeTextInput != nil {
		C.wlr_input_method_v2_send_activate(im)
		C.sysc_im_send_state(im, c.activeTextInput)
	}
}

func (c *Compositor) inputMethodGrab(keyboard *C.struct_wlr_keyboard) *C.struct_wlr_input_method_keyboard_grab_v2 {
	if c.sessionLocked || c.inputMethod == nil || c.inputMethod.keyboard_grab == nil ||
		bool(C.sysc_keyboard_from_input_method(keyboard, c.inputMethod)) {
		return nil
	}
	return c.inputMethod.keyboard_grab
}

// A key release follows its press destination. The application and each
// live IME grab see the union of their sources: first press, final release.
func (c *Compositor) forwardKeyboardKey(keyboard *C.struct_wlr_keyboard, event *C.struct_wlr_keyboard_key_event) {
	key := uint32(event.keycode)
	routes := c.keyboardRoutes[keyboard]
	pressed := event.state == C.WL_KEYBOARD_KEY_STATE_PRESSED
	var route keyboardRoute
	if pressed {
		if _, exists := routes[key]; exists {
			return
		}
		route.grab = c.inputMethodGrab(keyboard)
		route.announced = c.keyDestinationHeld(key, route.grab)
		if c.keyboardRoutes == nil {
			c.keyboardRoutes = make(map[*C.struct_wlr_keyboard]map[uint32]keyboardRoute)
		}
		if routes == nil {
			routes = make(map[uint32]keyboardRoute)
			c.keyboardRoutes[keyboard] = routes
		}
		routes[key] = route
		if route.announced {
			return
		}
	} else {
		var exists bool
		route, exists = routes[key]
		if !exists {
			return
		}
		delete(routes, key)
		if len(routes) == 0 {
			delete(c.keyboardRoutes, keyboard)
		}
		if !route.announced || c.keyDestinationHeld(key, route.grab) {
			return
		}
	}
	if route.grab != nil {
		C.wlr_input_method_keyboard_grab_v2_set_keyboard(route.grab, keyboard)
		C.wlr_input_method_keyboard_grab_v2_send_key(route.grab, event.time_msec, event.keycode, C.uint32_t(event.state))
		route.announced = true
	} else if c.activateKeyboard(keyboard) || !pressed {
		route.announced = c.updateSeatKey(event, true)
	}
	if pressed {
		routes[key] = route
	}
}

func (c *Compositor) keyDestinationHeld(key uint32, grab *C.struct_wlr_input_method_keyboard_grab_v2) bool {
	// ponytail: scan held routes (O(held input)); per-destination counts are
	// the upgrade if a measured device count makes this path expensive.
	for _, routes := range c.keyboardRoutes {
		if destination, exists := routes[key]; exists && destination.announced && destination.grab == grab {
			return true
		}
	}
	return false
}

func (c *Compositor) handleNewInputPopup(data unsafe.Pointer) {
	popup := (*C.struct_wlr_input_popup_surface_v2)(data)
	tree := C.wlr_scene_subsurface_tree_create(c.layerTrees[layerOverlay], popup.surface)
	if tree == nil {
		return
	}
	p := &inputPopup{popup: popup, tree: tree}
	c.inputPopups[popup] = p
	add := func(signal unsafe.Pointer, fn func(unsafe.Pointer)) {
		l := AddListener(signal, fn)
		p.listeners = append(p.listeners, l)
		c.keep(l)
	}
	add(unsafe.Pointer(&popup.surface.events._map), func(unsafe.Pointer) { c.placeInputPopups() })
	add(unsafe.Pointer(&popup.events.destroy), func(unsafe.Pointer) {
		for _, l := range p.listeners {
			c.drop(l)
		}
		delete(c.inputPopups, popup)
		C.wlr_scene_node_destroy(&p.tree.node)
	})
	c.placeInputPopups()
}

// placeInputPopups puts every input-method popup just below the active text
// input's cursor rectangle, and hides them while no text input is active.
func (c *Compositor) placeInputPopups() {
	input := c.activeTextInput
	var x, y C.int
	visible := input != nil && input.focused_surface != nil &&
		bool(C.sysc_surface_layout_position(c.scene, input.focused_surface, &x, &y))
	for _, p := range c.inputPopups {
		C.wlr_scene_node_set_enabled(&p.tree.node, C.bool(visible))
		if !visible {
			continue
		}
		box := input.current.cursor_rectangle
		C.wlr_scene_node_set_position(&p.tree.node, x+box.x, y+box.y+box.height)
		local := C.struct_wlr_box{x: 0, y: -box.height, width: box.width, height: box.height}
		C.wlr_input_popup_surface_v2_send_text_input_rectangle(p.popup, &local)
	}
}

// surfaceLayoutPosition is where a surface is drawn, in layout coordinates.
func (c *Compositor) surfaceLayoutPosition(surface unsafe.Pointer) (int, int, bool) {
	var x, y C.int
	found := bool(C.sysc_surface_layout_position(c.scene, (*C.struct_wlr_surface)(surface), &x, &y))
	return int(x), int(y), found
}

// sceneTreePosition is a tree's position in layout coordinates.
func sceneTreePosition(tree *C.struct_wlr_scene_tree) (int, int) {
	var x, y C.int
	C.wlr_scene_node_coords(&tree.node, &x, &y)
	return int(x), int(y)
}
