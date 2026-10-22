package wlr

import (
	"bytes"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// runTools runs commands one after another against a running compositor,
// each with the compositor's socket, and stops the compositor afterwards.
func runTools(t *testing.T, c *Compositor, steps func(env []string)) {
	t.Helper()
	watchdog := time.AfterFunc(10*time.Second, c.Terminate)
	t.Cleanup(func() { watchdog.Stop() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		steps(append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName()))
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
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
	runTools(t, c, func(env []string) {
		copyCmd := exec.Command("wl-copy", "sysc-clipboard-probe")
		copyCmd.Env = env
		if out, err := copyCmd.CombinedOutput(); err != nil {
			t.Errorf("wl-copy: %v: %s", err, out)
			return
		}
		paste := exec.Command("wl-paste", "--no-newline")
		paste.Env = append(env, "WAYLAND_DEBUG=1")
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
	runTools(t, c, func(env []string) {
		grim := exec.Command("grim", "-o", name, path)
		grim.Env = env
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
	runTools(t, c, func(env []string) {
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
		paste := exec.Command("wl-paste", "--type", "image/png")
		paste.Env = env
		out, err := paste.Output()
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
	runTools(t, c, func([]string) {
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
