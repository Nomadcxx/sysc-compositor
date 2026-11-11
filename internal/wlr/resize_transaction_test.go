package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestResizeTransactionsWaitForAllClientsAndThrottleSizes(t *testing.T) {
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}})
	runTouchClients(t, c, 2, func(clients []*testclient.ToplevelClient, views []*toplevelView) {
		onCompositor(t, c, func() {
			c.debugConfig.DisableTransactions = true
			c.debugConfig.DisableResizeThrottling = true
			c.model.FocusWindow(views[0].id)
			c.model.ConsumeWindowIntoColumn()
			c.applyWorkspaceModel()
		})
		for _, client := range clients {
			if err := client.AcknowledgeConfigure(); err != nil {
				t.Fatal(err)
			}
		}
		onCompositor(t, c, func() {
			c.debugConfig.DisableTransactions = false
			c.debugConfig.DisableResizeThrottling = false
			c.SetColumnWidth(layout.SizeChange{Fixed: 600})
		})
		for _, client := range clients {
			if err := client.SyncInput(); err != nil {
				t.Fatal(err)
			}
		}
		if err := clients[0].AcknowledgeConfigure(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			transaction := c.resizeTransaction
			if transaction == nil || len(transaction.members) != 2 {
				t.Fatalf("resize transaction=%+v", transaction)
			}
			if member := transaction.members[views[0]]; member == nil || !member.ready || member.locked == nil {
				t.Fatal("first commit was not held for the second client")
			}
		})
		if err := clients[1].AcknowledgeConfigure(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			if c.resizeTransaction != nil {
				t.Error("complete transaction retained a blocker")
			}
			c.debugConfig.DisableTransactions = true
			c.SetColumnWidth(layout.SizeChange{Fixed: 650})
		})
		if err := clients[0].SyncInput(); err != nil {
			t.Fatal(err)
		}
		before := clients[0].ConfigureCount()
		onCompositor(t, c, func() {
			c.SetColumnWidth(layout.SizeChange{Fixed: 660})
			c.SetColumnWidth(layout.SizeChange{Fixed: 670})
		})
		if err := clients[0].SyncInput(); err != nil {
			t.Fatal(err)
		}
		if got := clients[0].ConfigureCount(); got != before {
			t.Errorf("unacknowledged resize flooded client: %d -> %d", before, got)
		}
		if err := clients[0].AcknowledgeConfigure(); err != nil {
			t.Fatal(err)
		}
		if err := clients[0].SyncInput(); err != nil {
			t.Fatal(err)
		}
		if got := clients[0].ConfigureCount(); got <= before {
			t.Error("acknowledgement did not release the latest queued size")
		}
	})
}
