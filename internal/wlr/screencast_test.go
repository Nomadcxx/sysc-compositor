package wlr

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestCastIPCTracksDynamicTargetAndStop(t *testing.T) {
	requireTools(t, "pipewire")
	c := newNested(t)
	dir := t.TempDir()
	t.Setenv("PIPEWIRE_RUNTIME_DIR", dir)
	t.Setenv("PIPEWIRE_REMOTE", "pipewire-0")
	pwConfig := filepath.Join(dir, "pipewire.conf")
	if err := os.WriteFile(pwConfig, []byte(privatePipeWireConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		daemon := tool("pipewire", "-c", pwConfig)
		if err := daemon.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = daemon.Process.Kill(); _ = daemon.Wait() }()
		for deadline := time.Now().Add(3 * time.Second); ; {
			if _, err := os.Stat(filepath.Join(dir, "pipewire-0")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("private PipeWire socket missing")
			}
			time.Sleep(10 * time.Millisecond)
		}
		ready, lost := make(chan error, 1), make(chan error, 1)
		onCompositor(t, c, func() {
			if err := c.StartScreenCast(func(err error) { ready <- err }, func(err error) { lost <- err }); err != nil {
				t.Error(err)
			}
		})
		select {
		case err := <-ready:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("PipeWire not ready")
		}
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		defer window.Close()
		if err := window.SetIdentity("cast", "cast"); err != nil {
			t.Fatal(err)
		}
		if err := window.Paint(0xff3498db, 200, 150); err != nil {
			t.Fatal(err)
		}
		var id uint64
		nodes := make(chan uint32, 8)
		closed := make(chan struct{}, 8)
		onCompositor(t, c, func() {
			for _, view := range c.views {
				if cString(view.toplevel.app_id) == "cast" {
					id = view.id
				}
			}
			if id == 0 {
				t.Error("test window missing")
				return
			}
			dynamic := c.DynamicCastWindowID()
			if dynamic == 0 || dynamic == id {
				t.Error("invalid synthetic ID")
			}
			if err := c.StartCast(CastOptions{SessionID: 1, StreamID: 1, Target: ipc.CastTarget{WindowID: &dynamic}, NodeAdded: func(node uint32) { nodes <- node }, Closed: func() { closed <- struct{}{} }}); err != nil {
				t.Error(err)
			}
			pending := c.CastSnapshot()
			if len(pending) != 1 || pending[0].PWNodeID != nil || pending[0].Target.WindowID != nil || !pending[0].IsDynamicTarget {
				t.Errorf("pending state: %+v", pending)
			}
			c.SetDynamicCastWindow(&id)
		})
		var node uint32
		select {
		case node = <-nodes:
		case <-time.After(3 * time.Second):
			t.Fatal("dynamic target did not create node")
		}
		consumer, err := testclient.NewPipeWireReceiver(node, 200, 150)
		if err != nil {
			t.Fatal(err)
		}
		defer consumer.Close()
		assertPixel := func(w, h int, pixel [4]byte) {
			t.Helper()
			// Frames already queued before an owner change are valid. Require a
			// producer checkpoint before inspecting the changed target's pixels.
			var sequence uint64
			onCompositor(t, c, func() { sequence = uint64(c.casts[1].stream.native.sequence) })
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
				frame, err := consumer.NextFrame(3 * time.Second)
				if err != nil {
					t.Fatal(err)
				}
				if frame.Sequence < sequence || frame.Width != w || frame.Height != h {
					continue
				}
				if got := [4]byte(frame.Pixels[:4]); got != pixel {
					t.Fatalf("cast pixel=%v, want %v", got, pixel)
				}
				if frame.Pointer != nil && (frame.Pointer.Width != 1 || frame.Pointer.Height != 1 || frame.Pointer.Pixels[3] != 0) {
					t.Fatal("hidden pointer leaked")
				}
				return
			}
			t.Fatal("cast did not reach checkpoint")
		}
		assertPixel(200, 150, [4]byte{0x34, 0x98, 0xdb, 255})
		onCompositor(t, c, func() {
			dynamic := c.DynamicCastWindowID()
			if err := c.StartCast(CastOptions{SessionID: 4, StreamID: 4, Target: ipc.CastTarget{WindowID: &dynamic}}); err != nil {
				t.Error(err)
			}
			invalid := ^uint64(0)
			c.SetDynamicCastWindow(&invalid)
			if target := c.casts[1].state.Target.WindowID; target == nil || *target != id {
				t.Error("stale ID changed active target")
			}
			if c.casts[4] == nil {
				t.Error("stale ID stopped pending session")
			}
			c.StopCast(4)
		})
		onCompositor(t, c, func() {
			snapshot := c.CastSnapshot()
			if len(snapshot) != 1 || snapshot[0].Target.WindowID == nil || *snapshot[0].Target.WindowID != id || snapshot[0].PWNodeID == nil || !snapshot[0].IsActive {
				t.Errorf("active cast=%+v", snapshot)
			}
			*snapshot[0].Target.WindowID = 999
			if *c.CastSnapshot()[0].Target.WindowID != id {
				t.Error("mutable cast snapshot")
			}
			c.ClearDynamicCastTarget()
		})
		assertPixel(200, 150, [4]byte{0, 0, 0, 255})
		onCompositor(t, c, func() { c.SetDynamicCastWindow(&id) })
		assertPixel(200, 150, [4]byte{0x34, 0x98, 0xdb, 255})
		if err := window.Paint(0xff3498db, 240, 180); err != nil {
			t.Fatal(err)
		}
		assertPixel(240, 180, [4]byte{0x34, 0x98, 0xdb, 255})
		onCompositor(t, c, func() {
			blocked := "screencast"
			if err := c.Reconfigure(Options{WindowRules: []config.WindowRule{{BlockOutFrom: &blocked}}}); err != nil {
				t.Error(err)
			}
		})
		assertPixel(240, 180, [4]byte{0, 0, 0, 255})
		onCompositor(t, c, func() {
			if err := c.Reconfigure(Options{}); err != nil {
				t.Error(err)
			}
			c.setSessionLocked(true)
		})
		assertPixel(240, 180, [4]byte{0, 0, 0, 255})
		onCompositor(t, c, func() { c.setSessionLocked(false) })
		assertPixel(240, 180, [4]byte{0x34, 0x98, 0xdb, 255})
		// A normal cast and a dynamic cast of the same mapping have different
		// retirement rules: stop the normal session, clear the dynamic target.
		onCompositor(t, c, func() {
			if err := c.StartCast(CastOptions{SessionID: 3, StreamID: 3, Target: ipc.CastTarget{WindowID: &id}}); err != nil {
				t.Error(err)
			}
		})
		window.Close()
		onCompositor(t, c, func() {
			c.syncCasts()
			if _, exists := c.casts[3]; exists {
				t.Error("normal cast retained retired mapping")
			}
			if cast := c.casts[1]; cast == nil || cast.state.Target.WindowID != nil {
				t.Error("dynamic target not cleared on unmap")
			}
		})
		assertPixel(240, 180, [4]byte{0, 0, 0, 255})
		var outputFill unsafe.Pointer
		var outputWidth, outputHeight int
		defer onCompositor(t, c, func() {
			if outputFill != nil {
				sceneFXNodeDestroy(outputFill)
			}
		})
		onCompositor(t, c, func() {
			name := c.FocusedOutputName()
			output := c.outputByName(name)
			box := c.outputLayoutBox(output)
			outputFill = sceneFXCreateRect(unsafe.Pointer(&c.scene.tree), box.Dx(), box.Dy(), [4]float32{.2, .4, .6, 1})
			sceneFXNodeSetPosition(outputFill, box.Min.X, box.Min.Y)
			c.SetDynamicCastMonitor(&name)
			outputWidth, outputHeight, _, _, _, _ = c.casts[1].target()
		})
		assertPixel(outputWidth, outputHeight, [4]byte{51, 102, 153, 255})
		onCompositor(t, c, func() {
			dynamic := c.DynamicCastWindowID()
			if err := c.StartCast(CastOptions{SessionID: 2, StreamID: 2, Target: ipc.CastTarget{WindowID: &dynamic}}); err != nil {
				t.Error(err)
			}
			if c.CastSnapshot()[1].PWNodeID != nil {
				t.Error("new dynamic cast inherited earlier selection")
			}
			c.StopCast(1)
			c.StopCast(1)
			if got := c.CastSnapshot(); len(got) != 1 || got[0].SessionID != 2 {
				t.Errorf("stop removed wrong session: %+v", got)
			}
		})
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("stop did not close session")
		}
		_ = daemon.Process.Kill()
		select {
		case err := <-lost:
			if err == nil {
				t.Error("daemon loss has no error")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("daemon loss not reported")
		}
		onCompositor(t, c, func() {
			if len(c.CastSnapshot()) != 0 {
				t.Error("daemon loss retained pending cast")
			}
		})
	})
}

