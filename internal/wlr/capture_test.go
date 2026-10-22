package wlr

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

// TestM11CaptureGlobalsAdvertised lists the capture and clipboard globals
// owner decision Q9 puts in M11.
func TestM11CaptureGlobalsAdvertised(t *testing.T) {
	c := newNested(t)
	globals, _ := advertised(runClient(t, c, []string{"wayland-info"}, nil, nil))
	for iface, version := range map[string]int{
		"ext_data_control_manager_v1":                1,
		"zwlr_data_control_manager_v1":               2,
		"zwlr_screencopy_manager_v1":                 3,
		"ext_image_copy_capture_manager_v1":          1,
		"ext_output_image_capture_source_manager_v1": 1,
		"wp_presentation":                            2,
		"wp_single_pixel_buffer_manager_v1":          1,
		"wp_alpha_modifier_v1":                       1,
	} {
		if got, ok := globals[iface]; !ok || got < version {
			t.Errorf("%s: advertised version %d (present %v), want at least %d", iface, got, ok, version)
		}
	}
}

// runTools runs steps against a running compositor and stops the compositor
// afterwards. Tools started through tool() get the compositor's socket and
// are killed once the compositor stops, so a stuck tool cannot hang the test.
func runTools(t *testing.T, c *Compositor, steps func(tool func(name string, args ...string) *exec.Cmd)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
	tool := func(name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		return cmd
	}
	watchdog := time.AfterFunc(10*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		steps(tool)
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
}

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
}

// TestDataControlServesTheSelectionWithoutFocus: a selection set by one
// client is read by another that never has keyboard focus, through
// ext-data-control, as sysc-clipboard does.
func TestDataControlServesTheSelectionWithoutFocus(t *testing.T) {
	requireTools(t, "wl-copy", "wl-paste")
	c := newNested(t)
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		// wl-copy serves the selection until replaced; in the foreground it
		// can be stopped, and it holds no pipe the test waits on.
		copyCmd := tool("wl-copy", "--foreground", "sysc-clipboard-probe")
		copyCmd.Env = append(copyCmd.Env, "WAYLAND_DEBUG=1")
		var copyErr strings.Builder
		copyCmd.Stderr = &copyErr
		if err := copyCmd.Start(); err != nil {
			t.Errorf("wl-copy: %v", err)
			return
		}
		defer func() { _ = copyCmd.Process.Kill(); _ = copyCmd.Wait() }()
		// Wait for wl-copy's offer rather than a fixed time.
		offered := false
		for deadline := time.Now().Add(3 * time.Second); !offered && time.Now().Before(deadline); {
			types, _ := tool("wl-paste", "--list-types").Output()
			offered = strings.Contains(string(types), "text/plain")
			if !offered {
				time.Sleep(50 * time.Millisecond)
			}
		}
		if !offered {
			log := copyErr.String()
			if len(log) > 4000 {
				log = log[:4000]
			}
			t.Errorf("wl-copy never offered the selection; its protocol log:\n%s", log)
			return
		}
		paste := tool("wl-paste", "--no-newline")
		paste.Env = append(paste.Env, "WAYLAND_DEBUG=1")
		var stderr strings.Builder
		paste.Stderr = &stderr
		out, err := paste.Output()
		if err != nil {
			t.Errorf("wl-paste: %v: %s", err, stderr.String())
			return
		}
		if string(out) != "sysc-clipboard-probe" {
			t.Errorf("wl-paste read %q, want the copied text", out)
		}
		if !strings.Contains(stderr.String(), "data_control") {
			t.Error("wl-paste did not use data control")
		}
	})
}

// TestScreencopyCapturesTheOutputAtItsSize: grim captures an output through
// the capture globals into a PNG of exactly the output's pixel size.
func TestScreencopyCapturesTheOutputAtItsSize(t *testing.T) {
	requireTools(t, "grim")
	c := newNested(t)
	outputs := c.OutputSnapshot()
	if len(outputs) == 0 || outputs[0].CurrentMode == nil {
		t.Fatal("no output with a current mode")
	}
	name := outputs[0].Name
	mode := outputs[0].Modes[*outputs[0].CurrentMode]
	path := filepath.Join(t.TempDir(), "capture.png")
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		grim := tool("grim", "-o", name, path)
		if out, err := grim.CombinedOutput(); err != nil {
			t.Errorf("grim: %v: %s", err, out)
		}
	})
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != int(mode.Width) || config.Height != int(mode.Height) {
		t.Errorf("capture is %dx%d, want the output's %dx%d", config.Width, config.Height, mode.Width, mode.Height)
	}
}

