package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_xdg_shell.h>

// Iterate disabled trees too: close/resize and overview projections may keep
// buffers after their original surface has stopped being displayed.
static struct wlr_scene_node *sysc_capture_child(struct wlr_scene_node *node,
        struct wlr_scene_node *previous) {
    if (node->type != WLR_SCENE_NODE_TREE) return NULL;
    struct wlr_scene_tree *tree = wlr_scene_tree_from_node(node);
    struct wl_list *link = previous == NULL ? tree->children.next : previous->link.next;
    if (link == &tree->children) return NULL;
    struct wlr_scene_node *child = wl_container_of(link, child, link);
    return child;
}

// SceneFX retains a texture after releasing its source buffer. Cached
// dimensions remain valid until the scene node is destroyed.
static struct wlr_scene_node *sysc_capture_mask(struct wlr_scene_buffer *source) {
    int width = source->dst_width, height = source->dst_height;
    if (width == 0 || height == 0) {
        width = source->WLR_PRIVATE.buffer_width;
        height = source->WLR_PRIVATE.buffer_height;
        if (source->transform & 1) { int swap = width; width = height; height = swap; }
    }
    const float black[4] = {0, 0, 0, source->opacity};
    struct wlr_scene_rect *mask = wlr_scene_rect_create(source->node.parent, width, height, black);
    if (mask == NULL) return NULL;
    wlr_scene_rect_set_corner_radii(mask, source->corners);
    wlr_scene_node_set_position(&mask->node, source->node.x, source->node.y);
    wlr_scene_node_place_above(&mask->node, &source->node);
    wlr_scene_node_set_enabled(&mask->node, source->node.enabled);
    wlr_scene_node_set_enabled(&source->node, false);
    return &mask->node;
}
*/
import "C"

