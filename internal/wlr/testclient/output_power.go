package testclient

/*
#cgo pkg-config: wayland-client
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>
static const struct wl_interface pc_control;
static const struct wl_interface *pc_types[]={&pc_control,&wl_output_interface};
static const struct wl_message pc_manager_requests[]={{"get_output_power","no",pc_types},{"destroy","",NULL}};
static const struct wl_interface pc_manager={"zwlr_output_power_manager_v1",1,2,pc_manager_requests,0,NULL};
static const struct wl_message pc_control_requests[]={{"set_mode","u",NULL},{"destroy","",NULL}};
static const struct wl_message pc_control_events[]={{"mode","u",NULL},{"failed","",NULL}};
static const struct wl_interface pc_control={"zwlr_output_power_v1",1,2,pc_control_requests,2,pc_control_events};
struct pc_client { struct wl_display *display; struct wl_proxy *manager; struct wl_output *output; unsigned index,seen; int mode; bool failed; };
static void pc_geometry(void *data,struct wl_output *output,int32_t x,int32_t y,int32_t w,int32_t h,int32_t subpixel,const char *make,const char *model,int32_t transform) {}
static void pc_output_mode(void *data,struct wl_output *output,uint32_t flags,int32_t w,int32_t h,int32_t refresh) {}
static const struct wl_output_listener pc_output_listener={.geometry=pc_geometry,.mode=pc_output_mode};
static void pc_global(void *data,struct wl_registry *registry,uint32_t name,const char *interface,uint32_t version) {
	struct pc_client *client=data;
	if (strcmp(interface,"wl_output")==0 && client->seen++==client->index) {
		client->output=wl_registry_bind(registry,name,&wl_output_interface,1);
		wl_output_add_listener(client->output,&pc_output_listener,client);
	}
	if (strcmp(interface,pc_manager.name)==0) client->manager=wl_registry_bind(registry,name,&pc_manager,1);
}
static void pc_global_remove(void *data,struct wl_registry *registry,uint32_t name) {}
static const struct wl_registry_listener pc_registry_listener={pc_global,pc_global_remove};
static void pc_mode(void *data,struct wl_proxy *power,uint32_t mode) { ((struct pc_client *)data)->mode=mode; }
static void pc_failed(void *data,struct wl_proxy *power) { ((struct pc_client *)data)->failed=true; }
static void (*pc_listener[])(void)={(void(*)(void))pc_mode,(void(*)(void))pc_failed};
static bool pc_run(const char *socket,unsigned index,bool enabled) {
	struct pc_client client={.index=index,.mode=-1};
	client.display=wl_display_connect(socket);
	if (client.display==NULL) return false;
	bool ok=false;
	struct wl_registry *registry=wl_display_get_registry(client.display);
	wl_registry_add_listener(registry,&pc_registry_listener,&client);
	if (wl_display_roundtrip(client.display)<0 || client.manager==NULL || client.output==NULL) goto done;
	struct wl_proxy *control=wl_proxy_marshal_flags(client.manager,0,&pc_control,1,0,NULL,client.output);
	wl_proxy_add_listener(control,pc_listener,&client);
	if (wl_display_roundtrip(client.display)<0 || client.failed) goto done;
	wl_proxy_marshal_flags(control,0,NULL,1,0,(uint32_t)enabled);
	if (wl_display_roundtrip(client.display)<0) goto done;
	ok=!client.failed && client.mode==(int)enabled;
done:
	wl_display_disconnect(client.display);
	return ok;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func SetOutputPower(socket string, index uint, enabled bool) error {
	name := C.CString(socket)
	defer C.free(unsafe.Pointer(name))
	if !bool(C.pc_run(name, C.uint(index), C.bool(enabled))) {
		return fmt.Errorf("output-power request failed")
	}
	return nil
}
