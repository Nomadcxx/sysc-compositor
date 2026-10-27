package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server libdrm
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <time.h>
#include <drm_fourcc.h>
#include <wayland-server-core.h>
#include <wlr/interfaces/wlr_ext_image_capture_source_v1.h>
#include <wlr/types/wlr_ext_image_copy_capture_v1.h>
#include <wlr/types/wlr_buffer.h>
#include <wlr/types/wlr_output.h>
#include <wlr/util/box.h>
#include <wlr/util/transform.h>

extern void sysc_capture_create(uintptr_t, struct wl_client *, uint32_t, struct wl_resource *, int);
extern void sysc_capture_source_event(uintptr_t, int, int);
extern void sysc_capture_copy(uintptr_t, struct wlr_ext_image_copy_capture_frame_v1 *);
extern struct wlr_ext_image_capture_source_v1_cursor *sysc_capture_get_pointer(uintptr_t, struct wlr_seat *);
extern void sysc_legacy_create(uintptr_t, struct wl_resource *, struct wl_resource *,
			       struct wl_resource *, int, int, int, int, int, int);
extern void sysc_legacy_copy(uintptr_t, struct wl_resource *, struct wl_resource *, int);
extern void sysc_legacy_destroyed(uintptr_t, struct wl_resource *, int);

static const struct wl_interface sysc_capture_source_iface = {
    "ext_image_capture_source_v1", 1, 0, NULL, 0, NULL};
static const struct wl_interface sysc_capture_foreign_iface = {
    "ext_foreign_toplevel_handle_v1", 1, 0, NULL, 0, NULL};
static const struct wl_interface *sysc_capture_window_types[] = {&sysc_capture_source_iface,
								 &sysc_capture_foreign_iface};
static const struct wl_interface *sysc_capture_output_types[] = {&sysc_capture_source_iface,
								 &wl_output_interface};
static const struct wl_message sysc_capture_window_requests[] = {
    {"create_source", "no", sysc_capture_window_types}, {"destroy", "", NULL}};
static const struct wl_message sysc_capture_output_requests[] = {
    {"create_source", "no", sysc_capture_output_types}, {"destroy", "", NULL}};
static const struct wl_interface sysc_capture_window_iface = {
    "ext_foreign_toplevel_image_capture_source_manager_v1",
    1,
    2,
    sysc_capture_window_requests,
    0,
    NULL};
static const struct wl_interface sysc_capture_output_iface = {
    "ext_output_image_capture_source_manager_v1", 1, 2, sysc_capture_output_requests, 0, NULL};
static void sysc_capture_destroy_resource(struct wl_client *client, struct wl_resource *r)
{
	wl_resource_destroy(r);
}
static void sysc_capture_create_window(struct wl_client *client, struct wl_resource *r, uint32_t id,
				       struct wl_resource *target)
{
	sysc_capture_create((uintptr_t)wl_resource_get_user_data(r), client, id, target, 1);
}
static void sysc_capture_create_output(struct wl_client *client, struct wl_resource *r, uint32_t id,
				       struct wl_resource *target)
{
	sysc_capture_create((uintptr_t)wl_resource_get_user_data(r), client, id, target, 0);
}
static const struct {
	void (*create)(struct wl_client *, struct wl_resource *, uint32_t, struct wl_resource *);
	void (*destroy)(struct wl_client *, struct wl_resource *);
} sysc_capture_window_impl = {sysc_capture_create_window, sysc_capture_destroy_resource},
  sysc_capture_output_impl = {sysc_capture_create_output, sysc_capture_destroy_resource};
static void sysc_capture_bind_window(struct wl_client *client, void *data, uint32_t version,
				     uint32_t id)
{
	struct wl_resource *r = wl_resource_create(client, &sysc_capture_window_iface, 1, id);
	if (!r) {
		wl_client_post_no_memory(client);
		return;
	}
	wl_resource_set_implementation(r, &sysc_capture_window_impl, data, NULL);
}
static void sysc_capture_bind_output(struct wl_client *client, void *data, uint32_t version,
				     uint32_t id)
{
	struct wl_resource *r = wl_resource_create(client, &sysc_capture_output_iface, 1, id);
	if (!r) {
		wl_client_post_no_memory(client);
		return;
	}
	wl_resource_set_implementation(r, &sysc_capture_output_impl, data, NULL);
}

