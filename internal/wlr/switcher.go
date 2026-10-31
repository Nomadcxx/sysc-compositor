package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <scenefx/types/wlr_scene.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_xdg_shell.h>
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"os"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// The window switcher, Niri's WindowMruUi (src/ui/mru.rs): a recent-windows
// bind opens it on the active output and selects the next most recently
// used window; it shows after open-delay-ms; releasing every modifier
// focuses the selection. While it is open it holds the keyboard, no client
// gets the pointer, and only the recent-windows binds and its own keys run.

type switcher struct {
	// mru is the open switcher's windows; it stays while the switcher fades
	// out, with fading set.
	mru        *wm.MRU
	fading     bool
	output     unsafe.Pointer
	shown      bool
	timer      *time.Timer
	generation uint64
	// scope is the scope it last closed with, which a bind without scope=
	// opens it in.
	scope wm.MRUScope
	// viewPos is where the strip's view starts as drawn, and viewTarget
	// where it is heading; placed is false until the first layout, which
	// does not animate. freezeView keeps the view while the pointer selects.
	viewPos, viewTarget float64
	placed              bool
	freezeView          bool
	// syncing is set while syncSwitcher builds the scene, which an
	// animation's first value must not rebuild under it. dirty asks for a
	// rebuild after the animation tick, once however many springs moved.
	syncing, dirty bool
	// moves is the previews drawn away from their place as they slide back.
	moves map[uint64]*switcherMove
	alpha float64
	tree  *C.struct_wlr_scene_tree
	hits  []switcherHit
	// previews is each drawn window's preview tree.
	previews map[uint64]unsafe.Pointer
	// dynamic is the open switcher's keys taken from the general binds.
	dynamic []bind.Binding
	titles  map[switcherTitle]*image.RGBA
	panels  map[switcherPanel]*image.RGBA
}

// switcherMove is Niri's thumbnail MoveAnimation: a preview is drawn from
// away from its place, scaled by progress as that springs from 1 to 0.
type switcherMove struct{ from, progress float64 }

type switcherHit struct {
	id  uint64
	box wm.MRURect
}

type switcherTitle struct {
	title    string
	maxWidth int
	scale    float64
}

type switcherPanel struct {
	scope int
	scale float64
}

var (
	switcherCloseKey = animationKey{property: "switcher"}
	switcherViewKey  = animationKey{property: "switcher-view"}
)

// switcherMoveKey is a preview's move, owned by its window.
func switcherMoveKey(id uint64) animationKey {
	return animationKey{owner: id, property: "switcher-move"}
}

func (s *switcher) isOpen() bool { return s.mru != nil && !s.fading }

// SwitcherOpen reports whether the window switcher is open, and the window
// it has selected.
func (c *Compositor) SwitcherOpen() (selected uint64, open bool) {
	if !c.switcher.isOpen() {
		return 0, false
	}
	selected, _ = c.switcher.mru.Current()
	return selected, true
}

// switcherPresetBinds is Niri's make_preset_opened_binds: the open
// switcher's own keys, which match under any modifiers.
var switcherPresetBinds = func() []bind.Binding {
	key := func(name string, action bind.Action) bind.Binding {
		return bind.Binding{Chord: bind.Chord{Key: name}, Repeat: true, IgnoresInhibit: true, Actions: []bind.Action{action}}
	}
	return []bind.Binding{
		key("Escape", bind.Action{Name: bind.ActionMRUCancel}),
		key("Return", bind.Action{Name: bind.ActionMRUConfirm}),
		key("space", bind.Action{Name: bind.ActionMRUConfirm}),
		key("a", bind.Action{Name: bind.ActionMRUSetScope, MRUScope: "all"}),
		key("o", bind.Action{Name: bind.ActionMRUSetScope, MRUScope: "output"}),
		key("w", bind.Action{Name: bind.ActionMRUSetScope, MRUScope: "workspace"}),
		key("s", bind.Action{Name: bind.ActionMRUCycleScope}),
		key("Home", bind.Action{Name: bind.ActionMRUFirst}),
		key("End", bind.Action{Name: bind.ActionMRULast}),
		key("Left", bind.Action{Name: bind.ActionMRUAdvance, Backward: true}),
		key("Right", bind.Action{Name: bind.ActionMRUAdvance}),
	}
}()

