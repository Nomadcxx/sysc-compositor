package wlr

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestForeignToplevelIdentifierMatchesIPCFromFirstAnnouncement(t *testing.T) {
	c := newNested(t)
	done := make(chan error, 1)
	guard := time.AfterFunc(15*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer c.Terminate()
		done <- func() error {
			early, err := testclient.StartForeignClient(c.SocketName())
			if err != nil {
				return err
			}
			defer early.Close()
			client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.SetIdentity("first", "org.sysc.ForeignTest"); err != nil {
				return err
			}
			var view *toplevelView
			var id uint64
			onCompositor(t, c, func() {
				for _, v := range c.views {
					if v.mapped {
						view, id = v, v.id
					}
				}
			})
			if view == nil {
				return fmt.Errorf("test toplevel did not map")
			}
			late, err := testclient.StartForeignClient(c.SocketName())
			if err != nil {
				return err
			}
			defer late.Close()
			for _, observer := range []*testclient.ForeignClient{early, late} {
				if err := observer.Sync(); err != nil {
					return err
				}
				windows := observer.Windows(false)
				if len(windows) != 1 || windows[0].Identifier != strconv.FormatUint(id, 10) || windows[0].Identifiers != 1 || windows[0].Title != "first" || windows[0].AppID != "org.sysc.ForeignTest" || windows[0].Done == 0 {
					return fmt.Errorf("initial foreign state = %+v, want mapping %d", windows, id)
				}
			}
			if err := client.SetIdentity("changed", "org.sysc.Changed"); err != nil {
				return err
			}
			if err := early.Sync(); err != nil {
				return err
			}
			windows := early.Windows(false)
			if windows[0].Title != "changed" || windows[0].AppID != "org.sysc.Changed" || windows[0].Identifiers != 1 {
				return fmt.Errorf("changed foreign state = %+v", windows)
			}
			if err := late.DestroyHandle(0); err != nil {
				return err
			}
			if err := client.SetIdentity("still mapped", "org.sysc.Changed"); err != nil {
				return err
			}
			if err := late.Sync(); err != nil {
				return err
			}
			if len(late.Windows(false)) != 1 {
				return fmt.Errorf("destroyed handle was recreated before unmap")
			}
			onCompositor(t, c, func() { c.unmapView(view) })
			if err := early.Sync(); err != nil {
				return err
			}
			if early.Windows(false)[0].Closed != 1 {
				return fmt.Errorf("unmap did not close its handle")
			}
			if err := early.Stop(); err != nil {
				return err
			}
			if err := client.CommitBuffer(); err != nil {
				return err
			}
			onCompositor(t, c, func() { id = view.id })
			if err := late.Sync(); err != nil {
				return err
			}
			if err := early.Sync(); err != nil {
				return err
			}
			windows = late.Windows(false)
			if len(windows) != 2 || windows[1].Identifier != strconv.FormatUint(id, 10) || windows[1].Identifier == windows[0].Identifier {
				return fmt.Errorf("remapped handles = %+v, want new ID %d", windows, id)
			}
			if len(early.Windows(false)) != 1 {
				return fmt.Errorf("stopped list announced a remapped window")
			}
			// Disconnecting an observer must leave the mapped window and other observer intact.
			observer, err := testclient.StartForeignClient(c.SocketName())
			if err != nil {
				return err
			}
			observer.Close()
			if err := client.SetIdentity("after disconnect", "org.sysc.Changed"); err != nil {
				return err
			}
			if err := late.Sync(); err != nil {
				return err
			}
			if late.Windows(false)[1].Title != "after disconnect" {
				return fmt.Errorf("observer disconnect broke remaining handles")
			}
			return nil
		}()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("foreign-toplevel check did not finish")
	}
}

