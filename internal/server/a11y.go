package server

import (
	"fmt"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

// sessionAnnouncer is the session's screen-reader announcer.
type sessionAnnouncer interface {
	wlr.Announcer
	Close()
}

// configErrorAnnouncement is Niri's config error text, naming this binary.
const configErrorAnnouncement = "Failed to parse the config file. Please run sysc-compositor validate to see the errors."

// workspaceAnnouncement is Niri's refresh_a11y workspace text: when the
// focused workspace changes, "Workspace N" counting from 1 on its output,
// then its name. last is the workspace last announced, 0 for none.
func workspaceAnnouncement(workspaces []ipc.Workspace, last *uint64) (string, bool) {
	for _, w := range workspaces {
		if !w.IsFocused {
			continue
		}
		if w.ID == *last {
			return "", false
		}
		*last = w.ID
		text := fmt.Sprintf("Workspace %d", w.Idx)
		if w.Name != nil {
			text += " " + *w.Name
		}
		return text, true
	}
	*last = 0
	return "", false
}
