package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/control"
)

var (
	reSeatName      = regexp.MustCompile(`wl_seat#(\d+)\.name\("([^"]+)"\)`)
	reKeyboard      = regexp.MustCompile(`wl_seat#(\d+)\.get_keyboard\(new id wl_keyboard#(\d+)\)`)
	reKeyboardEnter = regexp.MustCompile(`wl_keyboard#(\d+)\.enter\(`)
	reKey           = regexp.MustCompile(`\} wl_keyboard#(\d+)\.key\(\d+, \d+, (\d+), (\d+)\)`)
)

// keysBySeat attributes each wl_keyboard.key event a client logged to the
// seat that owns that keyboard, as "code/state" strings in arrival order.
func keysBySeat(lines []string) map[string][]string {
	seatName, kbSeat := map[string]string{}, map[string]string{}
	for _, l := range lines {
		if m := reSeatName.FindStringSubmatch(l); m != nil {
			seatName[m[1]] = m[2]
		}
		if m := reKeyboard.FindStringSubmatch(l); m != nil {
			kbSeat[m[2]] = m[1]
		}
	}
	out := map[string][]string{}
	for _, l := range lines {
		if m := reKey.FindStringSubmatch(l); m != nil {
			seat := seatName[kbSeat[m[1]]]
			out[seat] = append(out[seat], m[2]+"/"+m[3])
		}
	}
	return out
}

// TestControlDrivesAClient drives weston-terminal through the real control
// socket: attach, point, click and type reach the client on the injected
// seat only; after Stop, typing is refused and nothing more arrives.
func TestControlDrivesAClient(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s, err := New(config.LoadResult{}, false)
	if err != nil {
		t.Skipf("no nested wayland session available: %v", err)
	}
	defer s.Destroy()
	path := s.ControlPath()
	if path == "" {
		t.Fatal("the control socket did not start")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu    sync.Mutex
		lines []string
	)
	typed := make(chan struct{})
	agentEntered := make(chan struct{})
	var once, enteredOnce sync.Once
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(reader)
		seatNames, keyboardSeats := map[string]string{}, map[string]string{}
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
			if match := reSeatName.FindStringSubmatch(line); match != nil {
				seatNames[match[1]] = match[2]
			}
			if match := reKeyboard.FindStringSubmatch(line); match != nil {
				keyboardSeats[match[2]] = match[1]
			}
			if match := reKeyboardEnter.FindStringSubmatch(line); match != nil &&
				seatNames[keyboardSeats[match[1]]] == injectedSeatName {
				enteredOnce.Do(func() { close(agentEntered) })
			}
			if strings.Contains(line, ".key(") && strings.HasSuffix(line, ", 30, 0)") {
				once.Do(func() { close(typed) })
			}
		}
	}()
	cmd := exec.Command(terminal)
	cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+s.SocketName(), "WAYLAND_DEBUG=client")
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()

	var steps []string
	clientDone := make(chan error, 1)
	go func() {
		defer s.Terminate()
		clientDone <- drive(path, typed, agentEntered, &steps)
	}()
	watchdog := time.AfterFunc(10*time.Second, s.Terminate)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	watchdog.Stop()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	_ = reader.Close()
	<-readerDone

	if err := <-clientDone; err != nil {
		mu.Lock()
		observedKeys := keysBySeat(lines)
		var keyboardEvents []string
		for _, line := range lines {
			if strings.Contains(line, "wl_keyboard#") {
				keyboardEvents = append(keyboardEvents, line)
			}
		}
		mu.Unlock()
		t.Fatalf("%v\nsteps: %v\nkeys: %v\nkeyboard events: %v", err, steps, observedKeys, keyboardEvents)
	}
	mu.Lock()
	keys := keysBySeat(lines)
	mu.Unlock()
	if got := strings.Join(keys["agent"], " "); got != "30/1 30/0" {
		t.Errorf("injected keyboard keys = %q, want 30/1 30/0 (a, and no b after Stop)", got)
	}
	if len(keys["seat0"]) != 0 {
		t.Errorf("user keyboard got keys %v", keys["seat0"])
	}
}

// drive waits for the window and injected keyboard focus before driving it.
func drive(path string, typed, agentEntered <-chan struct{}, steps *[]string) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	id := int64(0)
	call := func(req control.Request) (control.Reply, error) {
		id++
		req.ID = id
		line, _ := json.Marshal(req)
		if _, err := conn.Write(append(line, '\n')); err != nil {
			return control.Reply{}, err
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		out, err := r.ReadBytes('\n')
		if err != nil {
			return control.Reply{}, err
		}
		var rep control.Reply
		err = json.Unmarshal(out, &rep)
		*steps = append(*steps, fmt.Sprintf("%s=%s%s", req.Verb, map[bool]string{true: "ok", false: "err:"}[rep.OK], rep.Error))
		return rep, err
	}
	var target control.Window
	for deadline := time.Now().Add(5 * time.Second); ; {
		rep, err := call(control.Request{Verb: "windows"})
		if err != nil {
			return err
		}
		if len(rep.Windows) > 0 && rep.Windows[0].Width > 0 {
			target = rep.Windows[0]
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no window appeared")
		}
		time.Sleep(50 * time.Millisecond)
	}
	*steps = append(*steps, fmt.Sprintf("target=%+v", target))
	for _, req := range []control.Request{
		{Verb: "attach", Name: "test"},
		{Verb: "point", Target: fmt.Sprintf("id:%d", target.ID), X: float64(target.Width / 2), Y: float64(target.Height / 2)},
		{Verb: "button", Button: "left", Pressed: true},
		{Verb: "button", Button: "left", Pressed: false},
	} {
		if rep, err := call(req); err != nil || !rep.OK {
			return fmt.Errorf("%s: %+v %v", req.Verb, rep, err)
		}
	}
	select {
	case <-agentEntered:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("the client never entered the agent keyboard")
	}
	if rep, err := call(control.Request{Verb: "type", Text: "a"}); err != nil || !rep.OK {
		return fmt.Errorf("type: %+v %v", rep, err)
	}
	select {
	case <-typed:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("the client never logged the typed key")
	}
	if rep, err := call(control.Request{Verb: "stop"}); err != nil || !rep.OK {
		return fmt.Errorf("stop: %+v %v", rep, err)
	}
	if rep, err := call(control.Request{Verb: "type", Text: "b"}); err != nil || rep.Error != control.CodeStopped {
		return fmt.Errorf("type after stop = %+v %v, want %s", rep, err, control.CodeStopped)
	}
	time.Sleep(200 * time.Millisecond) // let any stray key reach the client log
	return nil
}
