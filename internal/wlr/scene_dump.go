package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_compositor.h>

// Diagnostic only: describes one scene node through void pointers, so no
// concrete scene struct crosses into Go from this file.
struct sysc_dump_node {
	int type, enabled, x, y, lx, ly, width, height, has_surface;
	float opacity;
};

static void sysc_dump_describe(void *pointer, struct sysc_dump_node *out) {
	struct wlr_scene_node *node = pointer;
	out->type = node->type;
	out->enabled = node->enabled;
	out->x = node->x;
	out->y = node->y;
	wlr_scene_node_coords(node, &out->lx, &out->ly);
	out->width = out->height = out->has_surface = 0;
	out->opacity = 1;
	if (node->type == WLR_SCENE_NODE_BUFFER) {
		struct wlr_scene_buffer *buffer = wlr_scene_buffer_from_node(node);
		out->opacity = buffer->opacity;
		out->width = buffer->dst_width ? buffer->dst_width : buffer->WLR_PRIVATE.buffer_width;
		out->height = buffer->dst_height ? buffer->dst_height : buffer->WLR_PRIVATE.buffer_height;
		out->has_surface = wlr_scene_surface_try_from_buffer(buffer) != NULL;
	} else if (node->type == WLR_SCENE_NODE_RECT) {
		struct wlr_scene_rect *rect = wlr_scene_rect_from_node(node);
		out->width = rect->width;
		out->height = rect->height;
		out->opacity = rect->color[3];
	}
}

static void *sysc_dump_child(void *pointer, void *previous) {
	struct wlr_scene_node *node = pointer;
	if (node->type != WLR_SCENE_NODE_TREE) return NULL;
	struct wlr_scene_tree *tree = wlr_scene_tree_from_node(node);
	struct wl_list *next = previous == NULL ? tree->children.next :
		((struct wlr_scene_node *)previous)->link.next;
	if (next == &tree->children) return NULL;
	return wl_container_of(next, (struct wlr_scene_node *)NULL, link);
}
*/
import "C"

import (
	"fmt"
	"io"
	"strings"
	"unsafe"
)

var sceneDumpTypes = [...]string{"tree", "rect", "buffer", "shadow", "optimized-blur", "blur"}

// DumpScene writes every scene node, labelled with what owns it, for
// diagnosing stale pixels. It runs on the compositor thread.
func (c *Compositor) DumpScene(w io.Writer) {
	labels := map[unsafe.Pointer]string{
		unsafe.Pointer(c.windowShadowTree):    "windowShadowTree",
		unsafe.Pointer(c.windowAnimationTree): "windowAnimationTree",
		unsafe.Pointer(c.overviewTree):        "overviewTree",
		unsafe.Pointer(c.insertHintTree):      "insertHintTree",
	}
	for i, name := range []string{"background", "bottom", "windows", "top", "overlay"} {
		labels[unsafe.Pointer(c.layerTrees[i])] = "layer:" + name
	}
	for _, view := range c.views {
		if view == nil {
			continue
		}
		appID := ""
		if view.toplevel != nil {
			appID = cString(view.toplevel.app_id)
		}
		state := fmt.Sprintf("mapped=%t visible=%t", view.mapped, view.presentationVisible)
		if view.tree != nil {
			labels[unsafe.Pointer(view.tree)] = fmt.Sprintf("view %d %s %s", view.id, appID, state)
		}
		if view.resizeSnapshot != nil {
			labels[view.resizeSnapshot] = fmt.Sprintf("resizeSnapshot of view %d %s", view.id, appID)
		}
		if view.closeSnapshot != nil {
			labels[view.closeSnapshot] = fmt.Sprintf("pending closeSnapshot of view %d %s", view.id, appID)
		}
	}
	for owner, node := range c.closingNodes {
		_, animating := c.animations[closeSnapshotKey(owner)]
		labels[node] = fmt.Sprintf("closing snapshot owner=%d animating=%t", owner, animating)
	}
	if c.animationShaders != nil {
		for key, effect := range c.animationShaders.effects {
			if effect != nil && effect.node != nil {
				labels[unsafe.Pointer(effect.node)] = fmt.Sprintf("shader effect %s owner=%d", key.property, key.owner)
			}
		}
	}
	fmt.Fprintf(w, "sysc-compositor: scene dump: views=%d closingNodes=%d animations=%d\n",
		len(c.views), len(c.closingNodes), len(c.animations))
	var walk func(node unsafe.Pointer, depth int)
	walk = func(node unsafe.Pointer, depth int) {
		var d C.struct_sysc_dump_node
		C.sysc_dump_describe(node, &d)
		kind := "?"
		if int(d._type) < len(sceneDumpTypes) {
			kind = sceneDumpTypes[d._type]
		}
		line := fmt.Sprintf("%s%s enabled=%d at=%d,%d abs=%d,%d", strings.Repeat("  ", depth), kind,
			int(d.enabled), int(d.x), int(d.y), int(d.lx), int(d.ly))
		if d.width != 0 || d.height != 0 {
			line += fmt.Sprintf(" size=%dx%d opacity=%.3f", int(d.width), int(d.height), float64(d.opacity))
		}
		if d.has_surface != 0 {
			line += " surface"
		}
		if label := labels[node]; label != "" {
			line += "  <" + label + ">"
		}
		fmt.Fprintf(w, "sysc-compositor: scene: %s\n", line)
		for child := C.sysc_dump_child(node, nil); child != nil; child = C.sysc_dump_child(node, child) {
			walk(child, depth+1)
		}
	}
	walk(unsafe.Pointer(&c.scene.tree.node), 0)
}