struct sysc_capture_source {
	struct wlr_ext_image_capture_source_v1_cursor cursor;
	uintptr_t handle;
	struct wl_listener resource_destroy;
	bool resource_linked, is_cursor;
};
static void sysc_capture_start(struct wlr_ext_image_capture_source_v1 *base, bool pointers)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	sysc_capture_source_event(s->handle, 0, pointers);
}
static void sysc_capture_stop(struct wlr_ext_image_capture_source_v1 *base)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	sysc_capture_source_event(s->handle, 1, 0);
}
static void sysc_capture_request(struct wlr_ext_image_capture_source_v1 *base, bool schedule)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	sysc_capture_source_event(s->handle, 2, schedule);
}
static void sysc_capture_copy_frame(struct wlr_ext_image_capture_source_v1 *base,
				    struct wlr_ext_image_copy_capture_frame_v1 *frame,
				    struct wlr_ext_image_capture_source_v1_frame_event *event)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	sysc_capture_copy(s->handle, frame);
}
static struct wlr_ext_image_capture_source_v1_cursor *
sysc_capture_cursor(struct wlr_ext_image_capture_source_v1 *base, struct wlr_seat *seat)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	return sysc_capture_get_pointer(s->handle, seat);
}
static const struct wlr_ext_image_capture_source_v1_interface sysc_capture_source_impl = {
    .start = sysc_capture_start,
    .stop = sysc_capture_stop,
    .request_frame = sysc_capture_request,
    .copy_frame = sysc_capture_copy_frame,
    .get_pointer_cursor = sysc_capture_cursor};
