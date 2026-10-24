package session

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
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

// Lifecycle publishes the compositor environment to systemd --user and D-Bus
// activation. graphical-session.target is not its to start: systemd refuses a
// manual start, and sysc-compositor.service binds it, as niri.service does.
type Lifecycle struct {
	keys     []string
	previous map[string]savedEnvironment
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
	return lifecycle, nil
}

// NotifyReady tells systemd the compositor is up, for Type=notify units. Like
// Niri, it does so in every mode; outside a notify service NOTIFY_SOCKET is
// unset and it does nothing.
func NotifyReady() error {
	path := os.Getenv("NOTIFY_SOCKET")
	if path == "" {
		return nil
	}
	// An abstract socket's leading '@' is mapped by the net package.
	conn, err := net.Dial("unixgram", path)
	if err != nil {
		return fmt.Errorf("notify systemd: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("READY=1")); err != nil {
		return fmt.Errorf("notify systemd: %w", err)
	}
	return nil
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
