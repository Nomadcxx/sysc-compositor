package testclient

/*
#cgo pkg-config: wayland-client
#define _GNU_SOURCE
#include <wayland-client.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <poll.h>
#include <errno.h>
#include <time.h>

static const struct wl_interface cap_source, cap_session, cap_frame, cap_handle, cap_cursor;
static const struct wl_interface *cap_handle_types[] = {&cap_handle};
static const struct wl_message cap_handle_req[] = {{"destroy", "", NULL}};
static const struct wl_message cap_handle_evt[] = {{"closed", "", NULL},
						   {"done", "", NULL},
						   {"title", "s", NULL},
						   {"app_id", "s", NULL},
						   {"identifier", "s", NULL}};
static const struct wl_interface cap_handle = {
    "ext_foreign_toplevel_handle_v1", 1, 1, cap_handle_req, 5, cap_handle_evt};
static const struct wl_message cap_list_req[] = {{"stop", "", NULL}, {"destroy", "", NULL}};
static const struct wl_message cap_list_evt[] = {{"toplevel", "n", cap_handle_types},
						 {"finished", "", NULL}};
static const struct wl_interface cap_list = {
    "ext_foreign_toplevel_list_v1", 1, 2, cap_list_req, 2, cap_list_evt};
static const struct wl_message cap_source_req[] = {{"destroy", "", NULL}};
static const struct wl_interface cap_source = {
    "ext_image_capture_source_v1", 1, 1, cap_source_req, 0, NULL};
static const struct wl_interface *cap_win_types[] = {&cap_source, &cap_handle};
static const struct wl_interface *cap_out_types[] = {&cap_source, &wl_output_interface};
static const struct wl_message cap_win_req[] = {{"create_source", "no", cap_win_types},
						{"destroy", "", NULL}};
static const struct wl_message cap_out_req[] = {{"create_source", "no", cap_out_types},
						{"destroy", "", NULL}};
static const struct wl_interface cap_win = {
    "ext_foreign_toplevel_image_capture_source_manager_v1", 1, 2, cap_win_req, 0, NULL};
static const struct wl_interface cap_out = {
    "ext_output_image_capture_source_manager_v1", 1, 2, cap_out_req, 0, NULL};
static const struct wl_interface *cap_copy_types[] = {&cap_session, &cap_source, NULL};
static const struct wl_interface *cap_cursor_types[] = {&cap_cursor, &cap_source, &wl_pointer_interface};
static const struct wl_message cap_cursor_req[] = {{"destroy", "", NULL}, {"get_capture_session", "n", cap_copy_types}};
static const struct wl_message cap_cursor_evt[] = {{"enter", "", NULL}, {"leave", "", NULL}, {"position", "ii", NULL}, {"hotspot", "ii", NULL}};
static const struct wl_interface cap_cursor = {"ext_image_copy_capture_cursor_session_v1",1,2,cap_cursor_req,4,cap_cursor_evt};
static const struct wl_message cap_copy_req[] = {{"create_session", "nou", cap_copy_types},
						 {"create_pointer_cursor_session", "noo", cap_cursor_types},
						 {"destroy", "", NULL}};
static const struct wl_interface cap_copy = {
    "ext_image_copy_capture_manager_v1", 1, 3, cap_copy_req, 0, NULL};
static const struct wl_interface *cap_frame_types[] = {&cap_frame};
static const struct wl_message cap_session_req[] = {{"create_frame", "n", cap_frame_types},
						    {"destroy", "", NULL}};
static const struct wl_message cap_session_evt[] = {
    {"buffer_size", "uu", NULL},   {"shm_format", "u", NULL}, {"dmabuf_device", "a", NULL},
    {"dmabuf_format", "ua", NULL}, {"done", "", NULL},	      {"stopped", "", NULL}};
static const struct wl_interface cap_session = {
    "ext_image_copy_capture_session_v1", 1, 2, cap_session_req, 6, cap_session_evt};
static const struct wl_interface *cap_buffer_types[] = {&wl_buffer_interface};
static const struct wl_message cap_frame_req[] = {{"destroy", "", NULL},
						  {"attach_buffer", "o", cap_buffer_types},
						  {"damage_buffer", "iiii", NULL},
						  {"capture", "", NULL}};
static const struct wl_message cap_frame_evt[] = {{"transform", "u", NULL},
						  {"damage", "iiii", NULL},
						  {"presentation_time", "uuu", NULL},
						  {"ready", "", NULL},
						  {"failed", "u", NULL}};
static const struct wl_interface cap_frame = {
    "ext_image_copy_capture_frame_v1", 1, 4, cap_frame_req, 5, cap_frame_evt};
static const struct wl_interface cap_legacy_frame;
static const struct wl_interface *cap_legacy_output_types[] = {&cap_legacy_frame,NULL,&wl_output_interface,NULL,NULL,NULL,NULL};
static const struct wl_message cap_legacy_req[] = {{"capture_output","nio",cap_legacy_output_types},{"capture_output_region","nioiiii",cap_legacy_output_types},{"destroy","3",NULL}};
static const struct wl_interface cap_legacy = {"zwlr_screencopy_manager_v1",3,3,cap_legacy_req,0,NULL};
static const struct wl_message cap_legacy_frame_req[] = {{"copy","o",cap_buffer_types},{"destroy","",NULL},{"copy_with_damage","2o",cap_buffer_types}};
static const struct wl_message cap_legacy_frame_evt[] = {{"buffer","uuuu",NULL},{"flags","u",NULL},{"ready","uuu",NULL},{"failed","",NULL},{"damage","2uuuu",NULL},{"linux_dmabuf","3uuu",NULL},{"buffer_done","3",NULL}};
static const struct wl_interface cap_legacy_frame = {"zwlr_screencopy_frame_v1",3,3,cap_legacy_frame_req,7,cap_legacy_frame_evt};
struct cap_client;
struct cap_entry {
	struct cap_client *owner;
	struct wl_proxy *proxy;
	char id[32];
};
struct cap_client {
	struct wl_display *display;
	struct wl_shm *shm;
	struct wl_output *output;
	struct wl_output *other_output;
	struct wl_seat *seat;
	struct wl_pointer *pointer;
	struct wl_proxy *cursor;
	bool cursor_entered;
	int pointer_x,pointer_y,hotspot_x,hotspot_y,enters,leaves;
	struct wl_proxy *list, *win, *out, *copy, *source, *session, *frame, *target,
	    *source_manager;
	struct wl_proxy *legacy;
	bool legacy_mode, legacy_armed, legacy_region;
	int32_t legacy_box[4];
	struct cap_entry entries[16];
	int count;
	uint32_t width, height, format, stride;
	int constraints, stopped, ready, failed, damages;
	void *data;
	size_t bytes;
	struct wl_buffer *buffer;
};
static void cap_nothing(void *d, struct wl_proxy *p) {}
static void cap_text(void *d, struct wl_proxy *p, const char *s) {}
static void cap_id(void *d, struct wl_proxy *p, const char *s)
{
	snprintf(((struct cap_entry *)d)->id, 32, "%s", s);
}
static void (*cap_handle_listener[])(void) = {(void (*)(void))cap_nothing,
					      (void (*)(void))cap_nothing, (void (*)(void))cap_text,
					      (void (*)(void))cap_text, (void (*)(void))cap_id};
static void cap_new_handle(void *d, struct wl_proxy *p, struct wl_proxy *h)
{
	struct cap_client *c = d;
	if (c->count == 16) {
		wl_proxy_destroy(h);
		return;
	}
	struct cap_entry *e = &c->entries[c->count++];
	e->owner = c;
	e->proxy = h;
	wl_proxy_add_listener(h, cap_handle_listener, e);
}
static void (*cap_list_listener[])(void) = {(void (*)(void))cap_new_handle,
					    (void (*)(void))cap_nothing};
static void cap_shm_format(void *d, struct wl_shm *s, uint32_t f) {}
static const struct wl_shm_listener cap_shm_listener = {cap_shm_format};
static void cap_geometry(void *d, struct wl_output *o, int32_t x, int32_t y, int32_t w, int32_t h,
			 int32_t sub, const char *make, const char *model, int32_t transform)
{
}
static void cap_mode(void *d, struct wl_output *o, uint32_t flags, int32_t w, int32_t h,
		     int32_t refresh)
{
}
static const struct wl_output_listener cap_output_listener = {.geometry = cap_geometry,
							      .mode = cap_mode};
static const struct wl_pointer_listener cap_pointer_listener;
static void cap_seat_capabilities(void*d,struct wl_seat*s,uint32_t caps){
 struct cap_client*c=d;
 if((caps&WL_SEAT_CAPABILITY_POINTER)&&!c->pointer){c->pointer=wl_seat_get_pointer(s);wl_pointer_add_listener(c->pointer,&cap_pointer_listener,c);}
}
static const struct wl_seat_listener cap_seat_listener={.capabilities=cap_seat_capabilities};
static void cap_pointer_enter(void*d,struct wl_pointer*p,uint32_t serial,struct wl_surface*s,wl_fixed_t x,wl_fixed_t y){}
static void cap_pointer_leave(void*d,struct wl_pointer*p,uint32_t serial,struct wl_surface*s){}
static void cap_pointer_motion(void*d,struct wl_pointer*p,uint32_t time,wl_fixed_t x,wl_fixed_t y){}
static void cap_pointer_button(void*d,struct wl_pointer*p,uint32_t serial,uint32_t time,uint32_t button,uint32_t state){}
static void cap_pointer_axis(void*d,struct wl_pointer*p,uint32_t time,uint32_t axis,wl_fixed_t value){}
static const struct wl_pointer_listener cap_pointer_listener={cap_pointer_enter,cap_pointer_leave,cap_pointer_motion,cap_pointer_button,cap_pointer_axis};
static void cap_global(void *d, struct wl_registry *r, uint32_t n, const char *s, uint32_t v)
{
	struct cap_client *c = d;
 if(!strcmp(s,"wl_seat")&&!c->seat){c->seat=wl_registry_bind(r,n,&wl_seat_interface,1);wl_seat_add_listener(c->seat,&cap_seat_listener,c);}
	if (!strcmp(s, "wl_shm")) {
		c->shm = wl_registry_bind(r, n, &wl_shm_interface, 1);
		wl_shm_add_listener(c->shm, &cap_shm_listener, c);
	}
	if (!strcmp(s, "wl_output") && !c->output) {
		c->output = wl_registry_bind(r, n, &wl_output_interface, 1);
		wl_output_add_listener(c->output, &cap_output_listener, c);
	} else if (!strcmp(s, "wl_output") && !c->other_output) {
		c->other_output = wl_registry_bind(r, n, &wl_output_interface, 1);
		wl_output_add_listener(c->other_output, &cap_output_listener, c);
	}
	if (!strcmp(s, cap_list.name)) {
		c->list = wl_registry_bind(r, n, &cap_list, 1);
		wl_proxy_add_listener(c->list, cap_list_listener, c);
	}
	if (!strcmp(s, cap_win.name))
		c->win = wl_registry_bind(r, n, &cap_win, 1);
	if (!strcmp(s, cap_out.name))
		c->out = wl_registry_bind(r, n, &cap_out, 1);
	if (!strcmp(s, cap_copy.name))
		c->copy = wl_registry_bind(r, n, &cap_copy, 1);
	if (!strcmp(s, cap_legacy.name))
		c->legacy = wl_registry_bind(r, n, &cap_legacy, v < 3 ? v : 3);
}
static void cap_remove(void *d, struct wl_registry *r, uint32_t n) {}
static const struct wl_registry_listener cap_registry_listener = {cap_global, cap_remove};
static void cap_size(void *d, struct wl_proxy *p, uint32_t w, uint32_t h)
{
	struct cap_client *c = d;
	c->width = w;
	c->height = h;
}
static void cap_format(void *d, struct wl_proxy *p, uint32_t f)
{
	((struct cap_client *)d)->format = f;
}
static void cap_device(void *d, struct wl_proxy *p, struct wl_array *a) {}
static void cap_dmabuf(void *d, struct wl_proxy *p, uint32_t f, struct wl_array *a) {}
static void cap_done(void *d, struct wl_proxy *p) { ((struct cap_client *)d)->constraints++; }
static void cap_stopped(void *d, struct wl_proxy *p) { ((struct cap_client *)d)->stopped++; }
static void (*cap_session_listener[])(void) = {
    (void (*)(void))cap_size,	(void (*)(void))cap_format, (void (*)(void))cap_device,
    (void (*)(void))cap_dmabuf, (void (*)(void))cap_done,   (void (*)(void))cap_stopped};
static void cap_transform(void *d, struct wl_proxy *p, uint32_t n) {}
static void cap_damage(void *d, struct wl_proxy *p, int32_t x, int32_t y, int32_t w, int32_t h)
{
	((struct cap_client *)d)->damages++;
}
static void cap_time(void *d, struct wl_proxy *p, uint32_t hi, uint32_t lo, uint32_t ns) {}
static void cap_ready(void *d, struct wl_proxy *p) { ((struct cap_client *)d)->ready++; }
static void cap_failed(void *d, struct wl_proxy *p, uint32_t reason)
{
	((struct cap_client *)d)->failed++;
}
static void (*cap_frame_listener[])(void) = {(void (*)(void))cap_transform,
					     (void (*)(void))cap_damage, (void (*)(void))cap_time,
					     (void (*)(void))cap_ready, (void (*)(void))cap_failed};
static struct cap_client *cap_connect(const char *socket)
{
	struct cap_client *c = calloc(1, sizeof(*c));
	if (!c)
		return NULL;
	c->display = wl_display_connect(socket);
	if (!c->display) {
		free(c);
		return NULL;
	}
	struct wl_registry *r = wl_display_get_registry(c->display);
	wl_registry_add_listener(r, &cap_registry_listener, c);
	if (wl_display_roundtrip(c->display) < 0 || wl_display_roundtrip(c->display) < 0 ||
	    !c->shm)
		goto fail;
	return c;
fail:
	wl_display_disconnect(c->display);
	free(c);
	return NULL;
}
static struct cap_client *cap_start(const char *socket, const char *identifier)
{
	struct cap_client *c = cap_connect(socket);
	if (!c) return NULL;
	if (!c->copy) goto fail;
	struct wl_proxy *manager = c->out, *target = (struct wl_proxy *)c->output;
	if (identifier[0]) {
		manager = c->win;
		target = NULL;
		for (int i = 0; i < c->count; i++)
			if (!strcmp(identifier, c->entries[i].id))
				target = c->entries[i].proxy;
	}
	if (!manager || !target)
		goto fail;
	c->target = target;
	c->source_manager = manager;
	c->source = wl_proxy_marshal_flags(manager, 0, &cap_source, 1, 0, NULL, target);
	c->session = wl_proxy_marshal_flags(c->copy, 0, &cap_session, 1, 0, NULL, c->source, 0);
	wl_proxy_add_listener(c->session, cap_session_listener, c);
	if (wl_display_roundtrip(c->display) < 0)
		goto fail;
	return c;
fail:
	wl_display_disconnect(c->display);
	free(c);
	return NULL;
}
static void cap_legacy_buffer(void*d,struct wl_proxy*p,uint32_t f,uint32_t w,uint32_t h,uint32_t stride){struct cap_client*c=d;c->format=f;c->width=w;c->height=h;c->stride=stride;}
static void cap_legacy_ready(void*d,struct wl_proxy*p,uint32_t hi,uint32_t lo,uint32_t ns){((struct cap_client*)d)->ready++;}
static void cap_legacy_failed(void*d,struct wl_proxy*p){((struct cap_client*)d)->failed++;}
static void cap_legacy_damage(void*d,struct wl_proxy*p,uint32_t x,uint32_t y,uint32_t w,uint32_t h){((struct cap_client*)d)->damages++;}
static void cap_legacy_dmabuf(void*d,struct wl_proxy*p,uint32_t f,uint32_t w,uint32_t h){}
static void (*cap_legacy_listener[])(void)={(void(*)(void))cap_legacy_buffer,(void(*)(void))cap_transform,(void(*)(void))cap_legacy_ready,(void(*)(void))cap_legacy_failed,(void(*)(void))cap_legacy_damage,(void(*)(void))cap_legacy_dmabuf,(void(*)(void))cap_done};
static int cap_legacy_next(struct cap_client*c){
 if(c->frame)wl_proxy_marshal_flags(c->frame,1,NULL,3,WL_MARSHAL_FLAG_DESTROY);
 c->legacy_armed=false;
 if(c->legacy_region)c->frame=wl_proxy_marshal_flags(c->legacy,1,&cap_legacy_frame,3,0,NULL,0,c->output,c->legacy_box[0],c->legacy_box[1],c->legacy_box[2],c->legacy_box[3]);
 else c->frame=wl_proxy_marshal_flags(c->legacy,0,&cap_legacy_frame,3,0,NULL,0,c->output);
 wl_proxy_add_listener(c->frame,cap_legacy_listener,c);
 return wl_display_roundtrip(c->display);
}
static struct cap_client *cap_start_legacy(const char*socket,int region,int32_t x,int32_t y,int32_t w,int32_t h){
 struct cap_client*c=cap_connect(socket);if(!c)return NULL;
 if(!c->legacy||!c->output){wl_display_disconnect(c->display);free(c);return NULL;}
 c->legacy_mode=true;c->legacy_region=region;c->legacy_box[0]=x;c->legacy_box[1]=y;c->legacy_box[2]=w;c->legacy_box[3]=h;
 if(cap_legacy_next(c)<0){wl_display_disconnect(c->display);free(c);return NULL;}return c;
}
static int cap_legacy_other_output(struct cap_client*c){
 if(!c->legacy_mode||!c->legacy||!c->other_output)return -1;
 struct wl_output*output=c->output;c->output=c->other_output;c->other_output=output;
 return cap_legacy_next(c);
}
static int cap_legacy_drop_manager(struct cap_client*c){
 if(!c->legacy)return -1;
 wl_proxy_marshal_flags(c->legacy,2,NULL,3,WL_MARSHAL_FLAG_DESTROY);c->legacy=NULL;
 return wl_display_roundtrip(c->display);
}
static void cap_cursor_enter(void*d,struct wl_proxy*p){struct cap_client*c=d;c->cursor_entered=true;c->enters++;}
static void cap_cursor_leave(void*d,struct wl_proxy*p){struct cap_client*c=d;c->cursor_entered=false;c->leaves++;}
static void cap_cursor_position(void*d,struct wl_proxy*p,int32_t x,int32_t y){struct cap_client*c=d;c->pointer_x=x;c->pointer_y=y;}
static void cap_cursor_hotspot(void*d,struct wl_proxy*p,int32_t x,int32_t y){struct cap_client*c=d;c->hotspot_x=x;c->hotspot_y=y;}
static void (*cap_cursor_listener[])(void)={(void(*)(void))cap_cursor_enter,(void(*)(void))cap_cursor_leave,(void(*)(void))cap_cursor_position,(void(*)(void))cap_cursor_hotspot};
static int cap_start_cursor(struct cap_client*c){
 if(!c->pointer)return -1;
 if(c->session){wl_proxy_marshal_flags(c->session,1,NULL,1,WL_MARSHAL_FLAG_DESTROY);c->session=NULL;}
 c->constraints=0;c->ready=0;c->failed=0;c->stopped=0;
 c->cursor=wl_proxy_marshal_flags(c->copy,1,&cap_cursor,1,0,NULL,c->source,c->pointer);
 wl_proxy_add_listener(c->cursor,cap_cursor_listener,c);
 c->session=wl_proxy_marshal_flags(c->cursor,1,&cap_session,1,0,NULL);
 wl_proxy_add_listener(c->session,cap_session_listener,c);
 return wl_display_roundtrip(c->display);
}
static int cap_drop_raw_source(struct cap_client*c){if(c->source){wl_proxy_marshal_flags(c->source,0,NULL,1,WL_MARSHAL_FLAG_DESTROY);c->source=NULL;}return wl_display_roundtrip(c->display);}
static int cap_sync(struct cap_client *c) { return wl_display_roundtrip(c->display); }
static void cap_buffer_release(void *data, struct wl_buffer *buffer) {}
static const struct wl_buffer_listener cap_buffer_listener = {cap_buffer_release};
static int cap_allocate(struct cap_client *c, int wrong_size, int invalid_stride)
{
	if (c->buffer) {
		wl_buffer_destroy(c->buffer);
		c->buffer = NULL;
	}
	if (c->data) {
		munmap(c->data, c->bytes);
		c->data = NULL;
	}
	uint32_t w = c->width + wrong_size, h = c->height;
	if (!w || !h || w > (64u << 20) / 4)
		return -1;
	c->stride = w*4 + (invalid_stride ? 4 : 0);
	if (h > (64u << 20) / c->stride) return -1;
	c->bytes = (size_t)c->stride * h;
	int fd = memfd_create("capture-test", MFD_CLOEXEC);
	if (fd < 0)
		return -1;
	if (ftruncate(fd, c->bytes) < 0) {
		close(fd);
		return -1;
	}
	c->data = mmap(NULL, c->bytes, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
	if (c->data == MAP_FAILED) {
		c->data = NULL;
		close(fd);
		return -1;
	}
	struct wl_shm_pool *pool = wl_shm_create_pool(c->shm, fd, c->bytes);
	close(fd);
	c->buffer = wl_shm_pool_create_buffer(pool, 0, w, h, c->stride, c->format);
	wl_buffer_add_listener(c->buffer, &cap_buffer_listener, c);
	wl_shm_pool_destroy(pool);
	return 0;
}
static int cap_arm(struct cap_client *c, int wrong_size)
{
	if (c->frame) {
		wl_proxy_marshal_flags(c->frame, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->frame = NULL;
	}
	if (cap_allocate(c,wrong_size,0)<0)return -1;
	uint32_t w=c->width+wrong_size,h=c->height;
	c->frame = wl_proxy_marshal_flags(c->session, 0, &cap_frame, 1, 0, NULL);
	wl_proxy_add_listener(c->frame, cap_frame_listener, c);
	wl_proxy_marshal_flags(c->frame, 1, NULL, 1, 0, c->buffer);
	wl_proxy_marshal_flags(c->frame, 2, NULL, 1, 0, 0, 0, w, h);
	wl_proxy_marshal_flags(c->frame, 3, NULL, 1, 0);
	return wl_display_flush(c->display) < 0 ? -1 : 0;
}
static int cap_arm_legacy(struct cap_client*c,int damage,int invalid_stride){
 if(!c->legacy_mode)return -1;
 if(c->legacy_armed&&cap_legacy_next(c)<0)return -1;
 if(cap_allocate(c,0,invalid_stride)<0)return -1;
 c->legacy_armed=true;
 wl_proxy_marshal_flags(c->frame,damage?2:0,NULL,3,0,c->buffer);
 return wl_display_flush(c->display)<0?-1:0;
}
static int cap_wait(struct cap_client *c, int ms)
{
	if (wl_display_dispatch_pending(c->display) < 0)
		return -1;
	struct pollfd p = {wl_display_get_fd(c->display), POLLIN, 0};
	struct timespec now, end;
	clock_gettime(CLOCK_MONOTONIC, &end);
	end.tv_sec += ms / 1000;
	end.tv_nsec += (long)(ms % 1000) * 1000000;
	int n;
	// The Go runtime's preemption signal interrupts poll, which never restarts.
	while ((n = poll(&p, 1, ms)) < 0 && errno == EINTR) {
		clock_gettime(CLOCK_MONOTONIC, &now);
		ms = (int)((end.tv_sec - now.tv_sec) * 1000 + (end.tv_nsec - now.tv_nsec) / 1000000);
		if (ms <= 0)
			return 0;
	}
	return n <= 0 ? n : wl_display_dispatch(c->display) < 0 ? -1 : 1;
}
static int cap_destroy_objects(struct cap_client *c)
{
	if (c->frame) {
		wl_proxy_marshal_flags(c->frame, c->legacy_mode?1:0, NULL, c->legacy_mode?3:1, WL_MARSHAL_FLAG_DESTROY);
		c->frame = NULL;
	}
	if (c->session) {
		wl_proxy_marshal_flags(c->session, 1, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->session = NULL;
	}
	if (c->source) {
		wl_proxy_marshal_flags(c->source, 0, NULL, 1, WL_MARSHAL_FLAG_DESTROY);
		c->source = NULL;
	}
	return wl_display_roundtrip(c->display);
}
static int cap_reopen(struct cap_client *c)
{
	if (cap_destroy_objects(c) < 0)
		return -1;
	c->source =
	    wl_proxy_marshal_flags(c->source_manager, 0, &cap_source, 1, 0, NULL, c->target);
	c->session = wl_proxy_marshal_flags(c->copy, 0, &cap_session, 1, 0, NULL, c->source, 0);
	wl_proxy_add_listener(c->session, cap_session_listener, c);
	return wl_display_roundtrip(c->display);
}
static void cap_close(struct cap_client *c)
{
	wl_display_disconnect(c->display);
	if (c->data)
		munmap(c->data, c->bytes);
	free(c);
}
*/
import "C"

