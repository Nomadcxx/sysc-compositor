package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 glesv2
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <math.h>
#include <drm_fourcc.h>
#include <GLES2/gl2.h>
#include <wlr/render/allocator.h>
#include <wlr/render/drm_format_set.h>
#include <wlr/render/pass.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/render/wlr_texture.h>
#include <wlr/types/wlr_buffer.h>
#include <scenefx/render/fx_renderer/fx_renderer.h>
#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/util/transform.h>


// SceneFX 0.5's installed render/pass.h references uninstalled private headers.
// These declarations are its public add-texture/rect/shadow ABI; no private
// render-pass fields are accessed.
struct fx_gles_render_pass;
struct sysc_float_radii { float top_left, top_right, bottom_right, bottom_left; };
struct sysc_float_clip { struct wlr_fbox area; struct sysc_float_radii corners; };
struct sysc_fx_texture_options {
 struct wlr_render_texture_options base;
 const struct wlr_box *clip_box;
 struct sysc_float_radii corners;
 bool discard_transparent;
 struct sysc_float_clip clipped_region;
};
struct sysc_fx_rect_options {
 struct wlr_render_rect_options base;
 struct sysc_float_radii corners;
 struct sysc_float_clip clipped_region;
};
struct sysc_fx_shadow_options {
 struct wlr_box box;
 struct sysc_float_clip clipped_region;
 const pixman_region32_t *clip;
 float blur_sigma;
 int corner_radius;
 struct wlr_render_color color;
};
struct fx_gles_render_pass *fx_get_render_pass(struct wlr_render_pass *);
void fx_render_pass_add_texture(struct fx_gles_render_pass *, const struct sysc_fx_texture_options *);
void fx_render_pass_add_rounded_rect(struct fx_gles_render_pass *, const struct sysc_fx_rect_options *);
void fx_render_pass_add_box_shadow(struct fx_gles_render_pass *, const struct sysc_fx_shadow_options *);
static struct sysc_float_radii sysc_animation_radii(struct fx_corner_radii radii,double scale) {
 return (struct sysc_float_radii){radii.top_left*scale,radii.top_right*scale,radii.bottom_right*scale,radii.bottom_left*scale};
}
static struct sysc_float_clip sysc_animation_clip(struct clipped_region clip,int x,int y,double scale) {
 return (struct sysc_float_clip){
  .area={.x=(x+clip.area.x)*scale,.y=(y+clip.area.y)*scale,.width=clip.area.width*scale,.height=clip.area.height*scale},
  .corners=sysc_animation_radii(clip.corners,scale),
 };
}
static void sysc_animation_flatten_node(struct wlr_scene_node *node,struct wlr_render_pass *pass,
 struct wlr_renderer *renderer,int x,int y,double scale,bool root) {
 if(!root && !node->enabled)return;
 x+=node->x;y+=node->y;
 struct fx_gles_render_pass *fx=fx_get_render_pass(pass);
 switch(node->type) {
 case WLR_SCENE_NODE_TREE: {
  struct wlr_scene_tree *tree=wlr_scene_tree_from_node(node);
  struct wlr_scene_node *child;
  wl_list_for_each(child,&tree->children,link)sysc_animation_flatten_node(child,pass,renderer,x,y,scale,false);
  break;
 }
 case WLR_SCENE_NODE_BUFFER: {
  struct wlr_scene_buffer *scene=wlr_scene_buffer_from_node(node);
  struct wlr_texture *texture=NULL;
  bool imported=false;
  if(scene->buffer!=NULL) {
   struct wlr_client_buffer *client=wlr_client_buffer_get(scene->buffer);
   texture=client ? client->texture:wlr_texture_from_buffer(renderer,scene->buffer);
   imported=client==NULL;
  }
  if(texture==NULL)texture=scene->WLR_PRIVATE.texture;
  if(texture==NULL)break;
  int w=scene->dst_width,h=scene->dst_height;
  if(w==0||h==0) {w=texture->width;h=texture->height;if(scene->transform&1){int swap=w;w=h;h=swap;}}
  struct wlr_box dst={.x=lround(x*scale),.y=lround(y*scale),.width=lround(w*scale),.height=lround(h*scale)};
  struct fx_corner_radii radii=scene->corners;
  enum wl_output_transform transform=wlr_output_transform_invert(scene->transform);
  fx_corner_radii_transform(transform,&radii);
  struct sysc_fx_texture_options options={
   .base={.texture=texture,.src_box=scene->src_box,.dst_box=dst,.transform=transform,
    .alpha=&scene->opacity,.filter_mode=scene->filter_mode,.blend_mode=WLR_RENDER_BLEND_MODE_PREMULTIPLIED,
    .wait_timeline=scene->WLR_PRIVATE.wait_timeline,.wait_point=scene->WLR_PRIVATE.wait_point},
   .clip_box=&dst,.corners=sysc_animation_radii(radii,scale),
  };
  fx_render_pass_add_texture(fx,&options);
  if(imported)wlr_texture_destroy(texture);
  break;
 }
 case WLR_SCENE_NODE_RECT: {
  struct wlr_scene_rect *rect=wlr_scene_rect_from_node(node);
  struct sysc_fx_rect_options options={
   .base={.box={lround(x*scale),lround(y*scale),lround(rect->width*scale),lround(rect->height*scale)},
    .color={rect->color[0],rect->color[1],rect->color[2],rect->color[3]},.blend_mode=WLR_RENDER_BLEND_MODE_PREMULTIPLIED},
   .corners=sysc_animation_radii(rect->corners,scale),.clipped_region=sysc_animation_clip(rect->clipped_region,x,y,scale),
  };
  fx_render_pass_add_rounded_rect(fx,&options);
  break;
 }
 case WLR_SCENE_NODE_SHADOW: {
  struct wlr_scene_shadow *shadow=wlr_scene_shadow_from_node(node);
  struct sysc_fx_shadow_options options={
   .box={lround(x*scale),lround(y*scale),lround(shadow->width*scale),lround(shadow->height*scale)},
   .clipped_region=sysc_animation_clip(shadow->clipped_region,x,y,scale),
   .blur_sigma=shadow->blur_sigma*scale,.corner_radius=lround(shadow->corner_radius*scale),
   .color={shadow->color[0],shadow->color[1],shadow->color[2],shadow->color[3]},
  };
  fx_render_pass_add_box_shadow(fx,&options);
  break;
 }
 default:break;
 }
}
static bool sysc_animation_flatten(struct wlr_renderer *renderer,struct wlr_buffer *dst,
 struct wlr_scene_node *node,int x,int y,double scale,bool local) {
 struct wlr_render_pass *pass=wlr_renderer_begin_buffer_pass(renderer,dst,NULL);
 if(pass==NULL)return false;
 wlr_render_pass_add_rect(pass,&(struct wlr_render_rect_options){.box={0,0,dst->width,dst->height},.blend_mode=WLR_RENDER_BLEND_MODE_NONE});
 if(local){x-=node->x;y-=node->y;}
 sysc_animation_flatten_node(node,pass,renderer,x,y,scale,true);
 return wlr_render_pass_submit(pass);
}

