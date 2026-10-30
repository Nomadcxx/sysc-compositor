package wlr

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// runClient runs argv against the nested compositor until stop reports true
// for a line of the client's output, the client exits, or five seconds pass.
// It returns every line the client wrote to stdout and stderr; WAYLAND_DEBUG
// output arrives on stderr. The compositor runs on the calling thread
// meanwhile, so hooks such as viewMapped fire while it waits.
func runClient(t *testing.T, c *Compositor, argv []string, env []string, stop func(string) bool) []string {
	t.Helper()
	return runClientWithTimeout(t, c, argv, env, stop, 5*time.Second)
}

func runClientWithTimeout(t *testing.T, c *Compositor, argv []string, env []string, stop func(string) bool, timeout time.Duration) []string {
	t.Helper()
	path, err := exec.LookPath(argv[0])
	if err != nil {
		t.Skipf("%s unavailable: %v", argv[0], err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu    sync.Mutex
		lines []string
	)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
			if stop != nil && stop(line) {
				c.Terminate()
			}
		}
	}()
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName())
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		c.Terminate()
		close(exited)
	}()
	timer := time.AfterFunc(timeout, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	timer.Stop()
	_ = cmd.Process.Kill()
	<-exited
	// Let the reader drain what the client wrote before closing the pipe:
	// closing first lost a fast client's whole output. A child the client
	// left behind can hold the pipe open, so the wait is bounded.
	select {
	case <-readerDone:
	case <-time.After(time.Second):
	}
	_ = reader.Close()
	<-readerDone
	mu.Lock()
	defer mu.Unlock()
	return lines
}

var reInterface = regexp.MustCompile(`^interface: '([a-z0-9_]+)',\s+version:\s+(\d+),`)

// advertised parses wayland-info output into interface -> highest version,
// and reports whether wl_shm listed ARGB8888 (format 0).
func advertised(lines []string) (map[string]int, bool) {
	globals := map[string]int{}
	argb := false
	inShm := false
	for _, line := range lines {
		if m := reInterface.FindStringSubmatch(line); m != nil {
			v, _ := strconv.Atoi(m[2])
			globals[m[1]] = max(globals[m[1]], v)
			inShm = m[1] == "wl_shm"
			continue
		}
		if inShm && strings.TrimSpace(line) == "0 = 'AR24'" {
			argb = true
		}
	}
	return globals, argb
}

// TestRequiredGlobalsAdvertised is M1's advertisement half: every global the
// design (§7) and sysc-shell's requiredSingletons need, at Niri's versions.
func TestRequiredGlobalsAdvertised(t *testing.T) {
	c := newNested(t)
	globals, argb := advertised(runClient(t, c, []string{"wayland-info"}, nil, nil))
	want := map[string]int{
		"wl_compositor":                  6,
		"wl_subcompositor":               1,
		"wl_shm":                         2,
		"wl_seat":                        1,
		"wl_output":                      1,
		"wl_data_device_manager":         1,
		"xdg_wm_base":                    7,
		"xdg_activation_v1":              1,
		"zwlr_layer_shell_v1":            5,
		"wp_fractional_scale_manager_v1": 1,
		"wp_viewporter":                  1,
		"zwp_linux_dmabuf_v1":            4,
		"zxdg_output_manager_v1":         3,
	}
	for iface, min := range want {
		if got, ok := globals[iface]; !ok || got < min {
			t.Errorf("%s: advertised version %d (present %v), want at least %d", iface, got, ok, min)
		}
	}
	for _, exact := range []string{"xdg_wm_base", "zwlr_layer_shell_v1", "wl_shm"} {
		if globals[exact] != want[exact] {
			t.Errorf("%s at version %d, want %d as the host Niri", exact, globals[exact], want[exact])
		}
	}
	if !argb {
		t.Error("wl_shm does not list ARGB8888: sysc-shell refuses to start")
	}
}

// TestM10InputIdleGlobalsAdvertised lists the M10 globals that have landed.
// Each one comes with its handler and test (owner decision Q7), so a global
// is added here in the change that implements it.
func TestM10InputIdleGlobalsAdvertised(t *testing.T) {
	c := newNested(t)
	globals, _ := advertised(runClient(t, c, []string{"wayland-info"}, nil, nil))
	want := map[string]int{
		"ext_background_effect_manager_v1":          1,
		"ext_idle_notifier_v1":                      1,
		"ext_session_lock_manager_v1":               1,
		"wp_cursor_shape_manager_v1":                1,
		"zwp_idle_inhibit_manager_v1":               1,
		"zwp_keyboard_shortcuts_inhibit_manager_v1": 1,
		"zwp_pointer_constraints_v1":                1,
		"zwp_relative_pointer_manager_v1":           1,
		"zwp_text_input_manager_v3":                 1,
		"zwp_input_method_manager_v2":               1,
	}
	for iface, version := range want {
		if got, ok := globals[iface]; !ok || got < version {
			t.Errorf("%s: advertised version %d (present %v), want at least %d", iface, got, ok, version)
		}
	}
}
