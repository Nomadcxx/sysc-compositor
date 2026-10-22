package wlr

import (
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