// switcherDynamicBinds is Niri's make_dynamic_opened_binds for the actions
// built here: a general bind of one of these actions also works in the open
// switcher, under any modifiers; per key, the bind with the fewest
// modifiers wins.
func switcherDynamicBinds(general []bind.Binding) []bind.Binding {
	var keys []string
	byKey := map[string]bind.Binding{}
	for _, b := range general {
		if len(b.Actions) == 0 {
			continue
		}
		var action bind.Action
		switch first := b.Actions[0]; first.Name {
		case bind.ActionFocusColumnRight, bind.ActionFocusColumnRightOrFirst,
			bind.ActionFocusColumnOrMonitorRight, bind.ActionFocusWindowDownOrColumnRight:
			action = bind.Action{Name: bind.ActionMRUAdvance}
		case bind.ActionFocusColumnLeft, bind.ActionFocusColumnLeftOrLast,
			bind.ActionFocusColumnOrMonitorLeft, bind.ActionFocusWindowUpOrColumnLeft:
			action = bind.Action{Name: bind.ActionMRUAdvance, Backward: true}
		case bind.ActionFocusColumnFirst:
			action = bind.Action{Name: bind.ActionMRUFirst}
		case bind.ActionFocusColumnLast:
			action = bind.Action{Name: bind.ActionMRULast}
		case bind.ActionCloseWindow:
			action = bind.Action{Name: bind.ActionMRUCloseCurrentWindow}
		case bind.ActionScreenshot:
			action = first
		default:
			continue
		}
		b.Actions = []bind.Action{action}
		if seen, ok := byKey[b.Chord.Key]; ok {
			if modifierCount(seen.Chord.Modifiers) <= modifierCount(b.Chord.Modifiers) {
				continue
			}
		} else {
			keys = append(keys, b.Chord.Key)
		}
		byKey[b.Chord.Key] = b
	}
	binds := make([]bind.Binding, len(keys))
	for i, key := range keys {
		binds[i] = byKey[key]
		binds[i].Chord = bind.Chord{Key: key, Trigger: byKey[key].Chord.Trigger}
	}
	return binds
}

func modifierCount(m bind.Modifiers) int {
	n := 0
	for ; m != 0; m &= m - 1 {
		n++
	}
	return n
}

// keyBindings is what a key press can run now, in Niri's make_binds_iter
// order: the general binds while the switcher is closed, the recent-windows
// binds while they are on or it is open, then the open switcher's keys under
// the held modifiers.
func (c *Compositor) keyBindings(modifiers bind.Modifiers) []bind.Binding {
	open := c.switcher.isOpen()
	var binds []bind.Binding
	if !open {
		binds = append(binds, c.bindings...)
	}
	if open || !c.recentWindows.Off {
		binds = append(binds, c.mruBindings...)
	}
	if open {
		for _, b := range append(append([]bind.Binding(nil), switcherPresetBinds...), c.switcher.dynamic...) {
			b.Chord.Modifiers = modifiers
			binds = append(binds, b)
		}
	}
	return binds
}

// configureSwitcher takes the recent-windows section and the general binds.
func (c *Compositor) configureSwitcher(settings config.RecentWindows) {
	c.recentWindows = settings
	c.mruBindings = make([]bind.Binding, len(settings.Binds))
	for i, b := range settings.Binds {
		b.Chord = b.Chord.ResolveMod(c.modKey)
		c.mruBindings[i] = b
	}
	c.switcher.dynamic = switcherDynamicBinds(c.bindings)
	c.syncSwitcher()
}