static void sysc_animation_bounds(struct wlr_scene_node *node,int x,int y,struct wlr_box *box,bool *found,bool root) {
 if(!root&&!node->enabled)return;
 x+=node->x;y+=node->y;
 if(node->type==WLR_SCENE_NODE_TREE) {
  struct wlr_scene_tree *tree=wlr_scene_tree_from_node(node);struct wlr_scene_node *child;
  wl_list_for_each(child,&tree->children,link)sysc_animation_bounds(child,x,y,box,found,false);
  return;
 }
 int w=0,h=0;
 if(node->type==WLR_SCENE_NODE_BUFFER) {
  struct wlr_scene_buffer *buffer=wlr_scene_buffer_from_node(node);w=buffer->dst_width;h=buffer->dst_height;
  if(w==0||h==0) {w=buffer->WLR_PRIVATE.buffer_width;h=buffer->WLR_PRIVATE.buffer_height;if(buffer->transform&1){int swap=w;w=h;h=swap;}}
 } else if(node->type==WLR_SCENE_NODE_RECT) {struct wlr_scene_rect *rect=wlr_scene_rect_from_node(node);w=rect->width;h=rect->height;}
 else if(node->type==WLR_SCENE_NODE_SHADOW) {struct wlr_scene_shadow *shadow=wlr_scene_shadow_from_node(node);w=shadow->width;h=shadow->height;}
 if(w<=0||h<=0)return;
 if(!*found) {*box=(struct wlr_box){x,y,w,h};*found=true;return;}
 int x1=box->x+box->width,y1=box->y+box->height;
 box->x=box->x<x?box->x:x;box->y=box->y<y?box->y:y;
 box->width=(x1>x+w?x1:x+w)-box->x;box->height=(y1>y+h?y1:y+h)-box->y;
}
static struct wlr_box sysc_animation_roots_bounds(struct wlr_scene_node **nodes,size_t count) {
 struct wlr_box box={0};bool found=false;
 for(size_t i=0;i<count;i++) {
  int x=0,y=0;
  for(struct wlr_scene_node *parent=(struct wlr_scene_node *)nodes[i]->parent;parent!=NULL;parent=(struct wlr_scene_node *)parent->parent){x+=parent->x;y+=parent->y;}
  sysc_animation_bounds(nodes[i],x,y,&box,&found,true);
 }
 return box;
}
static bool sysc_animation_flatten_roots(struct wlr_renderer *renderer,struct wlr_buffer *dst,
 struct wlr_scene_node **nodes,size_t count,int ox,int oy,double scale) {
 struct wlr_render_pass *pass=wlr_renderer_begin_buffer_pass(renderer,dst,NULL);
 if(pass==NULL)return false;
 wlr_render_pass_add_rect(pass,&(struct wlr_render_rect_options){.box={0,0,dst->width,dst->height},.blend_mode=WLR_RENDER_BLEND_MODE_NONE});
 for(size_t i=0;i<count;i++) {
  int x=-ox,y=-oy;
  for(struct wlr_scene_node *parent=(struct wlr_scene_node *)nodes[i]->parent;parent!=NULL;parent=(struct wlr_scene_node *)parent->parent){x+=parent->x;y+=parent->y;}
  sysc_animation_flatten_node(nodes[i],pass,renderer,x,y,scale,true);
 }
 return wlr_render_pass_submit(pass);
}

static void sysc_animation_set_buffer(struct wlr_scene_buffer *node,struct wlr_buffer *buffer,int x,int y,int width,int height) {
 wlr_scene_buffer_set_buffer(node,buffer);
 wlr_scene_buffer_set_dest_size(node,width,height);
 wlr_scene_node_set_position(&node->node,x,y);
}

static struct wlr_buffer *sysc_animation_buffer(struct wlr_renderer *renderer,
        struct wlr_allocator *allocator, int width, int height) {
 const struct wlr_drm_format_set *formats = renderer->WLR_PRIVATE.impl->get_render_formats(renderer);
 const struct wlr_drm_format *format = wlr_drm_format_set_get(formats,DRM_FORMAT_ARGB8888);
 if (format == NULL) return NULL;
 return wlr_allocator_create_buffer(allocator,width,height,format);
}

