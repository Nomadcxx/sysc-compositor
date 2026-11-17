package testclient

/*
#cgo pkg-config: wayland-client
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>
#define OG_MAX 8
struct og_output { struct wl_output *output; char name[64], make[128], model[128]; };
struct og_client { struct og_output outputs[OG_MAX]; int count; };
static void og_copy(char *dst, size_t size, const char *src) { snprintf(dst, size, "%s", src ? src : ""); }
static void og_geometry(void *data, struct wl_output *o, int32_t x, int32_t y, int32_t w, int32_t h,
		int32_t subpixel, const char *make, const char *model, int32_t transform) {
	struct og_output *out = data;
	og_copy(out->make, sizeof(out->make), make);
	og_copy(out->model, sizeof(out->model), model);
}
static void og_mode(void *data, struct wl_output *o, uint32_t flags, int32_t w, int32_t h, int32_t refresh) {}
static void og_done(void *data, struct wl_output *o) {}
static void og_scale(void *data, struct wl_output *o, int32_t factor) {}
static void og_name(void *data, struct wl_output *o, const char *name) {
	struct og_output *out = data;
	og_copy(out->name, sizeof(out->name), name);
}
static void og_description(void *data, struct wl_output *o, const char *description) {}
static const struct wl_output_listener og_output_listener = {og_geometry, og_mode, og_done, og_scale, og_name, og_description};
static void og_global(void *data, struct wl_registry *registry, uint32_t name, const char *interface, uint32_t version) {
	struct og_client *client = data;
	if (strcmp(interface, "wl_output") != 0 || client->count == OG_MAX || version < 4) return;
	struct og_output *out = &client->outputs[client->count++];
	out->output = wl_registry_bind(registry, name, &wl_output_interface, 4);
	wl_output_add_listener(out->output, &og_output_listener, out);
}
static void og_global_remove(void *data, struct wl_registry *registry, uint32_t name) {}
static const struct wl_registry_listener og_registry_listener = {og_global, og_global_remove};
static struct wl_display *og_connect(const char *socket, struct og_client *client) {
	struct wl_display *display = wl_display_connect(socket);
	if (display == NULL) return NULL;
	wl_registry_add_listener(wl_display_get_registry(display), &og_registry_listener, client);
	if (wl_display_roundtrip(display) < 0 || wl_display_roundtrip(display) < 0) {
		wl_display_disconnect(display);
		return NULL;
	}
	return display;
}
static int og_roundtrip(struct wl_display *display) { return wl_display_roundtrip(display); }
static const char *og_name_at(struct og_client *c, int i) { return c->outputs[i].name; }
static const char *og_make_at(struct og_client *c, int i) { return c->outputs[i].make; }
static const char *og_model_at(struct og_client *c, int i) { return c->outputs[i].model; }
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// OutputIdentity is the make and model one wl_output's geometry event carried.
type OutputIdentity struct{ Make, Model string }

// OutputGeometries binds every wl_output and returns, by output name, the make
// and model its geometry events last carried: once after binding, and again
// after between returns, on the same bound objects.
func OutputGeometries(socket string, between func()) (bound, after map[string]OutputIdentity, err error) {
	name := C.CString(socket)
	defer C.free(unsafe.Pointer(name))
	client := (*C.struct_og_client)(C.calloc(1, C.size_t(unsafe.Sizeof(C.struct_og_client{}))))
	defer C.free(unsafe.Pointer(client))
	display := C.og_connect(name, client)
	if display == nil {
		return nil, nil, fmt.Errorf("connecting to %s failed", socket)
	}
	defer C.wl_display_disconnect(display)
	read := func() map[string]OutputIdentity {
		outputs := map[string]OutputIdentity{}
		for i := 0; i < int(client.count); i++ {
			outputs[C.GoString(C.og_name_at(client, C.int(i)))] = OutputIdentity{
				Make: C.GoString(C.og_make_at(client, C.int(i))), Model: C.GoString(C.og_model_at(client, C.int(i)))}
		}
		return outputs
	}
	bound = read()
	if between != nil {
		between()
	}
	if C.og_roundtrip(display) < 0 || C.og_roundtrip(display) < 0 {
		return bound, nil, fmt.Errorf("roundtrip after the change failed")
	}
	return bound, read(), nil
}