// runSwitcherAction runs a window-switcher action, reporting whether action
// was one.
func (c *Compositor) runSwitcherAction(action bind.Action) bool {
	s := &c.switcher
	switch action.Name {
	case bind.ActionMRUAdvance:
		if !s.isOpen() {
			c.openSwitcher(action)
			return true
		}
		if action.MRUFilter != "" {
			c.relistSwitcher(func() bool { return s.mru.SetAppIDFilter(action.MRUFilter == "app-id") })
		}
		if action.Backward {
			s.mru.Backward()
		} else {
			s.mru.Forward()
		}
	case bind.ActionMRUConfirm:
		c.closeSwitcher(true)
		return true
	case bind.ActionMRUCancel:
		c.closeSwitcher(false)
		return true
	case bind.ActionMRUCloseCurrentWindow:
		if id, ok := s.selected(); ok {
			c.CloseWindow(&id)
		}
		return true
	case bind.ActionMRUFirst, bind.ActionMRULast, bind.ActionMRUSetScope, bind.ActionMRUCycleScope:
		if !s.isOpen() {
			return true
		}
		switch action.Name {
		case bind.ActionMRUFirst:
			s.mru.First()
		case bind.ActionMRULast:
			s.mru.Last()
		case bind.ActionMRUSetScope:
			c.relistSwitcher(func() bool { return s.mru.SetScope(mruScope(action.MRUScope)) })
		default:
			c.relistSwitcher(s.mru.CycleScope)
		}
	default:
		return false
	}
	s.freezeView = false
	c.syncSwitcher()
	return true
}

func (s *switcher) selected() (uint64, bool) {
	if !s.isOpen() {
		return 0, false
	}
	return s.mru.Current()
}

func mruScope(name string) wm.MRUScope {
	switch name {
	case "output":
		return wm.MRUScopeOutput
	case "workspace":
		return wm.MRUScopeWorkspace
	default:
		return wm.MRUScopeAll
	}
}

// openSwitcher is Niri's MruAdvance with the switcher closed.
func (c *Compositor) openSwitcher(action bind.Action) {
	s := &c.switcher
	if c.recentWindows.Off || c.sessionLocked || c.model == nil {
		return
	}
	c.applyPendingWindowFocus()
	outputName := c.FocusedOutputName()
	output := c.outputByName(outputName)
	if output == nil {
		return
	}
	m := wm.NewMRU(c.mruWindows(outputName))
	if m.Empty() {
		return
	}
	scope := s.scope
	if action.MRUScope != "" {
		scope = mruScope(action.MRUScope)
	}
	m.SetScope(scope)
	if action.MRUFilter != "" {
		m.SetAppIDFilter(action.MRUFilter == "app-id")
	}
	c.dropSwitcher()
	s.mru, s.output, s.alpha, s.placed, s.freezeView = m, output, 1, false, false
	// With nothing focused, the newest window is the one to go back to.
	if action.Backward || c.layoutFocusedWindow() {
		if action.Backward {
			m.Backward()
		} else {
			m.Forward()
		}
	}
	c.updateKeyboardFocus()
	c.refocusPointer(nowMsec())
	delay := time.Duration(c.recentWindows.OpenDelayMS) * time.Millisecond
	if delay == 0 {
		s.shown = true
		c.syncSwitcher()
		return
	}
	s.generation++
	generation := s.generation
	s.timer = time.AfterFunc(delay, func() {
		_ = c.Post(func() {
			if s.generation == generation && s.isOpen() {
				s.shown = true
				c.syncSwitcher()
			}
		})
	})
}

// layoutFocusedWindow reports whether the active output's workspace has a
// focused window, Niri's layout.focus().
func (c *Compositor) layoutFocusedWindow() bool {
	for _, ws := range c.model.Snapshot().Workspaces {
		if ws.IsFocused {
			return ws.ActiveWindowID != nil
		}
	}
	return false
}