static GLuint sysc_animation_compile_stage(GLenum type, const char *source, char *error, size_t error_size) {
 GLuint shader=glCreateShader(type);
 glShaderSource(shader,1,&source,NULL);glCompileShader(shader);
 GLint ok=0;glGetShaderiv(shader,GL_COMPILE_STATUS,&ok);
 if (!ok) { glGetShaderInfoLog(shader,error_size,NULL,error);glDeleteShader(shader);return 0; }
 return shader;
}

// A SceneFX pass owns the EGL context until submit. All GL calls stay inside
// that scope, including compilation/deletion outside a physical output frame.
static GLuint sysc_animation_compile(struct wlr_renderer *renderer,
        struct wlr_buffer *scratch, const char *vertex, const char *fragment, char *error, size_t error_size) {
 struct wlr_render_pass *pass=wlr_renderer_begin_buffer_pass(renderer,scratch,NULL);
 if (pass==NULL) { snprintf(error,error_size,"could not begin compiler render pass");return 0; }
 GLuint vs=sysc_animation_compile_stage(GL_VERTEX_SHADER,vertex,error,error_size);
 GLuint fs=vs ? sysc_animation_compile_stage(GL_FRAGMENT_SHADER,fragment,error,error_size) : 0;
 GLuint program=0;
 if (vs && fs) {
  program=glCreateProgram();glAttachShader(program,vs);glAttachShader(program,fs);glLinkProgram(program);
  GLint ok=0;glGetProgramiv(program,GL_LINK_STATUS,&ok);
  if (!ok) { glGetProgramInfoLog(program,error_size,NULL,error);glDeleteProgram(program);program=0; }
 }
 if (vs) glDeleteShader(vs);
 if (fs) glDeleteShader(fs);
 wlr_render_pass_submit(pass);
 return program;
}

static void sysc_animation_delete_program(struct wlr_renderer *renderer, struct wlr_buffer *scratch, GLuint program) {
 struct wlr_render_pass *pass=wlr_renderer_begin_buffer_pass(renderer,scratch,NULL);
 if (pass==NULL) return;
 glDeleteProgram(program);
 wlr_render_pass_submit(pass);
}

