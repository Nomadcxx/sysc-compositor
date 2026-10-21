package wlr

import (
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/animation"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func testSpringAnimation() config.Animation {
	return config.Animation{Spring: &config.SpringConfig{
		DampingRatio: 1, Stiffness: 120, Epsilon: 0.0001,
	}}
}

func laptopAnimations() config.Animations {
	animations := config.Default().Animations
	spring := func(stiffness uint32, epsilon float64) config.Animation {
		return config.Animation{Spring: &config.SpringConfig{DampingRatio: 1, Stiffness: stiffness, Epsilon: epsilon}}
	}
	animations.WindowOpen = spring(700, 0.0001)
	animations.WindowClose = spring(700, 0.1)
	animations.WindowResize = spring(800, 0.001)
	animations.WindowMovement = spring(600, 0.0001)
	animations.OverviewOpenClose = spring(900, 0.0001)
	animations.WorkspaceSwitch = spring(800, 0.00001)
	animations.HorizontalViewMovement = spring(700, 0.0001)
	return animations
}

func TestAnimateValuePreservesSameTargetAndRetargetsSample(t *testing.T) {
	oldSpring, err := animation.NewSpring(0, 1, 0, 1, 120, 0.0001)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-150 * time.Millisecond)
	firstKey := animationKey{owner: 1, property: "same-target"}
	secondKey := animationKey{owner: 1, property: "new-target"}
	first := &runningAnimation{spring: oldSpring, target: 1, started: started, set: func(float64) {}}
	second := &runningAnimation{spring: oldSpring, target: 1, started: started, set: func(float64) {}}
	c := &Compositor{
		animations:     map[animationKey]*runningAnimation{firstKey: first, secondKey: second},
		animationTimer: time.NewTimer(time.Hour),
	}
	defer c.animationTimer.Stop()

	c.animateValue(firstKey, testSpringAnimation(), 0, 1, 0, func(float64) {}, nil)
	if c.animations[firstKey] != first || first.started != started {
		t.Fatal("same-target request restarted the running spring")
	}

	now := time.Now()
	elapsed := now.Sub(started)
	wantValue := oldSpring.ValueAt(elapsed)
	wantVelocity := oldSpring.VelocityAt(elapsed)
	c.animateValue(secondKey, testSpringAnimation(), 0, 2, 0, func(float64) {}, nil)
	retargeted := c.animations[secondKey]
	if retargeted == second || retargeted.target != 2 {
		t.Fatalf("retargeted animation = %+v, want a new spring targeting 2", retargeted)
	}
	if got := retargeted.spring.ValueAt(0); math.Abs(got-wantValue) > 1e-4 {
		t.Errorf("retargeted start = %v, want sampled value %v", got, wantValue)
	}
	if got := retargeted.spring.VelocityAt(0); math.Abs(got-wantVelocity) > 1e-4 {
		t.Errorf("retargeted velocity = %v, want sampled velocity %v", got, wantVelocity)
	}
}