// mruWindows is every mapped window as the switcher on output sees it, in
// layout order.
func (c *Compositor) mruWindows(output string) []wm.MRUWindow {
	views := map[uint64]*toplevelView{}
	for _, v := range c.views {
		if v.toplevel != nil && v.mapped && bool(v.toplevel.base.surface.mapped) {
			views[v.id] = v
		}
	}
	snapshot := c.model.Snapshot()
	active := map[uint64]bool{}
	for _, ws := range snapshot.Workspaces {
		active[ws.ID] = ws.Output == output && ws.IsActive
	}
	var windows []wm.MRUWindow
	for _, w := range snapshot.Windows {
		v := views[w.ID]
		if v == nil {
			continue
		}
		windows = append(windows, wm.MRUWindow{
			ID: v.id, Stamp: v.focusTimestamp, Stamped: v.hasFocusTimestamp,
			OnOutput: w.Output == output, OnWorkspace: active[w.WorkspaceID],
			AppID: cStringPointer(v.toplevel.app_id),
		})
	}
	return windows
}

func (c *Compositor) outputByName(name string) unsafe.Pointer {
	for pointer := range c.sceneOutputs {
		if c.outputIsEnabled(pointer) && C.GoString((*C.struct_wlr_output)(pointer).name) == name {
			return pointer
		}
	}
	return nil
}

// closeSwitcher closes the switcher, focusing its selection when confirm,
// and fades it out if it was shown.
func (c *Compositor) closeSwitcher(confirm bool) {
	s := &c.switcher
	if !s.isOpen() {
		return
	}
	id, ok := s.mru.Current()
	s.scope = s.mru.Scope()
	s.generation++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.shown {
		s.fading = true
		c.animateValue(switcherCloseKey, c.animationConfig.RecentWindowsClose, 1, 0, 0,
			func(alpha float64) {
				s.alpha = clampOpacity(alpha)
				c.syncSwitcher()
			}, c.dropSwitcher)
	} else {
		c.dropSwitcher()
	}
	var target *toplevelView
	if confirm && ok {
		for _, v := range c.views {
			if v.id == id && v.mapped {
				target = v
			}
		}
	}
	if target != nil {
		c.focusView(target)
	} else {
		c.updateKeyboardFocus()
	}
	c.refocusPointer(nowMsec())
}

// dropSwitcher removes the switcher's scene at once.
func (c *Compositor) dropSwitcher() {
	s := &c.switcher
	if s.tree != nil {
		defer c.damageCaptureSources()
	}
	c.cancelAnimation(switcherCloseKey)
	c.cancelAnimation(switcherViewKey)
	for id := range s.moves {
		c.cancelAnimation(switcherMoveKey(id))
	}
	if s.tree != nil {
		C.wlr_scene_node_destroy(&s.tree.node)
		s.tree = nil
	}
	s.mru, s.fading, s.shown, s.output, s.hits, s.titles, s.moves = nil, false, false, nil, nil, nil, nil
	s.previews = nil
}

// switcherWindowGone takes a closed window out of the switcher, which
// closes once no window is left.
func (c *Compositor) switcherWindowGone(id uint64) {
	s := &c.switcher
	if s.mru == nil {
		return
	}
	c.cancelAnimation(switcherMoveKey(id))
	delete(s.moves, id)
	if strip, ok := c.switcherStrip(); ok && s.isOpen() {
		current, _ := s.mru.Current()
		moves, view := strip.RemovalMoves(listedIDs(s.mru), current, id, c.switcherPreviewWidth(strip, id))
		c.offsetSwitcherView(view)
		c.moveSwitcherPreviews(moves)
	}
	s.mru.Remove(id)
	if s.mru.Empty() && s.isOpen() {
		c.closeSwitcher(false)
		return
	}
	c.syncSwitcher()
}

// switcherOutputGone closes the switcher when its output goes away.
func (c *Compositor) switcherOutputGone(output unsafe.Pointer) {
	if c.switcher.output == output {
		if c.switcher.isOpen() {
			c.closeSwitcher(false)
		}
		c.dropSwitcher()
	}
}

