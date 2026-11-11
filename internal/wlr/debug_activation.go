package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdint.h>
#include <string.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_activation_v1.h>
#include <wlr/types/wlr_xdg_shell.h>
extern void sysc_listener_fire(uintptr_t token, void *data);
struct sysc_debug_activation_request {
	struct wlr_xdg_activation_token_v1 *token;
	struct wl_client *client;
};
static void sysc_debug_activation_log(void *data, enum wl_protocol_logger_type direction,
		const struct wl_protocol_logger_message *message) {
	if (direction != WL_PROTOCOL_LOGGER_REQUEST ||
			strcmp(wl_resource_get_class(message->resource),
					"xdg_activation_token_v1") ||
			strcmp(message->message->name, "commit")) {
		return;
	}
	struct wlr_xdg_activation_token_v1 *token = wl_resource_get_user_data(message->resource);
	if (token == NULL) {
		return;
	}
	struct sysc_debug_activation_request request = {
			token, wl_resource_get_client(message->resource)};
	sysc_listener_fire((uintptr_t)data, &request);
}
static struct wl_protocol_logger *sysc_debug_activation_logger(
		struct wl_display *display, uintptr_t token) {
	return wl_display_add_protocol_logger(display, sysc_debug_activation_log, (void *)token);
}
static bool sysc_debug_activation_valid_serial(
		struct wlr_xdg_activation_token_v1 *token, struct wl_client *client) {
	if (token->seat == NULL) {
		return false;
	}
	struct wlr_seat_client *seat = wlr_seat_client_for_wl_client(token->seat, client);
	return seat != NULL && wlr_seat_client_validate_event_serial(seat, token->serial) &&
			(token->surface == NULL ||
					token->surface ==
							token->seat->keyboard_state
									.focused_surface ||
					token->surface ==
							token->seat->pointer_state.focused_surface);
}
*/
import "C"

import (
	"fmt"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
	"time"
	"unsafe"
)

const debugActivationLifetime = 10 * time.Second

type debugActivationToken struct {
	focus   bool
	created time.Time
	destroy *Listener
}
type debugPendingActivation struct {
	focus   bool
	expires time.Time
	destroy *Listener
}

func debugActivationMayFocus(hasSerial, validSerial, seatValid, honorInvalid bool) bool {
	return hasSerial && seatValid && (validSerial || honorInvalid)
}

func (c *Compositor) watchDebugActivation(token *C.struct_wlr_xdg_activation_token_v1, focus bool) {
	pointer := unsafe.Pointer(token)
	if c.debugRuntime.activation == nil {
		c.debugRuntime.activation = map[unsafe.Pointer]*debugActivationToken{}
	}
	if old := c.debugRuntime.activation[pointer]; old != nil {
		old.focus = focus
		return
	}
	state := &debugActivationToken{focus: focus, created: time.Now()}
	state.destroy = AddListener(unsafe.Pointer(&token.events.destroy), func(unsafe.Pointer) { delete(c.debugRuntime.activation, pointer); c.drop(state.destroy) })
	c.debugRuntime.activation[pointer] = state
	c.keep(state.destroy)
}

func (c *Compositor) initializeDebugActivation() error {
	if c.xdgActivation == nil {
		return nil
	}
	c.xdgActivation.token_timeout_msec = C.uint32_t(debugActivationLifetime / time.Millisecond)
	c.debugRuntime.pendingActivation = map[unsafe.Pointer]*debugPendingActivation{}
	c.debugRuntime.activationToken = Register(func(data unsafe.Pointer) {
		event := (*C.struct_sysc_debug_activation_request)(data)
		token := event.token
		serial, seat := token.seat != nil, token.seat == c.seat
		valid := bool(C.sysc_debug_activation_valid_serial(token, event.client))
		focus := debugActivationMayFocus(serial, valid, seat, c.debugConfig.HonorXDGActivationWithInvalidSerial)
		c.watchDebugActivation(token, focus)
		// wlroots rejects an invalid serial before its new_token event. This
		// request-boundary override applies only to the compositor's own seat.
		if serial && seat && c.debugConfig.HonorXDGActivationWithInvalidSerial {
			token.seat = nil
		}
	})
	c.debugRuntime.activationLogger = C.sysc_debug_activation_logger(c.display, C.uintptr_t(c.debugRuntime.activationToken))
	if c.debugRuntime.activationLogger == nil {
		Unregister(c.debugRuntime.activationToken)
		c.debugRuntime.activationToken = 0
		return fmt.Errorf("activation protocol logger create failed")
	}
	return nil
}

func (c *Compositor) finishDebugActivation() {
	if c.debugRuntime.activationLogger != nil {
		C.wl_protocol_logger_destroy(c.debugRuntime.activationLogger)
		c.debugRuntime.activationLogger = nil
	}
	if c.debugRuntime.activationToken != 0 {
		Unregister(c.debugRuntime.activationToken)
		c.debugRuntime.activationToken = 0
	}
	for _, state := range c.debugRuntime.activation {
		c.drop(state.destroy)
	}
	for _, state := range c.debugRuntime.pendingActivation {
		c.drop(state.destroy)
	}
	c.debugRuntime.activation = nil
	c.debugRuntime.pendingActivation = nil
}

