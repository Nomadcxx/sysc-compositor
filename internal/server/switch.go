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
// are already owned.
func (s *Server) handleSwitchToggle(event wlr.SwitchToggle) {
	if event.Type == wlr.SwitchLid {
		s.lidClosed = event.On
	}
	if command := switchEventCommand(s.cfg.SwitchEvents, event); len(command) != 0 {
		if err := s.spawn.spawn(command); err != nil {
			os.Stderr.WriteString("sysc-compositor: switch event spawn: " + err.Error() + "\n")
		}
	}
}

// setLidClosed receives login1 state on the compositor thread. Hardware switch
// events remain the other source because either source can miss a transition.
func (s *Server) setLidClosed(closed bool) { s.lidClosed = closed }