func TestForeignToplevelRequestsUseWindowPolicy(t *testing.T) {
	c := newNested(t)
	attachFakeKeyboard(t, c)
	done := make(chan error, 1)
	guard := time.AfterFunc(15*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer c.Terminate()
		done <- func() error {
			first, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer first.Close()
			if err := first.SetIdentity("first", "org.sysc.First"); err != nil {
				return err
			}
			second, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer second.Close()
			if err := second.SetIdentity("second", "org.sysc.Second"); err != nil {
				return err
			}
			observer, err := testclient.StartForeignClient(c.SocketName())
			if err != nil {
				return err
			}
			defer observer.Close()
			if err := observer.Sync(); err != nil {
				return err
			}
			index := -1
			for i, w := range observer.Windows(true) {
				if w.Title == "first" {
					index = i
				}
			}
			if index < 0 {
				return fmt.Errorf("wlr manager omitted first toplevel: %+v", observer.Windows(true))
			}
			var firstView, secondView *toplevelView
			onCompositor(t, c, func() {
				for _, v := range c.views {
					if cString(v.toplevel.title) == "first" {
						firstView = v
					} else {
						secondView = v
					}
				}
			})
			if firstView == nil || secondView == nil {
				return fmt.Errorf("mapped test windows missing")
			}
			if err := observer.Request(index, 4); err != nil {
				return err
			}
			if observer.Windows(true)[index].State&(1<<2) == 0 {
				return fmt.Errorf("activated window missing from foreign state")
			}
			onCompositor(t, c, func() {
				if c.focusedView != firstView {
					t.Error("foreign activate did not focus first window")
				}
			})
			if err := observer.Request(index, 8); err != nil {
				return err
			}
			if observer.Windows(true)[index].State&(1<<3) == 0 {
				return fmt.Errorf("fullscreen request missing from foreign state")
			}
			if err := observer.Request(index, 9); err != nil {
				return err
			}
			if observer.Windows(true)[index].State&(1<<3) != 0 {
				return fmt.Errorf("unset fullscreen left foreign fullscreen state")
			}
			if observer.Windows(true)[index].Outputs != 1 {
				return fmt.Errorf("foreign handle lacks output enter")
			}
			var extra *headlessTestBackend
			var original, other string
			onCompositor(t, c, func() {
				original = firstView.outputName
				for _, output := range c.OutputSnapshot() {
					if output.Name == original && output.Logical != nil {
						extra, err = addHeadlessTestOutput(c, uint32(output.Logical.Width), uint32(output.Logical.Height))
						break
					}
				}
				for _, output := range c.OutputSnapshot() {
					if output.Name != original {
						other = output.Name
					}
				}
			})
			if err != nil {
				return err
			}
			if extra == nil || other == "" {
				return fmt.Errorf("could not add a matching second output")
			}
			t.Cleanup(extra.destroy)
			onCompositor(t, c, func() {
				if !c.MoveWindowToMonitor(&firstView.id, other) {
					t.Error("could not move focused window to second output")
				}
			})
			// The observer bound only the original output. No window-client
			// commit follows these model-only moves to repair a stale notification.
			if err := observer.Sync(); err != nil {
				return err
			}
			if observer.Windows(true)[index].Outputs != 0 {
				return fmt.Errorf("focused monitor move did not send output_leave")
			}
			onCompositor(t, c, func() { c.MoveWindowToMonitor(&firstView.id, original) })
			if err := observer.Sync(); err != nil {
				return err
			}
			if observer.Windows(true)[index].Outputs != 1 {
				return fmt.Errorf("return monitor move did not send output_enter")
			}
			onCompositor(t, c, func() { c.MoveWindowToMonitor(&firstView.id, other); extra.destroy() })
			if err := observer.Sync(); err != nil {
				return err
			}
			if observer.Windows(true)[index].Outputs != 1 {
				return fmt.Errorf("output disconnect did not restore foreign output association")
			}
			onCompositor(t, c, func() { c.focusView(secondView); c.sessionLocked = true })
			if err := observer.Request(index, 4); err != nil {
				return err
			}
			if err := observer.Request(index, 8); err != nil {
				return err
			}
			if err := observer.Request(index, 5); err != nil {
				return err
			}
			if err := first.CommitBuffer(); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if c.focusedView != secondView {
					t.Error("foreign activation changed focus while locked")
				}
				for _, w := range c.WorkspaceSnapshot().Windows {
					if w.ID == firstView.id && w.Fullscreen {
						t.Error("foreign fullscreen changed a locked window")
					}
				}
				c.sessionLocked = false
			})
			if first.CloseRequested() {
				return fmt.Errorf("foreign close reached a client while locked")
			}
			if err := observer.Request(index, 2); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if !firstView.mapped {
					t.Error("minimize changed mapped state; Niri ignores it")
				}
			})
			if err := observer.Request(index, 5); err != nil {
				return err
			}
			if err := first.CommitBuffer(); err != nil {
				return err
			}
			if !first.CloseRequested() {
				return fmt.Errorf("foreign close did not reach toplevel")
			}
			onCompositor(t, c, func() { c.unmapView(firstView) })
			if err := observer.Sync(); err != nil {
				return err
			}
			if observer.Windows(true)[index].Closed != 1 {
				return fmt.Errorf("unmap did not close wlr handle")
			}
			if err := observer.Request(index, 4); err != nil {
				return err
			}
			onCompositor(t, c, func() {
				if c.focusedView == firstView {
					t.Error("inert foreign handle reactivated an unmapped window")
				}
			})
			return nil
		}()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("foreign request check did not finish")
	}
}
