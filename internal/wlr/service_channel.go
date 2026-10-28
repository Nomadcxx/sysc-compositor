package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#include <stdlib.h>
#include <unistd.h>
#include <fcntl.h>
#include <wayland-server-core.h>
#include <wlr/types/wlr_compositor.h>

extern void sysc_listener_fire(uintptr_t handle, void *data);
struct sysc_service_client {
 struct wl_listener destroyed;
 uintptr_t handle;
};
static void sysc_service_client_gone(struct wl_listener *listener, void *data) {
 struct sysc_service_client *service=wl_container_of(listener,service,destroyed);
 wl_list_remove(&listener->link);
 sysc_listener_fire(service->handle,data);
 free(service);
}
static struct wl_client *sysc_service_client_create(struct wl_display *display,int fd,uintptr_t handle) {
 struct sysc_service_client *service=calloc(1,sizeof(*service));
 if(service==NULL)return NULL;
 int copy=fcntl(fd,F_DUPFD_CLOEXEC,0);
 if(copy<0){free(service);return NULL;}
 struct wl_client *client=wl_client_create(display,copy);
 if(client==NULL){close(copy);free(service);return NULL;}
 service->handle=handle;
 service->destroyed.notify=sysc_service_client_gone;
 wl_client_add_destroy_listener(client,&service->destroyed);
 return client;
}
static struct wl_client *sysc_surface_client(struct wlr_surface *surface){return wl_resource_get_client(surface->resource);}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// AddServiceClient borrows a socket endpoint and gives a duplicate to Wayland.
// Call on the compositor thread. These portal clients have normal privileged
// protocol access; socketpair credentials identify us, not the remote process.
func (c *Compositor) AddServiceClient(fd int) error {
	if c.display == nil {
		return ErrClosed
	}
	token := Register(func(pointer unsafe.Pointer) {
		token := c.serviceClients[pointer]
		delete(c.serviceClients, pointer)
		Unregister(token)
	})
	client := C.sysc_service_client_create(c.display, C.int(fd), C.uintptr_t(token))
	if client == nil {
		Unregister(token)
		return errors.New("wlr: creating service client failed")
	}
	if c.serviceClients == nil {
		c.serviceClients = map[unsafe.Pointer]uintptr{}
	}
	c.serviceClients[unsafe.Pointer(client)] = token
	return nil
}

func (c *Compositor) surfaceCredentialsUnknown(surface *C.struct_wlr_surface) bool {
	_, service := c.serviceClients[unsafe.Pointer(C.sysc_surface_client(surface))]
	return service
}
