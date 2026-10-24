package wlr

import (
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestOverviewCompositorActionsNotifyStateChanges(t *testing.T) {
	model := wm.New()
	changed := 0
	c := &Compositor{model: model, stateChanged: func() { changed++ }}
	if !c.OpenOverview() || !model.Snapshot().OverviewOpen || changed != 1 {
		t.Fatalf("open overview = %v, notifications %d", model.Snapshot().OverviewOpen, changed)
	}
	if c.OpenOverview() || changed != 1 {
		t.Fatalf("idempotent open changed overview or notified: %d", changed)
	}
	if !c.ToggleOverview() || model.Snapshot().OverviewOpen || changed != 2 {
		t.Fatalf("toggle overview = %v, notifications %d", model.Snapshot().OverviewOpen, changed)
	}
	if !c.ToggleOverview() || !model.Snapshot().OverviewOpen || changed != 3 {
		t.Fatalf("toggle overview open = %v, notifications %d", model.Snapshot().OverviewOpen, changed)
	}
	if !c.CloseOverview() || model.Snapshot().OverviewOpen || changed != 4 {
		t.Fatalf("close overview = %v, notifications %d", model.Snapshot().OverviewOpen, changed)
	}
}

func TestOverviewTreeStacksAboveWindowsAndBelowTopLayers(t *testing.T) {
	c := newNested(t)
	if err := c.Post(func() {
		if !sceneNodeBefore(&c.layerTrees[layerWindows].node, &c.overviewTree.node) {
			t.Error("overview tree is not above normal window content")
		}
		if !sceneNodeBefore(&c.overviewTree.node, &c.layerTrees[layerTop].node) {
			t.Error("top layer tree is not above overview")
		}
		if !sceneNodeBefore(&c.overviewTree.node, &c.layerTrees[layerOverlay].node) {
			t.Error("overlay layer tree is not above overview")
		}
		if !sceneNodeBefore(&c.insertHintTree.node, &c.layerTrees[layerTop].node) {
			t.Error("top layer tree is not above insert hints")
		}
		c.Terminate()
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestOverviewSceneProjectsAndSelectsWindow(t *testing.T) {
	color := config.Color{R: 0.1, G: 0.2, B: 0.3, A: 1}
	c := newNestedWith(t, Options{Overview: config.Overview{Zoom: 0.5, BackdropColor: color}})
	var pointer *fakePointer
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	selected := false
	c.viewMapped = func(view *toplevelView) {
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		if !c.OpenOverview() {
			t.Error("OpenOverview() = false")
			c.Terminate()
			return
		}
		output := c.WorkspaceSnapshot().FocusedOutput
		frame, ok := c.model.OverviewFrame(output, 0.5)
		if !ok || len(frame.Workspaces) == 0 || len(frame.Workspaces[0].Windows) == 0 {
			t.Error("overview frame omitted the mapped window")
			c.Terminate()
			return
		}
		if len(c.overviewBackdrops) != 1 || c.overviewBackdrops[0].Color != color {
			t.Errorf("overview backdrops = %+v, want one with %+v", c.overviewBackdrops, color)
		}
		bounds, exists := c.outputLayoutBounds(output)
		if !exists || c.overviewBackdrops[0].Bounds != bounds {
			t.Errorf("overview backdrop bounds = %+v, output bounds %+v, exists %v", c.overviewBackdrops[0].Bounds, bounds, exists)
		}
		if len(c.overviewProjections) != 1 {
			t.Errorf("overview projections = %+v, want one", c.overviewProjections)
			c.Terminate()
			return
		}
		projected := c.overviewProjections[0]
		window := frame.Workspaces[0].Windows[0]
		if projected.WindowID != view.id || projected.Output != output || projected.WorkspaceID != frame.Workspaces[0].ID ||
			projected.Bounds != window.Bounds || projected.BufferCount == 0 {
			t.Errorf("overview projection = %+v, want output %q, window %+v, nonempty buffers", projected, output, window)
		}
		sourceWidth, sourceHeight, sourceHasBuffer := sceneTreeBufferSize(view.tree)
		projectedWidth, projectedHeight, projectedHasBuffer := sceneTreeBufferSize(projected.tree)
		wantWidth := int(math.Round(float64(sourceWidth) * c.overviewConfig.Zoom))
		wantHeight := int(math.Round(float64(sourceHeight) * c.overviewConfig.Zoom))
		if !sourceHasBuffer || !projectedHasBuffer || projectedWidth != wantWidth || projectedHeight != wantHeight {
			t.Errorf("projected first buffer = %dx%d (source %dx%d), want %dx%d at zoom %v",
				projectedWidth, projectedHeight, sourceWidth, sourceHeight, wantWidth, wantHeight, c.overviewConfig.Zoom)
		}
		pointer.moveTo(float64(window.Bounds.X+window.Bounds.Width/2), float64(window.Bounds.Y+window.Bounds.Height/2))
		pointer.button(buttonLeft, true)
		pointer.button(buttonLeft, false)
		selected = !c.WorkspaceSnapshot().OverviewOpen
		if !selected {
			t.Error("clicking the projected window did not select it and close overview")
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !selected {
		t.Fatal("overview selection was not completed")
	}
}

func TestOverviewGestureUsesCurrentZoomForProjection(t *testing.T) {
	c := newNestedWith(t, Options{Overview: config.Overview{Zoom: 0.5}})
	var pointer *fakePointer
	done := false
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if !c.OpenOverview() {
			t.Error("could not open overview before gesture")
			c.Terminate()
			return
		}
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		pointer.swipeBegin(4)
		pointer.swipeUpdate(4, 0, 150)
		progress := c.model.OverviewProgress()
		projection, ok := overviewProjectionForWindow(c, view.id)
		if !ok {
			t.Error("overview projection disappeared during gesture")
			c.Terminate()
			return
		}
		sourceWidth, _, sourceHasBuffer := sceneTreeBufferSize(view.tree)
		projectedWidth, _, projectedHasBuffer := sceneTreeBufferSize(projection.tree)
		wantZoom := 1 - progress*(1-c.overviewConfig.Zoom)
		wantWidth := int(math.Round(float64(sourceWidth) * wantZoom))
		if !sourceHasBuffer || !projectedHasBuffer || math.Abs(progress-0.5) > 1e-9 || projectedWidth != wantWidth {
			t.Errorf("gesture projection width = %d, want %d at progress %v (source width %d)", projectedWidth, wantWidth, progress, sourceWidth)
		}
		pointer.swipeEnd(false)
		done = true
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !done {
		t.Fatal("overview gesture projection check did not complete")
	}
}

func TestOverviewProjectionUsesClampedZoom(t *testing.T) {
	c := newNestedWith(t, Options{Overview: config.Overview{Zoom: 1}})
	var pointer *fakePointer
	done := false
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if !c.OpenOverview() {
			t.Error("could not open overview")
			c.Terminate()
			return
		}
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		projection, ok := overviewProjectionForWindow(c, view.id)
		if !ok {
			t.Error("overview projection missing")
			c.Terminate()
			return
		}
		sourceWidth, _, sourceHasBuffer := sceneTreeBufferSize(view.tree)
		projectedWidth, _, projectedHasBuffer := sceneTreeBufferSize(projection.tree)
		wantWidth := int(math.Round(float64(sourceWidth) * 0.75))
		if !sourceHasBuffer || !projectedHasBuffer || projectedWidth != wantWidth {
			t.Errorf("clamped projection width = %d, want %d (source width %d)", projectedWidth, wantWidth, sourceWidth)
		}
		done = true
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !done {
		t.Fatal("clamped overview projection check did not complete")
	}
}

func TestOverviewGestureSettlesToProjectedEndpoint(t *testing.T) {
	c := newNested(t)
	var pointer *fakePointer
	done := false
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	if err := c.Post(func() {
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		pointer.swipeBeginAt(1000, 4)
		pointer.swipeUpdateAt(1100, 4, 0, -120)
		pointer.swipeEndAt(1200, false)
		if !c.model.OverviewOpen() {
			t.Error("fast opening swipe did not settle to its projected endpoint")
		}
		done = true
		c.Terminate()
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("overview gesture settling check did not complete")
	}
}

func TestOverviewZeroZoomPreservesBackdropColor(t *testing.T) {
	color := config.Color{R: 0.2, G: 0.3, B: 0.4, A: 0.8}
	want := config.Overview{Zoom: 0, BackdropColor: color}
	c := newNestedWith(t, Options{Overview: want})
	if c.overviewConfig != want {
		t.Fatalf("overview options = %+v, want %+v", c.overviewConfig, want)
	}
}

func TestOverviewProjectionSurvivesBufferCommitAndUnmap(t *testing.T) {
	c := newNested(t)

	var view *toplevelView
	var rootSurface, childSurface unsafe.Pointer
	var probe *projectionProbe
	var rootCommit, rootDestroy, childCommit, childDestroy *Listener
	var rootSubsurface, roleDestroy *Listener
	var previousChild uintptr
	var rootBufferShared, childBufferShared bool
	var childProjected, childBufferUpdated, childRemoved, projectionDestroyed bool
	var actualProjectionUpdated, actualProjectionRemoved, actualRootProjectionRemoved bool
	var actualRoleProjectionRemoved, childSurfaceAliveAtRoleDestroy, childSurfaceDestroyed bool
	var childUnmapListenerOwned bool
	var initialSourceBuffer uintptr
	var childHasCurrentBuffer bool
	var rootSubsurfaceCount int
	var rootCommitCount, childCommitCount int
	var latestStats [4]uintptr
	clientReturned := make(chan struct{})
	rootDestroyed := make(chan struct{})
	abort := make(chan struct{})
	t.Cleanup(func() {
		close(abort)
		if probe != nil {
			probe.destroy()
			probe.free()
		}
	})

	c.viewMapped = func(mapped *toplevelView) {
		if view != nil {
			return
		}
		view = mapped
		rootSurface = viewSurface(view)
		probe = newProjectionProbe(c, view)
		if probe == nil {
			c.Terminate()
			return
		}
		first, _, buffers, lockDelta := probe.stats()
		latestStats = [4]uintptr{first, 0, uintptr(buffers), uintptr(lockDelta)}
		rootBufferShared = buffers > 0 && lockDelta > 0
		rootCommitCount = 1
		if !c.OpenOverview() {
			t.Error("could not open overview for live projection test")
		}
		if projection, ok := overviewProjectionForWindow(c, view.id); !ok || projection.BufferCount == 0 {
			t.Error("overview did not project the mapped root buffer")
		} else {
			initialSourceBuffer = projection.SourceLast
		}
		rootSubsurface = AddListener(surfaceNewSubsurfaceSignal(rootSurface), func(data unsafe.Pointer) {
			roleDestroy = AddListener(subsurfaceDestroySignal(data), func(unsafe.Pointer) {
				c.drop(roleDestroy)
				roleDestroy = nil
				childSurfaceAliveAtRoleDestroy = !childSurfaceDestroyed
				if projection, ok := overviewProjectionForWindow(c, view.id); ok {
					actualRoleProjectionRemoved = projection.BufferCount == 1
				}
			})
			c.keep(roleDestroy)
		})
		c.keep(rootSubsurface)

		rootCommit = AddListener(surfaceCommitSignal(rootSurface), func(unsafe.Pointer) {
			_ = c.Post(func() {
				if listeners := view.overviewSurfaces[childSurface]; listeners != nil && listeners.unmap != nil {
					_, childUnmapListenerOwned = c.listeners[listeners.unmap]
				}
				if probe == nil {
					return
				}
				probe.refreshSource()
				first, _, buffers, lockDelta := probe.stats()
				latestStats = [4]uintptr{first, 0, uintptr(buffers), uintptr(lockDelta)}
				rootCommitCount++
				rootBufferShared = rootBufferShared || (buffers > 0 && lockDelta > 0)
			})
		})
		c.keep(rootCommit)
		rootDestroy = AddListener(surfaceDestroySignal(rootSurface), func(unsafe.Pointer) {
			if rootSubsurface != nil {
				c.drop(rootSubsurface)
				rootSubsurface = nil
			}
			if rootCommit != nil {
				c.drop(rootCommit)
				rootCommit = nil
			}
			c.drop(rootDestroy)
			rootDestroy = nil
			_ = c.Post(func() {
				if probe != nil {
					projectionDestroyed = probe.destroy()
				}
				_, actualRootProjectionRemoved = overviewProjectionForWindow(c, view.id)
				close(rootDestroyed)
			})
		})
		c.keep(rootDestroy)
	}

	surfaceListener := AddListener(c.surfaceCreatedSignal(), func(data unsafe.Pointer) {
		surface := data
		if view == nil || surface == rootSurface || childSurface != nil {
			return
		}
		childSurface = surface
		childCommit = AddListener(surfaceCommitSignal(surface), func(unsafe.Pointer) {
			// Capture the source state while the commit signal owns the current
			// surface state. The client can destroy the child before Post runs.
			currentBuffer, _ := surfaceSubsurfaceState(surface)
			childHasCurrentBuffer = childHasCurrentBuffer || currentBuffer != 0
			_ = c.Post(func() {
				if probe == nil {
					return
				}
				probe.refreshSource()
				first, last, buffers, lockDelta := probe.stats()
				_, children := surfaceSubsurfaceState(rootSurface)
				rootSubsurfaceCount = max(rootSubsurfaceCount, children)
				latestStats = [4]uintptr{first, last, uintptr(buffers), uintptr(lockDelta)}
				childCommitCount++
				childBufferShared = childBufferShared || (buffers >= 2 && lockDelta > 0)
				childProjected = childProjected || (buffers >= 2 && first != 0 && last != 0)
				if previousChild != 0 && last != previousChild {
					childBufferUpdated = true
					if projection, ok := overviewProjectionForWindow(c, view.id); ok &&
						projection.BufferCount >= 2 && projection.SourceLast == last {
						actualProjectionUpdated = true
					}
				}
				previousChild = last
			})
		})
		c.keep(childCommit)
		childDestroy = AddListener(surfaceDestroySignal(surface), func(unsafe.Pointer) {
			childSurfaceDestroyed = true
			if childCommit != nil {
				c.drop(childCommit)
				childCommit = nil
			}
			c.drop(childDestroy)
			childDestroy = nil
			_ = c.Post(func() {
				if probe == nil {
					return
				}
				probe.refreshSource()
				_, _, buffers, _ := probe.stats()
				childRemoved = buffers == 1
				if projection, ok := overviewProjectionForWindow(c, view.id); ok {
					actualProjectionRemoved = projection.BufferCount == 1
				}
			})
		})
		c.keep(childDestroy)
	})
	c.keep(surfaceListener)

	var clientError error
	go func() {
		clientError = testclient.RunSubsurfaceSequence(c.SocketName())
		close(clientReturned)
		select {
		case <-rootDestroyed:
			c.Terminate()
		case <-abort:
		}
	}()
	timeout := time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	timeout.Stop()
	select {
	case <-clientReturned:
	case <-time.After(time.Second):
		t.Fatal("Wayland subsurface test client did not return")
	}

	if clientError != nil {
		t.Fatal(clientError)
	}
	if view == nil || rootSurface == nil || childSurface == nil {
		t.Fatal("test client did not map an xdg toplevel and create its subsurface")
	}
	if !rootBufferShared || !childBufferShared {
		t.Fatalf("projection buffer references: root=%v child=%v stats=%v commits=%d/%d projected=%v current-buffer=%v root-subsurfaces=%d, want both shared", rootBufferShared, childBufferShared, latestStats, rootCommitCount, childCommitCount, childProjected, childHasCurrentBuffer, rootSubsurfaceCount)
	}
	if rootCommitCount == 0 || childCommitCount < 2 {
		t.Fatalf("surface commits: root=%d child=%d, want root and at least 2 child commits", rootCommitCount, childCommitCount)
	}
	if !childProjected || !childBufferUpdated || !actualProjectionUpdated || initialSourceBuffer == 0 {
		t.Fatalf("subsurface projection: present=%v updated=%v actual-updated=%v initial-source=%x, want live projected buffer refresh", childProjected, childBufferUpdated, actualProjectionUpdated, initialSourceBuffer)
	}
	if !childHasCurrentBuffer || rootSubsurfaceCount != 1 {
		t.Fatalf("source surface state: child buffer=%v root subsurfaces=%d, want mapped child and one attached subsurface", childHasCurrentBuffer, rootSubsurfaceCount)
	}
	if !childRemoved || !actualProjectionRemoved {
		t.Fatalf("projection retained the subsurface buffer after its wl_surface was destroyed: probe=%v actual=%v", childRemoved, actualProjectionRemoved)
	}
	if !actualRoleProjectionRemoved || !childSurfaceAliveAtRoleDestroy {
		t.Fatalf("projection after subsurface role destroy = removed %v, wl_surface alive %v, want buffer removed before surface destruction", actualRoleProjectionRemoved, childSurfaceAliveAtRoleDestroy)
	}
	if !childUnmapListenerOwned {
		t.Fatal("subsurface unmap listener is not owned by the compositor")
	}
	if !projectionDestroyed || actualRootProjectionRemoved {
		t.Fatalf("root projection teardown: probe-destroyed=%v actual-projection-present=%v", projectionDestroyed, !actualRootProjectionRemoved)
	}
}

func TestPointerGesturesAndHotCornerDriveOverviewOnCompositorThread(t *testing.T) {
	c := newNestedWith(t, Options{Gestures: config.Gestures{
		HotCorners: config.HotCorners{TopRight: true},
	}})
	output := ""
	openedByHotCorner, closedByReentry := false, false
	var pointer *fakePointer
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
	})
	if err := c.Post(func() {
		output = c.FocusedOutputName()
		if output == "" {
			t.Error("nested compositor has no focused output")
			c.Terminate()
			return
		}
		c.model.MapWindow(1, output)
		for id := uint64(100); id < 104; id++ {
			c.model.MapWindow(id, output)
		}
		c.model.FocusWindow(1)
		before := c.model.Snapshot().Workspaces[0].ID
		beforeX := windowPlacementX(c.model.Snapshot().Windows, 1)
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach synthetic pointer")
			c.Terminate()
			return
		}
		bounds, ok := c.outputLayoutBounds(output)
		if !ok {
			t.Error("focused output has no logical bounds")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(bounds.X+bounds.Width/2), float64(bounds.Y+bounds.Height/2))
		pointer.swipeBegin(3)
		pointer.swipeUpdate(3, 16, 0)
		pointer.swipeEnd(false)
		if got := windowPlacementX(c.model.Snapshot().Windows, 1); got == beforeX {
			t.Error("three-finger horizontal swipe did not scroll the active workspace")
		}
		pointer.swipeBegin(3)
		pointer.swipeUpdate(3, 0, -16)
		pointer.swipeEnd(false)
		activeChanged := false
		for _, workspace := range c.model.Snapshot().Workspaces {
			activeChanged = activeChanged || (workspace.Output == output && workspace.IsActive && workspace.ID != before)
		}
		if !activeChanged {
			t.Error("three-finger vertical swipe did not switch workspace")
		}

		pointer.swipeBegin(2)
		gestureDevice := unsafe.Pointer(pointer.pointer)
		if state := c.pointerSwipeStates[gestureDevice]; state == nil || !state.forwarded {
			t.Error("unsupported two-finger swipe was not marked for forwarding")
		}
		pointer.swipeUpdate(2, 0, -50)
		pointer.swipeEnd(false)
		if _, exists := c.pointerSwipeStates[gestureDevice]; exists {
			t.Error("forwarded swipe state remained after end")
		}
		if c.model.Snapshot().OverviewOpen {
			t.Error("unsupported two-finger swipe changed overview state")
		}
		pointer.swipeBegin(4)
		pointer.swipeUpdate(4, 0, -150)
		if math.Abs(c.model.OverviewProgress()-0.5) > 1e-9 {
			t.Errorf("four-finger progress = %v, want 0.5", c.model.OverviewProgress())
		}
		pointer.swipeEnd(false)
		if !c.model.Snapshot().OverviewOpen {
			t.Error("four-finger swipe did not settle overview open")
		}
		pointer.swipeBegin(4)
		pointer.swipeUpdate(4, 0, 151)
		pointer.swipeEnd(false)
		if c.model.Snapshot().OverviewOpen {
			t.Error("four-finger swipe did not settle overview closed")
		}

		pointer.moveTo(float64(bounds.X+1), float64(bounds.Y+1))
		if c.model.Snapshot().OverviewOpen {
			t.Error("unconfigured top-left corner opened overview")
		}
		rightX := float64(bounds.X+bounds.Width) - 0.5
		pointer.moveTo(rightX, float64(bounds.Y)+0.5)
		openedByHotCorner = c.model.Snapshot().OverviewOpen
		pointer.moveTo(rightX-10, float64(bounds.Y)+10)
		pointer.moveTo(rightX, float64(bounds.Y)+0.5)
		closedByReentry = !c.model.Snapshot().OverviewOpen
		c.Terminate()
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !openedByHotCorner || !closedByReentry {
		t.Fatalf("hot corner entry/re-entry = %v/%v, want true/true", openedByHotCorner, closedByReentry)
	}
}

func TestTiledModDragShowsInsertHintAndCommitsOnRelease(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	var pointer *fakePointer
	done := false
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		output := c.model.Snapshot().FocusedOutput
		c.model.MapWindow(900, output)
		c.model.MapWindow(901, output)
		c.applyWorkspaceModel()
		keyboard, pointer = c.addFakeKeyboard(), c.addFakePointer()
		if keyboard == nil || pointer == nil {
			t.Error("could not attach fake input devices")
			c.Terminate()
			return
		}
		keyboard.key(c.modKeycode(), true)
		pointer.moveTo(c.viewCenter(view))
		pointer.button(buttonLeft, true)
		if _, ok := c.model.WindowDragState(); !ok || c.tiledMove == nil || c.tiledMove.started {
			t.Error("tiled drag did not begin in threshold recognition")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(c.cursor.x+7), float64(c.cursor.y))
		if drag, _ := c.model.WindowDragState(); drag.Started {
			t.Error("tiled drag started before the eight-pixel threshold")
		}
		bounds, ok := c.outputLayoutBounds(output)
		if !ok {
			t.Error("focused output has no bounds")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(bounds.X+bounds.Width-2), float64(bounds.Y+bounds.Height/2))
		drag, exists := c.model.WindowDragState()
		if !exists || !drag.Started || drag.Target == nil || c.insertHintNode == nil {
			t.Error("crossing threshold did not update insertion target and hint")
			c.Terminate()
			return
		}
		color := c.layoutOptions.InsertHint.Color
		wantColor := [4]float32{
			float32(color.R * color.A), float32(color.G * color.A),
			float32(color.B * color.A), float32(color.A),
		}
		if got := sceneRectColor(c.insertHintNode); got != wantColor {
			t.Errorf("insert hint color = %v, want premultiplied %v", got, wantColor)
		}
		pointer.button(buttonLeft, false)
		windows := c.model.Snapshot().Windows
		moved := false
		for _, window := range windows {
			if window.ID == view.id {
				moved = window.Column != 0
			}
		}
		if len(windows) < 3 || !moved {
			t.Errorf("window order after release = %+v, want dragged window %d in its target column", windows, view.id)
		}
		if c.insertHintNode != nil || c.tiledMove != nil || c.pointerGrab != nil {
			t.Error("release retained insert hint or tiled move grab")
		}
		beforeCancel := c.model.Snapshot().Windows
		beforeX, beforeY := view.treePosition()
		c.layoutOptions.InsertHint.Off = true
		pointer.moveTo(c.viewCenter(view))
		pointer.button(buttonLeft, true)
		pointer.moveTo(float64(int(c.cursor.x)+40), float64(int(c.cursor.y)+20))
		if c.insertHintNode != nil {
			t.Error("disabled insert hint created a scene rectangle")
		}
		c.cancelTiledWindowMove()
		if afterCancel := c.model.Snapshot().Windows; !sameWorkspacePlacements(afterCancel, beforeCancel) {
			t.Errorf("cancelled tiled drag changed placement: %+v, want %+v", afterCancel, beforeCancel)
		}
		if gotX, gotY := view.treePosition(); gotX != beforeX || gotY != beforeY {
			t.Errorf("scene position after drag cancellation = (%d, %d), want (%d, %d)", gotX, gotY, beforeX, beforeY)
		}
		pointer.button(buttonLeft, false)
		keyboard.key(c.modKeycode(), false)
		done = true
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !done {
		t.Fatal("tiled drag was not completed")
	}
}

func TestDndEdgeScrollWaitsForDwellAndStopsOnDropAndDestroy(t *testing.T) {
	c := newNestedWith(t, Options{Gestures: config.Gestures{
		DNDEdgeViewScroll: config.DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 80, MaxSpeed: 1500},
	}})
	var pointer *fakePointer
	var drag *fakeDrag
	var stayedBeforeDwell, edgeExitStopped, reentryTimerRestarted bool
	var scrolledAfterDwell, stoppedOnDrop, removedOnDestroy bool
	completed := false
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
		if drag != nil {
			drag.destroy()
		}
	})
	if err := c.Post(func() {
		output := c.FocusedOutputName()
		for id := uint64(1000); id < 1005; id++ {
			c.model.MapWindow(id, output)
		}
		c.model.FocusWindow(1000)
		c.applyWorkspaceModel()
		before := windowPlacementX(c.model.Snapshot().Windows, 1000)
		bounds, ok := c.outputLayoutBounds(output)
		if !ok {
			t.Error("focused output has no logical bounds")
			c.Terminate()
			return
		}
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(bounds.X+bounds.Width-1), float64(bounds.Y+bounds.Height/2))
		drag = newFakeDrag(c)
		if drag == nil || c.dndScroll == nil || c.dndScroll.timer == nil {
			t.Error("client drag did not start edge dwell timer")
			c.Terminate()
			return
		}
		time.AfterFunc(30*time.Millisecond, func() {
			_ = c.Post(func() {
				stayedBeforeDwell = windowPlacementX(c.model.Snapshot().Windows, 1000) == before
			})
		})
		time.AfterFunc(40*time.Millisecond, func() {
			_ = c.Post(func() {
				pointer.moveTo(float64(bounds.X+bounds.Width/2), float64(bounds.Y+bounds.Height/2))
				drag.motion()
				edgeExitStopped = c.dndScroll != nil && c.dndScroll.direction == 0 && c.dndScroll.timer == nil
				pointer.moveTo(float64(bounds.X+bounds.Width-1), float64(bounds.Y+bounds.Height/2))
				drag.motion()
				reentryTimerRestarted = c.dndScroll != nil && c.dndScroll.direction == 1 && c.dndScroll.timer != nil
			})
		})
		time.AfterFunc(180*time.Millisecond, func() {
			_ = c.Post(func() {
				scrolledAfterDwell = windowPlacementX(c.model.Snapshot().Windows, 1000) != before
				drag.drop()
				stoppedOnDrop = c.dndScroll != nil && c.dndScroll.direction == 0 && c.dndScroll.timer == nil
				drag.destroy()
				removedOnDestroy = c.dndScroll == nil
				completed = true
				c.Terminate()
			})
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !completed || !stayedBeforeDwell || !edgeExitStopped || !reentryTimerRestarted ||
		!scrolledAfterDwell || !stoppedOnDrop || !removedOnDestroy {
		t.Fatalf("DND edge scroll checks = completed %v, pre-dwell %v, exit %v, re-entry %v, post-dwell %v, drop %v, destroy %v",
			completed, stayedBeforeDwell, edgeExitStopped, reentryTimerRestarted,
			scrolledAfterDwell, stoppedOnDrop, removedOnDestroy)
	}
}

func TestDndEdgeScrollTargetsWorkspaceUnderPointerInOverview(t *testing.T) {
	c := newNestedWith(t, Options{Gestures: config.Gestures{
		DNDEdgeViewScroll: config.DNDEdgeViewScroll{TriggerWidth: 30, DelayMS: 20, MaxSpeed: 1500},
	}})
	var pointer *fakePointer
	var drag *fakeDrag
	var completed, selectedTarget, targetScrolled, activeStayed bool
	var activeBefore, targetBefore, activeAfter, targetAfter int
	t.Cleanup(func() {
		if pointer != nil {
			pointer.remove()
		}
		if drag != nil {
			drag.destroy()
		}
	})
	setup := func() {
		output := c.FocusedOutputName()
		for id := uint64(1000); id < 1005; id++ {
			c.model.MapWindow(id, output)
		}
		var activeID uint64
		var targetID uint64
		for _, workspace := range c.model.Snapshot().Workspaces {
			if workspace.Output != output {
				continue
			}
			if workspace.IsActive {
				activeID = workspace.ID
			} else if targetID == 0 && len(workspace.WindowIDs) == 0 {
				targetID = workspace.ID
			}
		}
		if activeID == 0 || targetID == 0 {
			t.Error("could not find an active and trailing workspace")
			c.Terminate()
			return
		}
		for id := uint64(2000); id < 2005; id++ {
			if !c.model.MapWindowToWorkspace(id, targetID) {
				t.Errorf("could not map window %d to target workspace %d", id, targetID)
				c.Terminate()
				return
			}
		}
		if !c.model.FocusWorkspaceID(activeID) {
			t.Error("could not restore active workspace")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		activeBefore = windowPlacementX(c.model.Snapshot().Windows, 1000)
		targetBefore = windowPlacementX(c.model.Snapshot().Windows, 2000)
		if !c.OpenOverview() {
			t.Error("could not open overview")
			c.Terminate()
			return
		}
		frame, ok := c.model.OverviewFrame(output, c.overviewConfig.Zoom)
		if !ok {
			t.Error("could not build overview frame")
			c.Terminate()
			return
		}
		var target wm.OverviewWorkspace
		for _, workspace := range frame.Workspaces {
			if workspace.ID == targetID {
				target = workspace
			}
		}
		bounds := frame.Output
		rowTop, rowBottom := max(bounds.Y, target.Bounds.Y), min(bounds.Y+bounds.Height, target.Bounds.Y+target.Bounds.Height)
		if target.ID == 0 || rowBottom <= rowTop {
			t.Error("target workspace is not visible in overview")
			c.Terminate()
			return
		}
		pointer = c.addFakePointer()
		if pointer == nil {
			t.Error("could not attach fake pointer")
			c.Terminate()
			return
		}
		pointer.moveTo(float64(bounds.X+bounds.Width-1), float64(rowTop+(rowBottom-rowTop)/2))
		drag = newFakeDrag(c)
		if drag == nil || c.dndScroll == nil {
			t.Error("client drag did not start")
			c.Terminate()
			return
		}
		selectedTarget = c.dndScroll.workspaceID == targetID
		time.AfterFunc(250*time.Millisecond, func() {
			_ = c.Post(func() {
				windows := c.model.Snapshot().Windows
				activeAfter = windowPlacementX(windows, 1000)
				targetAfter = windowPlacementX(windows, 2000)
				targetScrolled = targetAfter != targetBefore
				activeStayed = activeAfter == activeBefore
				drag.drop()
				drag.destroy()
				completed = true
				c.Terminate()
			})
		})
	}
	if err := c.Post(func() {
		time.AfterFunc(250*time.Millisecond, func() { _ = c.Post(setup) })
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !completed || !selectedTarget || !targetScrolled || !activeStayed {
		t.Fatalf("overview DND target = selected %v, target scroll %d→%d, active scroll %d→%d, completed %v",
			selectedTarget, targetBefore, targetAfter, activeBefore, activeAfter, completed)
	}
}

func windowPlacementX(windows []wm.WindowSnapshot, id uint64) int {
	for _, window := range windows {
		if window.ID == id {
			return window.Tile.X
		}
	}
	return 0
}

func overviewProjectionForWindow(c *Compositor, id uint64) (overviewProjection, bool) {
	for _, projection := range c.overviewProjections {
		if projection.WindowID == id {
			return projection, true
		}
	}
	return overviewProjection{}, false
}

func sameWorkspacePlacements(left, right []wm.WindowSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].WorkspaceID != right[index].WorkspaceID || left[index].Tile != right[index].Tile {
			return false
		}
	}
	return true
}