func TestCastFramesNormalizeOutputTransforms(t *testing.T) {
	buffer := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		buffer.Pix[i*4], buffer.Pix[i*4+3] = byte('A'+i), 255
	}
	for transform, want := range []string{"ABCDEF", "DAEBFC", "FEDCBA", "CFBEAD", "CBAFED", "FCEBDA", "DEFABC", "ADBECF"} {
		img := uprightCastFrame(buffer, uint32(transform))
		var got []byte
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				got = append(got, img.NRGBAAt(x, y).R)
			}
		}
		if string(got) != want {
			t.Errorf("transform %d: got %s want %s", transform, got, want)
		}
	}
}

func TestWindowCastPointerFollowsHitAndGrabFocus(t *testing.T) {
	c := newNested(t)
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		defer window.Close()
		if err := window.Paint(0xff3498db, 200, 150); err != nil {
			t.Fatal(err)
		}
		var view *toplevelView
		var pointer *fakePointer
		var cast *screenCast
		var overlay unsafe.Pointer
		defer onCompositor(t, c, func() { pointer.remove() })
		onCompositor(t, c, func() {
			for _, candidate := range c.views {
				view = candidate
				break
			}
			pointer = c.addFakePointer()
			x, y := view.treePosition()
			pointer.moveTo(float64(x+40), float64(y+40))
			c.showDefaultImage()
			cast = &screenCast{c: c, state: ipc.Cast{Target: ipc.CastTarget{WindowID: &view.id}}, options: CastOptions{CursorMode: 2}}
			w, h, rate, _, _, _ := cast.target()
			cast.stream = &pipeWireStream{width: w, height: h, refreshMilliHz: rate}
		})
		check := func(visible bool) {
			t.Helper()
			onCompositor(t, c, func() {
				cast.options.CursorMode = 2
				if _, err := cast.frame(); err != nil {
					t.Error(err)
					return
				}
				if (cast.stream.pointer != nil) != visible {
					t.Errorf("metadata pointer visibility=%v, want %v", cast.stream.pointer != nil, visible)
				}
				cast.options.CursorMode = 1
				img, err := cast.frame()
				if err != nil {
					t.Error(err)
					return
				}
				changed := false
				for i := 0; i < len(img.Pix); i += 4 {
					if [4]byte(img.Pix[i:i+4]) != [4]byte{0x34, 0x98, 0xdb, 255} {
						changed = true
						break
					}
				}
				if changed != visible {
					t.Errorf("embedded pointer visibility=%v, want %v", changed, visible)
				}
			})
		}
		check(true)
		onCompositor(t, c, func() {
			// A compositor grab clears client focus while geometry still hits.
			c.startPointerGrab(view, grabMove, 0, 0)
		})
		check(false)
		onCompositor(t, c, func() { c.clearPointerGrab(); c.refocusPointer(0); c.showDefaultImage() })
		check(true)
		other, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		if err := other.Paint(0xff112233, 200, 150); err != nil {
			t.Fatal(err)
		}
		onCompositor(t, c, func() {
			x, y := view.treePosition()
			for _, candidate := range c.views {
				if candidate != view {
					overlay = unsafe.Pointer(&candidate.tree.node)
					sceneFXNodeSetPosition(overlay, x, y)
				}
			}
			pointer.moveTo(float64(x+40), float64(y+40))
			c.showDefaultImage()
		})
		check(false)
		onCompositor(t, c, func() {
			sceneFXSetNodeEnabled(overlay, false)
			sceneFXSetNodeEnabled(unsafe.Pointer(&view.tree.node), false)
			c.refocusPointer(0)
		})
		check(false)
	})
}
