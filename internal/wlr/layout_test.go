package wlr

import (
	"os"
	"os/exec"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

func TestDefaultColumnWidthUsesNiriGapSizing(t *testing.T) {
	c := &Compositor{layoutOptions: LayoutOptions{Gaps: 16, ColumnWidthProportion: 0.5}}
	if got, want := c.defaultColumnWidth(1000), 476; got != want {
		t.Fatalf("defaultColumnWidth(1000) = %d, want %d", got, want)
	}
	c.layoutOptions = LayoutOptions{FixedColumnWidth: 1200}
	if got, want := c.defaultColumnWidth(1000), 1200; got != want {
		t.Fatalf("fixed defaultColumnWidth(1000) = %d, want oversized fixed width %d", got, want)
	}
}

func TestApplyingEmptyWorkspaceActivationNotifiesState(t *testing.T) {
	model := wm.New()
	model.AddOutput("test", layout.Bounds{Width: 1000, Height: 800}, 500, 16)
	model.MapWindow(1, "test")
	if !model.FocusWorkspace("test", 2) {
		t.Fatal("could not activate the trailing empty workspace")
	}

	var got wm.Snapshot
	c := &Compositor{model: model, stateChanged: func() { got = model.Snapshot() }}
	c.applyWorkspaceModel()
	if len(got.Workspaces) != 2 || !got.Workspaces[1].IsActive || !got.Workspaces[1].IsFocused {
		t.Fatalf("state callback snapshot = %+v, want empty workspace 2 active and focused", got.Workspaces)
	}
}

func TestMappedWindowsUseScrollingColumnPlacements(t *testing.T) {
	c := newNested(t)
	var first, second *toplevelView
	var secondClient *exec.Cmd
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Workspaces) != 2 || !snapshot.Workspaces[0].IsActive || snapshot.Workspaces[1].IsActive {
			t.Errorf("workspace snapshot = %+v, want occupied active workspace and trailing empty workspace", snapshot.Workspaces)
		}
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].ID != first.id || snapshot.Windows[1].ID != second.id {
			t.Errorf("window snapshot = %+v, want two windows in column order", snapshot.Windows)
		} else {
			left, right := snapshot.Windows[0], snapshot.Windows[1]
			if left.Column != 0 || right.Column != 1 || right.Tile.X <= left.Tile.X {
				t.Errorf("column placements = %+v and %+v, want ordered visible columns", left, right)
			}
			if x, y := first.treePosition(); x != left.Tile.X || y != left.Tile.Y {
				t.Errorf("first scene position = (%d,%d), want (%d,%d)", x, y, left.Tile.X, left.Tile.Y)
			}
			if x, y := second.treePosition(); x != right.Tile.X || y != right.Tile.Y {
				t.Errorf("second scene position = (%d,%d), want (%d,%d)", x, y, right.Tile.X, right.Tile.Y)
			}
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestPointerResizeSurvivesWorkspaceModelReapplication(t *testing.T) {
	c := newNested(t)
	var keyboard *fakeKeyboard
	var pointer *fakePointer
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
		if pointer != nil {
			pointer.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		pointer = c.addFakePointer()
		if keyboard == nil || pointer == nil {
			t.Error("could not attach the test input devices")
			c.Terminate()
			return
		}
		keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
		placement := c.WorkspaceSnapshot().Windows[0].Tile
		pointer.moveTo(float64(placement.X+1), float64(placement.Y+1))
		pointer.button(buttonRight, true)
		if c.pointerGrab == nil || c.pointerGrab.mode != grabResize {
			t.Error("nested Mod+RMB did not begin a resize grab")
			c.Terminate()
			return
		}
		grab := *c.pointerGrab
		pointer.moveTo(60, 60)
		want := grab.geometry(float64(c.cursor.x), float64(c.cursor.y))
		pointer.button(buttonRight, false)
		c.applyWorkspaceModel()

		got := c.WorkspaceSnapshot().Windows[0].Tile
		if got.Width != want.width || got.Height != want.height {
			t.Errorf("workspace size after resize/reapply = %dx%d, want %dx%d", got.Width, got.Height, want.width, want.height)
		}
		if got.X != want.x || got.Y != want.y {
			t.Errorf("workspace position after resize/reapply = (%d,%d), want (%d,%d)", got.X, got.Y, want.x, want.y)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
}

func TestWorkspaceBindActivatesDynamicWorkspace(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+9", bind.ActionFocusWorkspace, "9"),
	}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			keyboard = c.addFakeKeyboard()
			if keyboard == nil {
				t.Error("could not attach fake keyboard")
				c.Terminate()
				return
			}
			keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
			if got := c.activeBindModifiers(); got != bind.Modifiers(bind.ModifierAlt) {
				t.Errorf("active modifiers after nested Alt = %b, want Alt", got)
			}
			if c.modKey != bind.ModifierAlt || c.bindings[0].Chord.Modifiers != bind.Modifiers(bind.ModifierAlt) {
				t.Errorf("nested resolved modifier = %v, chord = %+v, want Alt", c.modKey, c.bindings[0].Chord)
			}
			keyboard.key(10, true) // Top row 9.
			keyboard.key(10, false)
			keyboard.key(56, false)
			snapshot := c.WorkspaceSnapshot()
			if len(snapshot.Workspaces) != 2 || snapshot.Workspaces[0].IsActive || !snapshot.Workspaces[1].IsActive {
				t.Errorf("after Mod+9, workspaces = %+v; want workspace 2 active", snapshot.Workspaces)
			}
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Workspaces) != 3 || !snapshot.Workspaces[1].IsActive || snapshot.Workspaces[2].IsActive {
			t.Errorf("after second map, workspaces = %+v; want active workspace 2 and trailing workspace 3", snapshot.Workspaces)
		}
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].WorkspaceID != snapshot.Workspaces[0].ID ||
			snapshot.Windows[1].WorkspaceID != snapshot.Workspaces[1].ID {
			t.Errorf("windows = %+v; want one window in each occupied workspace", snapshot.Windows)
		}
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestSpawnBindUsesServerCallback(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Return", bind.ActionSpawn, "test-command", "--flag"),
	}})
	var got []string
	c.SetSpawnHandlers(func(argv []string) error {
		got = append([]string(nil), argv...)
		return nil
	}, nil)
	var keyboard *fakeKeyboard
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(*toplevelView) {
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
		keyboard.key(28, true) // Return.
		keyboard.key(28, false)
		keyboard.key(56, false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if got == nil || len(got) != 2 || got[0] != "test-command" || got[1] != "--flag" {
		t.Fatalf("spawn callback argv = %q, want %q", got, []string{"test-command", "--flag"})
	}
}

func keyBind(t *testing.T, chord, name string, args ...string) bind.Binding {
	t.Helper()
	parsedChord, err := bind.ParseChord(chord)
	if err != nil {
		t.Fatal(err)
	}
	action, supported, err := bind.ParseAction(name, args)
	if err != nil || !supported {
		t.Fatalf("ParseAction(%q, %v) = (%+v, %v, %v)", name, args, action, supported, err)
	}
	return bind.Binding{Chord: parsedChord, Actions: []bind.Action{action}}
}

func TestNestedKeyboardBindFocusesColumnAndConsumesKey(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{{
		Chord:   bind.Chord{Key: "Left", UsesMod: true},
		Actions: []bind.Action{{Name: bind.ActionFocusColumnLeft}},
	}}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		c.focusView(second)
		keyboard.key(56, true)  // Left Alt is Mod in a nested Wayland backend.
		keyboard.key(105, true) // Left arrow.
		if !c.suppressedKeys[105] {
			t.Error("configured bind key was not marked consumed")
		}
		if c.focusedView != first {
			t.Errorf("focused view = %p, want first window %p", c.focusedView, first)
		}
		keyboard.key(105, false)
		if c.suppressedKeys[105] {
			t.Error("consumed key release remained suppressed after release")
		}
		keyboard.key(56, false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}

func TestNestedKeyboardBindMovesFocusedColumnLeft(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{
		keyBind(t, "Mod+Ctrl+H", bind.ActionMoveColumnLeft),
	}})
	var first, second *toplevelView
	var keyboard *fakeKeyboard
	var secondClient *exec.Cmd
	t.Cleanup(func() {
		if keyboard != nil {
			keyboard.remove()
		}
	})
	c.viewMapped = func(view *toplevelView) {
		if first == nil {
			first = view
			secondClient = exec.Command("weston-terminal")
			secondClient.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
			if err := secondClient.Start(); err != nil {
				t.Errorf("start second terminal: %v", err)
				c.Terminate()
			}
			return
		}
		second = view
		keyboard = c.addFakeKeyboard()
		if keyboard == nil {
			t.Error("could not attach fake keyboard")
			c.Terminate()
			return
		}
		keyboard.key(56, true) // Left Alt is Mod in a nested Wayland backend.
		keyboard.key(29, true) // Left Ctrl.
		keyboard.key(35, true) // H.
		snapshot := c.WorkspaceSnapshot()
		if len(snapshot.Windows) != 2 || snapshot.Windows[0].ID != second.id || snapshot.Windows[1].ID != first.id {
			t.Errorf("windows after Mod+Ctrl+H = %+v; want focused column moved left", snapshot.Windows)
		}
		if !c.suppressedKeys[35] {
			t.Error("configured move bind key was not marked consumed")
		}
		keyboard.key(35, false)
		keyboard.key(29, false)
		keyboard.key(56, false)
		c.Terminate()
	}
	runClient(t, c, []string{"weston-terminal"}, nil, nil)
	if secondClient != nil && secondClient.Process != nil {
		_ = secondClient.Process.Kill()
		_ = secondClient.Wait()
	}
	if first == nil || second == nil {
		t.Fatalf("mapped windows = (%p, %p), want two", first, second)
	}
}
