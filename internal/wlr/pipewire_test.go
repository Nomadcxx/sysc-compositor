package wlr

import (
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestPipeWireProducesFramesAndSurvivesDaemonLoss(t *testing.T) {
	requireTools(t, "pipewire")
	c := newNested(t)
	dir := t.TempDir()
	t.Setenv("PIPEWIRE_RUNTIME_DIR", dir)
	t.Setenv("PIPEWIRE_REMOTE", "pipewire-0")
	config := filepath.Join(dir, "pipewire.conf")
	if err := os.WriteFile(config, []byte(privatePipeWireConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		daemon := tool("pipewire", "-c", config)
		daemon.Stderr = os.Stderr
		if err := daemon.Start(); err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = daemon.Process.Kill(); _ = daemon.Wait() }()
		for deadline := time.Now().Add(3 * time.Second); ; {
			if _, err := os.Stat(filepath.Join(dir, "pipewire-0")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Error("private PipeWire daemon did not create its socket")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		ready, lost := make(chan error, 1), make(chan error, 1)
		var before int64
		onCompositor(t, c, func() {
			before = LiveHandles()
			if err := c.startPipeWire(func(err error) { ready <- err }, func(err error) { lost <- err }); err != nil {
				t.Error(err)
			}
		})
		select {
		case err := <-ready:
			if err != nil {
				t.Error(err)
				return
			}
		case <-time.After(3 * time.Second):
			t.Error("PipeWire connection never completed")
			return
		}
		nodes := make(chan uint32, 1)
		var stream *pipeWireStream
		width, height := 64, 32
		onCompositor(t, c, func() {
			var err error
			stream, err = c.pipewire.newStream(64, 32, 60000, func() (*image.NRGBA, error) {
				icon := image.NewRGBA(image.Rect(0, 0, 2, 2))
				icon.SetRGBA(0, 0, color.RGBA{R: 23, G: 43, B: 59, A: 255})
				stream.pointer = &pipeWirePointer{Image: icon, Position: image.Pt(12, 10), Hotspot: image.Pt(1, 1)}
				img := image.NewNRGBA(image.Rect(0, 0, width, height))
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						img.SetNRGBA(x, y, color.NRGBA{R: 17, G: 71, B: 193, A: 255})
					}
				}
				return img, nil
			}, func(node uint32, active bool, err error) {
				if err != nil {
					t.Error(err)
				}
				if node != ^uint32(0) {
					select {
					case nodes <- node:
					default:
					}
				}
			})
			if err != nil {
				t.Error(err)
			}
		})
		var node uint32
		select {
		case node = <-nodes:
		case <-time.After(3 * time.Second):
			t.Error("PipeWire stream did not publish a node")
			return
		}
		consumer, err := testclient.NewPipeWireReceiver(node, 64, 32)
		if err != nil {
			t.Error(err)
			return
		}
		defer consumer.Close()
		var pts int64
		var sequence uint64
		for frameNumber := 0; frameNumber < 2; frameNumber++ {
			frame, err := consumer.NextFrame(3 * time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			if frame.Width != 64 || frame.Height != 32 || len(frame.Pixels) != 64*32*4 {
				t.Errorf("negotiated frame = %dx%d, %d bytes", frame.Width, frame.Height, len(frame.Pixels))
				return
			}
			if frame.RefreshMilliHz != 60000 {
				t.Errorf("initial refresh=%d", frame.RefreshMilliHz)
			}
			if got := [4]byte(frame.Pixels[:4]); got != [4]byte{17, 71, 193, 255} {
				t.Errorf("PipeWire pixel = %v", got)
			}
			if frame.PTS <= 0 || (frameNumber > 0 && (frame.PTS <= pts || frame.Sequence <= sequence)) {
				t.Errorf("nonmonotonic frame metadata: %+v", frame)
			}
			pts, sequence = frame.PTS, frame.Sequence
			if frame.Pointer == nil || frame.Pointer.Width != 2 || frame.Pointer.Height != 2 || frame.Pointer.Position != image.Pt(12, 10) || frame.Pointer.Hotspot != image.Pt(1, 1) || len(frame.Pointer.Pixels) != 16 || [4]byte(frame.Pointer.Pixels[:4]) != [4]byte{23, 43, 59, 255} {
				t.Errorf("pointer metadata = %+v", frame.Pointer)
			}
		}
		onCompositor(t, c, func() {
			width, height = 80, 40
			if err := stream.reconfigure(width, height, 30000); err != nil {
				t.Error(err)
			}
		})
		resized := false
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			frame, err := consumer.NextFrame(3 * time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			if frame.Width == 80 && frame.Height == 40 {
				if len(frame.Pixels) != 80*40*4 || frame.RefreshMilliHz != 30000 || [4]byte(frame.Pixels[:4]) != [4]byte{17, 71, 193, 255} {
					t.Errorf("renegotiated frame=%dx%d @%d", frame.Width, frame.Height, frame.RefreshMilliHz)
				}
				resized = true
				break
			}
		}
		if !resized {
			t.Error("consumer never received reconfigured dimensions")
		}
		// Keep the consumer and stream active: daemon loss must retire their
		// buffers and callbacks, not merely close an unused connection.
		_ = daemon.Process.Kill()
		select {
		case err := <-lost:
			if err == nil {
				t.Error("daemon loss returned no error")
			}
		case <-time.After(3 * time.Second):
			t.Error("PipeWire daemon loss was not reported")
		}
		onCompositor(t, c, func() {
			stream.close()
			stream.close()
			c.stopPipeWire()
			if c.pipewire != nil {
				t.Error("lost PipeWire connection remains live")
			}
			if got := LiveHandles(); got != before {
				t.Errorf("PipeWire handles = %d, want %d", got, before)
			}
		})
	})
}

func TestPipeWireRejectsInvalidFrameConstraints(t *testing.T) {
	p := &pipeWire{ready: true}
	frame := func() (*image.NRGBA, error) { return nil, nil }
	for _, tc := range []struct {
		width, height int
		rate          uint32
	}{
		{0, 1, 60000}, {1, 0, 60000}, {-1, 1, 60000}, {8192, 8192, 60000}, {1, 1, 0}, {1, 1, 1_000_001},
	} {
		if _, err := p.newStream(tc.width, tc.height, tc.rate, frame, nil); err == nil {
			t.Errorf("accepted %dx%d @%d", tc.width, tc.height, tc.rate)
		}
	}
	if _, err := p.newStream(1, 1, 60000, nil, nil); err == nil {
		t.Error("accepted nil frame callback")
	}
}

const privatePipeWireConfig = `
context.properties = { core.daemon = true core.name = pipewire-0 mem.warn-mlock = false }
context.spa-libs = { support.* = support/libspa-support }
context.modules = [
 { name = libpipewire-module-protocol-native }
 { name = libpipewire-module-client-node }
 { name = libpipewire-module-spa-node-factory }
 { name = libpipewire-module-adapter }
 { name = libpipewire-module-link-factory }
 { name = libpipewire-module-metadata }
 { name = libpipewire-module-access args = { access.force = unrestricted } }
]
`
