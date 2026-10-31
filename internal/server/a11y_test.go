package server

import (
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/a11y"
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

type announcements struct{ texts []string }

func (a *announcements) Announce(text string)             { a.texts = append(a.texts, text) }
func (a *announcements) Focus(a11y.Node)                  {}
func (a *announcements) SetRecentWindows(string, bool)    {}
func (a *announcements) SetDescription(a11y.Node, string) {}
func (a *announcements) Close()                           {}

// TestWorkspaceAnnouncementFollowsTheFocusedWorkspace is Niri's
// refresh_a11y: "Workspace N" and the name when the focused workspace
// changes, N counting from 1 on its output; nothing while it stays.
func TestWorkspaceAnnouncementFollowsTheFocusedWorkspace(t *testing.T) {
	name := "mail"
	workspaces := func(focused uint64) []ipc.Workspace {
		return []ipc.Workspace{
			{ID: 1, Idx: 1, IsFocused: focused == 1},
			{ID: 2, Idx: 2, Name: &name, IsFocused: focused == 2},
			{ID: 3, Idx: 1, IsFocused: focused == 3},
		}
	}
	var last uint64
	for _, step := range []struct {
		focused uint64
		want    string
	}{{1, "Workspace 1"}, {1, ""}, {2, "Workspace 2 mail"}, {3, "Workspace 1"}, {0, ""}, {3, "Workspace 1"}} {
		text, ok := workspaceAnnouncement(workspaces(step.focused), &last)
		if got := map[bool]string{true: text}[ok]; got != step.want {
			t.Errorf("focused %d: announced %q, want %q", step.focused, got, step.want)
		}
	}
}

// TestFailedConfigReloadIsAnnounced: a config that fails to load is
// announced, as Niri's config error notification is.
func TestFailedConfigReloadIsAnnounced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	writeConfigFile(t, path, "layout {\n    gaps 19\n}\n")
	loaded, err := config.LoadWithSources(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigReloadTestServer(t, loaded)
	heard := &announcements{}
	s.announcer = heard
	writeConfigFile(t, path, "include \"missing.kdl\"")
	candidate, loadErr := config.LoadWithSources(path)
	s.applyConfigReload(s.reloadSeq, reloadTarget{path: config.Path{Named: path}}, candidate, loadErr)
	want := "Failed to parse the config file. Please run sysc-compositor validate to see the errors."
	if len(heard.texts) != 1 || heard.texts[0] != want {
		t.Fatalf("announced %q, want %q", heard.texts, want)
	}
}
