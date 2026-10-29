package testclient

/*
#cgo pkg-config: wayland-client
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>

static const struct wl_interface sysc_test_gamma;
static const struct wl_interface *sysc_test_gamma_types[] = {&sysc_test_gamma, &wl_output_interface};
static const struct wl_message sysc_test_gamma_manager_requests[] = {
 {"get_gamma_control", "no", sysc_test_gamma_types}, {"destroy", "", NULL},
};
static const struct wl_interface sysc_test_gamma_manager = {
 "zwlr_gamma_control_manager_v1", 1, 2, sysc_test_gamma_manager_requests, 0, NULL,
};
static const struct wl_message sysc_test_gamma_requests[] = {
 {"set_gamma", "h", NULL}, {"destroy", "", NULL},
};
static const struct wl_message sysc_test_gamma_events[] = {
 {"gamma_size", "u", NULL}, {"failed", "", NULL},
};
static const struct wl_interface sysc_test_gamma = {
 "zwlr_gamma_control_v1", 1, 2, sysc_test_gamma_requests, 2, sysc_test_gamma_events,
};
struct sysc_test_gamma_client {
 struct wl_display *display;
 struct wl_registry *registry;
 struct wl_output *output;
 struct wl_proxy *manager, *control;
 uint32_t size;
 unsigned output_index, outputs_seen;
 bool failed;
};
static void sysc_test_gamma_geometry(void *data, struct wl_output *output, int32_t x,
 int32_t y, int32_t width, int32_t height, int32_t subpixel,
 const char *make, const char *model, int32_t transform) {}
static void sysc_test_gamma_mode(void *data, struct wl_output *output, uint32_t flags,
 int32_t width, int32_t height, int32_t refresh) {}
static const struct wl_output_listener sysc_test_gamma_output_listener = {
 .geometry = sysc_test_gamma_geometry, .mode = sysc_test_gamma_mode,
};
static void sysc_test_gamma_global(void *data, struct wl_registry *registry,
 uint32_t name, const char *interface, uint32_t version) {
 struct sysc_test_gamma_client *client = data;
 if (strcmp(interface, "wl_output") == 0 && client->outputs_seen++ == client->output_index) {
  client->output = wl_registry_bind(registry, name, &wl_output_interface, 1);
  wl_output_add_listener(client->output, &sysc_test_gamma_output_listener, client);
 }
 if (strcmp(interface, sysc_test_gamma_manager.name) == 0)
  client->manager = wl_registry_bind(registry, name, &sysc_test_gamma_manager, 1);
}
static void sysc_test_gamma_global_remove(void *data, struct wl_registry *registry, uint32_t name) {}
static const struct wl_registry_listener sysc_test_gamma_registry_listener = {
 sysc_test_gamma_global, sysc_test_gamma_global_remove,
};
static void sysc_test_gamma_size(void *data, struct wl_proxy *control, uint32_t size) {
 ((struct sysc_test_gamma_client *)data)->size = size;
}
static void sysc_test_gamma_failed(void *data, struct wl_proxy *control) {
 ((struct sysc_test_gamma_client *)data)->failed = true;
}
static void (*sysc_test_gamma_listener[])(void) = {
 (void (*)(void))sysc_test_gamma_size, (void (*)(void))sysc_test_gamma_failed,
};
static void sysc_test_gamma_close(struct sysc_test_gamma_client *client) {
 if (client == NULL) return;
 wl_display_disconnect(client->display);
 free(client);
}
static struct sysc_test_gamma_client *sysc_test_gamma_start(const char *socket, unsigned index) {
 struct sysc_test_gamma_client *client = calloc(1, sizeof(*client));
 if (client == NULL) return NULL;
 client->output_index = index;
 client->display = wl_display_connect(socket);
 if (client->display == NULL) {free(client); return NULL;}
 client->registry = wl_display_get_registry(client->display);
 wl_registry_add_listener(client->registry, &sysc_test_gamma_registry_listener, client);
 if (wl_display_roundtrip(client->display) < 0 || client->output == NULL || client->manager == NULL) goto failed;
 client->control = wl_proxy_marshal_flags(client->manager, 0, &sysc_test_gamma, 1, 0, NULL, client->output);
 if (client->control == NULL) goto failed;
 wl_proxy_add_listener(client->control, sysc_test_gamma_listener, client);
 if (wl_display_roundtrip(client->display) < 0) goto failed;
 return client;
failed:
 sysc_test_gamma_close(client);
 return NULL;
}
static int sysc_test_gamma_set(struct sysc_test_gamma_client *client, int fd) {
 wl_proxy_marshal_flags(client->control, 0, NULL, 1, 0, fd);
 return wl_display_roundtrip(client->display);
}
static int sysc_test_gamma_destroy(struct sysc_test_gamma_client *client) {
 if (client->control != NULL) {
  wl_proxy_marshal_flags(client->control, 1, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
  client->control = NULL;
 }
 return wl_display_roundtrip(client->display);
}
static const char *sysc_test_gamma_protocol_error(struct sysc_test_gamma_client *client, uint32_t *code) {
 const struct wl_interface *interface = NULL;
 *code = wl_display_get_protocol_error(client->display, &interface, NULL);
 return interface == NULL ? "" : interface->name;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

type GammaClient struct {
	native *C.struct_sysc_test_gamma_client
}

func StartGammaControl(socket string, outputIndex int) (*GammaClient, error) {
	if outputIndex < 0 {
		return nil, errors.New("negative output index")
	}
	name := C.CString(socket)
	defer C.free(unsafe.Pointer(name))
	client := C.sysc_test_gamma_start(name, C.uint(outputIndex))
	if client == nil {
		return nil, errors.New("gamma-control manager or output unavailable")
	}
	return &GammaClient{client}, nil
}

func (c *GammaClient) Size() uint32 { return uint32(c.native.size) }
func (c *GammaClient) Failed() bool { return bool(c.native.failed) }

func (c *GammaClient) SetGamma(fd int) error {
	if C.sysc_test_gamma_set(c.native, C.int(fd)) < 0 {
		return errors.New("gamma connection failed")
	}
	return nil
}

func (c *GammaClient) DestroyControl() error {
	if C.sysc_test_gamma_destroy(c.native) < 0 {
		return errors.New("gamma connection failed")
	}
	return nil
}

func (c *GammaClient) ProtocolError() (string, uint32) {
	var code C.uint32_t
	name := C.sysc_test_gamma_protocol_error(c.native, &code)
	return C.GoString(name), uint32(code)
}

func (c *GammaClient) Close() {
	if c != nil && c.native != nil {
		C.sysc_test_gamma_close(c.native)
		c.native = nil
	}
}
