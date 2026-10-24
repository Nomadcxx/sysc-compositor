package session

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// systemd refuses to start graphical-session.target by hand
// (RefuseManualStart=yes), so the session starts startTarget, which binds it, as
// niri.service does for Niri. Stopping is allowed, and is done on
// sessionTarget itself: a unit with Requisite= on it, such as sysc-shell.service,
// keeps it needed after startTarget stops. startTarget stops with it (BindsTo=).
const (
	startTarget   = "sysc-session.target"
	sessionTarget = "graphical-session.target"
)

var baseEnvironment = []string{"WAYLAND_DISPLAY", "NIRI_SOCKET", "SYSC_SOCKET", "XDG_CURRENT_DESKTOP", "XDG_SESSION_TYPE"}

type Environment struct {
	WaylandDisplay string
	NiriSocket     string
	SyscSocket     string
	Display        string
}

type savedEnvironment struct {
	value   string
	present bool
}

// Lifecycle publishes the compositor environment and owns graphical-session.target.
type Lifecycle struct {
	keys     []string
	previous map[string]savedEnvironment
	started  bool
	close    sync.Once
}

func Activate(environment Environment) (*Lifecycle, error) {
	if environment.WaylandDisplay == "" || environment.NiriSocket == "" || environment.SyscSocket == "" {
		return nil, fmt.Errorf("session: Wayland, Niri, and sysc sockets must all be available")
	}
	values := []struct{ key, value string }{
		{"WAYLAND_DISPLAY", environment.WaylandDisplay},
		{"NIRI_SOCKET", environment.NiriSocket},
		{"SYSC_SOCKET", environment.SyscSocket},
		{"XDG_CURRENT_DESKTOP", "niri:sysc"},
		{"XDG_SESSION_TYPE", "wayland"},
	}
	if environment.Display != "" {
		values = append(values, struct{ key, value string }{"DISPLAY", environment.Display})
	}
	lifecycle := &Lifecycle{previous: make(map[string]savedEnvironment, len(values))}
	assignments := make([]string, 0, len(values))
	for _, item := range values {
		old, present := os.LookupEnv(item.key)
		lifecycle.previous[item.key] = savedEnvironment{value: old, present: present}
		if err := os.Setenv(item.key, item.value); err != nil {
			lifecycle.restoreProcessEnvironment()
			return nil, fmt.Errorf("set %s: %w", item.key, err)
		}
		lifecycle.keys = append(lifecycle.keys, item.key)
		assignments = append(assignments, item.key+"="+item.value)
	}
	if err := run("dbus-update-activation-environment", assignments...); err != nil {
		lifecycle.clearPublishedEnvironment()
		lifecycle.restoreProcessEnvironment()
		return nil, err
	}
	if err := run("systemctl", append([]string{"--user", "import-environment"}, lifecycle.keys...)...); err != nil {
		lifecycle.clearPublishedEnvironment()
		lifecycle.restoreProcessEnvironment()
		return nil, err
	}
	if err := run("systemctl", "--user", "start", startTarget); err != nil {
		lifecycle.clearPublishedEnvironment()
		lifecycle.restoreProcessEnvironment()
		return nil, err
	}
	lifecycle.started = true
	return lifecycle, nil
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (l *Lifecycle) Close() {
	if l == nil {
		return
	}
	l.close.Do(func() {
		if l.started {
			// --no-block queues stop and returns without waiting for services that
			// may include this compositor process.
			if err := run("systemctl", "--user", "--no-block", "stop", sessionTarget); err != nil {
				warn("stop graphical session", err)
			}
		}
		l.clearPublishedEnvironment()
		l.restoreProcessEnvironment()
	})
}

func (l *Lifecycle) clearPublishedEnvironment() {
	if len(l.keys) == 0 {
		return
	}
	if err := run("systemctl", append([]string{"--user", "unset-environment"}, l.keys...)...); err != nil {
		warn("clear systemd environment", err)
	}
	empty := make([]string, len(l.keys))
	for i, key := range l.keys {
		empty[i] = key + "="
	}
	if err := run("dbus-update-activation-environment", empty...); err != nil {
		warn("clear D-Bus activation environment", err)
	}
}

func (l *Lifecycle) restoreProcessEnvironment() {
	for key, previous := range l.previous {
		if previous.present {
			_ = os.Setenv(key, previous.value)
		} else {
			_ = os.Unsetenv(key)
		}
	}
}

func warn(what string, err error) {
	fmt.Fprintf(os.Stderr, "sysc-compositor: %s: %v\n", what, err)
}