struct wlr_ext_image_capture_source_v1 *sysc_capture_source_new(uintptr_t handle, uint32_t width,
								uint32_t height, bool cursor)
{
	struct sysc_capture_source *s = calloc(1, sizeof(*s));
	if (!s)
		return NULL;
	s->is_cursor = cursor;
	if (cursor) wlr_ext_image_capture_source_v1_cursor_init(&s->cursor, &sysc_capture_source_impl);
	else wlr_ext_image_capture_source_v1_init(&s->cursor.base, &sysc_capture_source_impl);
	s->handle = handle;
	s->cursor.base.shm_formats = calloc(cursor ? 1 : 2, sizeof(uint32_t));
	if (!s->cursor.base.shm_formats) {
		if (cursor) wlr_ext_image_capture_source_v1_cursor_finish(&s->cursor);
		else wlr_ext_image_capture_source_v1_finish(&s->cursor.base);
		free(s);
		return NULL;
	}
	s->cursor.base.shm_formats[0] = DRM_FORMAT_ARGB8888;
	if (!cursor) s->cursor.base.shm_formats[1] = DRM_FORMAT_XRGB8888;
	s->cursor.base.shm_formats_len = cursor ? 1 : 2;
	s->cursor.base.width = width;
	s->cursor.base.height = height;
	return &s->cursor.base;
}
static void sysc_capture_resource_gone(struct wl_listener *l, void *data)
{
	struct sysc_capture_source *s = wl_container_of(l, s, resource_destroy);
	wl_list_remove(&l->link);
	s->resource_linked = false;
	sysc_capture_source_event(s->handle, 3, 0);
}
bool sysc_capture_source_resource(struct wlr_ext_image_capture_source_v1 *base,
				  struct wl_client *client, uint32_t id)
{
	if (!wlr_ext_image_capture_source_v1_create_resource(base, client, id))
		return false;
	if (base) {
		struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
		s->resource_destroy.notify = sysc_capture_resource_gone;
		wl_resource_add_destroy_listener(wl_client_get_object(client, id),
						 &s->resource_destroy);
		s->resource_linked = true;
	}
	return true;
}
void sysc_capture_source_finish(struct wlr_ext_image_capture_source_v1 *base)
{
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	if (s->resource_linked)
		wl_list_remove(&s->resource_destroy.link);
	if (s->is_cursor) wlr_ext_image_capture_source_v1_cursor_finish(&s->cursor);
	else wlr_ext_image_capture_source_v1_finish(base);
	free(s);
}
struct wlr_ext_image_capture_source_v1_cursor *sysc_capture_source_cursor(struct wlr_ext_image_capture_source_v1 *base) {
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	return &s->cursor;
}
void sysc_capture_cursor_update(struct wlr_ext_image_capture_source_v1 *base, bool entered, int32_t x, int32_t y, int32_t hx, int32_t hy) {
	struct sysc_capture_source *s = wl_container_of(base, s, cursor.base);
	struct wlr_ext_image_capture_source_v1_cursor *cursor = &s->cursor;
	bool changed = cursor->entered != entered;
	if (entered) {
		changed |= cursor->x != x || cursor->y != y || cursor->hotspot.x != hx || cursor->hotspot.y != hy;
		cursor->x = x; cursor->y = y; cursor->hotspot.x = hx; cursor->hotspot.y = hy;
	}
	cursor->entered = entered;
	if (changed) wl_signal_emit_mutable(&cursor->events.update, NULL);
}
void sysc_capture_source_size(struct wlr_ext_image_capture_source_v1 *base, uint32_t w, uint32_t h)
{
	base->width = w;
	base->height = h;
	wl_signal_emit_mutable(&base->events.constraints_update, NULL);
}
void sysc_capture_source_damage(struct wlr_ext_image_capture_source_v1 *base, bool dirty)
{
	pixman_region32_t damage;
	pixman_region32_init(&damage);
	if (dirty)
		pixman_region32_union_rect(&damage, &damage, 0, 0, base->width, base->height);
	struct wlr_ext_image_capture_source_v1_frame_event event = {.damage = &damage};
	wl_signal_emit_mutable(&base->events.frame, &event);
	pixman_region32_fini(&damage);
}
bool sysc_capture_frame_valid(struct wlr_ext_image_copy_capture_frame_v1 *frame, bool alpha_only)
{
	struct wlr_buffer *b = frame->buffer;
	struct wlr_shm_attributes attr;
	if (!b || b->width <= 0 || b->height <= 0 || !wlr_buffer_get_shm(b, &attr))
		return false;
	return (attr.format == DRM_FORMAT_ARGB8888 || (!alpha_only && attr.format == DRM_FORMAT_XRGB8888)) &&
	       attr.stride > 0 && (uint64_t)attr.stride >= (uint64_t)b->width * 4 &&
	       (uint64_t)attr.stride * b->height <= (64u << 20);
}
void sysc_capture_frame_ready(struct wlr_ext_image_copy_capture_frame_v1 *frame,
			      struct wlr_buffer *buffer, struct wlr_renderer *renderer, uint32_t transform)
{
	if (wlr_ext_image_copy_capture_frame_v1_copy_buffer(frame, buffer, renderer)) {
		struct timespec now;
		clock_gettime(CLOCK_MONOTONIC, &now);
		wlr_ext_image_copy_capture_frame_v1_ready(frame, transform, &now);
	}
}
void sysc_capture_frame_failed(struct wlr_ext_image_copy_capture_frame_v1 *frame, uint32_t reason)
{
	wlr_ext_image_copy_capture_frame_v1_fail(frame, reason);
}

static const struct wl_interface sysc_legacy_frame_iface;
static const struct wl_interface *sysc_legacy_output_types[] = {
    &sysc_legacy_frame_iface, NULL, &wl_output_interface, NULL, NULL, NULL, NULL};
static const struct wl_interface *sysc_legacy_buffer_types[] = {&wl_buffer_interface};
static const struct wl_message sysc_legacy_requests[] = {
    {"capture_output", "nio", sysc_legacy_output_types},
    {"capture_output_region", "nioiiii", sysc_legacy_output_types},
    {"destroy", "3", NULL}};
