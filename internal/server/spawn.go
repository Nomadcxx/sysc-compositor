package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// spawner starts commands the way Niri does (src/utils/spawning.rs): stdio on
// /dev/null, a leading ~ expanded, and an environment that points at this
// compositor. Niri double-forks so the child is not its own; Go cannot fork
// safely, so the child is reaped by a goroutine instead. Go already restores
// the original RLIMIT_NOFILE in children.
type spawner struct {
	env []string
}

// newSpawner builds the child environment once: ours, with the nested Wayland
// socket and the niri socket set, NOTIFY_SOCKET and DISPLAY removed (no
// Xwayland yet), and the config's environment applied last. A nil value
// removes a variable, as `NAME null` does in Niri's `environment {}`.
func newSpawner(waylandDisplay, niriSocket string, configured map[string]*string) spawner {
	set := map[string]*string{}
	str := func(v string) *string { return &v }
	set["WAYLAND_DISPLAY"] = str(waylandDisplay)
	if niriSocket != "" {
		set["NIRI_SOCKET"] = str(niriSocket)
	}
	set["NOTIFY_SOCKET"], set["DISPLAY"] = nil, nil
	for k, v := range configured {
		set[k] = v
	}
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if _, ok := set[name]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range set {
		if v != nil {
			env = append(env, k+"="+*v)
		}
	}
	return spawner{env: env}
}

// spawn starts argv and returns once it is running or has failed to start.
func (s spawner) spawn(argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	name := expandHome(argv[0])
	cmd := exec.Command(name, argv[1:]...)
	cmd.Env = s.env
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
