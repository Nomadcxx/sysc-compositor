package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"testing"
)

func TestConfigErrorNotificationHonorsDisableAndClears(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	c.NotifyConfigLoadFailed(true)
	if c.configNotice.tree == nil || len(c.configNotice.panels) != len(c.sceneOutputs) {
		t.Fatal("config failure did not draw on each output")
	}
	c.NotifyConfigLoadFailed(false)
	if c.configNotice.tree != nil {
		t.Fatal("successful load did not clear config error")
	}
	c.configNotification.DisableFailed = true
	c.NotifyConfigLoadFailed(true)
	if c.configNotice.tree != nil {
		t.Fatal("disable-failed still drew a panel")
	}
}