static const struct wl_interface sysc_legacy_manager_iface = {
    "zwlr_screencopy_manager_v1", 3, 3, sysc_legacy_requests, 0, NULL};
static const struct wl_message sysc_legacy_frame_requests[] = {
    {"copy", "o", sysc_legacy_buffer_types},
    {"destroy", "", NULL},
    {"copy_with_damage", "2o", sysc_legacy_buffer_types}};
static const struct wl_message sysc_legacy_frame_events[] = {
    {"buffer", "uuuu", NULL},  {"flags", "u", NULL},	  {"ready", "uuu", NULL},
    {"failed", "", NULL},      {"damage", "2uuuu", NULL}, {"linux_dmabuf", "3uuu", NULL},
    {"buffer_done", "3", NULL}};
static const struct wl_interface sysc_legacy_frame_iface = {
    "zwlr_screencopy_frame_v1", 3, 3, sysc_legacy_frame_requests, 7, sysc_legacy_frame_events};
static void sysc_legacy_copy_buffer(struct wl_client *client, struct wl_resource *r,
				    struct wl_resource *buffer)
{
	sysc_legacy_copy((uintptr_t)wl_resource_get_user_data(r), r, buffer, 0);
}
static void sysc_legacy_copy_damage(struct wl_client *client, struct wl_resource *r,
				    struct wl_resource *buffer)
{
	sysc_legacy_copy((uintptr_t)wl_resource_get_user_data(r), r, buffer, 1);
}
static const struct {
	void (*copy)(struct wl_client *, struct wl_resource *, struct wl_resource *);
	void (*destroy)(struct wl_client *, struct wl_resource *);
	void (*damage)(struct wl_client *, struct wl_resource *, struct wl_resource *);
} sysc_legacy_frame_impl = {sysc_legacy_copy_buffer, sysc_capture_destroy_resource,
			    sysc_legacy_copy_damage};