import (
	"errors"
	"image"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// Privacy identity follows scene nodes, including texture-only retained images.
// An identity freezes at unmap; remapping cannot change a closing image's policy.
type captureIdentity struct {
	appID, title string
	blocked      bool
}

type captureNodeOwner struct {
	identity *captureIdentity
	destroy  *Listener
}

func walkCaptureScene(node *C.struct_wlr_scene_node, visit func(*C.struct_wlr_scene_node)) {
	visit(node)
	for child := C.sysc_capture_child(node, nil); child != nil; child = C.sysc_capture_child(node, child) {
		walkCaptureScene(child, visit)
	}
}

func (c *Compositor) viewBlocksCapture(view *toplevelView, target config.CaptureTarget) bool {
	return view != nil && view.toplevel != nil && config.ResolveWindowRules(c.windowRules,
		cString(view.toplevel.app_id), cString(view.toplevel.title)).BlocksCapture(target)
}

func (c *Compositor) rememberCaptureNodes(view *toplevelView) {
	if view == nil || view.tree == nil || view.toplevel == nil {
		return
	}
	if view.captureIdentity == nil {
		view.captureIdentity = &captureIdentity{}
	}
	identity := view.captureIdentity
	identity.appID, identity.title = cString(view.toplevel.app_id), cString(view.toplevel.title)
	c.rememberCaptureTree(unsafe.Pointer(view.tree), identity)
}

func (c *Compositor) rememberCaptureTree(root unsafe.Pointer, identity *captureIdentity) {
	if root == nil || identity == nil {
		return
	}
	if c.captureNodes == nil {
		c.captureNodes = make(map[*C.struct_wlr_scene_node]*captureNodeOwner)
	}
	walkCaptureScene((*C.struct_wlr_scene_node)(root), func(node *C.struct_wlr_scene_node) {
		if node._type != C.WLR_SCENE_NODE_BUFFER {
			return
		}
		owner := c.captureNodes[node]
		if owner == nil {
			owner = &captureNodeOwner{}
			owner.destroy = AddListener(unsafe.Pointer(&node.events.destroy), func(unsafe.Pointer) {
				c.drop(owner.destroy)
				delete(c.captureNodes, node)
			})
			c.captureNodes[node] = owner
			c.keep(owner.destroy)
		}
		owner.identity = identity
	})
}

// Rematch metadata synchronously, without depending on the IPC snapshot having
// changed. A stricter rule invalidates frozen screenshot pixels immediately.
func (c *Compositor) syncCapturePrivacy() {
	for _, view := range c.views {
		if view.mapped {
			c.rememberCaptureNodes(view)
		}
	}
	for _, owner := range c.captureNodes {
		identity := owner.identity
		blocked := config.ResolveWindowRules(c.windowRules, identity.appID, identity.title).BlocksCapture(config.CaptureScreen)
		if blocked && !identity.blocked {
			c.closeScreenshotUI()
		}
		identity.blocked = blocked
	}
}

func (c *Compositor) destroyCapturePrivacy() {
	for node, owner := range c.captureNodes {
		c.drop(owner.destroy)
		delete(c.captureNodes, node)
	}
}

type captureMask struct {
	node    *C.struct_wlr_scene_node
	rect    *C.struct_wlr_scene_node
	enabled bool
}
type captureBlurMask struct {
	blur   *C.struct_wlr_scene_blur
	source *C.struct_wlr_scene_buffer
}

func (c *Compositor) beginCapturePrivacy(target config.CaptureTarget) (func(), error) {
	if target == config.CaptureDisplay {
		return func() {}, nil
	}
	c.syncCapturePrivacy()
	// ponytail: frozen screenshot UI is capture-transparent for casts. Render
	// the filtered live scene below instead of keeping a second full snapshot.
	var shotTree unsafe.Pointer
	if target == config.CaptureCast && c.shotUI != nil {
		shotTree = c.shotUI.tree
		sceneFXTreeSetEnabled(shotTree, false)
	}
	blocked := make(map[*C.struct_wlr_scene_node]bool)
	for node, owner := range c.captureNodes {
		identity := owner.identity
		if config.ResolveWindowRules(c.windowRules, identity.appID, identity.title).BlocksCapture(target) {
			blocked[node] = true
		}
	}
	var masks []captureMask
	var blurMasks []captureBlurMask
	var blurs []*C.struct_wlr_scene_optimized_blur
	restore := func() {
		for _, mask := range masks {
			C.wlr_scene_node_destroy(mask.rect)
			C.wlr_scene_node_set_enabled(mask.node, C.bool(mask.enabled))
		}
		for _, mask := range blurMasks {
			C.wlr_scene_blur_set_transparency_mask_source(mask.blur, mask.source)
		}
		// A physical frame must not reuse optimized blur cached for capture.
		for _, blur := range blurs {
			C.wlr_scene_optimized_blur_mark_dirty(blur)
		}
		if shotTree != nil {
			sceneFXTreeSetEnabled(shotTree, true)
		}
	}
	if len(blocked) == 0 && shotTree == nil {
		return restore, nil
	}
	var maskErr error
	walkCaptureScene(&c.scene.tree.node, func(node *C.struct_wlr_scene_node) {
		if node._type == C.WLR_SCENE_NODE_OPTIMIZED_BLUR {
			blur := C.wlr_scene_optimized_blur_from_node(node)
			blurs = append(blurs, blur)
			C.wlr_scene_optimized_blur_mark_dirty(blur)
		}
		if node._type == C.WLR_SCENE_NODE_BLUR {
			blur := C.wlr_scene_blur_from_node(node)
			source := C.wlr_scene_blur_get_transparency_mask_source(blur)
			if source != nil && blocked[&source.node] {
				blurMasks = append(blurMasks, captureBlurMask{blur, source})
				C.wlr_scene_blur_set_transparency_mask_source(blur, nil)
			}
		}
		if !blocked[node] || maskErr != nil {
			return
		}
		enabled := bool(node.enabled)
		mask := C.sysc_capture_mask(C.wlr_scene_buffer_from_node(node))
		if mask == nil {
			maskErr = errors.New("wlr: cannot allocate capture mask")
			return
		}
		masks = append(masks, captureMask{node, mask, enabled})
	})
	if maskErr != nil {
		restore()
		return nil, maskErr
	}
	// ponytail: synchronous O(scene nodes) masking on the compositor thread;
	// persistent filtered scenes are the upgrade if measured cost warrants it.
	// Never dispatch clients while masks are active. Buffers, cached textures
	// and acquire timelines remain untouched.
	return restore, nil
}

// renderWindowCapture is shared by window screenshots and live window sources.
func (c *Compositor) renderWindowCapture(view *toplevelView, target config.CaptureTarget) (*image.RGBA, error) {
	if view == nil || !view.mapped || view.toplevel == nil {
		return nil, errors.New("wlr: no window to capture")
	}
	xdg := view.toplevel.base
	width, height := c.windowCaptureSize(view)
	if _, err := captureAllocation(width, height, width*4); err != nil {
		return nil, err
	}
	scale := c.windowCaptureScale(view)
	if c.viewBlocksCapture(view, target) {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
		return img, nil
	}
	return c.renderSurfaceTree(xdg.surface, int(xdg.geometry.x), int(xdg.geometry.y), scale, width, height)
}
