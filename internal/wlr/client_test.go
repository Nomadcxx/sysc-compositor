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
		close(exited)
		c.Terminate()
	}()
	timeout := time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	timeout.Stop()
	_ = cmd.Process.Kill()
	<-exited
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
		"zwlr_layer_shell_v1":            5,
		"wp_fractional_scale_manager_v1": 1,
		"wp_viewporter":                  1,
		"zwp_linux_dmabuf_v1":            4,
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
