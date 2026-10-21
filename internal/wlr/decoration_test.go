package wlr

import (
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestPreferNoCSDAllowsClientSideDecorationRequest(t *testing.T) {
	c := newNestedWith(t, Options{PreferNoCSD: true})
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- testclient.RunDecorationModeSequence(c.SocketName())
		c.Terminate()
	}()
	time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-clientDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("xdg-decoration client did not finish")
	}
}