// switcherModifiersReleased confirms the selection once no modifier is
// held.
func (c *Compositor) switcherModifiersReleased() {
	if c.switcher.isOpen() && c.activeBindModifiers() == 0 {
		c.closeSwitcher(true)
	}
}

// switcherMotion keeps the pointer from clients while the switcher is open,
// and once it shows, selects the preview under the pointer.
func (c *Compositor) switcherMotion() bool {
	s := &c.switcher
	if !s.isOpen() {
		return false
	}
	C.wlr_seat_pointer_notify_clear_focus(c.seat)
	c.showDefaultImage()
	// Once shown, pointer motion keeps the view still, as in Niri.
	s.freezeView = s.freezeView || s.shown
	if id, ok := c.switcherHitAt(float64(c.cursor.x), float64(c.cursor.y)); ok {
		if current, _ := s.mru.Current(); current != id {
			s.mru.SetCurrent(id)
			c.syncSwitcher()
		}
	}
	return true
}

// switcherButton takes button presses while the switcher is open: a left
// press on a preview focuses its window, and anywhere else cancels.
func (c *Compositor) switcherButton(button uint32, pressed bool) bool {
	if !c.switcher.isOpen() || !pressed {
		return false
	}
	c.consumedPointerButtons[button] = true
	if button == evdevButtonLeft {
		if id, ok := c.switcherHitAt(float64(c.cursor.x), float64(c.cursor.y)); ok {
			c.switcher.mru.SetCurrent(id)
			c.closeSwitcher(true)
		} else {
			c.closeSwitcher(false)
		}
	}
	C.wlr_seat_pointer_notify_frame(c.seat)
	return true
}

func (c *Compositor) switcherHitAt(x, y float64) (uint64, bool) {
	s := &c.switcher
	if !s.shown || s.output == nil {
		return 0, false
	}
	box := c.outputLayoutBox(s.output)
	for _, hit := range s.hits {
		if hit.box.Contains(x-float64(box.Min.X), y-float64(box.Min.Y)) {
			return hit.id, true
		}
	}
	return 0, false
}

