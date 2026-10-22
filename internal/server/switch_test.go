package server

import (
	"reflect"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr"
)

func TestSwitchEventCommandSelection(t *testing.T) {
	events := config.SwitchEvents{
		LidOpen: []string{"lid-open"}, LidClose: []string{"lid-close"},
		TabletModeOn: []string{"tablet", "on"}, TabletModeOff: []string{"tablet", "off"},
	}
	for _, tc := range []struct {
		event wlr.SwitchToggle
		want  []string
	}{
		{wlr.SwitchToggle{Type: wlr.SwitchLid}, events.LidOpen},
		{wlr.SwitchToggle{Type: wlr.SwitchLid, On: true}, events.LidClose},
		{wlr.SwitchToggle{Type: wlr.SwitchTabletMode}, events.TabletModeOff},
		{wlr.SwitchToggle{Type: wlr.SwitchTabletMode, On: true}, events.TabletModeOn},
	} {
		if got := switchEventCommand(events, tc.event); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("switchEventCommand(%+v) = %q, want %q", tc.event, got, tc.want)
		}
	}

	s := &Server{cfg: config.Default()}
	s.handleSwitchToggle(wlr.SwitchToggle{Type: wlr.SwitchLid, On: true})
	if !s.lidClosed {
		t.Fatal("hardware lid event did not update lid state")
	}
	s.setLidClosed(false)
	if s.lidClosed {
		t.Fatal("login1 lid update did not replace the current lid state")
	}
}