// TestScreenshotReadsBackAnOutputAndOffersItAsPNG: an output renders into a
// buffer of its own size and reads back; the PNG is offered as the user
// seat's clipboard and a data-control client reads the same bytes.
func TestScreenshotReadsBackAnOutputAndOffersItAsPNG(t *testing.T) {
	requireTools(t, "wl-paste")
	c := newNested(t)
	runTools(t, c, func(tool func(string, ...string) *exec.Cmd) {
		var data []byte
		onCompositor(t, c, func() {
			for pointer := range c.sceneOutputs {
				img, err := c.captureOutput(pointer)
				if err != nil {
					t.Error(err)
					return
				}
				w, h := outputPixelSize(pointer)
				if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
					t.Errorf("screenshot is %v, want the output's %dx%d", img.Bounds(), w, h)
				}
				if data, err = encodePNG(img); err != nil {
					t.Error(err)
					return
				}
				if err := c.setPNGSelection(data); err != nil {
					t.Error(err)
				}
				return
			}
		})
		if data == nil {
			return
		}
		out, err := tool("wl-paste", "--type", "image/png").Output()
		if err != nil {
			t.Errorf("wl-paste: %v", err)
			return
		}
		if !bytes.Equal(out, data) {
			t.Errorf("clipboard holds %d bytes, want the %d-byte screenshot", len(out), len(data))
		}
	})
}

// TestScreenshotPathExpandsStrftimeAndHome: the pattern goes through
// strftime on local time and a leading ~ becomes the home directory, as
// Niri's make_screenshot_path does. It builds no compositor.
func TestScreenshotPathExpandsStrftimeAndHome(t *testing.T) {
	t.Setenv("HOME", "/home/probe")
	got, err := expandScreenshotPath("~/Shots/%Y %%.png")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/home/probe/Shots", time.Now().Format("2006")+" %.png")
	if got != want {
		t.Errorf("expanded path = %q, want %q", got, want)
	}
	if got, _ := expandScreenshotPath("/abs/name.png"); got != "/abs/name.png" {
		t.Errorf("a path without specifiers changed to %q", got)
	}
}

// TestScreenshotScreenWritesThePNGAndReportsIt: ScreenshotScreen writes the
// focused output as a PNG to the expanded screenshot-path, creating its
// directory, and reports the path; with write-to-disk off it reports nil.
func TestScreenshotScreenWritesThePNGAndReportsIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shots")
	pattern := filepath.Join(dir, "shot-%Y.png")
	c := newNestedWith(t, Options{ScreenshotPath: &pattern})
	reports := make(chan *string, 2)
	c.SetScreenshotHandler(func(path *string) { reports <- path })
	var w, h int
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		onCompositor(t, c, func() {
			for pointer := range c.sceneOutputs {
				w, h = outputPixelSize(pointer)
			}
			if err := c.ScreenshotScreen(true, ""); err != nil {
				t.Error(err)
			}
		})
		select {
		case path := <-reports:
			want := filepath.Join(dir, "shot-"+time.Now().Format("2006")+".png")
			if path == nil || *path != want {
				t.Errorf("reported path %v, want %q", path, want)
			}
		case <-time.After(5 * time.Second):
			t.Error("no screenshot was reported")
			return
		}
		onCompositor(t, c, func() {
			if err := c.ScreenshotScreen(false, ""); err != nil {
				t.Error(err)
			}
		})
		select {
		case path := <-reports:
			if path != nil {
				t.Errorf("write-to-disk off reported %q, want nil", *path)
			}
		case <-time.After(5 * time.Second):
			t.Error("no report for the clipboard-only screenshot")
		}
	})
	file, err := os.Open(filepath.Join(dir, "shot-"+time.Now().Format("2006")+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != w || config.Height != h {
		t.Errorf("written screenshot is %dx%d, want %dx%d", config.Width, config.Height, w, h)
	}
}

// TestScreenshotWindowCapturesTheWindowAlone: the focused window is captured
// at its own geometry, not the output's.
func TestScreenshotWindowCapturesTheWindowAlone(t *testing.T) {
	dir := t.TempDir()
	c := newNested(t)
	reports := make(chan *string, 1)
	c.SetScreenshotHandler(func(path *string) { reports <- path })
	mapped := make(chan *toplevelView, 1)
	c.viewMapped = func(view *toplevelView) {
		select {
		case mapped <- view:
		default:
		}
	}
	path := filepath.Join(dir, "window.png")
	runTools(t, c, func(func(string, ...string) *exec.Cmd) {
		client, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		var view *toplevelView
		select {
		case view = <-mapped:
		case <-time.After(2 * time.Second):
			t.Error("the toplevel never mapped")
			return
		}
		onCompositor(t, c, func() {
			c.focusView(view)
			if err := c.ScreenshotWindow(nil, true, path); err != nil {
				t.Error(err)
			}
		})
		select {
		case got := <-reports:
			if got == nil || *got != path {
				t.Errorf("reported %v, want the explicit path %q", got, path)
			}
		case <-time.After(5 * time.Second):
			t.Error("no screenshot was reported")
		}
	})
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	// The test client maps a 200x150 buffer and sets no window geometry.
	if config.Width != 200 || config.Height != 150 {
		t.Errorf("window screenshot is %dx%d, want the window's 200x150", config.Width, config.Height)
	}
}