func TestOverviewToggleAnimatesPresentedProgress(t *testing.T) {
	animations := laptopAnimations()
	c := newNestedWith(t, Options{Animations: animations})
	checked := false
	c.viewMapped = func(*toplevelView) {
		if !c.ToggleOverview() {
			t.Error("ToggleOverview() = false, want opening transition")
			c.Terminate()
			return
		}
		if !c.model.OverviewOpen() {
			t.Error("overview target did not commit open immediately")
			c.Terminate()
			return
		}
		if got := c.model.OverviewProgress(); got < 0 || got >= 1 {
			t.Errorf("presented overview progress = %v, want the spring start before its target", got)
			c.Terminate()
			return
		}
		time.AfterFunc(50*time.Millisecond, func() {
			_ = c.Post(func() {
				if got := c.model.OverviewProgress(); got <= 0 || got >= 1 {
					t.Errorf("presented overview progress after a tick = %v, want an in-flight value", got)
					c.Terminate()
					return
				}
				time.AfterFunc(1200*time.Millisecond, func() {
					_ = c.Post(func() {
						if got := c.model.OverviewProgress(); got != 1 {
							t.Errorf("settled overview progress = %v, want 1", got)
							c.Terminate()
							return
						}
						if !c.ToggleOverview() || c.model.OverviewOpen() {
							t.Error("ToggleOverview() did not commit overview close")
							c.Terminate()
							return
						}
						time.AfterFunc(50*time.Millisecond, func() {
							_ = c.Post(func() {
								if got := c.model.OverviewProgress(); got <= 0 || got >= 1 {
									t.Errorf("closing overview progress = %v, want an in-flight value", got)
									c.Terminate()
									return
								}
								time.AfterFunc(1200*time.Millisecond, func() {
									_ = c.Post(func() {
										if got := c.model.OverviewProgress(); got != 0 {
											t.Errorf("closed overview progress = %v, want 0", got)
										}
										checked = true
										c.Terminate()
									})
								})
							})
						})
					})
				})
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested overview animation check did not complete")
	}
}

func TestWindowMovementAnimatesSceneWhileSnapshotUsesTarget(t *testing.T) {
	animations := laptopAnimations()
	c := newNestedWith(t, Options{Animations: animations})
	checked := false
	c.viewMapped = func(view *toplevelView) {
		geometry := view.geometry()
		if !c.model.SetWindowFloating(view.id, true, layout.Bounds{X: 100, Y: 100, Width: geometry.w, Height: geometry.h}) {
			t.Error("SetWindowFloating() = false, want floating window")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		beforeX, beforeY := view.treePosition()
		if !c.model.MoveFloatingWindow(view.id,
			layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: 180},
			layout.PositionChange{Kind: layout.PositionAdjustFixed, Value: 90}) {
			t.Error("MoveFloatingWindow() = false, want updated placement")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		var target layout.Placement
		for _, window := range c.model.Snapshot().Windows {
			if window.ID == view.id {
				target = window.Tile
				break
			}
		}
		if gotX, gotY := view.treePosition(); gotX == target.X && gotY == target.Y {
			t.Errorf("scene position = (%d,%d), want an in-flight position before target (%d,%d)", gotX, gotY, target.X, target.Y)
			c.Terminate()
			return
		} else if gotX == beforeX && gotY == beforeY {
			// A spring may start at the current scene position; the first tick moves it.
		}
		windows := c.Windows()
		if len(windows) != 1 || windows[0].X != target.X || windows[0].Y != target.Y {
			t.Errorf("window snapshot = %+v, want committed target (%d,%d)", windows, target.X, target.Y)
			c.Terminate()
			return
		}
		time.AfterFunc(50*time.Millisecond, func() {
			_ = c.Post(func() {
				if gotX, gotY := view.treePosition(); gotX == target.X && gotY == target.Y {
					t.Errorf("scene position after one tick = (%d,%d), want between start and target", gotX, gotY)
					c.Terminate()
					return
				}
				time.AfterFunc(1200*time.Millisecond, func() {
					_ = c.Post(func() {
						if gotX, gotY := view.treePosition(); gotX != target.X || gotY != target.Y {
							t.Errorf("settled scene position = (%d,%d), want target (%d,%d)", gotX, gotY, target.X, target.Y)
						}
						checked = true
						c.Terminate()
					})
				})
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested window movement animation check did not complete")
	}
}

func TestWorkspaceSwitchKeepsOutgoingWindowUntilSpringSettles(t *testing.T) {
	animations := laptopAnimations()
	c := newNestedWith(t, Options{
		Animations: animations,
		Workspaces: []wm.WorkspaceConfig{{Name: "one"}, {Name: "two"}},
	})
	checked := false
	c.viewMapped = func(view *toplevelView) {
		var target uint64
		for _, workspace := range c.model.Snapshot().Workspaces {
			if !workspace.IsActive {
				target = workspace.ID
				break
			}
		}
		if target == 0 || !c.model.FocusWorkspaceID(target) {
			t.Error("FocusWorkspaceID() could not activate the other workspace")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		if !sceneFXNodeEnabled(view.sceneNode()) {
			t.Error("outgoing workspace window was hidden before its spring ran")
			c.Terminate()
			return
		}
		if c.model.Snapshot().Windows[0].WorkspaceID == target {
			t.Error("workspace switch changed the window's target workspace")
			c.Terminate()
			return
		}
		time.AfterFunc(1200*time.Millisecond, func() {
			_ = c.Post(func() {
				if sceneFXNodeEnabled(view.sceneNode()) {
					t.Error("outgoing workspace window remained enabled after the spring settled")
				}
				checked = true
				c.Terminate()
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested workspace switch animation check did not complete")
	}
}

func TestHorizontalViewMovementAnimatesSceneToCommittedTarget(t *testing.T) {
	animations := laptopAnimations()
	c := newNestedWith(t, Options{Animations: animations})
	var views []*toplevelView
	checked := false
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		target := views[0]
		window := c.model.Snapshot().Windows[0]
		if !c.model.MapWindow(999999, window.Output) {
			t.Error("could not add a layout column for the viewport test")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		beforeX, beforeY := target.treePosition()
		moved := c.model.ScrollWorkspaceView(window.WorkspaceID, 10000)
		if !moved {
			moved = c.model.ScrollWorkspaceView(window.WorkspaceID, -10000)
		}
		if !moved {
			t.Error("ScrollWorkspaceView() did not move the populated strip")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		var targetWindow wm.WindowSnapshot
		for _, current := range c.model.Snapshot().Windows {
			if current.ID == target.id {
				targetWindow = current
				break
			}
		}
		placement := targetWindow.Tile
		expectedX, expectedY, _, _ := c.windowSceneTarget(target, targetWindow)
		if x, y := target.treePosition(); x == expectedX && y == expectedY {
			t.Errorf("scene position = (%d,%d), want an in-flight viewport position", x, y)
			c.Terminate()
			return
		}
		if beforeX == expectedX && beforeY == expectedY {
			t.Error("viewport scroll changed no target geometry")
			c.Terminate()
			return
		}
		for _, info := range c.Windows() {
			if info.ID == target.id && (info.X != placement.X || info.Y != placement.Y) {
				t.Errorf("window snapshot = %+v, want viewport target (%d,%d)", info, placement.X, placement.Y)
				c.Terminate()
				return
			}
		}
		time.AfterFunc(1200*time.Millisecond, func() {
			_ = c.Post(func() {
				var current wm.WindowSnapshot
				for _, candidate := range c.model.Snapshot().Windows {
					if candidate.ID == target.id {
						current = candidate
						break
					}
				}
				wantX, wantY, _, _ := c.windowSceneTarget(target, current)
				if x, y := target.treePosition(); x != wantX || y != wantY {
					t.Errorf("settled scene position = (%d,%d), want (%d,%d)", x, y, wantX, wantY)
				}
				checked = true
				c.Terminate()
			})
		})
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if !checked {
		t.Fatal("nested horizontal view movement check did not complete")
	}
}

func TestWindowOpenAndCloseAnimateReferencedBuffers(t *testing.T) {
	animations := laptopAnimations()
	radius := config.CornerRadius{4, 8, 12, 2}
	clip := true
	c := newNestedWith(t, Options{
		Animations:  animations,
		WindowRules: []config.WindowRule{{GeometryCornerRadius: &radius, ClipToGeometry: &clip}},
	})
	checked := false
	mapCount := 0
	remapOpened := false
	c.viewMapped = func(view *toplevelView) {
		mapCount++
		if mapCount > 1 {
			key := animationKey{owner: view.id, property: "open-opacity"}
			if !view.openAnimationStarted || view.openAnimationFinished || view.presentationOpacity != 0 || !c.hasAnimation(key) {
				t.Errorf("remapped open state = started:%v finished:%v opacity:%v animating:%v", view.openAnimationStarted, view.openAnimationFinished, view.presentationOpacity, c.hasAnimation(key))
			}
			remapOpened = true
			return
		}
		if view.presentationOpacity != 0 {
			t.Errorf("new window presentation opacity = %v, want 0 at spring start", view.presentationOpacity)
			c.Terminate()
			return
		}
		time.AfterFunc(200*time.Millisecond, func() {
			_ = c.Post(func() {
				if view.resizeSnapshot != nil {
					t.Error("window resize snapshot hid the live tree before the open animation settled")
					c.Terminate()
					return
				}
				_, _, opacity, found := sceneFXTreeFirstBuffer(unsafe.Pointer(view.tree))
				if !found || opacity <= 0 || opacity >= 1 {
					width, height, hasBuffer := sceneTreeBufferSize(view.tree)
					t.Errorf("opening window buffer opacity = %v (found %v), tree buffer %dx%d (found %v)", opacity, found, width, height, hasBuffer)
					c.Terminate()
					return
				}
				time.AfterFunc(1200*time.Millisecond, func() {
					_ = c.Post(func() {
						_, _, opacity, found := sceneFXTreeFirstBuffer(unsafe.Pointer(view.tree))
						if !found || opacity != 1 {
							t.Errorf("opened window buffer opacity = %v (found %v), want 1", opacity, found)
							c.Terminate()
							return
						}
						c.captureCloseSnapshot(view)
						if view.closeSnapshot == nil {
							t.Error("close transition did not retain the mapped window buffer")
							c.Terminate()
							return
						}
						if got, want := sceneFXTreeRoundedBufferCornerRadii(unsafe.Pointer(view.tree)), sceneFXRadii(view.cornerRadii); got != want {
							t.Errorf("live buffer corner radii = %v, want %v", got, want)
						}
						sceneFXSetNodeEnabled(view.closeSnapshot, true)
						got, want := sceneFXTreeRoundedBufferCornerRadii(view.closeSnapshot), sceneFXRadii(view.cornerRadii)
						sceneFXSetNodeEnabled(view.closeSnapshot, false)
						if got != want {
							t.Errorf("close snapshot corner radii = %v, want %v", got, want)
							c.Terminate()
							return
						}
						c.unmapView(view)
						if view.mapped || view.presentationVisible || view.openAnimationStarted || view.openAnimationFinished {
							t.Errorf("unmapped open state = started:%v finished:%v mapped:%v visible:%v", view.openAnimationStarted, view.openAnimationFinished, view.mapped, view.presentationVisible)
							c.Terminate()
							return
						}
						if len(c.closingNodes) != 1 {
							t.Errorf("closing scene snapshots = %d, want one retained buffer clone", len(c.closingNodes))
							c.Terminate()
							return
						}
						var closing unsafe.Pointer
						for _, node := range c.closingNodes {
							closing = node
						}
						if !sceneFXNodeEnabled(closing) {
							t.Error("closing snapshot was not enabled after unmap")
							c.Terminate()
							return
						}
						time.AfterFunc(50*time.Millisecond, func() {
							_ = c.Post(func() {
								_, _, opacity, found := sceneFXTreeFirstBuffer(closing)
								if !found || opacity <= 0 || opacity >= 1 {
									t.Errorf("closing snapshot opacity = %v (found %v), want an in-flight fade", opacity, found)
									c.Terminate()
									return
								}
								time.AfterFunc(1200*time.Millisecond, func() {
									_ = c.Post(func() {
										if len(c.closingNodes) != 0 {
											t.Errorf("closing snapshots after settle = %d, want 0", len(c.closingNodes))
											c.Terminate()
											return
										}
										if !remapOpened {
											t.Error("window remap did not restart its open animation")
											c.Terminate()
											return
										}
										checked = true
										c.Terminate()
									})
								})
							})
						})
					})
				})
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested window open and close animation check did not complete")
	}
}

func TestWindowTransitionOpacityClampsUnderdampedOvershoot(t *testing.T) {
	animations := laptopAnimations()
	underdamped := config.Animation{Spring: &config.SpringConfig{
		DampingRatio: 0.1, Stiffness: 120, Epsilon: 0.0001,
	}}
	animations.WindowOpen = underdamped
	animations.WindowClose = underdamped
	c := newNestedWith(t, Options{Animations: animations})
	checked := false
	c.viewMapped = func(view *toplevelView) {
		time.AfterFunc(300*time.Millisecond, func() {
			_ = c.Post(func() {
				openKey := animationKey{owner: view.id, property: "open-opacity"}
				open := c.animations[openKey]
				if open == nil {
					t.Error("underdamped open animation stopped before its overshoot")
					c.Terminate()
					return
				}
				openProgress := open.spring.ValueAt(time.Since(open.started))
				_, _, openOpacity, openFound := sceneFXTreeFirstBuffer(unsafe.Pointer(view.tree))
				if openProgress <= 1 {
					t.Errorf("test spring open progress = %v, want an overshoot above 1", openProgress)
				}
				if !openFound || view.presentationOpacity < 0 || view.presentationOpacity > 1 || openOpacity < 0 || openOpacity > 1 {
					t.Errorf("opening alpha progress=%v presentation=%v buffer=%v found=%v, want all alpha in [0,1]", openProgress, view.presentationOpacity, openOpacity, openFound)
					c.Terminate()
					return
				}

				snapshot, count := sceneFXCloneView(unsafe.Pointer(c.windowAnimationTree), unsafe.Pointer(view.tree))
				if snapshot == nil || count == 0 {
					t.Error("could not clone the mapped view for the close alpha check")
					c.Terminate()
					return
				}
				owner := c.nextAnimationOwner()
				c.closingNodes[owner] = snapshot
				c.startCloseSnapshot(owner, snapshot)
				closeKey := closeSnapshotKey(owner)
				time.AfterFunc(300*time.Millisecond, func() {
					_ = c.Post(func() {
						closing := c.animations[closeKey]
						if closing == nil {
							t.Error("underdamped close animation stopped before its undershoot")
							c.Terminate()
							return
						}
						closeProgress := closing.spring.ValueAt(time.Since(closing.started))
						_, _, closeOpacity, closeFound := sceneFXTreeFirstBuffer(snapshot)
						if closeProgress >= 0 {
							t.Errorf("test spring close progress = %v, want an undershoot below 0", closeProgress)
						}
						if !closeFound || closeOpacity < 0 || closeOpacity > 1 {
							t.Errorf("closing alpha progress=%v buffer=%v found=%v, want alpha in [0,1]", closeProgress, closeOpacity, closeFound)
						}
						checked = true
						c.Terminate()
					})
				})
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested underdamped transition alpha check did not complete")
	}
}

func TestWindowResizeScalesSnapshotWhileLayoutTargetsCommit(t *testing.T) {
	animations := laptopAnimations()
	animations.WindowOpen.Off = true
	defaults := config.Default()
	layoutOptions := &LayoutOptions{
		Gaps: defaults.Layout.Gaps, ColumnWidthProportion: defaults.Layout.ColumnWidthProportion,
		FixedColumnWidth: defaults.Layout.FixedColumnWidth, InsertHint: defaults.Layout.InsertHint,
		FocusRing: config.FocusRing{Width: 4,
			ActiveColor: config.Color{R: 1, A: 1}, InactiveColor: config.Color{B: 1, A: 1}},
		Shadow: config.Shadow{On: true, Softness: 8, Spread: 3,
			Color: config.Color{A: 0.5}},
	}
	c := newNestedWith(t, Options{Animations: animations, Layout: layoutOptions})
	checked := false
	c.viewMapped = func(view *toplevelView) {
		geometry := view.geometry()
		initial := layout.Bounds{X: 100, Y: 100, Width: geometry.w, Height: geometry.h}
		if !c.model.SetWindowFloating(view.id, true, initial) {
			t.Error("SetWindowFloating() = false, want floating window")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		target := layout.Bounds{X: 100, Y: 100, Width: geometry.w + 120, Height: geometry.h + 80}
		if !c.model.SetFloatingGeometry(view.id, target) {
			t.Error("SetFloatingGeometry() = false, want resize target")
			c.Terminate()
			return
		}
		c.applyWorkspaceModel()
		if view.resizeSnapshot == nil || sceneFXNodeEnabled(view.sceneNode()) {
			t.Error("resize did not retain and show a snapshot while hiding the live tree")
			c.Terminate()
			return
		}
		windows := c.Windows()
		if len(windows) != 1 || windows[0].Width != target.Width || windows[0].Height != target.Height {
			t.Errorf("window snapshot = %+v, want committed size %dx%d", windows, target.Width, target.Height)
			c.Terminate()
			return
		}
		time.AfterFunc(50*time.Millisecond, func() {
			_ = c.Post(func() {
				if view.resizeSnapshot == nil || view.resizeWidth <= float64(geometry.w) || view.resizeWidth >= float64(target.Width) {
					t.Errorf("presented resize width = %v, want an in-flight value between %d and %d", view.resizeWidth, geometry.w, target.Width)
					c.Terminate()
					return
				}
				visualWidth, visualHeight := int(math.Round(view.resizeWidth)), int(math.Round(view.resizeHeight))
				shadowWidth, shadowHeight := sceneFXShadowSize(view.shadowNode)
				if shadowWidth != visualWidth+6 || shadowHeight != visualHeight+6 {
					t.Errorf("resizing shadow = %dx%d, want animated window size %dx%d plus six pixels", shadowWidth, shadowHeight, visualWidth, visualHeight)
				}
				ringWidth, ringHeight := sceneFXRectSize(view.focusRingNode)
				if ringWidth != visualWidth+8 || ringHeight != visualHeight+8 {
					t.Errorf("resizing focus ring = %dx%d, want animated window size %dx%d plus eight pixels", ringWidth, ringHeight, visualWidth, visualHeight)
				}
				settleStarted := time.Now()
				var waitForResizeSettle func()
				waitForResizeSettle = func() {
					time.AfterFunc(50*time.Millisecond, func() {
						_ = c.Post(func() {
							wKey, hKey := c.resizeKeys(view)
							if view.resizeSnapshot == nil && sceneFXNodeEnabled(view.sceneNode()) && !c.hasAnimation(wKey) && !c.hasAnimation(hKey) {
								checked = true
								c.Terminate()
								return
							}
							if time.Since(settleStarted) >= 5*time.Second {
								t.Errorf("resize did not settle: snapshot=%p live-enabled=%v size=(%v,%v) target=(%d,%d)", view.resizeSnapshot, sceneFXNodeEnabled(view.sceneNode()), view.resizeWidth, view.resizeHeight, view.resizeTargetWidth, view.resizeTargetHeight)
								checked = true
								c.Terminate()
								return
							}
							waitForResizeSettle()
						})
					})
				}
				waitForResizeSettle()
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if !checked {
		t.Fatal("nested window resize animation check did not complete")
	}
}

func TestWindowResizeSnapshotPreservesGeometryClip(t *testing.T) {
	clip := true
	c := newNestedWith(t, Options{WindowRules: []config.WindowRule{{ClipToGeometry: &clip}}})
	var checked bool
	c.viewMapped = func(view *toplevelView) {
		geometry := view.geometry()
		if geometry.w != 100 || geometry.h != 60 {
			t.Errorf("mapped geometry = %dx%d, want 100x60", geometry.w, geometry.h)
			return
		}
		rootSurface := viewSurface(view)
		var created, childCommit, childDestroy, rootDestroy *Listener
		created = AddListener(c.surfaceCreatedSignal(), func(data unsafe.Pointer) {
			if data == rootSurface {
				return
			}
			childSurface := data
			c.drop(created)
			created = nil
			childCommit = AddListener(surfaceCommitSignal(childSurface), func(unsafe.Pointer) {
				if buffer, _ := surfaceSubsurfaceState(childSurface); buffer == 0 {
					return
				}
				_ = c.Post(func() {
					childBounds, childFound := sceneFXSurfaceBounds(unsafe.Pointer(view.tree), childSurface)
					if !childFound || childBounds != (rect{x: 80, y: 50, w: 20, h: 10}) {
						t.Errorf("live clipped child bounds = %+v found=%v, want 20x10 at (80,50)", childBounds, childFound)
					}
					snapshot, count := sceneFXCloneView(unsafe.Pointer(c.windowAnimationTree), unsafe.Pointer(view.tree))
					if snapshot == nil || count == 0 {
						t.Error("could not clone the mapped view for the geometry clip check")
						c.Terminate()
						return
					}
					bounds, found := sceneFXTreeBounds(snapshot)
					sceneFXNodeDestroy(snapshot)
					if !found || bounds != (rect{w: geometry.w, h: geometry.h}) {
						t.Errorf("snapshot bounds = %+v found=%v, want clipped geometry %+v", bounds, found, rect{w: geometry.w, h: geometry.h})
					}
					checked = true
					c.drop(childCommit)
					childCommit = nil
				})
			})
			c.keep(childCommit)
			childDestroy = AddListener(surfaceDestroySignal(childSurface), func(unsafe.Pointer) {
				if childCommit != nil {
					c.drop(childCommit)
				}
				c.drop(childDestroy)
				childDestroy = nil
			})
			c.keep(childDestroy)
		})
		c.keep(created)
		rootDestroy = AddListener(surfaceDestroySignal(rootSurface), func(unsafe.Pointer) {
			if created != nil {
				c.drop(created)
			}
			c.drop(rootDestroy)
			rootDestroy = nil
		})
		c.keep(rootDestroy)
	}
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- testclient.RunWindowGeometryOutsetWithChild(c.SocketName())
		c.Terminate()
	}()
	time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-clientDone; err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("nested client did not commit an outset subsurface within a clipped window geometry")
	}
}
