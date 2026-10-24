package wlr

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

const keyB = 48 // evdev KEY_B

func TestPowerOffPreservesOutputAndWorkspaceStateAndRestoresOnlyItsOutputs(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	type outputPowerActions interface {
		PowerOffMonitors() error
		PowerOnMonitors() error
		setOutputPower(unsafe.Pointer, bool) error
		outputIsEnabled(unsafe.Pointer) bool
	}
	power, ok := any(c).(outputPowerActions)
	if !ok {
		t.Fatal("compositor does not own monitor power state")
	}
	if len(c.sceneOutputs) != 2 {
		t.Fatalf("scene outputs = %d, want nested and headless outputs", len(c.sceneOutputs))
	}
	var originallyOff, originallyOn unsafe.Pointer
	for output := range c.sceneOutputs {
		if originallyOff == nil {
			originallyOff = output
		} else {
			originallyOn = output
		}
	}
	if err := power.setOutputPower(originallyOff, false); err != nil {
		t.Fatalf("disable one output before PowerOffMonitors: %v", err)
	}
	if !power.outputIsEnabled(originallyOn) || power.outputIsEnabled(originallyOff) {
		t.Fatal("failed to establish one enabled and one disabled output")
	}
	outputsBefore := c.OutputSnapshot()
	workspacesBefore := c.WorkspaceSnapshot()

	if err := power.PowerOffMonitors(); err != nil {
		t.Fatalf("PowerOffMonitors: %v", err)
	}
	if power.outputIsEnabled(originallyOn) || power.outputIsEnabled(originallyOff) {
		t.Fatal("PowerOffMonitors left an output enabled")
	}
	if got := c.OutputSnapshot(); !reflect.DeepEqual(got, outputsBefore) {
		t.Errorf("output layout changed while monitors were off\n got: %+v\nwant: %+v", got, outputsBefore)
	}
	if got := c.WorkspaceSnapshot(); !reflect.DeepEqual(got, workspacesBefore) {
		t.Errorf("workspace state changed while monitors were off\n got: %+v\nwant: %+v", got, workspacesBefore)
	}

	if err := power.PowerOnMonitors(); err != nil {
		t.Fatalf("PowerOnMonitors: %v", err)
	}
	if !power.outputIsEnabled(originallyOn) || power.outputIsEnabled(originallyOff) {
		t.Fatal("PowerOnMonitors did not restore only the output it turned off")
	}
	if got := c.OutputSnapshot(); !reflect.DeepEqual(got, outputsBefore) {
		t.Errorf("output layout changed after PowerOnMonitors\n got: %+v\nwant: %+v", got, outputsBefore)
	}
	if got := c.WorkspaceSnapshot(); !reflect.DeepEqual(got, workspacesBefore) {
		t.Errorf("workspace state changed after PowerOnMonitors\n got: %+v\nwant: %+v", got, workspacesBefore)
	}
}

