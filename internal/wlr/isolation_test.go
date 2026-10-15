package wlr

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

const btnLeft = 272 // evdev BTN_LEFT

var (
	reSeatName  = regexp.MustCompile(`wl_seat#(\d+)\.name\("([^"]+)"\)`)
	reSeatChild = regexp.MustCompile(`wl_seat#(\d+)\.get_(keyboard|pointer)\(new id wl_(?:keyboard|pointer)#(\d+)\)`)
	reDataDev   = regexp.MustCompile(`get_data_device\(new id wl_data_device#(\d+), wl_seat#(\d+)\)`)
	reEvent     = regexp.MustCompile(`\} (wl_keyboard|wl_pointer|wl_data_device)#(\d+)\.(\w+)\(`)
	reRequest   = regexp.MustCompile(`-> wl_pointer#(\d+)\.set_cursor\(`)
)

// seatLog attributes every keyboard, pointer and data-device event a client
// logged under WAYLAND_DEBUG to the seat that owns the object.
type seatLog struct {
	seatOf map[string]string // "wl_keyboard#18" -> "agent"
	events []seatEvent       // in arrival order
	cursor map[string]int    // seat -> set_cursor requests the client sent
}

type seatEvent struct {
	seat, object, name string
}

func parseSeatLog(lines []string) seatLog {
	names := map[string]string{}
	owner := map[string]string{}
	for _, l := range lines {
		if m := reSeatName.FindStringSubmatch(l); m != nil {
			names[m[1]] = m[2]
		}
		if m := reSeatChild.FindStringSubmatch(l); m != nil {
			owner["wl_"+m[2]+"#"+m[3]] = m[1]
		}
		if m := reDataDev.FindStringSubmatch(l); m != nil {
			owner["wl_data_device#"+m[1]] = m[2]
		}
	}
	log := seatLog{seatOf: map[string]string{}, cursor: map[string]int{}}
	for object, seat := range owner {
		log.seatOf[object] = names[seat]
	}
	for _, l := range lines {
		if m := reRequest.FindStringSubmatch(l); m != nil {
			log.cursor[log.seatOf["wl_pointer#"+m[1]]]++
			continue
		}
		if strings.Contains(l, " -> ") {
			continue
		}
		if m := reEvent.FindStringSubmatch(l); m != nil {
			object := m[1] + "#" + m[2]
			log.events = append(log.events, seatEvent{log.seatOf[object], object, m[3]})
		}
	}
	return log
}

// TestInjectedSeatIsolation is rules D2.4.1 and D2.4.2 observed from a real
// client: weston-terminal binds both seats, the user seat focuses it, then the
// injected seat points, clicks and types into the same window.
func TestInjectedSeatIsolation(t *testing.T) {
	terminal, err := exec.LookPath("weston-terminal")
	if err != nil {
		t.Skipf("weston-terminal unavailable: %v", err)
	}
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}

	var (
		acted                     bool
		userBefore, userAfter     unsafe.Pointer
		viewBefore, viewAfter     *toplevelView
		injectedFocus, viewSurf   unsafe.Pointer
		cursorX, cursorY, gx, gy  float64
		userListens, otherListens bool
	)
	c.viewMapped = func(v *toplevelView) {
		if acted {
			return
		}
		acted = true
		c.focusView(v) // the user's own click
		userBefore, viewBefore = c.userFocus()
		cursorX, cursorY = c.cursorPosition()
		x, y := c.viewCenter(v)
		for _, step := range []func() error{
			func() error { return c.InjectPointerMotion("agent", x, y) },
			func() error { return c.InjectPointerButton("agent", btnLeft, true) },
			func() error { return c.InjectPointerButton("agent", btnLeft, false) },
			func() error { return c.InjectKey("agent", keyA, true) },
			func() error { return c.InjectKey("agent", keyA, false) },
		} {
			if err := step(); err != nil {
				t.Errorf("injection: %v", err)
			}
		}
		userAfter, viewAfter = c.userFocus()
		injectedFocus = c.injectedFocus("agent")
		viewSurf = unsafe.Pointer(v.toplevel.base.surface)
		gx, gy = c.cursorPosition()
		userListens, otherListens = c.setCursorListeners("agent")
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
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
			// The release of the injected key is the last event the test
			// needs; stop once it has been seen.
			if strings.Contains(line, ".key(") && strings.HasSuffix(line, ", 30, 0)") {
				c.Terminate()
			}
		}
	}()
	cmd := exec.Command(terminal)
	cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+c.SocketName(), "WAYLAND_DEBUG=client")
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	timeout := time.AfterFunc(5*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	timeout.Stop()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	_ = reader.Close()
	<-readerDone

	if !acted {
		t.Fatal("weston-terminal never mapped a window")
	}
	mu.Lock()
	log := parseSeatLog(lines)
	mu.Unlock()

	// D2.4.1: the user's focus and selection do not move.
	if userAfter != userBefore || viewAfter != viewBefore || userBefore == nil {
		t.Errorf("user keyboard focus %p/%p -> %p/%p: the injected seat moved it",
			userBefore, viewBefore, userAfter, viewAfter)
	}
	if injectedFocus != viewSurf {
		t.Errorf("injected seat focus = %p, want the clicked window %p", injectedFocus, viewSurf)
	}
	keys := map[string]int{}
	firstInjected := -1
	for i, e := range log.events {
		if e.name == "key" {
			keys[e.seat]++
		}
		// keymap and repeat_info arrive when the client binds the seat, long
		// before anything is injected; input starts at the first enter.
		if e.seat == "agent" && e.name == "enter" && firstInjected < 0 {
			firstInjected = i
		}
	}
	if keys["agent"] != 2 {
		t.Errorf("injected keyboard got %d key events, want press and release", keys["agent"])
	}
	if keys["seat0"] != 0 {
		t.Errorf("user keyboard got %d key events: injected input reached the user seat", keys["seat0"])
	}
	if firstInjected < 0 {
		t.Fatal("the client logged no event on the injected seat")
	}
	for _, e := range log.events[firstInjected:] {
		if e.seat == "seat0" && (strings.HasPrefix(e.object, "wl_keyboard") || strings.HasPrefix(e.object, "wl_data_device")) {
			t.Errorf("user seat %s.%s after injection began: focus or selection moved", e.object, e.name)
		}
	}

	// D2.4.2: the injected seat cannot change the drawn pointer.
	if log.cursor["agent"] == 0 {
		t.Error("the client never asked the injected seat to set its image; the rule went unexercised")
	}
	if !userListens || otherListens {
		t.Errorf("set-image listeners: user=%v injected=%v, want true/false", userListens, otherListens)
	}
	if gx != cursorX || gy != cursorY {
		t.Errorf("drawn pointer moved from (%v,%v) to (%v,%v)", cursorX, cursorY, gx, gy)
	}
}
