package server

import (
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

func switchEventCommand(events config.SwitchEvents, event wlr.SwitchToggle) []string {
	switch event.Type {
	case wlr.SwitchLid:
		if event.On {
			return events.LidClose
		}
		return events.LidOpen
	case wlr.SwitchTabletMode:
		if event.On {
			return events.TabletModeOn
		}
		return events.TabletModeOff
	default:
		return nil
	}
}

// handleSwitchToggle runs on the compositor thread, where config and spawning
// are already owned. The compositor has already applied a lid change to its
// output policy.
func (s *Server) handleSwitchToggle(event wlr.SwitchToggle) {
	if command := switchEventCommand(s.cfg.SwitchEvents, event); len(command) != 0 {
		if err := s.spawn.spawn(command); err != nil {
			os.Stderr.WriteString("sysc-compositor: switch event spawn: " + err.Error() + "\n")
		}
	}
}
