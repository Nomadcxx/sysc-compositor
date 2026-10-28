package wlr

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestNativeIBusComposesInGTK uses packaged clients rather than a protocol
// double. GTK must use text-input-v3, so its D-Bus IBus module is disabled.
// All bus, engine and configuration state belongs to this test.
func TestNativeIBusComposesInGTK(t *testing.T) {
	requireTools(t, "dbus-daemon", "ibus-daemon", "python3", "/usr/lib/ibus/ibus-wayland")
	if out, err := exec.Command("python3", "-c", "import gi; gi.require_version('Gtk', '4.0'); gi.require_version('IBus', '1.0'); from gi.repository import Gtk, IBus").CombinedOutput(); err != nil {
		t.Skipf("GTK/IBus introspection unavailable: %v: %s", err, out)
	}
	c := newNested(t)
	kb := attachFakeKeyboard(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	watchdog := time.AfterFunc(25*time.Second, c.Terminate)
	defer watchdog.Stop()
	dir := t.TempDir()
	env := append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName(), "GDK_BACKEND=wayland",
		"IBUS_ENABLE_CTRL_SHIFT_U=1", "IBUS_ENABLE_CONTROL_SHIFT_U=1",
		"GIO_USE_VFS=local", "GSETTINGS_BACKEND=memory", "GTK_A11Y=none", "NO_AT_BRIDGE=1", "GDK_DEBUG=no-portals",
		"GTK_IM_MODULE=wayland", "XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		"XDG_CACHE_HOME="+filepath.Join(dir, "cache"), "IBUS_ADDRESS=unix:path="+filepath.Join(dir, "ibus"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Terminate()
		protocol, err := os.Create(filepath.Join(dir, "wire.log"))
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = protocol.Close()
			data, _ := os.ReadFile(protocol.Name())
			keys := regexp.MustCompile(`zwp_virtual_keyboard_v1#\d+\.key\(\d+, 14, ([01])\)`).FindAllSubmatch(data, -1)
			if !t.Failed() && (len(keys) != 2 || string(keys[0][1]) != "1" || string(keys[1][1]) != "0") {
				t.Errorf("IBus returned %d Backspace events, want one press followed by one release", len(keys))
			}
			if !t.Failed() {
				return
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines[max(0, len(lines)-120):] {
				t.Log(line)
			}
		}()
		start := func(name string, args ...string) (*exec.Cmd, *bufio.Scanner) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = env
			cmd.Stderr = os.Stderr
			if filepath.Base(name) == "ibus-wayland" || name == "python3" {
				cmd.Env = append(cmd.Env, "WAYLAND_DEBUG=client")
				cmd.Stderr = protocol
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Error(err)
				return nil, nil
			}
			if err := cmd.Start(); err != nil {
				t.Error(err)
				_ = stdout.Close()
				return nil, nil
			}
			return cmd, bufio.NewScanner(stdout)
		}
		stop := func(cmd *exec.Cmd) { _ = cmd.Process.Kill(); _ = cmd.Wait() }
		bus, address := start("dbus-daemon", "--session", "--nofork", "--print-address=1")
		if bus == nil {
			return
		}
		defer stop(bus)
		if !address.Scan() {
			t.Error("private bus did not provide an address")
			return
		}
		env = append(env, "DBUS_SESSION_BUS_ADDRESS="+address.Text())
		daemon, _ := start("ibus-daemon", "--single", "--panel=disable", "--config=disable", "--emoji-extension=disable", "--address=unix:path="+filepath.Join(dir, "ibus"))
		if daemon == nil {
			return
		}
		defer stop(daemon)
		gtk, output := start("python3", "-u", "-c", nativeIBusGTK)
		if gtk == nil {
			return
		}
		defer stop(gtk)
		if !output.Scan() || output.Text() != "IBUS_READY" {
			t.Error("private IBus engine did not become ready")
			return
		}
		bridge, _ := start("/usr/lib/ibus/ibus-wayland")
		if bridge == nil {
			return
		}
		defer stop(bridge)
		active := false
		for deadline := time.Now().Add(5 * time.Second); !active && time.Now().Before(deadline); {
			onCompositor(t, c, func() {
				active = c.activeTextInput != nil && c.inputMethod != nil && c.inputMethod.keyboard_grab != nil
			})
		}
		if !active {
			t.Error("GTK/IBus never established text-input-v3 and a native keyboard grab")
			return
		}
		waitFor := func(want string) bool {
			for output.Scan() {
				if output.Text() == want {
					return true
				}
				t.Logf("GTK state awaiting %q: %q", want, output.Text())
			}
			t.Errorf("GTK did not report %q", want)
			return false
		}
		onCompositor(t, c, func() { kb.key(keyA, true); kb.key(keyA, false) })
		if !waitFor("TEXT:a") {
			return
		}
		onCompositor(t, c, func() {
			// IBus Simple's Unicode composition: Ctrl+Shift+U, 00e9, Enter.
			kb.key(29, true)
			kb.key(42, true)
			kb.key(22, true)
			kb.key(22, false)
			kb.key(42, false)
			kb.key(29, false)
		})
		if !waitFor("PREEDIT:u") {
			return
		}
		for i, key := range []uint32{11, 11, 18, 10} {
			onCompositor(t, c, func() { kb.key(key, true); kb.key(key, false) })
			if !waitFor("PREEDIT:u" + "00e9"[:i+1]) {
				return
			}
		}
		onCompositor(t, c, func() { kb.key(28, true); kb.key(28, false) })
		if !waitFor("TEXT:aé") {
			return
		}
		onCompositor(t, c, func() { kb.key(14, true); kb.key(14, false) })
		if !waitFor("TEXT:a") {
			return
		}
		waitFor("RELEASE:BackSpace")
	}()
	if err := c.Run(); err != nil {
		t.Error(err)
	}
	cancel()
	<-done
}