struct sysc_animation_uniforms {
 float size[2], geo_size[2], progress, clamped_progress, random_seed, alpha, scale;
 float input_to_geo[9], geo_to_tex[9];
 float curr_to_prev[9],curr_to_next[9],geo_to_prev[9],geo_to_next[9];
 float corner_radius[4], clip_to_geometry;
};
static void sysc_animation_mat(GLuint program,const char *name,const float *matrix) {
 glUniformMatrix3fv(glGetUniformLocation(program,name),1,GL_FALSE,matrix);
}
static void sysc_animation_float(GLuint program,const char *name,float value) {
 glUniform1f(glGetUniformLocation(program,name),value);
}
static void sysc_animation_vec2(GLuint program,const char *name,const float *value) {
 glUniform2fv(glGetUniformLocation(program,name),1,value);
}
// A texture GL has never filtered keeps the mipmapped default minification
// filter and is incomplete without mipmaps, so every sample reads black.
static void sysc_animation_sampling(void) {
 glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_MIN_FILTER,GL_LINEAR);
 glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_MAG_FILTER,GL_LINEAR);
 glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_WRAP_S,GL_CLAMP_TO_EDGE);
 glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_WRAP_T,GL_CLAMP_TO_EDGE);
}
static bool sysc_animation_render(struct wlr_renderer *renderer, GLuint program,
        struct wlr_buffer *dst,struct wlr_buffer *previous,struct wlr_buffer *next,
        const struct sysc_animation_uniforms *u) {
 struct wlr_texture *prev=wlr_texture_from_buffer(renderer,previous);
 struct wlr_texture *after=next ? wlr_texture_from_buffer(renderer,next) : NULL;
 if (prev==NULL || (next && after==NULL)) { if(prev)wlr_texture_destroy(prev);if(after)wlr_texture_destroy(after);return false; }
 struct fx_texture_attribs prev_attrs,next_attrs;
 fx_texture_get_attribs(prev,&prev_attrs);
 if(after)fx_texture_get_attribs(after,&next_attrs);
 // Flattening below produces ordinary 2D textures even for external client imports.
 if(prev_attrs.target!=GL_TEXTURE_2D || (after && next_attrs.target!=GL_TEXTURE_2D)) { wlr_texture_destroy(prev);if(after)wlr_texture_destroy(after);return false; }
 struct wlr_render_pass *pass=wlr_renderer_begin_buffer_pass(renderer,dst,NULL);
 if(pass==NULL) { wlr_texture_destroy(prev);if(after)wlr_texture_destroy(after);return false; }
 glDisable(GL_BLEND);glDisable(GL_SCISSOR_TEST);glClearColor(0,0,0,0);glClear(GL_COLOR_BUFFER_BIT);
 glUseProgram(program);
 glActiveTexture(GL_TEXTURE0);glBindTexture(GL_TEXTURE_2D,prev_attrs.tex);sysc_animation_sampling();
 glUniform1i(glGetUniformLocation(program,"niri_tex"),0);
 glUniform1i(glGetUniformLocation(program,"niri_tex_prev"),0);
 glActiveTexture(GL_TEXTURE1);glBindTexture(GL_TEXTURE_2D,after ? next_attrs.tex : prev_attrs.tex);sysc_animation_sampling();
 glUniform1i(glGetUniformLocation(program,"niri_tex_next"),1);
 sysc_animation_vec2(program,"niri_size",u->size);
 sysc_animation_vec2(program,"niri_geo_size",u->geo_size);
 sysc_animation_vec2(program,"niri_curr_geo_size",u->geo_size);
 sysc_animation_mat(program,"niri_input_to_geo",u->input_to_geo);
 sysc_animation_mat(program,"niri_input_to_curr_geo",u->input_to_geo);
 sysc_animation_mat(program,"niri_geo_to_tex",u->geo_to_tex);
 sysc_animation_mat(program,"niri_curr_geo_to_prev_geo",u->curr_to_prev);
 sysc_animation_mat(program,"niri_curr_geo_to_next_geo",u->curr_to_next);
 sysc_animation_mat(program,"niri_geo_to_tex_prev",u->geo_to_prev);
 sysc_animation_mat(program,"niri_geo_to_tex_next",u->geo_to_next);
 sysc_animation_float(program,"niri_progress",u->progress);
 sysc_animation_float(program,"niri_clamped_progress",u->clamped_progress);
 sysc_animation_float(program,"niri_random_seed",u->random_seed);
 sysc_animation_float(program,"niri_alpha",u->alpha);
 sysc_animation_float(program,"niri_scale",u->scale);
 sysc_animation_float(program,"niri_clip_to_geometry",u->clip_to_geometry);
 glUniform4fv(glGetUniformLocation(program,"niri_corner_radius"),1,u->corner_radius);
 const GLfloat positions[]={-1,-1,1,-1,-1,1,1,1};
 GLint attr=glGetAttribLocation(program,"position");
 glBindBuffer(GL_ARRAY_BUFFER,0);glEnableVertexAttribArray(attr);
 glVertexAttribPointer(attr,2,GL_FLOAT,GL_FALSE,0,positions);glDrawArrays(GL_TRIANGLE_STRIP,0,4);
 glDisableVertexAttribArray(attr);glUseProgram(0);
 glActiveTexture(GL_TEXTURE1);glBindTexture(GL_TEXTURE_2D,0);
 glActiveTexture(GL_TEXTURE0);glBindTexture(GL_TEXTURE_2D,0);
 bool ok=glGetError()==GL_NO_ERROR;
 bool submitted=wlr_render_pass_submit(pass);
 wlr_texture_destroy(prev);if(after)wlr_texture_destroy(after);
 return ok && submitted;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// These uniforms and entry points are Niri 26.04's custom-animation shader ABI.
// Its prelude/user-code/epilogue composition lets existing user shaders run as-is.
const animationShaderVertex = `#version 100
attribute vec2 position;
varying vec2 niri_v_coords;
void main() { niri_v_coords=position*.5+.5; gl_Position=vec4(position,0.0,1.0); }
`
const animationShaderCommon = `#version 100
precision highp float;
varying vec2 niri_v_coords;
uniform vec2 niri_size;
uniform float niri_progress, niri_clamped_progress, niri_random_seed, niri_alpha, niri_scale;
`
const animationShaderSingle = `uniform mat3 niri_input_to_geo, niri_geo_to_tex;
uniform vec2 niri_geo_size;
uniform sampler2D niri_tex;
`
const animationShaderResize = `uniform mat3 niri_input_to_curr_geo, niri_curr_geo_to_prev_geo, niri_curr_geo_to_next_geo;
uniform mat3 niri_geo_to_tex_prev, niri_geo_to_tex_next;
uniform vec2 niri_curr_geo_size;
uniform sampler2D niri_tex_prev, niri_tex_next;
uniform vec4 niri_corner_radius;
uniform float niri_clip_to_geometry;
float niri_rounding_alpha(vec2 coords, vec2 size, vec4 corner_radius);
`

// Niri uses the same signed-distance corner antialiasing for resize shaders.
const animationShaderRounding = `
float niri_rounding_alpha(vec2 coords, vec2 size, vec4 radius) {
 vec2 half_size=size*.5;
 vec2 centered=coords-half_size;
 float r=centered.x<0.0 ? (centered.y<0.0 ? radius.x:radius.w) : (centered.y<0.0 ? radius.y:radius.z);
 vec2 q=abs(centered)-half_size+vec2(r);
 float distance=length(max(q,vec2(0.0)))+min(max(q.x,q.y),0.0)-r;
 return 1.0-smoothstep(-.5/niri_scale,.5/niri_scale,distance);
}
`

type animationShaderProgram struct {
	id     C.GLuint
	source string
}
type animationShaderEffect struct {
	program        *animationShaderProgram
	kind           string
	view           *toplevelView
	source         *C.struct_wlr_buffer
	next           *C.struct_wlr_buffer
	node           *C.struct_wlr_scene_buffer
	identity       *captureIdentity
	area           image.Rectangle
	geometry       image.Rectangle
	sourceGeometry image.Rectangle
	nextGeometry   image.Rectangle
	lastProgress   float64
	scale          float64
	random         float32
}

type animationShaderSet struct {
	renderer *C.struct_wlr_renderer
	scratch  *C.struct_wlr_buffer
	programs map[string]*animationShaderProgram
	retired  []*animationShaderProgram
	effects  map[animationKey]*animationShaderEffect
}

// configureAnimationShaders runs with the compositor's SceneFX renderer current
// only inside a native pass. A bad replacement preserves the previous program.
func (c *Compositor) configureAnimationShaders() {
	if c.renderer == nil || c.alloc == nil {
		return
	}
	if c.animationShaders == nil {
		scratch := C.sysc_animation_buffer(c.renderer, c.alloc, 1, 1)
		if scratch == nil {
			fmt.Fprintln(os.Stderr, "sysc-compositor: animation shader scratch allocation failed")
			return
		}
		c.animationShaders = &animationShaderSet{renderer: c.renderer, scratch: scratch, programs: map[string]*animationShaderProgram{}, effects: map[animationKey]*animationShaderEffect{}}
	}
	set := c.animationShaders
	for kind, setting := range map[string]config.Animation{"open": c.animationConfig.WindowOpen, "close": c.animationConfig.WindowClose, "resize": c.animationConfig.WindowResize} {
		previous := set.programs[kind]
		if setting.CustomShader == nil {
			if previous != nil {
				set.retired = append(set.retired, previous)
				delete(set.programs, kind)
			}
			continue
		}
		source := *setting.CustomShader
		if previous != nil && previous.source == source {
			continue
		}
		if strings.ContainsRune(source, 0) {
			fmt.Fprintln(os.Stderr, "sysc-compositor: custom animation shader contains NUL")
			continue
		}
		prelude := animationShaderCommon + animationShaderSingle
		suffix := fmt.Sprintf("\nvoid main() { vec3 c=niri_input_to_geo*vec3(niri_v_coords,1.0); gl_FragColor=%s_color(c,vec3(niri_geo_size,1.0))*niri_alpha; }", kind)
		if kind == "resize" {
			prelude = animationShaderCommon + animationShaderResize
			suffix = `
void main() {
 vec3 c=niri_input_to_curr_geo*vec3(niri_v_coords,1.0);
 vec4 color=resize_color(c,vec3(niri_curr_geo_size,1.0));
 if(niri_clip_to_geometry==1.0) {
  if(c.x<0.0 || c.x>1.0 || c.y<0.0 || c.y>1.0) color=vec4(0.0);
  else color*=niri_rounding_alpha(c.xy*niri_curr_geo_size,niri_curr_geo_size,niri_corner_radius);
 }
 gl_FragColor=color*niri_alpha;
}
` + animationShaderRounding
		}
		vertex, fragment := C.CString(animationShaderVertex), C.CString(prelude+source+suffix)
		var log [4096]C.char
		program := C.sysc_animation_compile(c.renderer, set.scratch, vertex, fragment, &log[0], C.size_t(len(log)))
		C.free(unsafe.Pointer(vertex))
		C.free(unsafe.Pointer(fragment))
		if program == 0 {
			fmt.Fprintf(os.Stderr, "sysc-compositor: custom %s shader: %s\n", kind, C.GoString(&log[0]))
			continue
		}
		set.programs[kind] = &animationShaderProgram{id: program, source: source}
		if previous != nil {
			set.retired = append(set.retired, previous)
		}
	}
	// Preserve the sampled position when a live reload changes slowdown.
	now := time.Now()
	for _, running := range c.animations {
		elapsed := float64(now.Sub(running.started)) / running.rate()
		running.slowdown = c.animationSlowdown()
		running.started = now.Add(-time.Duration(elapsed * running.slowdown))
	}
	c.pruneAnimationShaders()
}

func (c *Compositor) pruneAnimationShaders() {
	set := c.animationShaders
	if set == nil {
		return
	}
	retained := set.retired[:0]
	for _, program := range set.retired {
		used := false
		for _, effect := range set.effects {
			if effect.program == program {
				used = true
				break
			}
		}
		if used {
			retained = append(retained, program)
		} else {
			C.sysc_animation_delete_program(set.renderer, set.scratch, program.id)
		}
	}
	set.retired = retained
}

func (c *Compositor) finishAnimationShaders() {
	set := c.animationShaders
	if set == nil {
		return
	}
	for key := range set.effects {
		c.finishAnimationShaderEffect(key)
	}
	for _, program := range set.programs {
		C.sysc_animation_delete_program(set.renderer, set.scratch, program.id)
	}
	for _, program := range set.retired {
		C.sysc_animation_delete_program(set.renderer, set.scratch, program.id)
	}
	C.wlr_buffer_drop(set.scratch)
	c.animationShaders = nil
}

func animationIdentityMatrix() [9]C.float { return [9]C.float{1, 0, 0, 0, 1, 0, 0, 0, 1} }
func animationShaderUniforms(width, height int, progress, clamped float64) C.struct_sysc_animation_uniforms {
	identity := animationIdentityMatrix()
	return C.struct_sysc_animation_uniforms{size: [2]C.float{C.float(width), C.float(height)}, geo_size: [2]C.float{C.float(width), C.float(height)}, progress: C.float(progress), clamped_progress: C.float(clamped), random_seed: .5, alpha: 1, scale: 1, input_to_geo: identity, geo_to_tex: identity, curr_to_prev: identity, curr_to_next: identity, geo_to_prev: identity, geo_to_next: identity}
}

func (c *Compositor) renderAnimationShader(program *animationShaderProgram, prev, next *C.struct_wlr_buffer, width, height int, uniforms C.struct_sysc_animation_uniforms) (*C.struct_wlr_buffer, error) {
	if program == nil || prev == nil {
		return nil, errors.New("wlr: custom animation shader unavailable")
	}
	if _, err := captureAllocation(width, height, width*4); err != nil {
		return nil, err
	}
	dst := C.sysc_animation_buffer(c.renderer, c.alloc, C.int(width), C.int(height))
	if dst == nil {
		return nil, errors.New("wlr: animation buffer allocation failed")
	}
	if !bool(C.sysc_animation_render(c.renderer, program.id, dst, prev, next, &uniforms)) {
		C.wlr_buffer_drop(dst)
		return nil, errors.New("wlr: custom animation render failed")
	}
	return dst, nil
}

// renderAnimationShaderImage is also the pixel proof seam: the same GLES path
// used for live effects, with CPU-owned inputs to make the expected pixels exact.
func (c *Compositor) renderAnimationShaderImage(kind string, prev, next *image.RGBA, progress, clamped float64) (*image.RGBA, error) {
	if c.animationShaders == nil || c.animationShaders.programs[kind] == nil {
		return nil, errors.New("wlr: custom animation shader unavailable")
	}
	before, after := imageBuffer(prev), imageBuffer(next)
	if before == nil || after == nil {
		if before != nil {
			C.wlr_buffer_drop(before)
		}
		if after != nil {
			C.wlr_buffer_drop(after)
		}
		return nil, errors.New("wlr: input buffer allocation failed")
	}
	defer C.wlr_buffer_drop(before)
	defer C.wlr_buffer_drop(after)
	size := prev.Bounds().Size()
	buffer, err := c.renderAnimationShader(c.animationShaders.programs[kind], before, after, size.X, size.Y, animationShaderUniforms(size.X, size.Y, progress, clamped))
	if err != nil {
		return nil, err
	}
	defer C.wlr_buffer_drop(buffer)
	pixels, err := c.readBuffer(buffer, image.Rect(0, 0, size.X, size.Y))
	if err != nil {
		return nil, err
	}
	result := image.NewRGBA(pixels.Bounds())
	for y := range size.Y {
		for x := range size.X {
			result.Set(x, y, pixels.At(x, y))
		}
	}
	return result, nil
}

func (c *Compositor) openShaderOwnsView(view *toplevelView) bool {
	return c.animationShaders != nil && c.animationShaders.effects[animationKey{owner: view.id, property: "open-opacity"}] != nil
}
func (c *Compositor) animationShaderOwnsView(view *toplevelView) bool {
	if c.animationShaders == nil {
		return false
	}
	return c.openShaderOwnsView(view) || c.animationShaders.effects[animationKey{owner: view.id, property: "resize-shader"}] != nil
}
func (c *Compositor) shaderViewRoots(view *toplevelView, decorations bool) []*C.struct_wlr_scene_node {
	var roots []*C.struct_wlr_scene_node
	if decorations {
		for _, pointer := range []unsafe.Pointer{view.shadowNode, view.borderNode} {
			if pointer != nil && bool((*C.struct_wlr_scene_node)(pointer).enabled) {
				roots = append(roots, (*C.struct_wlr_scene_node)(pointer))
			}
		}
	}
	roots = append(roots, &view.tree.node)
	if decorations && view.focusRingNode != nil && bool((*C.struct_wlr_scene_node)(view.focusRingNode).enabled) {
		roots = append(roots, (*C.struct_wlr_scene_node)(view.focusRingNode))
	}
	return roots
}
func (c *Compositor) flattenAnimationRoots(roots []*C.struct_wlr_scene_node, fallback image.Rectangle, scale float64) (*C.struct_wlr_buffer, image.Rectangle, error) {
	if len(roots) == 0 {
		return nil, image.Rectangle{}, errors.New("wlr: no animation source")
	}
	box := C.sysc_animation_roots_bounds(&roots[0], C.size_t(len(roots)))
	area := image.Rect(int(box.x), int(box.y), int(box.x+box.width), int(box.y+box.height)).Union(fallback)
	width, height := max(1, int(math.Round(float64(area.Dx())*scale))), max(1, int(math.Round(float64(area.Dy())*scale)))
	if _, err := captureAllocation(width, height, width*4); err != nil {
		return nil, area, err
	}
	buffer := C.sysc_animation_buffer(c.renderer, c.alloc, C.int(width), C.int(height))
	if buffer == nil {
		return nil, area, errors.New("wlr: animation source allocation failed")
	}
	if !bool(C.sysc_animation_flatten_roots(c.renderer, buffer, &roots[0], C.size_t(len(roots)), C.int(area.Min.X), C.int(area.Min.Y), C.double(scale))) {
		C.wlr_buffer_drop(buffer)
		return nil, area, errors.New("wlr: animation source rendering failed")
	}
	return buffer, area, nil
}
func (c *Compositor) captureAnimationView(view *toplevelView, decorations bool) (*C.struct_wlr_buffer, image.Rectangle, error) {
	key := animationKey{owner: view.id, property: "open-opacity"}
	var active *animationShaderEffect
	if c.animationShaders != nil {
		active = c.animationShaders.effects[key]
		delete(c.animationShaders.effects, key)
	}
	defer func() {
		if active != nil && c.animationShaders != nil {
			c.animationShaders.effects[key] = active
		}
		c.syncViewOpacity(view)
		c.syncViewDecorations(view)
	}()
	geometry := view.geometry()
	x, y := view.treePosition()
	alpha := 1.0
	if !view.fullscreen && !view.ignoreRuleOpacity && view.appearance.Opacity != nil {
		alpha = clampOpacity(*view.appearance.Opacity)
	}
	sceneFXSetTreeOpacity(view.sceneNode(), alpha)
	if view.toplevel != nil {
		c.syncPopupOpacity(view.toplevel.base.surface, alpha, view.appearance.Popups.Opacity)
	}
	// The source uses the unanimated tile, while the shader takes its place.
	c.syncViewDecorations(view)
	roots := c.shaderViewRoots(view, decorations)
	buffer, area, err := c.flattenAnimationRoots(roots, image.Rect(x, y, x+max(1, geometry.w), y+max(1, geometry.h)), c.windowCaptureScale(view))
	return buffer, area, err
}
func (c *Compositor) finishAnimationShaderEffect(key animationKey) {
	if c.animationShaders == nil {
		return
	}
	effect := c.animationShaders.effects[key]
	if effect == nil {
		return
	}
	delete(c.animationShaders.effects, key)
	if effect.node != nil {
		C.wlr_scene_node_destroy(&effect.node.node)
	}
	if effect.source != nil {
		C.wlr_buffer_drop(effect.source)
	}
	if effect.next != nil {
		C.wlr_buffer_drop(effect.next)
	}
	if effect.view != nil && effect.view.mapped {
		c.syncViewOpacity(effect.view)
		c.syncViewDecorations(effect.view)
	}
	c.pruneAnimationShaders()
}
func (c *Compositor) makeAnimationEffect(key animationKey, kind string, view *toplevelView, source *C.struct_wlr_buffer, sourceArea, geometry image.Rectangle, scale float64, identity *captureIdentity) *animationShaderEffect {
	if c.animationShaders == nil || c.animationShaders.programs[kind] == nil {
		return nil
	}
	c.finishAnimationShaderEffect(key)
	effect := &animationShaderEffect{program: c.animationShaders.programs[kind], kind: kind, view: view, source: source, sourceGeometry: sourceArea.Sub(geometry.Min), geometry: geometry, scale: scale, identity: identity, random: rand.Float32()}
	effect.node = C.wlr_scene_buffer_create(c.windowAnimationTree, nil)
	if effect.node == nil {
		return nil
	}
	sceneImageIgnoreInput(effect.node)
	c.animationShaders.effects[key] = effect
	c.rememberCaptureTree(unsafe.Pointer(&effect.node.node), identity)
	return effect
}
func shaderMatrix(sx, sy, tx, ty float64) [9]C.float {
	return [9]C.float{C.float(sx), 0, 0, 0, C.float(sy), 0, C.float(tx), C.float(ty), 1}
}
func (c *Compositor) renderAnimationEffect(effect *animationShaderEffect, progress float64) error {
	geometry := effect.geometry
	if effect.view != nil {
		x, y := effect.view.treePosition()
		geometry = geometry.Add(image.Pt(x-geometry.Min.X, y-geometry.Min.Y))
		if effect.kind == "resize" {
			geometry.Max = geometry.Min.Add(image.Pt(max(1, int(math.Round(effect.view.resizeWidth))), max(1, int(math.Round(effect.view.resizeHeight)))))
		}
		effect.geometry = geometry
	}
	area := geometry
	if effect.kind == "open" {
		area = effect.sourceGeometry.Add(geometry.Min)
		expandX := int(math.Round(math.Max(500, float64(area.Dx())*.25)*effect.scale) / effect.scale)
		expandY := int(math.Round(math.Max(500, float64(area.Dy())*.25)*effect.scale) / effect.scale)
		area = area.Inset(0)
		area.Min = area.Min.Sub(image.Pt(expandX, expandY))
		area.Max = area.Max.Add(image.Pt(expandX, expandY))
	} else if effect.kind == "close" {
		for pointer := range c.sceneOutputs {
			box := c.outputLayoutBox(pointer)
			if geometry.Overlaps(box) {
				area = box
				break
			}
		}
	}
	width, height := max(1, int(math.Round(float64(area.Dx())*effect.scale))), max(1, int(math.Round(float64(area.Dy())*effect.scale)))
	u := animationShaderUniforms(width, height, progress, clampOpacity(progress))
	u.size = [2]C.float{C.float(area.Dx()), C.float(area.Dy())}
	u.geo_size = [2]C.float{C.float(geometry.Dx()), C.float(geometry.Dy())}
	u.scale = C.float(effect.scale)
	u.random_seed = C.float(effect.random)
	u.input_to_geo = shaderMatrix(float64(area.Dx())/float64(geometry.Dx()), float64(area.Dy())/float64(geometry.Dy()), float64(area.Min.X-geometry.Min.X)/float64(geometry.Dx()), float64(area.Min.Y-geometry.Min.Y)/float64(geometry.Dy()))
	previous := effect.sourceGeometry
	u.geo_to_tex = shaderMatrix(float64(geometry.Dx())/float64(previous.Dx()), float64(geometry.Dy())/float64(previous.Dy()), -float64(previous.Min.X)/float64(previous.Dx()), -float64(previous.Min.Y)/float64(previous.Dy()))
	u.geo_to_prev = u.geo_to_tex
	if effect.kind == "resize" {
		beforeW, beforeH := effect.view.resizeBaseWidth, effect.view.resizeBaseHeight
		after := effect.nextGeometry
		if after.Empty() {
			after = previous
		}
		u.curr_to_prev = shaderMatrix(float64(geometry.Dx())/float64(beforeW), float64(geometry.Dy())/float64(beforeH), 0, 0)
		current := effect.view.geometry()
		u.curr_to_next = shaderMatrix(float64(geometry.Dx())/float64(max(1, current.w)), float64(geometry.Dy())/float64(max(1, current.h)), 0, 0)
		u.geo_to_prev = shaderMatrix(float64(beforeW)/float64(previous.Dx()), float64(beforeH)/float64(previous.Dy()), -float64(previous.Min.X)/float64(previous.Dx()), -float64(previous.Min.Y)/float64(previous.Dy()))
		u.geo_to_next = shaderMatrix(float64(max(1, current.w))/float64(after.Dx()), float64(max(1, current.h))/float64(after.Dy()), -float64(after.Min.X)/float64(after.Dx()), -float64(after.Min.Y)/float64(after.Dy()))
		if effect.view.clipToGeometry {
			u.clip_to_geometry = 1
		}
		radii := effect.view.cornerRadii
		for i, r := range radii {
			u.corner_radius[i] = C.float(r)
		}
	}
	rendered, err := c.renderAnimationShader(effect.program, effect.source, effect.next, width, height, u)
	if err != nil {
		return err
	}
	defer C.wlr_buffer_drop(rendered)
	C.sysc_animation_set_buffer(effect.node, rendered, C.int(area.Min.X), C.int(area.Min.Y), C.int(area.Dx()), C.int(area.Dy()))
	effect.area = area
	effect.lastProgress = progress
	return nil
}
func (c *Compositor) beginWindowOpenShader(view *toplevelView, key animationKey) bool {
	if c.animationShaders == nil || c.animationShaders.programs["open"] == nil {
		return false
	}
	source, area, err := c.captureAnimationView(view, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sysc-compositor: opening shader source:", err)
		return false
	}
	x, y := view.treePosition()
	g := view.geometry()
	effect := c.makeAnimationEffect(key, "open", view, source, area, image.Rect(x, y, x+g.w, y+g.h), c.windowCaptureScale(view), view.captureIdentity)
	if effect == nil {
		C.wlr_buffer_drop(source)
		return false
	}
	if err = c.renderAnimationEffect(effect, 0); err != nil {
		c.finishAnimationShaderEffect(key)
		return false
	}
	c.syncViewDecorations(view)
	return true
}
func (c *Compositor) updateWindowOpenShader(view *toplevelView, key animationKey, progress float64) bool {
	if c.animationShaders == nil {
		return false
	}
	effect := c.animationShaders.effects[key]
	if effect == nil {
		return false
	}
	source, area, err := c.captureAnimationView(view, true)
	if err == nil {
		C.wlr_buffer_drop(effect.source)
		effect.source, effect.sourceGeometry = source, area.Sub(effect.geometry.Min)
		err = c.renderAnimationEffect(effect, progress)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sysc-compositor: opening shader:", err)
		c.finishAnimationShaderEffect(key)
		return false
	}
	return true
}
func (c *Compositor) beginCloseShader(key animationKey, node unsafe.Pointer) bool {
	if c.animationShaders == nil || c.animationShaders.programs["close"] == nil {
		return false
	}
	root := (*C.struct_wlr_scene_node)(node)
	roots := []*C.struct_wlr_scene_node{root}
	source, area, err := c.flattenAnimationRoots(roots, image.Rectangle{}, 1)
	if err != nil {
		return false
	}
	var identity *captureIdentity
	walkCaptureScene(root, func(child *C.struct_wlr_scene_node) {
		if owner := c.captureNodes[child]; owner != nil {
			identity = owner.identity
		}
	})
	effect := c.makeAnimationEffect(key, "close", nil, source, area, area, 1, identity)
	if effect == nil {
		C.wlr_buffer_drop(source)
		return false
	}
	if err = c.renderAnimationEffect(effect, 0); err != nil {
		c.finishAnimationShaderEffect(key)
		return false
	}
	sceneFXSetNodeEnabled(node, false)
	return true
}
func (c *Compositor) updateCloseShader(key animationKey, progress float64) bool {
	if c.animationShaders == nil {
		return false
	}
	effect := c.animationShaders.effects[key]
	if effect == nil {
		return false
	}
	if err := c.renderAnimationEffect(effect, progress); err != nil {
		c.finishAnimationShaderEffect(key)
		return false
	}
	return true
}
func (c *Compositor) beginResizeShader(view *toplevelView, kind config.Animation) bool {
	if c.animationShaders == nil || c.animationShaders.programs["resize"] == nil {
		return false
	}
	key := animationKey{owner: view.id, property: "resize-shader"}
	c.cancelAnimation(key)
	c.finishAnimationShaderEffect(key)
	root := (*C.struct_wlr_scene_node)(view.resizeSnapshot)
	source, area, err := c.flattenAnimationRoots([]*C.struct_wlr_scene_node{root}, image.Rectangle{}, c.windowCaptureScale(view))
	if err != nil {
		return false
	}
	x, y := view.treePosition()
	effect := c.makeAnimationEffect(key, "resize", view, source, area, image.Rect(x, y, x+int(view.resizeWidth), y+int(view.resizeHeight)), c.windowCaptureScale(view), view.captureIdentity)
	if effect == nil {
		C.wlr_buffer_drop(source)
		return false
	}
	fromW, fromH := view.resizeWidth, view.resizeHeight
	toW, toH := float64(view.resizeTargetWidth), float64(view.resizeTargetHeight)
	c.animateValue(key, kind, 0, 1, 0, func(progress float64) {
		view.resizeWidth = fromW + (toW-fromW)*progress
		view.resizeHeight = fromH + (toH-fromH)*progress
		if c.animationShaders == nil || c.animationShaders.effects[key] != effect {
			return
		}
		after, afterArea, failure := c.captureAnimationView(view, false)
		if failure == nil {
			if effect.next != nil {
				C.wlr_buffer_drop(effect.next)
			}
			effect.next, effect.nextGeometry = after, afterArea.Sub(effect.geometry.Min)
			failure = c.renderAnimationEffect(effect, progress)
		}
		if failure != nil {
			fmt.Fprintln(os.Stderr, "sysc-compositor: resize shader:", failure)
			c.finishAnimationShaderEffect(key)
			sceneFXSetNodeEnabled(view.resizeSnapshot, true)
			c.syncResizeScale(view)
		}
	}, func() { c.finishAnimationShaderEffect(key); c.finishResizeIfSettled(view) })
	if c.animationShaders != nil && c.animationShaders.effects[key] != nil {
		sceneFXSetNodeEnabled(view.resizeSnapshot, false)
	}
	return true
}
func (c *Compositor) syncAnimationShaderPosition(view *toplevelView) {
	if c.animationShaders == nil {
		return
	}
	for _, property := range []string{"open-opacity", "resize-shader"} {
		if effect := c.animationShaders.effects[animationKey{owner: view.id, property: property}]; effect != nil {
			_ = c.renderAnimationEffect(effect, effect.lastProgress)
		}
	}
}
