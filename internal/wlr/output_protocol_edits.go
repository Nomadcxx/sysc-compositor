package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <wlr/types/wlr_output_management_v1.h>
#include <string.h>
#include <stdlib.h>
#include <stdint.h>
extern void sysc_listener_fire(uintptr_t token, void *data);
struct sysc_output_edit {
	struct wlr_output_configuration_v1 *config;
	struct wlr_output_configuration_head_v1 *head;
	bool enabled;
};
static void sysc_output_log_request(void *data, enum wl_protocol_logger_type direction,
		const struct wl_protocol_logger_message *message) {
	if (direction != WL_PROTOCOL_LOGGER_REQUEST || strcmp(wl_resource_get_class(message->resource),"zwlr_output_configuration_head_v1") != 0 || strcmp(message->message->name,"set_adaptive_sync") != 0) return;
	struct wlr_output_configuration_head_v1 *head = wl_resource_get_user_data(message->resource);
	if (head == NULL) return;
	struct sysc_output_edit edit = {head->config,head,message->arguments[0].u == 1};
	sysc_listener_fire((uintptr_t)data,&edit);
}
static struct wl_protocol_logger *sysc_output_logger(struct wl_display *display, uintptr_t token) {
	return wl_display_add_protocol_logger(display,sysc_output_log_request,(void *)token);
}
struct sysc_output_resource_watch { struct wl_listener destroy; uintptr_t token; };
static void sysc_output_resource_destroy(struct wl_listener *listener, void *data) {
	struct sysc_output_resource_watch *watch = wl_container_of(listener,watch,destroy);
	wl_list_remove(&watch->destroy.link);
	sysc_listener_fire(watch->token,NULL);
	free(watch);
}
static bool sysc_output_watch_resource(struct wl_resource *resource, uintptr_t token) {
	struct sysc_output_resource_watch *watch=calloc(1,sizeof(*watch));
	if (watch==NULL) return false;
	watch->token=token;watch->destroy.notify=sysc_output_resource_destroy;
	wl_resource_add_destroy_listener(resource,&watch->destroy);
	return true;
}
*/
import "C"

import "unsafe"

type outputProtocolEdit struct{ adaptive map[unsafe.Pointer]bool }

// wlroots exposes final values without the set of setters a client used.
// Record adaptive-sync edits so scale-only applies preserve on-demand VRR.
func (c *Compositor) startOutputProtocolEdits() bool {
	c.outputProtocolEdits = map[*C.struct_wlr_output_configuration_v1]*outputProtocolEdit{}
	c.outputLoggerToken = Register(func(data unsafe.Pointer) {
		event := (*C.struct_sysc_output_edit)(data)
		edit := c.outputProtocolEdits[event.config]
		if edit == nil {
			edit = &outputProtocolEdit{adaptive: map[unsafe.Pointer]bool{}}
			configuration := event.config
			var token uintptr
			token = Register(func(unsafe.Pointer) {
				if c.outputProtocolEdits[configuration] == edit {
					delete(c.outputProtocolEdits, configuration)
				}
				Unregister(token)
			})
			if !bool(C.sysc_output_watch_resource(configuration.resource, C.uintptr_t(token))) {
				Unregister(token)
				C.wl_resource_post_no_memory(configuration.resource)
				return
			}
			c.outputProtocolEdits[configuration] = edit
		}
		edit.adaptive[unsafe.Pointer(event.head)] = bool(event.enabled)
	})
	c.outputLogger = C.sysc_output_logger(c.display, C.uintptr_t(c.outputLoggerToken))
	if c.outputLogger == nil {
		Unregister(c.outputLoggerToken)
		c.outputLoggerToken = 0
		return false
	}
	return true
}

func (c *Compositor) stopOutputProtocolEdits() {
	if c.outputLogger != nil {
		C.wl_protocol_logger_destroy(c.outputLogger)
		c.outputLogger = nil
		Unregister(c.outputLoggerToken)
		c.outputLoggerToken = 0
	}
}