import (
	"errors"
	"image"
	"image/color"
	"time"
	"unsafe"
)

type CaptureClient struct{ state *C.struct_cap_client }
type CaptureState struct{ Width, Height, Constraints, Stopped, Ready, Failed, Damage int }

func StartCapture(socket, identifier string) (*CaptureClient, error) {
	s, id := C.CString(socket), C.CString(identifier)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(id))
	c := C.cap_start(s, id)
	if c == nil {
		return nil, errors.New("capture globals or source unavailable")
	}
	return &CaptureClient{c}, nil
}

// StartLegacyCapture uses only wlr-screencopy requests; nil selects the full output.
func StartLegacyCapture(socket string, region *[4]int32) (*CaptureClient, error) {
	s := C.CString(socket)
	defer C.free(unsafe.Pointer(s))
	var box [4]int32
	var selected C.int
	if region != nil {
		box = *region
		selected = 1
	}
	c := C.cap_start_legacy(s, selected, C.int32_t(box[0]), C.int32_t(box[1]), C.int32_t(box[2]), C.int32_t(box[3]))
	if c == nil {
		return nil, errors.New("legacy screencopy unavailable")
	}
	return &CaptureClient{c}, nil
}

func (c *CaptureClient) ArmLegacy(withDamage, invalidStride bool) error {
	var damage, invalid C.int
	if withDamage {
		damage = 1
	}
	if invalidStride {
		invalid = 1
	}
	if C.cap_arm_legacy(c.state, damage, invalid) < 0 {
		return errors.New("cannot allocate or arm legacy capture frame")
	}
	return nil
}

