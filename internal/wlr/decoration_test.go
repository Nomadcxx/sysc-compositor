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
	watchdog := time.AfterFunc(5*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
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

func TestPreferNoCSDExposesKDEServerDecoration(t *testing.T) {
	c := newNestedWith(t, Options{PreferNoCSD: true})
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- testclient.RunKDEServerDecorationSequence(c.SocketName())
		c.Terminate()
	}()
	watchdog := time.AfterFunc(5*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-clientDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("KDE server-decoration client did not finish")
	}
}

func TestReconfigureEnablesPreferNoCSDManagers(t *testing.T) {
	c := newNested(t)
	if err := c.Reconfigure(Options{PreferNoCSD: true}); err != nil {
		t.Fatal(err)
	}
	if c.xdgDecorationManager == nil || c.kdeDecorationManager == nil {
		t.Fatalf("decoration managers after enabling preference = xdg %p, kde %p", c.xdgDecorationManager, c.kdeDecorationManager)
	}
}

func TestPreferNoCSDSetsInitialTiledStateWithoutDecorationProtocol(t *testing.T) {
	c := newNestedWith(t, Options{PreferNoCSD: true})
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- testclient.RunPreferNoCSDTiledState(c.SocketName())
		c.Terminate()
	}()
	watchdog := time.AfterFunc(5*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-clientDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("undecorated client did not receive an initial configure")
	}
}