// CreateActivationToken is called on the compositor thread before spawning.
// Native token storage supplies random names, timeout and single-use ownership.
func (c *Compositor) CreateActivationToken() (string, error) {
	if c.xdgActivation == nil {
		return "", ErrClosed
	}
	token := C.wlr_xdg_activation_token_v1_create(c.xdgActivation)
	if token == nil {
		return "", fmt.Errorf("activation token allocation failed")
	}
	c.watchDebugActivation(token, true)
	return C.GoString(C.wlr_xdg_activation_token_v1_get_name(token)), nil
}

func (c *Compositor) applyDebugActivation(token *C.struct_wlr_xdg_activation_token_v1, surface *C.struct_wlr_surface) {
	state := c.debugRuntime.activation[unsafe.Pointer(token)]
	focus := false
	if state != nil {
		if time.Since(state.created) >= debugActivationLifetime {
			return
		}
		focus = state.focus
	}
	if view := c.viewForSurface(surface); view != nil && view.mapped {
		if focus {
			if !c.sessionLocked {
				c.FocusWindow(view.id)
			}
		} else {
			c.SetWindowUrgent(view.id, true)
		}
		return
	}
	pointer := unsafe.Pointer(surface)
	if c.debugRuntime.pendingActivation == nil {
		c.debugRuntime.pendingActivation = map[unsafe.Pointer]*debugPendingActivation{}
	}
	if old := c.debugRuntime.pendingActivation[pointer]; old != nil {
		c.drop(old.destroy)
	}
	pending := &debugPendingActivation{focus: focus, expires: time.Now().Add(debugActivationLifetime)}
	pending.destroy = AddListener(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
		if c.debugRuntime.pendingActivation[pointer] == pending {
			delete(c.debugRuntime.pendingActivation, pointer)
		}
		c.drop(pending.destroy)
	})
	c.debugRuntime.pendingActivation[pointer] = pending
	c.keep(pending.destroy)
}

func (c *Compositor) debugActivationFocusOnMap(view *toplevelView) bool {
	if view == nil || view.toplevel == nil {
		return false
	}
	pending := c.debugRuntime.pendingActivation[unsafe.Pointer(view.toplevel.base.surface)]
	return pending != nil && pending.focus && time.Now().Before(pending.expires)
}
func (c *Compositor) debugWindowMapped(view *toplevelView) {
	if view == nil || view.toplevel == nil {
		return
	}
	pointer := unsafe.Pointer(view.toplevel.base.surface)
	pending := c.debugRuntime.pendingActivation[pointer]
	if pending == nil {
		return
	}
	delete(c.debugRuntime.pendingActivation, pointer)
	c.drop(pending.destroy)
	if !pending.focus && time.Now().Before(pending.expires) {
		c.SetWindowUrgent(view.id, true)
	}
}

// Niri keeps each output's active column activated; tabbed columns retain
// all their tabs unless the debug policy asks for only the keyboard target.
func debugActivatedWindows(snapshot wm.Snapshot, keyboardID uint64, onlyFocused bool) map[uint64]bool {
	result := map[uint64]bool{}
	if onlyFocused {
		if keyboardID != 0 {
			result[keyboardID] = true
		}
		return result
	}
	active := map[uint64]uint64{}
	for _, workspace := range snapshot.Workspaces {
		if workspace.IsActive && workspace.ActiveWindowID != nil {
			active[workspace.ID] = *workspace.ActiveWindowID
		}
	}
	byID := map[uint64]wm.WindowSnapshot{}
	for _, window := range snapshot.Windows {
		byID[window.ID] = window
	}
	for _, window := range snapshot.Windows {
		id, ok := active[window.WorkspaceID]
		if !ok {
			continue
		}
		selected := byID[id]
		result[window.ID] = window.ID == id || !window.Tile.Floating && !selected.Tile.Floating && window.Column == selected.Column && window.Tile.Display == layout.ColumnDisplayTabbed
	}
	return result
}
func (c *Compositor) debugViewActivated(view *toplevelView, keyboardFocused bool) bool {
	if view == nil || !view.mapped || c.model == nil {
		return false
	}
	id := uint64(0)
	if keyboardFocused {
		id = view.id
	}
	return debugActivatedWindows(c.model.Snapshot(), id, c.debugConfig.DeactivateUnfocusedWindows)[view.id]
}
func (c *Compositor) syncDebugActivations(focused *toplevelView, layerFocused bool) {
	if c.model == nil {
		return
	}
	id := uint64(0)
	if focused != nil && !layerFocused {
		id = focused.id
	}
	activated := debugActivatedWindows(c.model.Snapshot(), id, c.debugConfig.DeactivateUnfocusedWindows)
	for _, view := range c.views {
		if view.toplevel == nil || !view.mapped {
			continue
		}
		value := activated[view.id]
		if bool(view.toplevel.scheduled.activated) != value {
			C.wlr_xdg_toplevel_set_activated(view.toplevel, C.bool(value))
		}
	}
}