// syncSwitcher redraws the shown switcher: a backdrop on every output, and
// on its own output the scope panel and the previews in view, the selected
// one highlighted, each with its title.
func (c *Compositor) syncSwitcher() {
	defer c.refreshA11y()
	s := &c.switcher
	if s.tree != nil || (s.mru != nil && s.shown) {
		defer c.damageCaptureSources()
	}
	s.syncing, s.dirty = true, false
	defer func() { s.syncing = false }()
	if s.tree != nil {
		C.wlr_scene_node_destroy(&s.tree.node)
		s.tree = nil
	}
	s.hits, s.previews = nil, nil
	if s.mru == nil || !s.shown || s.output == nil {
		return
	}
	below := &c.lockTree.node
	if c.hotkeyOverlay.tree != nil {
		below = &c.hotkeyOverlay.tree.node
	} else if c.shotUI != nil {
		below = &(*C.struct_wlr_scene_tree)(c.shotUI.tree).node
	}
	s.tree = C.wlr_scene_tree_create(&c.scene.tree)
	if s.tree == nil {
		return
	}
	C.wlr_scene_node_place_below(&s.tree.node, below)
	tree := unsafe.Pointer(s.tree)
	for pointer := range c.sceneOutputs {
		if !c.outputIsEnabled(pointer) {
			continue
		}
		box := c.outputLayoutBox(pointer)
		backdrop := ui.MRUBackdrop
		backdrop[3] *= float32(s.alpha)
		if node := sceneFXCreateRect(tree, box.Dx(), box.Dy(), backdrop); node != nil {
			sceneFXNodeSetPosition(node, box.Min.X, box.Min.Y)
		}
	}

	box := c.outputLayoutBox(s.output)
	strip, _ := c.switcherStrip()
	scale := strip.Scale
	hl := c.recentWindows.Highlight
	views := map[uint64]*toplevelView{}
	for _, v := range c.views {
		if v.toplevel != nil && v.tree != nil && v.mapped {
			views[v.id] = v
		}
	}
	urgent := map[uint64]bool{}
	for _, w := range c.model.Snapshot().Windows {
		urgent[w.ID] = w.IsUrgent
	}
	var listed []*toplevelView
	var sizes [][2]float64
	current := -1
	selected, _ := s.mru.Current()
	for _, w := range s.mru.Listed() {
		v := views[w.ID]
		if v == nil {
			continue
		}
		if w.ID == selected {
			current = len(listed)
		}
		g := v.geometry()
		listed = append(listed, v)
		sizes = append(sizes, [2]float64{float64(max(g.w, 1)), float64(max(g.h, 1))})
	}
	rects := strip.Layout(sizes)
	if !s.freezeView {
		c.moveSwitcherView(strip.ViewPos(s.viewTarget, rects, current), strip)
	}
	viewPos := strip.Round(s.viewPos)
	at := func(x, y float64) (int, int) {
		return box.Min.X + int(math.Round(x)), box.Min.Y + int(math.Round(y))
	}
	for i, r := range rects {
		v := listed[i]
		// As Niri, a moving preview is drawn moved and found where it is
		// heading.
		r.X -= viewPos
		placed := r
		r.X += strip.Round(c.switcherMoveOffset(v.id))
		if r.X+r.W < 0 || strip.OutputWidth < r.X {
			continue
		}
		title := c.switcherTitleImage(cString(v.toplevel.title), int(math.Round(r.W*scale)), scale)
		titleHeight := 0.
		if title != nil {
			titleHeight = float64(title.Rect.Dy()) / scale
		}
		highlight := strip.Highlight(r, titleHeight)
		if placed.X+placed.W >= 0 && placed.X <= strip.OutputWidth {
			s.hits = append(s.hits, switcherHit{id: v.id, box: strip.Highlight(placed, titleHeight)})
		}

		active := i == current
		if active || urgent[v.id] {
			color := hl.ActiveColor
			if urgent[v.id] {
				color = hl.UrgentColor
			}
			if !active {
				color.A *= 0.4
			}
			x, y := at(highlight.X, highlight.Y)
			w, h := int(math.Round(highlight.W)), int(math.Round(highlight.H))
			radius := int(math.Round(hl.CornerRadius))
			if node := sceneFXCreateRoundedRect(tree, w, h, radius, premultiplied(color, 0.5*s.alpha)); node != nil {
				sceneFXNodeSetPosition(node, x, y)
			}
			if active {
				border := int(math.Round(strip.Round(wm.MRUBorder)))
				outer := uint16(radius + border)
				if node := sceneFXCreateRect(tree, w+2*border, h+2*border, [4]float32{}); node != nil {
					sceneFXRectUpdateRing(node, x-border, y-border, w+2*border, h+2*border, border,
						[4]uint16{outer, outer, outer, outer}, [4]uint16{uint16(radius), uint16(radius), uint16(radius), uint16(radius)},
						premultiplied(color, s.alpha))
				}
			}
		}
		if title != nil {
			size := image.Pt(int(math.Round(float64(title.Rect.Dx())/scale)), int(math.Round(titleHeight)))
			x, y := at(r.X+(r.W-float64(size.X))/2, r.Y+r.H+strip.Round(wm.MRUTitleGap))
			if node := sceneImage(s.tree, title); node != nil {
				sceneImageIgnoreInput(node)
				sceneImagePlace(node, image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(size)}, s.alpha)
			}
		}
		x, y := at(r.X, r.Y)
		zoom := r.W / float64(max(v.geometry().w, 1))
		c.rememberCaptureNodes(v)
		if preview := projectViewPreview(s.tree, v, x, y, zoom); preview != nil {
			c.rememberCaptureTree(unsafe.Pointer(preview), v.captureIdentity)
			// Niri clips previews to the window geometry and its corner
			// radius whether or not the window itself is clipped.
			var radii config.CornerRadius
			for i, radius := range v.cornerRadii {
				radii[i] = radius * zoom
			}
			sceneFXClipTree(unsafe.Pointer(preview), rect{x, y, int(math.Round(r.W)), int(math.Round(r.H))}, sceneFXRadii(radii))
			if s.alpha < 1 {
				sceneFXScaleTreeOpacity(unsafe.Pointer(preview), s.alpha)
			}
			if s.previews == nil {
				s.previews = make(map[uint64]unsafe.Pointer)
			}
			s.previews[v.id] = unsafe.Pointer(preview)
		}
	}

	if panel := c.switcherPanelImage(s.mru.Scope(), scale); panel != nil {
		size := image.Pt(int(math.Round(float64(panel.Rect.Dx())/scale)), int(math.Round(float64(panel.Rect.Dy())/scale)))
		x, y := at((strip.OutputWidth-float64(size.X))/2, 2*strip.Round(ui.MRUPanelPadding))
		if node := sceneImage(s.tree, panel); node != nil {
			sceneImageIgnoreInput(node)
			sceneImagePlace(node, image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(size)}, s.alpha)
		}
	}
}

