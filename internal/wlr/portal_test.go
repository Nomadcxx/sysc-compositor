package wlr

import (
	"bytes"
	"context"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestPortalServiceConnectionUsesNativeClient(t *testing.T) {
	c := newNested(t)
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer syscall.Close(pair[1])
		endpoint := os.NewFile(uintptr(pair[0]), "service")
		defer endpoint.Close()
		onCompositor(t, c, func() {
			if err := c.AddServiceClient(pair[1]); err != nil {
				t.Error(err)
			}
			if len(c.serviceClients) != 1 {
				t.Errorf("service client count = %d", len(c.serviceClients))
			}
		})
		command := tool("wayland-info")
		command.ExtraFiles = []*os.File{endpoint}
		command.Env = append(command.Env, "WAYLAND_SOCKET=3")
		output, err := command.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "wl_compositor") {
			t.Errorf("service connection did not enumerate globals: %v\n%s", err, output)
		}
		endpoint.Close()
		onCompositor(t, c, func() {
			if len(c.serviceClients) != 0 {
				t.Errorf("service-client classification outlived client: %d", len(c.serviceClients))
			}
		})
	})
}

func TestPortalColorPickerReturnsPixelAndCancels(t *testing.T) {
	c := newNested(t)
	type result struct {
		rgb [3]float64
		err error
	}
	results := make(chan result, 4)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		var pointer *fakePointer
		var solid unsafe.Pointer
		defer onCompositor(t, c, func() {
			pointer.remove()
			if solid != nil {
				sceneFXNodeDestroy(solid)
			}
		})
		onCompositor(t, c, func() {
			pointer = c.addFakePointer()
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				solid = sceneFXCreateRect(unsafe.Pointer(&c.scene.tree), box.Dx(), box.Dy(), [4]float32{.25, .5, .75, 1})
				sceneFXNodeSetPosition(solid, box.Min.X, box.Min.Y)
				pointer.moveTo(float64(box.Min.X+box.Dx()/2), float64(box.Min.Y+box.Dy()/2))
				break
			}
			if _, err := c.PickColor(func(rgb [3]float64, err error) { results <- result{rgb, err} }); err != nil {
				t.Error(err)
			}
			pointer.button(evdevButtonLeft, true)
			pointer.button(evdevButtonLeft, false)
		})
		select {
		case got := <-results:
			if got.err != nil {
				t.Error(got.err)
			}
			for i, want := range [3]float64{.25, .5, .75} {
				if math.Abs(got.rgb[i]-want) > 1.0/255 {
					t.Errorf("picked RGB = %v, want controlled pixel [.25 .5 .75]", got.rgb)
				}
			}
		case <-time.After(3 * time.Second):
			t.Fatal("picker did not reply")
		}
		onCompositor(t, c, func() {
			cancel, err := c.PickColor(func(rgb [3]float64, err error) { results <- result{rgb, err} })
			if err != nil {
				t.Error(err)
				return
			}
			cancel()
			cancel()
		})
		select {
		case got := <-results:
			if got.err == nil {
				t.Fatal("cancelled pick succeeded")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancelled picker did not reply")
		}
		select {
		case <-results:
			t.Fatal("picker replied twice")
		default:
		}
	})
}

func TestPortalScreenshotReturnsWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.png")
	c := newNestedWith(t, Options{ScreenshotPath: &path})
	done := make(chan error, 1)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		onCompositor(t, c, func() {
			if err := c.ScreenshotScreenResult(context.Background(), false, func(_ string, err error) { done <- err }); err != nil {
				t.Error(err)
			}
		})
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("screenshot result missing")
		}
		// A directory cannot be replaced by PNG bytes; the correlated result must
		// report that error rather than waiting for a global screenshot event.
		onCompositor(t, c, func() {
			dir := filepath.Dir(path)
			c.screenshotPattern = &dir
			if err := c.ScreenshotScreenResult(context.Background(), false, func(_ string, err error) { done <- err }); err != nil {
				t.Error(err)
			}
		})
		select {
		case err := <-done:
			if err == nil {
				t.Error("PNG write failure reported success")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("failed screenshot result missing")
		}
	})
}

