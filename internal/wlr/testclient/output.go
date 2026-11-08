package testclient

/*
#cgo pkg-config: wayland-client
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <wayland-client.h>

// The v1 wire is sufficient to exercise publication, test and apply. Keep the
// descriptors here as in gamma.go; no generated C source is checked in.
static const struct wl_interface om_manager, om_head, om_mode, om_config, om_config_head;
static const struct wl_interface *om_head_types[] = {&om_head};
static const struct wl_interface *om_mode_types[] = {&om_mode};
static const struct wl_interface *om_config_types[] = {&om_config, NULL};
static const struct wl_interface *om_enable_types[] = {&om_config_head, &om_head};
static const struct wl_message om_manager_requests[] = {{"create_configuration","nu",om_config_types},{"stop","",NULL}};
static const struct wl_message om_manager_events[] = {{"head","n",om_head_types},{"done","u",NULL},{"finished","",NULL}};
static const struct wl_interface om_manager = {"zwlr_output_manager_v1",4,2,om_manager_requests,3,om_manager_events};
static const struct wl_message om_head_events[] = {{"name","s",NULL},{"description","s",NULL},{"physical_size","ii",NULL},{"mode","n",om_mode_types},{"enabled","i",NULL},{"current_mode","o",om_mode_types},{"position","ii",NULL},{"transform","i",NULL},{"scale","f",NULL},{"finished","",NULL},{"make","2s",NULL},{"model","2s",NULL},{"serial_number","2s",NULL},{"adaptive_sync","4u",NULL}};
static const struct wl_message om_head_requests[]={{"release","3",NULL}};
static const struct wl_interface om_head = {"zwlr_output_head_v1",4,1,om_head_requests,14,om_head_events};
static const struct wl_message om_mode_events[] = {{"size","ii",NULL},{"refresh","i",NULL},{"preferred","",NULL},{"finished","",NULL}};
static const struct wl_message om_mode_requests[]={{"release","3",NULL}};
static const struct wl_interface om_mode = {"zwlr_output_mode_v1",4,1,om_mode_requests,4,om_mode_events};
static const struct wl_message om_config_requests[] = {{"enable_head","no",om_enable_types},{"disable_head","o",om_head_types},{"apply","",NULL},{"test","",NULL},{"destroy","",NULL}};
static const struct wl_message om_config_events[] = {{"succeeded","",NULL},{"failed","",NULL},{"cancelled","",NULL}};
static const struct wl_interface om_config = {"zwlr_output_configuration_v1",4,5,om_config_requests,3,om_config_events};
static const struct wl_message om_config_head_requests[] = {{"set_mode","o",om_mode_types},{"set_custom_mode","iii",NULL},{"set_position","ii",NULL},{"set_transform","i",NULL},{"set_scale","f",NULL},{"set_adaptive_sync","4u",NULL}};
static const struct wl_interface om_config_head = {"zwlr_output_configuration_head_v1",4,6,om_config_head_requests,0,NULL};
struct om_client;
struct om_client_head {
	struct om_client_head *next;
	struct wl_proxy *proxy, *mode;
	char *name;
	int enabled, x, y, transform;
	wl_fixed_t scale;
};
struct om_client {
	struct wl_display *display;
	struct wl_proxy *manager;
	struct om_client_head *heads;
	uint32_t serial;
	int count, result;
};
static void om_no_args(void *data, struct wl_proxy *object) {}
static void om_string(void *data, struct wl_proxy *object, const char *value) {}
static void om_int(void *data, struct wl_proxy *object, int32_t value) {}
static void om_two_ints(void *data, struct wl_proxy *object, int32_t a, int32_t b) {}
static void (*om_mode_listener[])(void) = {(void(*)(void))om_two_ints,(void(*)(void))om_int,(void(*)(void))om_no_args,(void(*)(void))om_no_args};
static void om_name(void *data, struct wl_proxy *object, const char *name) {
	struct om_client_head *head = data; free(head->name); head->name = strdup(name);
}
static void om_new_mode(void *data, struct wl_proxy *object, struct wl_proxy *mode) {
	wl_proxy_add_listener(mode,om_mode_listener,NULL);
}
static void om_enabled(void *data, struct wl_proxy *object, int32_t value) { ((struct om_client_head *)data)->enabled=value; }
static void om_current_mode(void *data, struct wl_proxy *object, struct wl_proxy *mode) { ((struct om_client_head *)data)->mode=mode; }
static void om_position(void *data, struct wl_proxy *object, int32_t x, int32_t y) { struct om_client_head *head=data; head->x=x; head->y=y; }
static void om_transform(void *data, struct wl_proxy *object, int32_t value) { ((struct om_client_head *)data)->transform=value; }
static void om_scale(void *data, struct wl_proxy *object, wl_fixed_t value) { ((struct om_client_head *)data)->scale=value; }
static void (*om_head_listener[])(void) = {(void(*)(void))om_name,(void(*)(void))om_string,(void(*)(void))om_two_ints,(void(*)(void))om_new_mode,(void(*)(void))om_enabled,(void(*)(void))om_current_mode,(void(*)(void))om_position,(void(*)(void))om_transform,(void(*)(void))om_scale,(void(*)(void))om_no_args,(void(*)(void))om_string,(void(*)(void))om_string,(void(*)(void))om_string,(void(*)(void))om_int};
static void om_new_head(void *data, struct wl_proxy *object, struct wl_proxy *proxy) {
	struct om_client *client=data;
	struct om_client_head *head=calloc(1,sizeof(*head));
	if (head == NULL) return;
	head->proxy=proxy; head->next=client->heads; client->heads=head; client->count++;
	wl_proxy_add_listener(proxy,om_head_listener,head);
}
static void om_done(void *data, struct wl_proxy *object, uint32_t serial) { ((struct om_client *)data)->serial=serial; }
static void (*om_manager_listener[])(void) = {(void(*)(void))om_new_head,(void(*)(void))om_done,(void(*)(void))om_no_args};
static void om_global(void *data, struct wl_registry *registry, uint32_t name, const char *interface, uint32_t version) {
	struct om_client *client=data;
	if (strcmp(interface,om_manager.name)==0) {
		client->manager=wl_registry_bind(registry,name,&om_manager,version<4?version:4);
		wl_proxy_add_listener(client->manager,om_manager_listener,client);
	}
}
static void om_global_remove(void *data, struct wl_registry *registry, uint32_t name) {}
static const struct wl_registry_listener om_registry_listener={om_global,om_global_remove};
static void om_succeeded(void *data, struct wl_proxy *object) { ((struct om_client *)data)->result=1; }
static void om_failed(void *data, struct wl_proxy *object) { ((struct om_client *)data)->result=-1; }
static void (*om_config_listener[])(void) = {(void(*)(void))om_succeeded,(void(*)(void))om_failed,(void(*)(void))om_failed};
static int om_run(const char *socket, const char *target, double scale, bool test_only, int adaptive, int *count) {
	struct om_client client={0};
	client.display=wl_display_connect(socket);
	if (client.display==NULL) return -2;
	struct wl_registry *registry=wl_display_get_registry(client.display);
	wl_registry_add_listener(registry,&om_registry_listener,&client);
	int result=-2;
	if (wl_display_roundtrip(client.display)<0 || client.manager==NULL || wl_display_roundtrip(client.display)<0) goto done;
	*count=client.count;
	struct wl_proxy *config=wl_proxy_marshal_flags(client.manager,0,&om_config,wl_proxy_get_version(client.manager),0,NULL,client.serial);
	wl_proxy_add_listener(config,om_config_listener,&client);
	bool found=false;
	for (struct om_client_head *head=client.heads; head!=NULL; head=head->next) {
		bool selected=head->name!=NULL && strcmp(head->name,target)==0;
		found |= selected;
		if (!head->enabled || (selected && scale==0)) { wl_proxy_marshal_flags(config,1,NULL,1,0,head->proxy); continue; }
		struct wl_proxy *change=wl_proxy_marshal_flags(config,0,&om_config_head,wl_proxy_get_version(config),0,NULL,head->proxy);
		if (head->mode!=NULL) wl_proxy_marshal_flags(change,0,NULL,1,0,head->mode);
		wl_proxy_marshal_flags(change,2,NULL,1,0,head->x,head->y);
		wl_proxy_marshal_flags(change,3,NULL,1,0,head->transform);
		wl_proxy_marshal_flags(change,4,NULL,1,0,selected && scale>0?wl_fixed_from_double(scale):head->scale);
		if (selected && adaptive>=0) wl_proxy_marshal_flags(change,5,NULL,1,0,(uint32_t)adaptive);
	}
	if (!found) goto done;
	wl_proxy_marshal_flags(config,test_only?3:2,NULL,1,0);
	if (wl_display_roundtrip(client.display)<0) goto done;
	result=client.result;
done:
	wl_display_disconnect(client.display);
	while (client.heads!=NULL) { struct om_client_head *next=client.heads->next; free(client.heads->name); free(client.heads); client.heads=next; }
	return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// ConfigureOutputScale exercises the output-management v1 wire. Tests verify
// both the returned feedback and the compositor's resulting geometry.
func ConfigureOutputScale(socket, target string, scale float64, testOnly bool) (int, bool, error) {
	address, name := C.CString(socket), C.CString(target)
	defer C.free(unsafe.Pointer(address))
	defer C.free(unsafe.Pointer(name))
	var heads C.int
	result := C.om_run(address, name, C.double(scale), C.bool(testOnly), -1, &heads)
	if result == -2 || result == 0 {
		return int(heads), false, fmt.Errorf("output-management client failed (%d)", result)
	}
	return int(heads), result == 1, nil
}

func DisableOutputVRR(socket, target string) error {
	address, name := C.CString(socket), C.CString(target)
	defer C.free(unsafe.Pointer(address))
	defer C.free(unsafe.Pointer(name))
	var heads C.int
	if result := C.om_run(address, name, -1, false, 0, &heads); result != 1 {
		return fmt.Errorf("disabling VRR failed (%d)", result)
	}
	return nil
}
