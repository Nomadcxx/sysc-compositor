package server

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestX11SatelliteMapsAndSharesNativeFocus(t *testing.T) {
	if os.Getenv("SYSC_NESTED_TESTS") != "1" {
		t.Skip("requires consent for this nested run")
	}
	for _, name := range []string{"xwayland-satellite", "Xwayland", "xmessage"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("X11 gate requires %s: %v", name, err)
		}
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg := config.Default()
	cfg.Animations.Off = true
	cfg.HotkeyOverlay.SkipAtStartup = true
	s, err := New(config.LoadResult{Config: cfg}, config.Path{}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Destroy()
	if s.Display() == "" {
		t.Fatal("satellite has no reserved display")
	}
	guard := time.AfterFunc(20*time.Second, s.Terminate)
	defer guard.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer s.Terminate()
		native, err := testclient.StartToplevelClient(s.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer native.Close()
		if err := native.SetIdentity("sysc-native-test", "sysc-native-test"); err != nil {
			t.Error(err)
			return
		}
		window := func(title string, focused bool) uint64 {
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
				for _, w := range s.state.snapshot().windows {
					if w.Title != nil && *w.Title == title && (!focused || w.IsFocused) {
						return w.ID
					}
				}
			}
			t.Errorf("window %q never appeared (focused=%v)", title, focused)
			return 0
		}
		nativeID := window("sysc-native-test", true)
		if nativeID == 0 {
			return
		}
		cmd := exec.Command("xmessage", "-title", "sysc-x11-test", "-buttons", "OK:0", "X11 gate")
		cmd.Env = s.spawn.childEnvironment()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Error(err)
			return
		}
		defer func() {
			cmd.Process.Kill()
			cmd.Wait()
			if stderr.Len() != 0 {
				t.Log(stderr.String())
			}
		}()
		xID := window("sysc-x11-test", false)
		if xID == 0 {
			return
		}
		onThread := func(fn func()) bool {
			finished := make(chan struct{})
			if err := s.c.Post(func() { fn(); close(finished) }); err != nil {
				t.Error(err)
				return false
			}
			select {
			case <-finished:
				return true
			case <-time.After(time.Second):
				t.Error("compositor stopped answering posted work")
				return false
			}
		}
		for _, target := range []struct {
			id    uint64
			title string
		}{{xID, "sysc-x11-test"}, {nativeID, "sysc-native-test"}, {xID, "sysc-x11-test"}} {
			if !onThread(func() {
				if !s.c.FocusWindow(target.id) {
					t.Error("focus rejected mapped window")
				}
			}) {
				return
			}
			if window(target.title, true) == 0 {
				return
			}
		}
		if !onThread(func() {
			if !s.c.CloseWindow(&xID) {
				t.Error("close rejected X11 window")
			}
		}) {
			return
		}
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			present := false
			for _, w := range s.state.snapshot().windows {
				present = present || w.ID == xID
			}
			if !present {
				return
			}
		}
		t.Error("closed X11 window stayed mapped")
	}()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	<-done
}