// TestPortalScreenshotCreatesThePatternDirectory: a portal screenshot to
// the default ~/Pictures/Screenshots pattern on a fresh account creates the
// directory, as Niri's save_screenshot does for a configured pattern.
func TestPortalScreenshotCreatesThePatternDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Pictures", "Screenshots", "capture.png")
	c := newNestedWith(t, Options{ScreenshotPath: &path})
	done := make(chan error, 1)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		onCompositor(t, c, func() {
			if err := c.ScreenshotScreenResult(context.Background(), false, func(_ string, err error) { done <- err }); err != nil {
				t.Error(err)
			}
		})
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("screenshot into a missing directory: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("screenshot result missing")
		}
		if _, err := os.Stat(path); err != nil {
			t.Error(err)
		}
	})
}

func TestPortalScreenshotFallbackPreservesClipboard(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	c := newNested(t)
	type result struct {
		id   int
		path string
		err  error
	}
	done := make(chan result, 2)
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		seed, err := encodePNG(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
		if err != nil {
			t.Fatal(err)
		}
		var solid unsafe.Pointer
		defer onCompositor(t, c, func() {
			if solid != nil {
				sceneFXNodeDestroy(solid)
			}
		})
		onCompositor(t, c, func() {
			if err := c.setPNGSelection(seed); err != nil {
				t.Error(err)
			}
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				solid = sceneFXCreateRect(unsafe.Pointer(&c.scene.tree), box.Dx(), box.Dy(), [4]float32{1, 0, 0, 1})
				sceneFXNodeSetPosition(solid, box.Min.X, box.Min.Y)
				break
			}
			if err := c.ScreenshotScreenResult(context.Background(), false, func(path string, err error) { done <- result{0, path, err} }); err != nil {
				t.Error(err)
			}
			sceneFXNodeDestroy(solid)
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				solid = sceneFXCreateRect(unsafe.Pointer(&c.scene.tree), box.Dx(), box.Dy(), [4]float32{0, 0, 1, 1})
				sceneFXNodeSetPosition(solid, box.Min.X, box.Min.Y)
				break
			}
			invalid := strings.Repeat("x", 3000)
			c.screenshotPattern = &invalid
			if err := c.ScreenshotScreenResult(context.Background(), false, func(path string, err error) { done <- result{1, path, err} }); err != nil {
				t.Error(err)
			}
		})
		paths := map[string]bool{}
		for range 2 {
			select {
			case got := <-done:
				if got.err != nil {
					t.Fatal(got.err)
				}
				if filepath.Dir(got.path) != os.TempDir() || !strings.HasPrefix(filepath.Base(got.path), "sysc-screenshot-") {
					t.Fatalf("unsafe fallback path %q", got.path)
				}
				if paths[got.path] {
					t.Fatalf("concurrent fallback requests reused %q", got.path)
				}
				paths[got.path] = true
				info, err := os.Stat(got.path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Errorf("fallback permissions = %o", info.Mode().Perm())
				}
				img := decodePNG(t, got.path)
				r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
				if (got.id == 0 && (r != 0xffff || g != 0 || b != 0)) || (got.id == 1 && (r != 0 || g != 0 || b != 0xffff)) {
					t.Errorf("request %d received wrong pixels: %x %x %x", got.id, r, g, b)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("fallback screenshot did not return")
			}
		}
		data, err := tool("wl-paste", "--type", "image/png").Output()
		if err != nil || !bytes.Equal(seed, data) {
			t.Errorf("portal screenshot changed clipboard: %v, PNG bytes %d -> %d", err, len(seed), len(data))
		}
	})
}

func TestPortalScreenshotRejectsMultipleOutputs(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		onCompositor(t, c, func() {
			if err := c.ScreenshotScreenResult(context.Background(), false, func(string, error) {}); err == nil {
				t.Error("portal screenshot silently selected one of multiple outputs")
			}
		})
	})
}