func (c *CaptureClient) SelectOtherLegacyOutput() error {
	if C.cap_legacy_other_output(c.state) < 0 {
		return errors.New("second legacy output unavailable")
	}
	return nil
}

func (c *CaptureClient) DropLegacyManager() error {
	if C.cap_legacy_drop_manager(c.state) < 0 {
		return errors.New("cannot destroy legacy manager")
	}
	return nil
}
func (c *CaptureClient) State() CaptureState {
	s := c.state
	return CaptureState{int(s.width), int(s.height), int(s.constraints), int(s.stopped), int(s.ready), int(s.failed), int(s.damages)}
}
func (c *CaptureClient) Sync() error {
	if C.cap_sync(c.state) < 0 {
		return errors.New("capture client disconnected")
	}
	return nil
}

func (c *CaptureClient) ProtocolError() (string, uint32) {
	var iface *C.struct_wl_interface
	var id C.uint32_t
	code := C.wl_display_get_protocol_error(c.state.display, &iface, &id)
	if iface == nil {
		return "", uint32(code)
	}
	return C.GoString(iface.name), uint32(code)
}
func (c *CaptureClient) Arm(wrongSize bool) error {
	wrong := 0
	if wrongSize {
		wrong = 1
	}
	if C.cap_arm(c.state, C.int(wrong)) < 0 {
		return errors.New("cannot allocate or arm capture frame")
	}
	return nil
}
func (c *CaptureClient) Wait(timeout time.Duration) error {
	if C.cap_wait(c.state, C.int(timeout.Milliseconds())) < 0 {
		return errors.New("capture dispatch failed")
	}
	return nil
}
func (c *CaptureClient) Image() *image.NRGBA {
	s := c.state
	img := image.NewNRGBA(image.Rect(0, 0, int(s.width), int(s.height)))
	p := unsafe.Slice((*byte)(s.data), int(s.bytes))
	for y := 0; y < int(s.height); y++ {
		for x := 0; x < int(s.width); x++ {
			i := y*int(s.stride) + x*4
			img.SetNRGBA(x, y, color.NRGBAModel.Convert(color.RGBA{p[i+2], p[i+1], p[i], captureAlpha(p[i+3], uint32(s.format))}).(color.NRGBA))
		}
	}
	return img
}
func (c *CaptureClient) Close() { C.cap_close(c.state); c.state = nil }

