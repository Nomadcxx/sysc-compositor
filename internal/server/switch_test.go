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
}