const nativeIBusGTK = `
import gi
gi.require_version('Gtk', '4.0')
gi.require_version('IBus', '1.0')
from gi.repository import Gtk, IBus, GLib, Gdk
IBus.init()
bus = IBus.Bus.new_async()
window = None
def report(line):
    # Surrounding/preedit changes may send another text-input commit. Sync
    # those requests before the next physical key, as a real typing interval
    # would, so the IME sees its new done serial before that key.
    def synced():
        Gdk.Display.get_default().sync()
        print(line, flush=True)
        return False
    clock = window.get_frame_clock() if window else None
    if clock is None:
        GLib.idle_add(synced)
    else:
        def painted(frame):
            frame.disconnect(handler)
            synced()
        handler = clock.connect_after('after-paint', painted)
        clock.request_phase(Gdk.FrameClockPhase.AFTER_PAINT)
def ready():
    global window
    if not bus.is_connected():
        return True
    if not bus.set_global_engine('xkb:us::eng'):
        raise RuntimeError('cannot select packaged IBus Simple engine')
    print('IBUS_READY', flush=True)
    Gtk.init()
    window = Gtk.Window(title='sysc native IME acceptance')
    entry = Gtk.Entry()
    entry.connect('changed', lambda e: report('TEXT:' + e.get_text()))
    entry.get_delegate().connect('preedit-changed', lambda e, text: report('PREEDIT:' + text))
    keys = Gtk.EventControllerKey()
    keys.set_propagation_phase(Gtk.PropagationPhase.CAPTURE)
    keys.connect('key-released', lambda controller, keyval, keycode, state:
                 report('RELEASE:BackSpace') if keyval == Gdk.KEY_BackSpace else None)
    entry.add_controller(keys)
    window.set_child(entry)
    window.present()
    entry.grab_focus()
    return False
GLib.timeout_add(10, ready)
GLib.MainLoop().run()
`