static void sysc_legacy_frame_gone(struct wl_resource *r)
{
	sysc_legacy_destroyed((uintptr_t)wl_resource_get_user_data(r), r, 0);
}
static void sysc_legacy_manager_gone(struct wl_resource *r)
{
	sysc_legacy_destroyed((uintptr_t)wl_resource_get_user_data(r), r, 1);
}
static void sysc_legacy_make(struct wl_client *client, struct wl_resource *r, uint32_t id,
			     int32_t pointer, struct wl_resource *out, int region, int x, int y,
			     int w, int h)
{
	struct wl_resource *f =
	    wl_resource_create(client, &sysc_legacy_frame_iface, wl_resource_get_version(r), id);
	if (!f) {
		wl_client_post_no_memory(client);
		return;
	}
	wl_resource_set_implementation(f, &sysc_legacy_frame_impl, wl_resource_get_user_data(r),
				       sysc_legacy_frame_gone);
	sysc_legacy_create((uintptr_t)wl_resource_get_user_data(r), r, f, out, pointer, region, x,
			   y, w, h);
}
static void sysc_legacy_output(struct wl_client *c, struct wl_resource *r, uint32_t id,
			       int32_t pointer, struct wl_resource *out)
{
	sysc_legacy_make(c, r, id, pointer, out, 0, 0, 0, 0, 0);
}
static void sysc_legacy_region(struct wl_client *c, struct wl_resource *r, uint32_t id,
			       int32_t pointer, struct wl_resource *out, int32_t x, int32_t y,
			       int32_t w, int32_t h)
{
	sysc_legacy_make(c, r, id, pointer, out, 1, x, y, w, h);
}
static const struct {
	void (*output)(struct wl_client *, struct wl_resource *, uint32_t, int32_t,
		       struct wl_resource *);
	void (*region)(struct wl_client *, struct wl_resource *, uint32_t, int32_t,
		       struct wl_resource *, int32_t, int32_t, int32_t, int32_t);
	void (*destroy)(struct wl_client *, struct wl_resource *);
} sysc_legacy_impl = {sysc_legacy_output, sysc_legacy_region, sysc_capture_destroy_resource};
static void sysc_legacy_bind(struct wl_client *c, void *data, uint32_t version, uint32_t id)
{
	struct wl_resource *r = wl_resource_create(c, &sysc_legacy_manager_iface, version, id);
	if (!r) {
		wl_client_post_no_memory(c);
		return;
	}
	wl_resource_set_implementation(r, &sysc_legacy_impl, data, sysc_legacy_manager_gone);
}
bool sysc_capture_sources_create(struct wl_display *display, uintptr_t handle)
{
	return wl_global_create(display, &sysc_capture_window_iface, 1, (void *)handle,
				sysc_capture_bind_window) &&
	       wl_global_create(display, &sysc_capture_output_iface, 1, (void *)handle,
				sysc_capture_bind_output) &&
	       wl_global_create(display, &sysc_legacy_manager_iface, 3, (void *)handle,
				sysc_legacy_bind);
}
void sysc_legacy_constraints(struct wl_resource *r, int width, int height)
{
	wl_resource_post_event(r, 0, WL_SHM_FORMAT_XRGB8888, width, height, width * 4);
	if (wl_resource_get_version(r) >= 3)
		wl_resource_post_event(r, 6);
}
void sysc_legacy_failed(struct wl_resource *r) { wl_resource_post_event(r, 3); }
void sysc_legacy_invalid(struct wl_resource *r, bool used)
{
	wl_resource_post_error(r, used ? 1 : 0,
			       used ? "frame already copied" : "invalid capture buffer");
}
void sysc_legacy_ready(struct wl_resource *r, int width, int height, bool damage)
{
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	wl_resource_post_event(r, 1, 0);
	if (damage && wl_resource_get_version(r) >= 2)
		wl_resource_post_event(r, 4, 0, 0, width, height);
	wl_resource_post_event(r, 2, (uint32_t)((uint64_t)now.tv_sec >> 32), (uint32_t)now.tv_sec,
			       (uint32_t)now.tv_nsec);
}
bool sysc_legacy_valid(struct wlr_buffer *b, int width, int height)
{
	struct wlr_shm_attributes a;
	return b && b->width == width && b->height == height && wlr_buffer_get_shm(b, &a) &&
	       a.format == DRM_FORMAT_XRGB8888 && a.stride == (uint64_t)width * 4 &&
	       (uint64_t)a.stride * height <= (64u << 20);
}
bool sysc_legacy_write(struct wlr_buffer *b, const uint8_t *rgba, int source_stride)
{
	void *dst;
	uint32_t format;
	size_t stride;
	if (!wlr_buffer_begin_data_ptr_access(b, WLR_BUFFER_DATA_PTR_ACCESS_WRITE, &dst, &format,
					      &stride))
		return false;
	for (int y = 0; y < b->height; y++) {
		const uint8_t *s = rgba + (size_t)y * source_stride;
		uint8_t *d = (uint8_t *)dst + (size_t)y * stride;
		for (int x = 0; x < b->width; x++) {
			d[x * 4] = s[x * 4 + 2];
			d[x * 4 + 1] = s[x * 4 + 1];
			d[x * 4 + 2] = s[x * 4];
			d[x * 4 + 3] = 255;
		}
	}
	wlr_buffer_end_data_ptr_access(b);
	return true;
}
bool sysc_legacy_box(struct wlr_output *out, int x, int y, int width, int height,
		     struct wlr_box *box)
{
	int ow, oh;
	wlr_output_transformed_resolution(out, &ow, &oh);
	if (x < 0 || y < 0 || width <= 0 || height <= 0 || (int64_t)x + width > ow ||
	    (int64_t)y + height > oh)
		return false;
	*box = (struct wlr_box){x, y, width, height};
	wlr_box_transform(box, box, wlr_output_transform_invert(out->transform), ow, oh);
	return true;
}
*/
import "C"