func TestInjectedInputIsReleasedAndBlockedByLockOrBlanking(t *testing.T) {
	c := newNested(t)
	if err := c.AddInjectedSeat("agent"); err != nil {
		t.Fatal(err)
	}
	type inputGate interface {
		sealSession()
		unsealSession()
		injectedInputBlocked() bool
		PowerOffMonitors() error
		PowerOnMonitors() error
	}
	gate, ok := any(c).(inputGate)
	if !ok {
		t.Fatal("compositor does not gate injected input on lock and monitor power")
	}
	buttonEvent := regexp.MustCompile(`\} wl_pointer#(\d+)\.button\(\d+, \d+, (\d+), (\d+)\)`)
	keyboard := attachFakeKeyboard(t, c)
	c.viewMapped = func(view *toplevelView) {
		c.focusView(view)
		x, y := c.viewCenter(view)
		if err := c.InjectPointerMotion("agent", x, y); err != nil {
			t.Errorf("initial injected motion: %v", err)
		}
		// The press gives the agent seat keyboard focus, so the key after it
		// is delivered as a key event, not only in the enter's held keys.
		if err := c.InjectPointerButton("agent", btnLeft, true); err != nil {
			t.Errorf("initial injected button press: %v", err)
		}
		if err := c.InjectKey("agent", keyA, true); err != nil {
			t.Errorf("initial injected key press: %v", err)
		}
		gate.sealSession()
		if !gate.injectedInputBlocked() {
			t.Error("session lock did not block injected input")
		}
		checkBlocked := func(label string) {
			if err := c.InjectKey("agent", keyB, true); err == nil || !strings.Contains(err.Error(), "blocked") {
				t.Errorf("%s injected key press error = %v, want blocked", label, err)
			}
			if err := c.InjectPointerButton("agent", btnLeft, true); err == nil || !strings.Contains(err.Error(), "blocked") {
				t.Errorf("%s injected button press error = %v, want blocked", label, err)
			}
			beforeX, beforeY := c.injectedSeat("agent").x, c.injectedSeat("agent").y
			if err := c.InjectPointerMotion("agent", x+1, y+1); err == nil || !strings.Contains(err.Error(), "blocked") {
				t.Errorf("%s injected motion error = %v, want blocked", label, err)
			}
			if s := c.injectedSeat("agent"); s.x != beforeX || s.y != beforeY {
				t.Errorf("%s blocked motion moved the injected pointer to (%v,%v)", label, s.x, s.y)
			}
			if err := c.InjectKey("agent", keyA, false); err != nil {
				t.Errorf("%s key release cleanup: %v", label, err)
			}
			if err := c.InjectPointerButton("agent", btnLeft, false); err != nil {
				t.Errorf("%s button release cleanup: %v", label, err)
			}
		}
		checkBlocked("locked")

		keyboard.key(keyA, true)
		keyboard.key(keyA, false)
		gate.unsealSession()
		if err := c.InjectKey("agent", keyA, true); err != nil {
			t.Errorf("injected key press after unlock: %v", err)
		}
		if err := c.InjectPointerButton("agent", btnLeft, true); err != nil {
			t.Errorf("injected button press before blanking: %v", err)
		}
		if err := gate.PowerOffMonitors(); err != nil {
			t.Errorf("PowerOffMonitors: %v", err)
		}
		if !gate.injectedInputBlocked() {
			t.Error("all-monitors-off did not block injected input")
		}
		checkBlocked("blanked")
		if err := gate.PowerOnMonitors(); err != nil {
			t.Errorf("PowerOnMonitors: %v", err)
		}
		if gate.injectedInputBlocked() {
			t.Error("PowerOnMonitors left injected input blocked")
		}
		keyboard.key(keyA, true)
		keyboard.key(keyA, false)
	}
	buttonEvents := 0
	var lines []string
	runClient(t, c, []string{"weston-terminal"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
		lines = append(lines, line)
		if buttonEvent.MatchString(line) {
			buttonEvents++
			return buttonEvents == 4
		}
		return false
	})
	if buttonEvents != 4 {
		t.Fatalf("agent pointer button events = %d, want press/release for lock and blank transitions", buttonEvents)
	}
	log := parseSeatLog(lines)
	var agentKeys, userKeys, agentButtons []string
	keyEvent := regexp.MustCompile(`\} wl_keyboard#(\d+)\.key\(\d+, \d+, (\d+), (\d+)\)`)
	for _, line := range lines {
		if match := keyEvent.FindStringSubmatch(line); match != nil {
			state := match[2] + "/" + match[3]
			switch log.seatOf["wl_keyboard#"+match[1]] {
			case "agent":
				agentKeys = append(agentKeys, state)
			case "seat0":
				userKeys = append(userKeys, state)
			}
		}
		if match := buttonEvent.FindStringSubmatch(line); match != nil && match[2] == "272" &&
			log.seatOf["wl_pointer#"+match[1]] == "agent" {
			agentButtons = append(agentButtons, match[2]+"/"+match[3])
		}
	}
	if got, want := strings.Join(agentKeys, " "), "30/1 30/0 30/1 30/0"; got != want {
		t.Errorf("agent key events = %q, want %q", got, want)
	}
	// The seal keeps the user's keys from the window (no lock surface is
	// mapped here); after unlock and blanking they reach it again.
	if got, want := strings.Join(userKeys, " "), "30/1 30/0"; got != want {
		t.Errorf("seat0 key events = %q, want only the pair after unlock %q", got, want)
	}
	if got, want := strings.Join(agentButtons, " "), "272/1 272/0 272/1 272/0"; got != want {
		t.Errorf("agent button events = %q, want %q", got, want)
	}
}