// moveSwitcherView heads the view for target, springing there with
// window-movement as Niri's ViewPos does once it moves more than a pixel.
func (c *Compositor) moveSwitcherView(target float64, strip wm.MRUStrip) {
	s := &c.switcher
	if s.placed && target == s.viewTarget {
		return
	}
	s.viewTarget = target
	if !s.placed || math.Abs(target-s.viewPos) <= 1/strip.Scale {
		s.placed = true
		c.cancelAnimation(switcherViewKey)
		s.viewPos = target
		return
	}
	c.animateValue(switcherViewKey, c.animationConfig.WindowMovement, s.viewPos, target, 0,
		func(pos float64) {
			s.viewPos = pos
			c.markSwitcherDirty()
		}, nil)
}

// markSwitcherDirty asks for the switcher's scene to be rebuilt after this
// animation tick, unless it is being built now.
func (c *Compositor) markSwitcherDirty() {
	if !c.switcher.syncing {
		c.switcher.dirty = true
	}
}

// switcherStrip is the open switcher's preview strip on its output.
func (c *Compositor) switcherStrip() (wm.MRUStrip, bool) {
	s := &c.switcher
	if s.output == nil {
		return wm.MRUStrip{}, false
	}
	box := c.outputLayoutBox(s.output)
	return wm.MRUStrip{
		OutputWidth: float64(box.Dx()), OutputHeight: float64(box.Dy()),
		Scale:     float64((*C.struct_wlr_output)(s.output).scale),
		Padding:   c.recentWindows.Highlight.Padding,
		MaxHeight: c.recentWindows.Previews.MaxHeight, MaxScale: c.recentWindows.Previews.MaxScale,
	}, true
}

// switcherPreviewWidth is the width of id's preview in strip.
func (c *Compositor) switcherPreviewWidth(strip wm.MRUStrip, id uint64) float64 {
	for _, v := range c.views {
		if v.id == id && v.toplevel != nil {
			g := v.geometry()
			w, _ := strip.PreviewSize(float64(max(g.w, 1)), float64(max(g.h, 1)))
			return w
		}
	}
	return 0
}

// switcherMoveOffset is how far id's preview is drawn from its place.
func (c *Compositor) switcherMoveOffset(id uint64) float64 {
	if m := c.switcher.moves[id]; m != nil {
		return m.from * m.progress
	}
	return 0
}

