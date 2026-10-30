package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// spawner starts commands the way Niri does (src/utils/spawning.rs): stdio on
// /dev/null, a leading ~ expanded, and an environment that points at this
// compositor. Niri double-forks so the child is not its own; Go cannot fork
// safely, so the child is reaped by a goroutine instead. Go already restores
// the original RLIMIT_NOFILE in children.
type spawner struct {
	environment *spawnEnvironment
}

type spawnEnvironment struct {
	mu  sync.RWMutex
	set map[string]*string
}

// newSpawner records what children get on top of this process's
// environment: the nested Wayland socket and the niri socket, NOTIFY_SOCKET
// removed, the reserved X11 display when available, and config environment last.
// A nil value removes a variable, as `NAME null` does in Niri's
// `environment {}`. The process environment itself is read at each spawn,
// so children see what --session sets after the spawner exists, as Niri's
// children inherit its environment.
func newSpawner(waylandDisplay, niriSocket string, configured map[string]*string, cursor config.Cursor, display string) spawner {
	return spawner{environment: &spawnEnvironment{set: spawnOverlay(waylandDisplay, niriSocket, configured, cursor, display)}}
}

func spawnOverlay(waylandDisplay, niriSocket string, configured map[string]*string, cursor config.Cursor, display string) map[string]*string {
	set := map[string]*string{}
	str := func(v string) *string { return &v }
	set["WAYLAND_DISPLAY"] = str(waylandDisplay)
	set["WAYLAND_SOCKET"] = nil
	set["XCURSOR_THEME"] = str(cursor.Theme)
	set["XCURSOR_SIZE"] = str(strconv.Itoa(cursor.Size))
	if niriSocket != "" {
		set["NIRI_SOCKET"] = str(niriSocket)
	}
	set["NOTIFY_SOCKET"], set["DISPLAY"] = nil, nil
	if display != "" {
		set["DISPLAY"] = str(display)
	}
	for k, v := range configured {
		set[k] = v
	}
	return set
}

func (s spawner) updateEnvironment(waylandDisplay, niriSocket string, configured map[string]*string, cursor config.Cursor, display string) {
	next := spawnOverlay(waylandDisplay, niriSocket, configured, cursor, display)
	s.environment.mu.Lock()
	s.environment.set = next
	s.environment.mu.Unlock()
}

func (s spawner) childEnvironment() []string {
	s.environment.mu.RLock()
	defer s.environment.mu.RUnlock()
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if _, ok := s.environment.set[name]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range s.environment.set {
		if v != nil {
			env = append(env, k+"="+*v)
		}
	}
	return env
}

// spawn starts argv and returns once it is running or has failed to start.
func (s spawner) spawn(argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	name := expandHome(argv[0])
	cmd := exec.Command(name, argv[1:]...)
	cmd.Env = s.childEnvironment()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s spawner) spawnSh(command string) error {
	return s.spawn([]string{"sh", "-c", command})
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
