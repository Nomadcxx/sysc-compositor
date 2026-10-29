package wlr

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestLegacyScreencopyReportsCastLifecycle(t *testing.T) {
	c := newNested(t)
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if err := client.ArmLegacy(false, false); err != nil {
			t.Fatal(err)
		}
		if err := waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			if casts := c.CastSnapshot(); len(casts) != 0 {
				t.Errorf("screenshot reported as cast: %+v", casts)
			}
		})
		if err := client.ArmLegacy(true, false); err != nil {
			t.Fatal(err)
		}
		if err := client.Sync(); err != nil {
			t.Fatal(err)
		}
		var cast ipc.Cast
		onCompositor(t, c, func() {
			casts := c.CastSnapshot()
			if len(casts) != 1 {
				t.Errorf("damage-copy cast missing: %+v", casts)
				return
			}
			cast = casts[0]
			if cast.Kind != ipc.CastKindWlrScreencopy || cast.SessionID == 0 || cast.StreamID == 0 || cast.PID == nil || *cast.PID != int32(os.Getpid()) || cast.Target.Output == nil || cast.PWNodeID != nil || !cast.IsActive || cast.IsDynamicTarget {
				t.Errorf("invalid legacy cast: %+v", cast)
			}
			*casts[0].Target.Output = "reader mutation"
			*casts[0].PID = -1
			fresh := c.CastSnapshot()
			if fresh[0].Target.Output == nil || *fresh[0].Target.Output == "reader mutation" || fresh[0].PID == nil || *fresh[0].PID != int32(os.Getpid()) {
				t.Error("cast snapshot aliases owner")
			}
		})
		if cast.StreamID == 0 {
			return
		}
		// Destroy the pending frame. Keep the manager alive: its activity must
		// remain visible until the inactivity timeout, as portal-wlr requires.
		if err := client.DestroyObjects(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			if casts := c.CastSnapshot(); len(casts) != 1 || casts[0].StreamID != cast.StreamID {
				t.Errorf("frame retirement discarded live manager activity: %+v", casts)
			}
		})
		// A second connection owns independent session and stream IDs.
		other, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if other != nil {
				other.Close()
			}
		}()
		if err := other.ArmLegacy(true, false); err != nil {
			t.Fatal(err)
		}
		if err := other.Sync(); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			casts := c.CastSnapshot()
			if len(casts) != 2 || casts[0].SessionID == casts[1].SessionID || casts[0].StreamID == casts[1].StreamID {
				t.Errorf("independent managers share cast identity: %+v", casts)
			}
		})
		other.Close()
		other = nil
		if casts := waitForCastCount(t, c, 1); casts[0].StreamID != cast.StreamID {
			t.Errorf("departure stopped wrong cast: %+v", casts)
		}
	})
}

