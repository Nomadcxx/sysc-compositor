package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE -I${SRCDIR}/protocols

#include <stdbool.h>
#include <stdlib.h>

#include <wlr/types/wlr_layer_shell_v1.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_scene.h>

static struct wlr_scene_tree *sysc_tree_create(struct wlr_scene_tree *parent) {
	return wlr_scene_tree_create(parent);
}

static struct wlr_scene_layer_surface_v1 *sysc_scene_layer_create(
		struct wlr_scene_tree *parent, struct wlr_layer_surface_v1 *layer) {
	return wlr_scene_layer_surface_v1_create(parent, layer);
}

static void sysc_reparent(struct wlr_scene_tree *tree, struct wlr_scene_tree *parent) {
	wlr_scene_node_reparent(&tree->node, parent);
}

static void sysc_output_layout_box(struct wlr_output_layout *layout,
		struct wlr_output *output, struct wlr_box *box) {
	wlr_output_layout_get_box(layout, output, box);
}

static struct wlr_output *sysc_output_at(struct wlr_output_layout *layout, double x, double y) {
	return wlr_output_layout_output_at(layout, x, y);
}

static void sysc_layer_configure(struct wlr_scene_layer_surface_v1 *scene,
		struct wlr_box *full, struct wlr_box *usable) {
	wlr_scene_layer_surface_v1_configure(scene, full, usable);
}

static void sysc_tree_coords_layer(struct wlr_scene_tree *tree, int *x, int *y) {
	wlr_scene_node_coords(&tree->node, x, y);
}
*/
import "C"

