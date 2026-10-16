package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

// niriSocketPath is where Niri puts its socket (src/ipc/server.rs:81).
func niriSocketPath(runtimeDir, display string, pid int) (string, error) {
	if !filepath.IsAbs(runtimeDir) {
		return "", errors.New("server: XDG_RUNTIME_DIR must be an absolute path")
	}
	return filepath.Join(runtimeDir, fmt.Sprintf("niri.%s.%d.sock", display, pid)), nil
}

// Version is what the Version request answers: ours, and the wire it speaks.
func Version() string {
	v := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			v = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				v += " " + s.Value[:7]
			}
		}
	}
	return "sysc-compositor " + v + " (niri-ipc 26.4.0)"
}

// niriHandler answers the niri-ipc requests M2 builds; every other request
// and action decodes and replies that it is not built yet.
type niriHandler struct {
	quit  func()
	spawn spawner
}

func (h niriHandler) handle(r ipc.Request) (ipc.Reply, func()) {
	switch r.Name {
	case "Version":
		return ipc.Reply{Ok: ipc.VersionResponse(Version())}, nil
	case "ReturnError":
		// Niri's own string (src/ipc/server.rs:273).
		return ipc.Reply{Err: "example compositor error"}, nil
	case "Action":
		return h.action(*r.Action)
	}
	return ipc.Unimplemented(r.Name), nil
}

func (h niriHandler) action(a ipc.Action) (ipc.Reply, func()) {
	var err error
	switch a.Name {
	case "Quit":
		// There is no confirmation dialog yet, so skip_confirmation changes
		// nothing. The reply goes out before the shutdown starts.
		return ipc.Reply{Ok: ipc.Handled}, h.quit
	case "Spawn":
		err = h.spawn.spawn(a.Command)
	case "SpawnSh":
		err = h.spawn.spawnSh(a.ShellCommand)
	default:
		return ipc.Unimplemented(a.Name), nil
	}
	// Niri spawns on a thread and only logs a failure; the request is
	// handled either way.
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: spawn: " + err.Error() + "\n")
	}
	return ipc.Reply{Ok: ipc.Handled}, nil
}

func (h niriHandler) serve(c *net.UnixConn) { ipc.ServeConn(c, h.handle) }