func waitForCastCount(t *testing.T, c *Compositor, count int) []ipc.Cast {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); ; {
		var casts []ipc.Cast
		onCompositor(t, c, func() { casts = c.CastSnapshot() })
		if len(casts) == count {
			return casts
		}
		if time.Now().After(deadline) {
			t.Fatalf("cast count=%d, want %d: %+v", len(casts), count, casts)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLegacyCastExpiresAfterInactivity(t *testing.T) {
	c := newNested(t)
	removed := make(chan struct{}, 1)
	seen := false
	c.SetStateChanged(func() {
		if len(c.CastSnapshot()) > 0 {
			seen = true
		} else if seen {
			select {
			case removed <- struct{}{}:
			default:
			}
		}
	})
	done := make(chan struct{})
	guard := time.AfterFunc(18*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer close(done)
		defer c.Terminate()
		client, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		started := time.Now()
		if err := client.ArmLegacy(true, false); err != nil {
			t.Error(err)
			return
		}
		if err := waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Error(err)
			return
		}
		before := waitForCastCount(t, c, 1)[0]
		select {
		case <-removed:
			if time.Since(started) < 9*time.Second {
				t.Error("cast expired before the ten-second inactivity window")
			}
		case <-time.After(12 * time.Second):
			t.Error("idle manager retained cast after deadline")
			return
		}
		if err := client.ArmLegacy(true, false); err != nil {
			t.Error(err)
			return
		}
		if err := client.Sync(); err != nil {
			t.Error(err)
			return
		}
		after := waitForCastCount(t, c, 1)[0]
		if before.SessionID == after.SessionID || before.StreamID == after.StreamID {
			t.Error("activity after timeout reused expired IDs")
		}
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestLegacyCastQueueTracksHeadAndExpiry(t *testing.T) {
	c := &Compositor{capture: &captureProtocol{}}
	client := &legacyCaptureClient{c: c, managerAlive: true, pending: 3}
	defer client.stopCast()
	first := &legacyCapture{client: client, outputName: "DP-1"}
	second := &legacyCapture{client: client, outputName: "DP-2"}
	unarmed := &legacyCapture{client: client, outputName: "DP-3"}
	client.startCastFrame(first)
	before := *client.cast
	client.startCastFrame(second)
	if *client.cast.Target.Output != "DP-1" {
		t.Fatal("queued request replaced front output")
	}
	client.expireCast(time.Now().Add(time.Hour))
	if client.cast == nil {
		t.Fatal("outstanding frame expired")
	}
	first.finish()
	if client.cast == nil || *client.cast.Target.Output != "DP-2" || client.cast.StreamID != before.StreamID {
		t.Fatal("front completion lost stream identity or next target")
	}
	second.finish()
	deadline := client.deadline
	if delay := time.Until(deadline); delay < 9*time.Second || delay > legacyCastTimeout {
		t.Fatalf("inactivity timeout=%v", delay)
	}
	unarmed.finish()
	if !client.deadline.Equal(deadline) {
		t.Fatal("uncopied frame extended cast activity")
	}
	client.expireCast(deadline.Add(-time.Nanosecond))
	if client.cast == nil {
		t.Fatal("cast expired before deadline")
	}
	client.expireCast(deadline)
	if len(c.CastSnapshot()) != 0 {
		t.Fatal("expired cast remains in snapshot")
	}
	client.pending++
	third := &legacyCapture{client: client, outputName: "DP-3"}
	client.startCastFrame(third)
	if client.cast.SessionID == before.SessionID || client.cast.StreamID == before.StreamID {
		t.Fatal("new activity reused expired identity")
	}
	if client.cast.PID != nil {
		t.Fatal("unknown credentials gained a PID")
	}
	client.managerAlive = false
	if len(c.CastSnapshot()) != 1 {
		t.Fatal("manager retirement discarded unfinished frame")
	}
	third.finish()
	third.finish()
	if len(c.CastSnapshot()) != 0 || client.pending != 0 || client.timer != nil {
		t.Fatal("last frame did not retire destroyed manager")
	}
}

func TestLegacyCastFollowsOutputAndRetainsPendingFrames(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer headless.destroy()
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if err := client.ArmLegacy(true, false); err != nil {
			t.Fatal(err)
		}
		if err := waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Fatal(err)
		}
		first := waitForCastCount(t, c, 1)[0]
		if err := client.SelectOtherLegacyOutput(); err != nil {
			t.Fatal(err)
		}
		if err := client.ArmLegacy(true, false); err != nil {
			t.Fatal(err)
		}
		if err := waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
			t.Fatal(err)
		}
		second := waitForCastCount(t, c, 1)[0]
		if first.StreamID != second.StreamID || first.SessionID != second.SessionID || *first.Target.Output == *second.Target.Output {
			t.Fatal("output switch lost identity or retained old target")
		}
		if err := client.ArmLegacy(true, false); err != nil {
			t.Fatal(err)
		}
		if err := client.Sync(); err != nil {
			t.Fatal(err)
		}
		if state := client.State(); state.Ready != 2 {
			t.Fatalf("unchanged damage frame did not remain pending: %+v", state)
		}
		if err := client.DropLegacyManager(); err != nil {
			t.Fatal(err)
		}
		if casts := waitForCastCount(t, c, 1); casts[0].StreamID != first.StreamID {
			t.Fatal("manager destruction retired pending frame")
		}
		onCompositor(t, c, headless.destroy)
		if err := waitCapture(client, func(s testclient.CaptureState) bool { return s.Failed == 1 }); err != nil {
			t.Fatal(err)
		}
		waitForCastCount(t, c, 0)
	})
}
