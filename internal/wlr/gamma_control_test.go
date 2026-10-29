package wlr

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestGammaControlRejectsUnsupportedOutput(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer headless.destroy()
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartGammaControl(c.SocketName(), 1)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if !client.Failed() || client.Size() != 0 {
			t.Fatalf("unsupported output claimed gamma size %d (failed=%v)", client.Size(), client.Failed())
		}
		// An inert failed control must safely consume the supplied descriptor.
		file, err := os.CreateTemp(t.TempDir(), "gamma")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := client.SetGamma(int(file.Fd())); err != nil {
			t.Fatal(err)
		}
		if err := client.DestroyControl(); err != nil {
			t.Fatal(err)
		}
	})
}

type gammaCommit struct {
	rgb   [3]float64
	reset bool
}

func gammaTarget(t *testing.T, c *Compositor) (unsafe.Pointer, int) {
	t.Helper()
	if len(c.sceneOutputs) != 1 {
		t.Fatal("gamma check requires one initial output")
	}
	for output := range c.sceneOutputs {
		size := gammaSizeForTest(output)
		if size == 0 {
			t.Skip("backend has no gamma LUT; unsupported behavior has its own check")
		}
		if size < 2 || size > 65536 {
			t.Fatalf("unexpected gamma table size %d", size)
		}
		return output, size
	}
	t.Fatal("output missing")
	return nil, 0
}

func gammaObserver(t *testing.T, c *Compositor, output unsafe.Pointer) (<-chan gammaCommit, func()) {
	t.Helper()
	commits := make(chan gammaCommit, 16)
	listener := observeGammaCommitsForTest(output, func(rgb [3]float64, reset bool) { commits <- gammaCommit{rgb, reset} })
	c.keep(listener)
	return commits, func() { c.drop(listener) }
}

func expectGammaCommit(t *testing.T, commits <-chan gammaCommit, reset bool) {
	t.Helper()
	select {
	case commit := <-commits:
		if commit.reset != reset {
			t.Fatalf("gamma reset=%v, want %v", commit.reset, reset)
		}
		if !reset {
			for i, want := range [3]float64{.375, .25, .125} {
				if math.Abs(commit.rgb[i]-want) > 2.0/65535 {
					t.Fatalf("committed ramp evaluates to %v", commit.rgb)
				}
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("output did not commit the requested gamma state")
	}
}

func gammaRampFile(t *testing.T, size int) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "gamma-ramp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	table := make([]uint16, size*3)
	for channel, scale := range [3]float64{.75, .5, .25} {
		for i := 0; i < size; i++ {
			table[channel*size+i] = uint16(float64(i) * 65535 * scale / float64(size-1))
		}
	}
	if err := binary.Write(file, binary.NativeEndian, table); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestGammaControlAppliesAndRestoresOutputRamp(t *testing.T) {
	c := newNested(t)
	output, size := gammaTarget(t, c)
	commits, stop := gammaObserver(t, c, output)
	defer stop()
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if client.Failed() || int(client.Size()) != size {
			t.Fatalf("gamma size=%d failed=%v, want %d", client.Size(), client.Failed(), size)
		}
		file := gammaRampFile(t, size)
		if err := client.SetGamma(int(file.Fd())); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, false)
		other, err := testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		if !other.Failed() || other.Size() != 0 {
			t.Error("second client gained exclusive gamma control")
		}
		other.Close()
		onCompositor(t, c, func() {
			if err := c.PowerOffMonitors(); err != nil {
				t.Error(err)
			}
			if err := c.PowerOnMonitors(); err != nil {
				t.Error(err)
			}
		})
		expectGammaCommit(t, commits, false)
		if err := client.DestroyControl(); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, true)
		client.Close()
		client, err = testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if client.Failed() {
			t.Fatal("destroyed control retained ownership")
		}
		if err := client.SetGamma(int(file.Fd())); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, false)
		client.Close()
		expectGammaCommit(t, commits, true)
	})
}

func TestGammaCapturePreservesPendingRamp(t *testing.T) {
	c := newNested(t)
	output, size := gammaTarget(t, c)
	commits, stop := gammaObserver(t, c, output)
	defer stop()
	captured := false
	request := observeGammaRequestsForTest(c, func() {
		if captured {
			return
		}
		captured = true
		if _, err := c.captureOutput(output); err != nil {
			t.Error(err)
		}
	})
	if request == nil {
		t.Fatal("gamma integration missing")
	}
	c.keep(request)
	defer c.drop(request)
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		file := gammaRampFile(t, size)
		if err := client.SetGamma(int(file.Fd())); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, false)
		if err := client.DestroyControl(); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, true)
	})
	if !captured {
		t.Fatal("gamma request did not run capture before output commit")
	}
}

func TestGammaControlRejectsInvalidRampsAndReleasesOwnership(t *testing.T) {
	c := newNested(t)
	output, size := gammaTarget(t, c)
	commits, stop := gammaObserver(t, c, output)
	defer stop()
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		file := gammaRampFile(t, size)
		if err := client.SetGamma(int(file.Fd())); err != nil {
			t.Fatal(err)
		}
		expectGammaCommit(t, commits, false)
		if err := file.Truncate(2); err != nil {
			t.Fatal(err)
		}
		if err := client.SetGamma(int(file.Fd())); err == nil {
			t.Fatal("short gamma table accepted")
		}
		if name, code := client.ProtocolError(); name != "zwlr_gamma_control_v1" || code != 1 {
			t.Fatalf("invalid ramp error=%s/%d", name, code)
		}
		expectGammaCommit(t, commits, true)
		client.Close()
		client, err = testclient.StartGammaControl(c.SocketName(), 0)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if client.Failed() {
			t.Fatal("malformed ramp retained exclusive ownership")
		}
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer read.Close()
		defer write.Close()
		if err := client.SetGamma(int(read.Fd())); err != nil {
			t.Fatal(err)
		}
		if !client.Failed() {
			t.Fatal("non-seekable gamma descriptor accepted")
		}
		expectGammaCommit(t, commits, true)
	})
}