// DestroyObjects checkpoints capture object destruction without disconnecting the client.
func (c *CaptureClient) DestroyObjects() error {
	if C.cap_destroy_objects(c.state) < 0 {
		return errors.New("capture object destruction failed")
	}
	return nil
}

// Reopen reuses the original foreign handle, including after it closes.
func (c *CaptureClient) Reopen() error {
	if C.cap_reopen(c.state) < 0 {
		return errors.New("capture source reopen failed")
	}
	return nil
}

func captureAlpha(alpha byte, format uint32) byte {
	if format == uint32(C.WL_SHM_FORMAT_ARGB8888) {
		return alpha
	}
	return 255
}
func (c *CaptureClient) StartPointerCapture() error {
	if C.cap_start_cursor(c.state) < 0 {
		return errors.New("pointer capture unavailable")
	}
	return nil
}
func (c *CaptureClient) DropRawSource() error {
	if C.cap_drop_raw_source(c.state) < 0 {
		return errors.New("source destroy failed")
	}
	return nil
}

type CapturePointerState struct {
	Entered                                  bool
	X, Y, HotspotX, HotspotY, Enters, Leaves int
}

func (c *CaptureClient) PointerState() CapturePointerState {
	s := c.state
	return CapturePointerState{bool(s.cursor_entered), int(s.pointer_x), int(s.pointer_y), int(s.hotspot_x), int(s.hotspot_y), int(s.enters), int(s.leaves)}
}