// moveSwitcherPreviews starts each preview in moves that far from its place,
// plus wherever it is drawn now, and springs it home with window-movement:
// Niri's animate_move_from_with_config.
func (c *Compositor) moveSwitcherPreviews(moves map[uint64]float64) {
	s := &c.switcher
	for id, from := range moves {
		if s.moves == nil {
			s.moves = make(map[uint64]*switcherMove)
		}
		m := &switcherMove{from: from + c.switcherMoveOffset(id), progress: 1}
		s.moves[id] = m
		key := switcherMoveKey(id)
		c.cancelAnimation(key)
		c.animateValue(key, c.animationConfig.WindowMovement, 1, 0, 0, func(progress float64) {
			m.progress = progress
			c.markSwitcherDirty()
		}, func() {
			if s.moves[id] == m {
				delete(s.moves, id)
			}
		})
	}
}

// offsetSwitcherView shifts the view, and its spring, by delta: Niri's
// ViewPos::offset, which keeps the selection where it is drawn when the
// strip changes left of it.
func (c *Compositor) offsetSwitcherView(delta float64) {
	if delta == 0 {
		return
	}
	s := &c.switcher
	s.viewPos += delta
	s.viewTarget += delta
	c.offsetAnimation(switcherViewKey, delta)
}

// relistSwitcher applies a scope or filter change, then slides the previews
// still listed from where they were: Niri's animate_scope_filter_change. A
// switcher that listed nothing places its view anew.
func (c *Compositor) relistSwitcher(change func() bool) {
	s := &c.switcher
	_, wasListing := s.mru.Current()
	was := listedSet(s.mru)
	if !change() {
		return
	}
	current, ok := s.mru.Current()
	strip, shown := c.switcherStrip()
	if !ok || !shown {
		return
	}
	if !wasListing {
		s.placed = false
		return
	}
	is := listedSet(s.mru)
	var windows []uint64
	for _, w := range s.mru.Windows() {
		windows = append(windows, w.ID)
	}
	moves, view := strip.RelistMoves(windows, func(id uint64) bool { return was[id] },
		func(id uint64) bool { return is[id] }, current,
		func(id uint64) float64 { return c.switcherPreviewWidth(strip, id) })
	c.offsetSwitcherView(view)
	c.moveSwitcherPreviews(moves)
}

func listedIDs(m *wm.MRU) []uint64 {
	var ids []uint64
	for _, w := range m.Listed() {
		ids = append(ids, w.ID)
	}
	return ids
}

func listedSet(m *wm.MRU) map[uint64]bool {
	set := make(map[uint64]bool)
	for _, w := range m.Listed() {
		set[w.ID] = true
	}
	return set
}

// premultiplied is a config colour with its alpha scaled by alpha, as the
// scene's rects take it.
func premultiplied(color config.Color, alpha float64) [4]float32 {
	a := color.A * alpha
	return [4]float32{float32(color.R * a), float32(color.G * a), float32(color.B * a), float32(a)}
}

func (c *Compositor) switcherTitleImage(title string, maxWidth int, scale float64) *image.RGBA {
	if title == "" || maxWidth <= 0 {
		return nil
	}
	s := &c.switcher
	key := switcherTitle{title, maxWidth, scale}
	if img, ok := s.titles[key]; ok {
		return img
	}
	img, err := ui.MRUTitle(title, maxWidth, scale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: window switcher title: %v\n", err)
		img = nil
	}
	if s.titles == nil {
		s.titles = map[switcherTitle]*image.RGBA{}
	}
	s.titles[key] = img
	return img
}

func (c *Compositor) switcherPanelImage(scope wm.MRUScope, scale float64) *image.RGBA {
	s := &c.switcher
	index := map[wm.MRUScope]int{wm.MRUScopeAll: ui.MRUScopeAll, wm.MRUScopeWorkspace: ui.MRUScopeWorkspace,
		wm.MRUScopeOutput: ui.MRUScopeOutput}[scope]
	key := switcherPanel{index, scale}
	if img, ok := s.panels[key]; ok {
		return img
	}
	img, err := ui.MRUScopePanel(index, scale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: window switcher panel: %v\n", err)
		img = nil
	}
	if s.panels == nil {
		s.panels = map[switcherPanel]*image.RGBA{}
	}
	s.panels[key] = img
	return img
}