import (
	"slices"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// Stacking order, bottom to top, as layer-shell defines it with the windows
// between the bottom and top layers.
const (
	layerBackground = iota
	layerBottom
	layerWindows
	layerTop
	layerOverlay
	layerCount
)

// sceneLayer maps a zwlr_layer_shell_v1 layer to its stacking tree.
func sceneLayer(layer C.enum_zwlr_layer_shell_v1_layer) int {
	switch layer {
	case C.ZWLR_LAYER_SHELL_V1_LAYER_BACKGROUND:
		return layerBackground
	case C.ZWLR_LAYER_SHELL_V1_LAYER_BOTTOM:
		return layerBottom
	case C.ZWLR_LAYER_SHELL_V1_LAYER_TOP:
		return layerTop
	default:
		return layerOverlay
	}
}

// layerSurface is one layer-shell surface the compositor draws.
type layerSurface struct {
	surface             *C.struct_wlr_layer_surface_v1
	scene               *C.struct_wlr_scene_layer_surface_v1
	tree                *C.struct_wlr_scene_tree
	layer               int
	output              unsafe.Pointer // *wlr_output
	outputName          string
	placeWithinBackdrop bool
	listeners           []*Listener
}

type LayerKind uint8

const (
	LayerBackgroundKind LayerKind = iota
	LayerBottomKind
	LayerTopKind
	LayerOverlayKind
)

type LayerKeyboardMode uint8

const (
	LayerKeyboardNoneMode LayerKeyboardMode = iota
	LayerKeyboardExclusiveMode
	LayerKeyboardOnDemandMode
)

type LayerSnapshot struct {
	Namespace             string
	Output                string
	Layer                 LayerKind
	KeyboardInteractivity LayerKeyboardMode
}

func snapshotLayerKind(layer int) LayerKind {
	switch layer {
	case layerBackground:
		return LayerBackgroundKind
	case layerBottom:
		return LayerBottomKind
	case layerTop:
		return LayerTopKind
	default:
		return LayerOverlayKind
	}
}

func snapshotKeyboardMode(mode int) LayerKeyboardMode {
	switch mode {
	case int(C.ZWLR_LAYER_SURFACE_V1_KEYBOARD_INTERACTIVITY_EXCLUSIVE):
		return LayerKeyboardExclusiveMode
	case int(C.ZWLR_LAYER_SURFACE_V1_KEYBOARD_INTERACTIVITY_ON_DEMAND):
		return LayerKeyboardOnDemandMode
	default:
		return LayerKeyboardNoneMode
	}
}

type layerFocusCandidate struct {
	index           int
	layer           int
	output          string
	keyboard        LayerKeyboardMode
	mapped          bool
	onDemandFocused bool
}

func chooseLayerKeyboardFocus(candidates []layerFocusCandidate, activeOutput string, hasWindow bool) int {
	find := func(layer int, mode LayerKeyboardMode, onDemand bool) int {
		for i := len(candidates) - 1; i >= 0; i-- {
			candidate := candidates[i]
			if candidate.mapped && candidate.output == activeOutput && candidate.layer == layer && candidate.keyboard == mode &&
				(!onDemand || candidate.onDemandFocused) {
				return candidate.index
			}
		}
		return -1
	}
	for _, layer := range []int{layerOverlay, layerTop} {
		if index := find(layer, LayerKeyboardExclusiveMode, false); index >= 0 {
			return index
		}
		if index := find(layer, LayerKeyboardOnDemandMode, true); index >= 0 {
			return index
		}
	}
	for _, layer := range []int{layerBottom, layerBackground} {
		if index := find(layer, LayerKeyboardOnDemandMode, true); index >= 0 {
			return index
		}
	}
	if hasWindow {
		return -1
	}
	for _, layer := range []int{layerBottom, layerBackground} {
		if index := find(layer, LayerKeyboardExclusiveMode, false); index >= 0 {
			return index
		}
	}
	return -1
}

// createLayerTrees makes one scene tree per stacking layer under the scene
// root. Creation order is drawing order.
func (c *Compositor) createLayerTrees() bool {
	for i := range c.layerTrees {
		if c.layerTrees[i] = C.sysc_tree_create(&c.scene.tree); c.layerTrees[i] == nil {
			return false
		}
	}
	return true
}

// layerSurfaceTree is the scene tree of a layer surface, or nil.
func (c *Compositor) layerSurfaceTree(surface *C.struct_wlr_surface) *C.struct_wlr_scene_tree {
	if l := c.layerSurfaces[unsafe.Pointer(surface)]; l != nil {
		return l.tree
	}
	return nil
}

func (c *Compositor) layerSurfaceForSurface(surface *C.struct_wlr_surface) *layerSurface {
	if surface == nil {
		return nil
	}
	root := C.wlr_surface_get_root_surface(surface)
	return c.layerSurfaces[unsafe.Pointer(root)]
}

// handleNewLayerSurface gives a layer surface an output, when the client left
// that to the compositor, and a tree in its layer. Its first configure is sent
// by the arrangement on its initial commit.
func (c *Compositor) handleNewLayerSurface(data unsafe.Pointer) {
	surface := (*C.struct_wlr_layer_surface_v1)(data)
	if surface.output == nil {
		surface.output = c.defaultOutput()
	}
	if surface.output == nil {
		// No output to put it on; the client is told to close it.
		C.wlr_layer_surface_v1_destroy(surface)
		return
	}
	layer := sceneLayer(surface.pending.layer)
	scene := C.sysc_scene_layer_create(c.layerTrees[layer], surface)
	if scene == nil {
		C.wlr_layer_surface_v1_destroy(surface)
		return
	}
	l := &layerSurface{
		surface:             surface,
		scene:               scene,
		tree:                scene.tree,
		layer:               layer,
		output:              unsafe.Pointer(surface.output),
		outputName:          C.GoString(surface.output.name),
		placeWithinBackdrop: c.resolveLayerRules(C.GoString(surface.namespace)),
	}
	key := unsafe.Pointer(surface.surface)
	c.layerSurfaces[key] = l
	c.layerOrder = append(c.layerOrder, l)

	add := func(signal Signal, fn func(unsafe.Pointer)) {
		listener := AddListener(signal, fn)
		l.listeners = append(l.listeners, listener)
		c.keep(listener)
	}
	add(unsafe.Pointer(&surface.surface.events.commit), func(unsafe.Pointer) {
		if !bool(surface.initialized) {
			return
		}
		if layer := sceneLayer(surface.current.layer); layer != l.layer {
			l.layer = layer
			C.sysc_reparent(l.tree, c.layerTrees[layer])
		}
		if surface.current.committed != 0 || bool(surface.initial_commit) {
			c.arrangeLayers(l.output)
		}
		c.updateKeyboardFocus()
		c.notifyStateChanged()
	})
	// wlroots counts an exclusive zone only while the surface is mapped
	// (types/scene/layer_shell_v1.c), and the commit that maps it usually
	// changes no layer state, so map and unmap both re-arrange.
	add(unsafe.Pointer(&surface.surface.events._map), func(unsafe.Pointer) {
		c.arrangeLayers(l.output)
		if snapshotKeyboardMode(int(surface.current.keyboard_interactive)) == LayerKeyboardOnDemandMode {
			c.onDemandLayerFocus = l
			c.setActiveOutputName(l.outputName)
		}
		c.updateKeyboardFocus()
		c.notifyStateChanged()
	})
	add(unsafe.Pointer(&surface.surface.events.unmap), func(unsafe.Pointer) {
		if c.onDemandLayerFocus == l {
			c.onDemandLayerFocus = nil
		}
		c.arrangeLayers(l.output)
		c.updateKeyboardFocus()
		c.notifyStateChanged()
	})
	add(unsafe.Pointer(&surface.events.destroy), func(unsafe.Pointer) {
		for _, listener := range l.listeners {
			c.drop(listener)
		}
		l.listeners = nil
		delete(c.layerSurfaces, key)
		// A mapped surface is unmapped, and re-arranged without, before
		// this; an unmapped one never counted.
		c.layerOrder = slices.DeleteFunc(c.layerOrder, func(o *layerSurface) bool { return o == l })
		if c.onDemandLayerFocus == l {
			c.onDemandLayerFocus = nil
		}
		c.updateKeyboardFocus()
		c.notifyStateChanged()
	})
}

func (c *Compositor) resolveLayerRules(namespace string) bool {
	return config.ResolveLayerRules(c.layerRules, namespace)
}

func (c *Compositor) reapplyLayerRules() {
	for _, layer := range c.layerOrder {
		layer.placeWithinBackdrop = c.resolveLayerRules(C.GoString(layer.surface.namespace))
	}
}

// defaultOutput is the output under the pointer, else any enabled output.
func (c *Compositor) defaultOutput() *C.struct_wlr_output {
	if out := C.sysc_output_at(c.layout, c.cursor.x, c.cursor.y); out != nil {
		return out
	}
	for out := range c.sceneOutputs {
		return (*C.struct_wlr_output)(out)
	}
	return nil
}

// arrangeLayers places every layer surface on one output and records what is
// left for windows. Surfaces with an exclusive zone go first, top layer down,
// so each one shrinks the area the next is placed in; the rest are then placed
// in what remains (the order sway and tinywl-derived compositors use).
func (c *Compositor) arrangeLayers(output unsafe.Pointer) {
	if _, live := c.sceneOutputs[output]; !live {
		return
	}
	var full C.struct_wlr_box
	C.sysc_output_layout_box(c.layout, (*C.struct_wlr_output)(output), &full)
	usable := full
	for _, exclusive := range []bool{true, false} {
		for layer := layerOverlay; layer >= layerBackground; layer-- {
			for _, l := range c.layerOrder {
				if l.output != output || l.layer != layer ||
					(l.surface.current.exclusive_zone > 0) != exclusive {
					continue
				}
				C.sysc_layer_configure(l.scene, &full, &usable)
			}
		}
	}
	c.usable[output] = rect{int(usable.x), int(usable.y), int(usable.width), int(usable.height)}
	c.syncWorkspaceOutput(output)
}

// closeOutputLayers tells every layer surface on an output that is going away
// to close.
func (c *Compositor) closeOutputLayers(output unsafe.Pointer) {
	for _, l := range slices.Clone(c.layerOrder) {
		if l.output == output {
			C.wlr_layer_surface_v1_destroy(l.surface)
		}
	}
}

// layerPlacement is where one layer surface sits, for tests.
type layerPlacement struct {
	layer               int
	namespace           string
	mapped              bool
	placeWithinBackdrop bool
	box                 rect
}

func (c *Compositor) layerPlacements() []layerPlacement {
	var out []layerPlacement
	for _, l := range c.layerOrder {
		var x, y C.int
		C.sysc_tree_coords_layer(l.tree, &x, &y)
		out = append(out, layerPlacement{
			layer:               l.layer,
			namespace:           C.GoString(l.surface.namespace),
			mapped:              bool(l.surface.surface.mapped),
			placeWithinBackdrop: l.placeWithinBackdrop,
			box: rect{int(x), int(y),
				int(l.surface.current.actual_width), int(l.surface.current.actual_height)},
		})
	}
	return out
}

// LayerSnapshot copies mapped layer-shell surfaces into plain Go values. Call
// it on the compositor thread; IPC readers consume the immutable copies.
func (c *Compositor) LayerSnapshot() []LayerSnapshot {
	out := make([]LayerSnapshot, 0, len(c.layerOrder))
	for _, l := range c.layerOrder {
		if !bool(l.surface.surface.mapped) {
			continue
		}
		out = append(out, LayerSnapshot{
			Namespace: cString(l.surface.namespace), Output: l.outputName,
			Layer:                 snapshotLayerKind(l.layer),
			KeyboardInteractivity: snapshotKeyboardMode(int(l.surface.current.keyboard_interactive)),
		})
	}
	return out
}

// usableArea is the first output's usable area, for tests.
func (c *Compositor) usableArea() rect {
	for out := range c.sceneOutputs {
		return c.usable[out]
	}
	return rect{}
}

// closeLayerSurfaces closes every layer surface, for tests.
func (c *Compositor) closeLayerSurfaces() {
	for out := range c.sceneOutputs {
		c.closeOutputLayers(out)
	}
}
