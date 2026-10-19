package wlr

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWindowsReportMappedToplevelState(t *testing.T) {
	c := newNested(t)
	var windows []WindowInfo
	stateChanges := 0
	c.SetStateChanged(func() { stateChanges++ })
	c.viewMapped = func(view *toplevelView) {
		windows = c.Windows()
		changesAtMap := stateChanges
		c.notifyWindowStateChanged()
		if stateChanges != changesAtMap {
			t.Errorf("unchanged mapped-window snapshot notified state change: %d -> %d", changesAtMap, stateChanges)
		}
		windowID := view.id
		c.unmapView(view)
		if got := c.Windows(); len(got) != 0 {
			t.Errorf("unmapped windows = %+v, want none", got)
		}
		if view.id != windowID {
			t.Errorf("window ID changed during unmap: %d -> %d", windowID, view.id)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if len(windows) != 1 {
		t.Fatalf("mapped windows = %d, want exactly one: %+v", len(windows), windows)
	}
	window := windows[0]
	if window.ID == 0 || window.AppID == nil || *window.AppID == "" || window.Title == nil || *window.Title == "" || window.PID <= 0 {
		t.Fatalf("window identity/metadata incomplete: %+v", window)
	}
	if window.Width <= 0 || window.Height <= 0 || window.Output == "" {
		t.Fatalf("window geometry/output incomplete: %+v", window)
	}
	if !window.IsFocused || !window.HasFocusTimestamp {
		t.Fatalf("new mapped window should have keyboard focus and an immediate timestamp: %+v", window)
	}
}

func TestFocusWindowSelectsStableID(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var views []*toplevelView
	focused := false
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		target := views[0]
		if !c.FocusWindow(target.id) {
			t.Error("FocusWindow rejected a mapped stable ID")
		}
		if c.FocusWindow(target.id + 1000000) {
			t.Error("FocusWindow accepted an unknown ID")
		}
		focused = c.keyboardFocusedView() == target
		c.Terminate()
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 2 {
		t.Fatalf("mapped windows = %d, want two", len(views))
	}
	if !focused {
		t.Fatal("FocusWindow did not move actual keyboard focus to the target")
	}
}

func TestFocusWindowOffscreenCenterUsesFocusedOutput(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var views []*toplevelView
	var focused bool
	var focusedOutput string
	var targetOutput string
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		target := views[0]
		c.placeView(target, 100000, 100000)
		focusedOutput = c.FocusedOutputName()
		if c.outputNameAt(c.viewCenter(target)) != "" {
			t.Error("test target center still resolves to an output after placement")
		}
		if !c.FocusWindow(target.id) {
			t.Error("FocusWindow rejected the mapped offscreen target")
		}
		focused = c.keyboardFocusedView() == target
		for _, window := range c.Windows() {
			if window.ID == target.id {
				targetOutput = window.Output
				break
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 2 {
		t.Fatalf("mapped windows = %d, want two", len(views))
	}
	if !focused {
		t.Fatal("FocusWindow did not move actual keyboard focus to the offscreen target")
	}
	if targetOutput != focusedOutput {
		t.Fatalf("offscreen target output = %q, want focused output %q", targetOutput, focusedOutput)
	}
}

func TestCloseFocusedWindowSendsProtocolRequest(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	c.viewMapped = func(*toplevelView) {
		unknown := uint64(1_000_000)
		if c.CloseWindow(&unknown) {
			t.Error("CloseWindow accepted an unknown ID")
		}
		if !c.CloseWindow(nil) {
			t.Error("CloseWindow without an ID rejected the keyboard-focused window")
		}
	}
	lines := runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		return strings.Contains(line, ".close(")
	})
	for _, line := range lines {
		if strings.Contains(line, ".close(") {
			return
		}
	}
	t.Fatal("client did not receive xdg_toplevel.close")
}

func TestLayerKeyboardFocusKeepsWindowActionsAndSnapshot(t *testing.T) {
	wofiPath, err := exec.LookPath("wofi")
	if err != nil {
		t.Skipf("wofi unavailable: %v", err)
	}
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var views []*toplevelView
	var windowChanges int
	var focusedCandidateUpdated, closeRetainedFocus bool
	var wofi *exec.Cmd
	c.SetStateChanged(func() { windowChanges++ })
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 || wofi != nil {
			return
		}
		wofi = exec.Command(wofiPath, "--show", "drun")
		wofi.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
		if err := wofi.Start(); err != nil {
			t.Errorf("start wofi: %v", err)
			c.Terminate()
			return
		}
		pollThenStop(c, func() bool {
			if len(c.LayerSnapshot()) == 0 || c.keyboardFocusedView() != nil {
				return false
			}
			before := windowChanges
			target := views[0]
			if !c.FocusWindow(target.id) {
				t.Errorf("FocusWindow rejected mapped target %d", target.id)
				return true
			}
			windows := c.Windows()
			if len(windows) != 2 || windows[0].ID != target.id || windows[0].IsFocused {
				t.Errorf("windows after focusing under layer = %+v; want target %d first but not keyboard-focused", windows, target.id)
			}
			focusedCandidateUpdated = windowChanges > before
			if c.keyboardFocusedView() != nil {
				t.Error("window action stole keyboard focus from the layer surface")
			}
			closeRetainedFocus = c.CloseWindow(nil)
			return true
		})
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if wofi != nil && wofi.Process != nil {
		_ = wofi.Process.Kill()
		_ = wofi.Wait()
	}
	if !focusedCandidateUpdated {
		t.Error("changing the retained window focus under a layer surface did not notify state changes")
	}
	if !closeRetainedFocus {
		t.Error("CloseWindow without an ID rejected the retained layout-focused window")
	}
}

func TestWindowFocusTimestampDebounces(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var views []*toplevelView
	var before time.Duration
	c.viewMapped = func(view *toplevelView) {
		views = append(views, view)
		if len(views) != 2 {
			return
		}
		target := views[0]
		before = target.focusTimestamp
		if !c.FocusWindow(target.id) {
			t.Error("FocusWindow rejected a mapped stable ID")
			c.Terminate()
			return
		}
		time.AfterFunc(850*time.Millisecond, c.Terminate)
	}
	runClient(t, c, []string{"sh", "-c", "weston-terminal & weston-terminal & wait"}, nil, nil)
	if len(views) != 2 {
		t.Fatalf("mapped windows = %d, want two", len(views))
	}
	if views[0].focusTimestamp <= before {
		t.Fatalf("focus timestamp did not commit after the debounce: before %v, after %v", before, views[0].focusTimestamp)
	}
}

func TestWindowFocusTimestampIgnoresStaleTimerPost(t *testing.T) {
	if _, err := exec.LookPath("weston-terminal"); err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	var baseline, newer time.Duration
	var committedEarly bool
	c.viewMapped = func(view *toplevelView) {
		baseline = view.focusTimestamp
		c.recordWindowFocus(view)
		time.AfterFunc(500*time.Millisecond, func() {
			_ = c.Post(func() {
				// Hold the compositor thread while the old timer expires and posts
				// its callback. Then focus the same view again before that callback
				// can run, reproducing the stale same-view match.
				time.Sleep(400 * time.Millisecond)
				c.recordWindowFocus(view)
				newer = c.pendingFocusStamp
				time.AfterFunc(200*time.Millisecond, func() {
					_ = c.Post(func() {
						committedEarly = view.focusTimestamp == newer
						c.Terminate()
					})
				})
			})
		})
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if newer <= baseline {
		t.Fatalf("new focus timestamp %v is not later than baseline %v", newer, baseline)
	}
	if committedEarly {
		t.Fatal("stale timer callback committed the newer timestamp before its debounce elapsed")
	}
}
