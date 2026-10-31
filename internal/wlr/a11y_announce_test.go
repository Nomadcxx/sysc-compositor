package wlr

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ui"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// recordingAnnouncer keeps what the compositor tells a screen reader. It is
// used on the compositor thread only.
type recordingAnnouncer struct {
	focus        a11y.Node
	announced    []string
	recent       []string
	descriptions map[a11y.Node]string
}

func (r *recordingAnnouncer) Announce(text string) { r.announced = append(r.announced, text) }
func (r *recordingAnnouncer) Focus(node a11y.Node) { r.focus = node }
func (r *recordingAnnouncer) SetRecentWindows(name string, button bool) {
	r.recent = append(r.recent, fmt.Sprintf("%s/%v", name, button))
}
func (r *recordingAnnouncer) SetDescription(node a11y.Node, text string) {
	if r.descriptions == nil {
		r.descriptions = map[a11y.Node]string{}
	}
	r.descriptions[node] = text
}

// TestA11yFocusFollowsCompositorUI: as Niri's a11y_focus, the screen
// reader's focus follows the exit dialog, the overview and the screenshot UI
// while they hold the keyboard, and returns to applications after; the exit
// dialog node carries the dialog's words.
func TestA11yFocusFollowsCompositorUI(t *testing.T) {
	c := newNested(t)
	rec := &recordingAnnouncer{}
	c.SetAnnouncer(rec)
	if got := rec.descriptions[a11y.ExitDialog]; got != ui.ExitDialogText() {
		t.Errorf("exit dialog description %q, want the dialog's text", got)
	}
	go func() {
		defer c.Terminate()
		for _, step := range []struct {
			name string
			act  func()
			want a11y.Node
		}{
			{"exit dialog", func() { c.showExitDialog() }, a11y.ExitDialog},
			{"exit dialog hidden", c.hideExitDialog, a11y.NoFocus},
			{"overview", func() { c.OpenOverview() }, a11y.Overview},
			{"overview closed", func() { c.CloseOverview() }, a11y.NoFocus},
			{"screenshot UI", func() {
				if err := c.Screenshot(false, ""); err != nil {
					t.Error(err)
				}
			}, a11y.ScreenshotUI},
			{"screenshot UI closed", c.closeScreenshotUI, a11y.NoFocus},
		} {
			onCompositor(t, c, func() {
				step.act()
				if rec.focus != step.want {
					t.Errorf("%s: screen reader focus %v, want %v", step.name, rec.focus, step.want)
				}
			})
		}
	}()
	guard := time.AfterFunc(10*time.Second, c.Terminate)
	defer guard.Stop()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}

// TestA11yRecentWindowsNamesTheSelection: with the window switcher open the
// recent-windows node is a button named for the selected window, moving with
// the selection, and a scope change with the selection unchanged is
// announced, as Niri's refresh_a11y does.
func TestA11yRecentWindowsNamesTheSelection(t *testing.T) {
	recent := config.Default().RecentWindows
	recent.OpenDelayMS = 0
	c := newNestedWith(t, Options{RecentWindows: &recent})
	rec := &recordingAnnouncer{}
	c.SetAnnouncer(rec)
	withFocusedClient(t, c, func(mail *testclient.ToplevelClient) error {
		if err := mail.SetIdentity("Mail", "mail"); err != nil {
			return err
		}
		notes, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			return err
		}
		defer notes.Close()
		if err := notes.SetIdentity("Notes", "notes"); err != nil {
			return err
		}
		for deadline := time.Now().Add(2 * time.Second); ; {
			var count int
			onCompositor(t, c, func() { count = len(c.Windows()) })
			if count == 2 {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("the two windows never mapped")
			}
			time.Sleep(10 * time.Millisecond)
		}
		var errs []string
		run := func(action bind.Action) {
			onCompositor(t, c, func() { c.executeBindAction(action) })
		}
		run(bind.Action{Name: bind.ActionMRUAdvance})
		onCompositor(t, c, func() {
			if rec.focus != a11y.RecentWindows || len(rec.recent) != 1 || rec.recent[0] != "Mail/true" {
				errs = append(errs, fmt.Sprintf("opened: focus %v recent %q, want Mail selected", rec.focus, rec.recent))
			}
			if len(rec.announced) != 0 {
				errs = append(errs, fmt.Sprintf("opened with a selection announced %q", rec.announced))
			}
		})
		run(bind.Action{Name: bind.ActionMRUAdvance})
		onCompositor(t, c, func() {
			if n := len(rec.recent); n != 2 || rec.recent[1] != "Notes/true" {
				errs = append(errs, fmt.Sprintf("advanced: recent %q, want Notes selected", rec.recent))
			}
		})
		run(bind.Action{Name: bind.ActionMRUSetScope, MRUScope: "workspace"})
		onCompositor(t, c, func() {
			if len(rec.announced) != 1 || rec.announced[0] != "Scope workspace" {
				errs = append(errs, fmt.Sprintf("scope change announced %q, want Scope workspace", rec.announced))
			}
		})
		run(bind.Action{Name: bind.ActionMRUCancel})
		onCompositor(t, c, func() {
			if rec.focus != a11y.NoFocus {
				errs = append(errs, fmt.Sprintf("closed: focus %v, want none", rec.focus))
			}
		})
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	})
}

// TestHotkeyOverlayActionIsAnnounced: opening the hotkey overlay through
// its action announces its text once, as Niri's ShowHotkeyOverlay does.
func TestHotkeyOverlayActionIsAnnounced(t *testing.T) {
	c := newNestedWith(t, Options{Binds: []bind.Binding{keyBind(t, "Mod+Shift+Slash", bind.ActionShowHotkeyOverlay)},
		OverlayBinds: []config.OverlayBind{{Action: bind.ActionShowHotkeyOverlay}}})
	rec := &recordingAnnouncer{}
	c.SetAnnouncer(rec)
	go func() {
		defer c.Terminate()
		onCompositor(t, c, func() {
			c.executeBindAction(bind.Action{Name: bind.ActionShowHotkeyOverlay})
			c.executeBindAction(bind.Action{Name: bind.ActionShowHotkeyOverlay})
			if len(rec.announced) != 1 || !strings.HasPrefix(rec.announced[0], "Important Hotkeys\n") {
				t.Errorf("announced %q, want the overlay text once", rec.announced)
			}
		})
	}()
	guard := time.AfterFunc(10*time.Second, c.Terminate)
	defer guard.Stop()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
}
