package testclient

/*
#cgo pkg-config: libpipewire-0.3
#include <errno.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <stdio.h>
#include <pipewire/pipewire.h>
#include <spa/param/video/format-utils.h>
#include <spa/buffer/meta.h>

struct sysc_pw_receiver {
 struct pw_thread_loop *loop;
 struct pw_context *context;
 struct pw_core *core;
 struct pw_registry *registry;
 struct pw_stream *stream;
 struct pw_proxy *link;
 struct spa_hook core_hook, registry_hook, stream_hook;
 uint32_t target, input_port, output_port, width, height, refresh;
 uint8_t *pixels, *pointer;
 size_t size, pointer_size;
 uint32_t pointer_width, pointer_height;
 int x, y, hotspot_x, hotspot_y;
 uint64_t generation, sequence;
 int64_t pts;
 bool started, core_listening, registry_listening, stream_listening;
 char error[256];
};
static void sysc_pw_receiver_fail(struct sysc_pw_receiver *r, const char *error) {
 snprintf(r->error, sizeof(r->error), "%s", error == NULL ? "PipeWire consumer failed" : error);
 pw_thread_loop_signal(r->loop, false);
}
static void sysc_pw_receiver_link(struct sysc_pw_receiver *r) {
 uint32_t node = pw_stream_get_node_id(r->stream);
 if (r->link != NULL || node == PW_ID_ANY || r->input_port == PW_ID_ANY || r->output_port == PW_ID_ANY) return;
 char output_node[32], input_node[32], output_port[32], input_port[32];
 snprintf(output_node, sizeof(output_node), "%u", r->target);
 snprintf(input_node, sizeof(input_node), "%u", node);
 snprintf(output_port, sizeof(output_port), "%u", r->output_port);
 snprintf(input_port, sizeof(input_port), "%u", r->input_port);
 struct pw_properties *props = pw_properties_new(
  PW_KEY_LINK_OUTPUT_NODE, output_node, PW_KEY_LINK_OUTPUT_PORT, output_port,
  PW_KEY_LINK_INPUT_NODE, input_node, PW_KEY_LINK_INPUT_PORT, input_port, NULL);
 if (props == NULL) { sysc_pw_receiver_fail(r,"link properties allocation failed"); return; }
 r->link = pw_core_create_object(r->core, "link-factory", PW_TYPE_INTERFACE_Link,
  PW_VERSION_LINK, &props->dict, 0);
 pw_properties_free(props);
 if (r->link == NULL) sysc_pw_receiver_fail(r,"link creation failed");
}
static void sysc_pw_receiver_global(void *data, uint32_t id, uint32_t permissions,
 const char *type, uint32_t version, const struct spa_dict *props) {
 struct sysc_pw_receiver *r = data;
 if (strcmp(type, PW_TYPE_INTERFACE_Port) != 0 || props == NULL) return;
 const char *node_text = spa_dict_lookup(props, PW_KEY_NODE_ID);
 const char *direction = spa_dict_lookup(props, PW_KEY_PORT_DIRECTION);
 if (node_text == NULL || direction == NULL) return;
 uint32_t node = strtoul(node_text, NULL, 10);
 if (node == r->target && strcmp(direction, "out") == 0) r->output_port = id;
 if (r->stream != NULL && node == pw_stream_get_node_id(r->stream) && strcmp(direction, "in") == 0) r->input_port = id;
 if (r->stream != NULL) sysc_pw_receiver_link(r);
}
static const struct pw_registry_events sysc_pw_receiver_registry_events = {
 PW_VERSION_REGISTRY_EVENTS, .global = sysc_pw_receiver_global,
};
static void sysc_pw_receiver_core_error(void *data, uint32_t id, int seq, int res, const char *message) {
 sysc_pw_receiver_fail(data,message);
}
static const struct pw_core_events sysc_pw_receiver_core_events = {
 PW_VERSION_CORE_EVENTS, .error = sysc_pw_receiver_core_error,
};
static void sysc_pw_receiver_state(void *data, enum pw_stream_state old,
 enum pw_stream_state state, const char *error) {
 struct sysc_pw_receiver *r = data;
 if (state == PW_STREAM_STATE_ERROR) sysc_pw_receiver_fail(r,error);
 else if (state >= PW_STREAM_STATE_PAUSED) sysc_pw_receiver_link(r);
}
static void sysc_pw_receiver_format(void *data, uint32_t id, const struct spa_pod *param) {
 struct sysc_pw_receiver *r = data;
 if (id != SPA_PARAM_Format || param == NULL) return;
 struct spa_video_info_raw info = {0};
 if (spa_format_video_raw_parse(param,&info) < 0 || info.format != SPA_VIDEO_FORMAT_RGBA ||
  info.size.width == 0 || info.size.height == 0 ||
  (uint64_t)info.size.width * info.size.height * 4 > UINT64_C(64)*1024*1024) {
  sysc_pw_receiver_fail(r,"invalid negotiated video format"); return;
 }
 r->width=info.size.width; r->height=info.size.height;
 r->refresh=info.framerate.denom==0?0:(uint64_t)info.framerate.num*1000/info.framerate.denom;
 uint8_t storage[512];
 struct spa_pod_builder builder=SPA_POD_BUILDER_INIT(storage,sizeof(storage));
 const struct spa_pod *params[2];
 params[0]=spa_pod_builder_add_object(&builder, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
  SPA_PARAM_META_type,SPA_POD_Id(SPA_META_Header),SPA_PARAM_META_size,SPA_POD_Int(sizeof(struct spa_meta_header)));
 params[1]=spa_pod_builder_add_object(&builder, SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
  SPA_PARAM_META_type,SPA_POD_Id(SPA_META_Cursor),SPA_PARAM_META_size,
  SPA_POD_Int(sizeof(struct spa_meta_cursor)+sizeof(struct spa_meta_bitmap)+384*384*4));
 pw_stream_update_params(r->stream,params,2);
}
static void sysc_pw_receiver_process(void *data) {
 struct sysc_pw_receiver *r=data;
 struct pw_buffer *pwbuffer=pw_stream_dequeue_buffer(r->stream);
 if(pwbuffer==NULL)return;
 struct spa_buffer *buffer=pwbuffer->buffer;
 if(buffer->n_datas==1 && r->width>0 && r->height>0) {
  struct spa_data *plane=&buffer->datas[0];
  struct spa_chunk *chunk=plane->chunk;
  size_t row=(size_t)r->width*4, size=row*r->height;
  if(plane->data!=NULL && chunk!=NULL && !(chunk->flags & SPA_CHUNK_FLAG_CORRUPTED) &&
   chunk->stride>0 && (size_t)chunk->stride>=row &&
   (uint64_t)chunk->size>=(uint64_t)(r->height-1)*chunk->stride+row &&
   (uint64_t)chunk->offset+(uint64_t)(r->height-1)*chunk->stride+row<=plane->maxsize && size<=64*1024*1024) {
   if(size!=r->size) {
    uint8_t *pixels=realloc(r->pixels,size);
    if(pixels==NULL) {sysc_pw_receiver_fail(r,"frame allocation failed");goto queued;}
    r->pixels=pixels; r->size=size;
   }
   for(uint32_t y=0;y<r->height;y++)memcpy(r->pixels+y*row,(uint8_t *)plane->data+chunk->offset+y*chunk->stride,row);
   struct spa_meta_header *header=spa_buffer_find_meta_data(buffer,SPA_META_Header,sizeof(*header));
   r->pts=header==NULL?0:header->pts; r->sequence=header==NULL?0:header->seq;
   r->pointer_size=0;
   struct spa_meta *meta=spa_buffer_find_meta(buffer,SPA_META_Cursor);
   if(meta!=NULL && meta->size>=sizeof(struct spa_meta_cursor)) {
    struct spa_meta_cursor *cursor=meta->data;
    r->x=cursor->position.x; r->y=cursor->position.y;
    r->hotspot_x=cursor->hotspot.x; r->hotspot_y=cursor->hotspot.y;
    if(cursor->id!=0 && cursor->bitmap_offset>=sizeof(*cursor) &&
     (uint64_t)cursor->bitmap_offset+sizeof(struct spa_meta_bitmap)<=meta->size) {
     struct spa_meta_bitmap *bitmap=SPA_PTROFF(cursor,cursor->bitmap_offset,struct spa_meta_bitmap);
     uint64_t bytes=(uint64_t)bitmap->size.width*bitmap->size.height*4;
     if(bitmap->format==SPA_VIDEO_FORMAT_RGBA && bitmap->size.width>0 && bitmap->size.height>0 &&
      bitmap->size.width<=384 && bitmap->size.height<=384 && bitmap->stride>0 &&
      (uint32_t)bitmap->stride>=bitmap->size.width*4 && bitmap->offset>=sizeof(*bitmap) &&
      (uint64_t)cursor->bitmap_offset+bitmap->offset+(uint64_t)(bitmap->size.height-1)*bitmap->stride+bitmap->size.width*4<=meta->size) {
      uint8_t *pixels=realloc(r->pointer,bytes);
      if(pixels!=NULL) {
       r->pointer=pixels; r->pointer_size=bytes; r->pointer_width=bitmap->size.width; r->pointer_height=bitmap->size.height;
       for(uint32_t y=0;y<bitmap->size.height;y++)memcpy(pixels+y*bitmap->size.width*4,SPA_PTROFF(bitmap,bitmap->offset+y*bitmap->stride,uint8_t),bitmap->size.width*4);
      }
     }
    }
   }
   r->generation++;
   pw_thread_loop_signal(r->loop,false);
  }
 }
queued:
 pw_stream_queue_buffer(r->stream,pwbuffer);
}
static const struct pw_stream_events sysc_pw_receiver_stream_events={
 PW_VERSION_STREAM_EVENTS,.state_changed=sysc_pw_receiver_state,
 .param_changed=sysc_pw_receiver_format,.process=sysc_pw_receiver_process,
};
static void sysc_pw_receiver_free(struct sysc_pw_receiver *r) {
 if(r==NULL)return;
 if(r->started)pw_thread_loop_stop(r->loop);
 if(r->stream_listening)spa_hook_remove(&r->stream_hook);
 if(r->registry_listening)spa_hook_remove(&r->registry_hook);
 if(r->core_listening)spa_hook_remove(&r->core_hook);
 if(r->link!=NULL)pw_proxy_destroy(r->link);
 if(r->stream!=NULL)pw_stream_destroy(r->stream);
 if(r->registry!=NULL)pw_proxy_destroy((struct pw_proxy *)r->registry);
 if(r->core!=NULL)pw_core_disconnect(r->core);
 if(r->context!=NULL)pw_context_destroy(r->context);
 if(r->loop!=NULL)pw_thread_loop_destroy(r->loop);
 free(r->pixels);free(r->pointer);free(r);
}
static struct sysc_pw_receiver *sysc_pw_receiver_new(uint32_t target,uint32_t width,uint32_t height) {
 struct sysc_pw_receiver *r=calloc(1,sizeof(*r));
 if(r==NULL)return NULL;
 r->target=target;r->input_port=r->output_port=PW_ID_ANY;
 r->loop=pw_thread_loop_new("sysc test receiver",NULL);
 if(r->loop==NULL)goto failed;
 r->context=pw_context_new(pw_thread_loop_get_loop(r->loop),NULL,0);
 if(r->context==NULL)goto failed;
 r->core=pw_context_connect(r->context,NULL,0);
 if(r->core==NULL)goto failed;
 pw_core_add_listener(r->core,&r->core_hook,&sysc_pw_receiver_core_events,r);r->core_listening=true;
 r->registry=pw_core_get_registry(r->core,PW_VERSION_REGISTRY,0);
 if(r->registry==NULL)goto failed;
 pw_registry_add_listener(r->registry,&r->registry_hook,&sysc_pw_receiver_registry_events,r);r->registry_listening=true;
 r->stream=pw_stream_new(r->core,"sysc test receiver",pw_properties_new(PW_KEY_MEDIA_TYPE,"Video",PW_KEY_MEDIA_CATEGORY,"Capture",NULL));
 if(r->stream==NULL)goto failed;
 pw_stream_add_listener(r->stream,&r->stream_hook,&sysc_pw_receiver_stream_events,r);r->stream_listening=true;
 uint8_t storage[512];struct spa_pod_builder builder=SPA_POD_BUILDER_INIT(storage,sizeof(storage));
 struct spa_rectangle size={width,height},minimum={1,1},maximum={8192,8192};
 const struct spa_pod *format=spa_pod_builder_add_object(&builder,SPA_TYPE_OBJECT_Format,SPA_PARAM_EnumFormat,
  SPA_FORMAT_mediaType,SPA_POD_Id(SPA_MEDIA_TYPE_video),SPA_FORMAT_mediaSubtype,SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
  SPA_FORMAT_VIDEO_format,SPA_POD_Id(SPA_VIDEO_FORMAT_RGBA),
  SPA_FORMAT_VIDEO_size,SPA_POD_CHOICE_RANGE_Rectangle(&size,&minimum,&maximum));
 if(pw_stream_connect(r->stream,PW_DIRECTION_INPUT,PW_ID_ANY,PW_STREAM_FLAG_MAP_BUFFERS,&format,1)<0)goto failed;
 if(pw_thread_loop_start(r->loop)<0)goto failed;
 r->started=true;
 return r;
failed:
 sysc_pw_receiver_free(r);return NULL;
}
static int sysc_pw_receiver_wait(struct sysc_pw_receiver *r,uint64_t generation,int64_t timeout) {
 struct timespec deadline;
 pw_thread_loop_get_time(r->loop,&deadline,timeout);
 while(r->generation<=generation && r->error[0]=='\0') {
  int result=pw_thread_loop_timed_wait_full(r->loop,&deadline);
  if(result<0)return result;
 }
 return r->error[0]=='\0'?0:-EPIPE;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type PipeWireFrame struct {
	Width, Height  int
	RefreshMilliHz uint32
	Pixels         []byte
	PTS            int64
	Sequence       uint64
	Pointer        *PipeWirePointer
}
type PipeWirePointer struct {
	Width, Height     int
	Pixels            []byte
	Position, Hotspot image.Point
}
type PipeWireReceiver struct {
	mu         sync.Mutex
	native     *C.struct_sysc_pw_receiver
	generation uint64
}

var receiverInit sync.Once

func NewPipeWireReceiver(node uint32, width, height int) (*PipeWireReceiver, error) {
	if width <= 0 || height <= 0 || width > 8192 || height > 8192 || int64(width)*int64(height)*4 > 64<<20 {
		return nil, errors.New("invalid receiver size")
	}
	receiverInit.Do(func() { C.pw_init(nil, nil) })
	r := C.sysc_pw_receiver_new(C.uint32_t(node), C.uint32_t(width), C.uint32_t(height))
	if r == nil {
		return nil, errors.New("creating PipeWire receiver failed")
	}
	return &PipeWireReceiver{native: r}, nil
}
func (r *PipeWireReceiver) NextFrame(timeout time.Duration) (PipeWireFrame, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.native == nil {
		return PipeWireFrame{}, errors.New("receiver closed")
	}
	n := r.native
	// PipeWire's recursive mutex belongs to an OS thread, including across
	// the wait below. Go may otherwise migrate before the deferred unlock.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	C.pw_thread_loop_lock(n.loop)
	defer C.pw_thread_loop_unlock(n.loop)
	if result := C.sysc_pw_receiver_wait(n, C.uint64_t(r.generation), C.int64_t(timeout)); result < 0 {
		return PipeWireFrame{}, fmt.Errorf("receiving PipeWire frame: %d: %s", result, C.GoString(&n.error[0]))
	}
	r.generation = uint64(n.generation)
	frame := PipeWireFrame{Width: int(n.width), Height: int(n.height), RefreshMilliHz: uint32(n.refresh), Pixels: C.GoBytes(unsafe.Pointer(n.pixels), C.int(n.size)), PTS: int64(n.pts), Sequence: uint64(n.sequence)}
	if n.pointer_size > 0 {
		frame.Pointer = &PipeWirePointer{Width: int(n.pointer_width), Height: int(n.pointer_height), Pixels: C.GoBytes(unsafe.Pointer(n.pointer), C.int(n.pointer_size)), Position: image.Pt(int(n.x), int(n.y)), Hotspot: image.Pt(int(n.hotspot_x), int(n.hotspot_y))}
	}
	return frame, nil
}
func (r *PipeWireReceiver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.native != nil {
		C.sysc_pw_receiver_free(r.native)
		r.native = nil
	}
}
