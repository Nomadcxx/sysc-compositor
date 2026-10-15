package control

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Seat is the injected seat the dispatcher drives. The compositor's
// implementation posts each call onto the compositor thread and waits.
type Seat interface {
	Attach(name string) error
	Detach() error
	// Windows lists toplevels, most recently focused first.
	Windows() ([]Window, error)
	// Point moves the injected pointer to layout coordinates.
	Point(x, y float64) error
	Button(code uint32, pressed bool) error
	Key(code uint32, pressed bool) error
}

// Dispatcher is the single place every request is decided. Stop is enforced
// here, before any verb runs, so a mutating verb cannot reach the seat once
// the user has stopped it (design D3.5).
type Dispatcher struct {
	mu          sync.Mutex
	seat        Seat
	owner       ConnID
	stopped     bool
	heldKeys    map[uint32]bool
	heldButtons map[uint32]bool
	facts       *facts
}

type verb struct {
	// mutating verbs change what the injected seat does and are refused once
	// stopped. stop, detach and the read verbs are not.
	mutating bool
	run      func(d *Dispatcher, id ConnID, r Request, c *Conn) Reply
}

var verbs map[string]verb

func init() {
	verbs = map[string]verb{
		"attach":    {true, (*Dispatcher).attach},
		"point":     {true, (*Dispatcher).point},
		"button":    {true, (*Dispatcher).button},
		"key":       {true, (*Dispatcher).key},
		"type":      {true, (*Dispatcher).typeText},
		"detach":    {false, (*Dispatcher).detach},
		"stop":      {false, (*Dispatcher).stop},
		"windows":   {false, (*Dispatcher).windows},
		"state":     {false, (*Dispatcher).state},
		"subscribe": {false, (*Dispatcher).subscribe},
	}
}

// NewDispatcher drives seat. Facts start at their idle values.
func NewDispatcher(seat Seat) *Dispatcher {
	d := &Dispatcher{seat: seat, heldKeys: map[uint32]bool{}, heldButtons: map[uint32]bool{},
		facts: newFacts()}
	d.facts.publish("agent.active", false)
	d.facts.publish("agent.name", "")
	d.facts.publish("agent.stopped", false)
	d.facts.publish("agent.0.down", false)
	return d
}

// Handle answers one request. It implements Handler.
func (d *Dispatcher) Handle(id ConnID, r Request, c *Conn) Reply {
	v, ok := verbs[r.Verb]
	if !ok {
		return fail(r.ID, CodeUnknownVerb, fmt.Sprintf("no verb %q", r.Verb))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if v.mutating && d.stopped {
		return fail(r.ID, CodeStopped, "stopped by the user")
	}
	if v.mutating && r.Verb != "attach" && d.owner != id {
		return fail(r.ID, CodeNotAttached, "this connection does not drive the seat")
	}
	return v.run(d, id, r, c)
}

// Disconnect releases whatever the connection held. It implements Handler.
func (d *Dispatcher) Disconnect(id ConnID) {
	d.facts.unsubscribe(id)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.owner == id {
		d.release()
		_ = d.seat.Detach()
		d.owner = 0
		d.facts.publish("agent.active", false)
	}
}

func (d *Dispatcher) attach(id ConnID, r Request, _ *Conn) Reply {
	if d.owner == id {
		return Reply{ID: r.ID, OK: true}
	}
	if d.owner != 0 {
		return fail(r.ID, CodeBusy, "another connection drives the seat")
	}
	if err := d.seat.Attach(r.Name); err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	d.owner = id
	d.facts.publish("agent.name", r.Name)
	d.facts.publish("agent.active", true)
	return Reply{ID: r.ID, OK: true}
}

func (d *Dispatcher) detach(id ConnID, r Request, _ *Conn) Reply {
	if d.owner != id {
		return fail(r.ID, CodeNotAttached, "this connection does not drive the seat")
	}
	d.release()
	err := d.seat.Detach()
	d.owner = 0
	d.facts.publish("agent.active", false)
	if err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	return Reply{ID: r.ID, OK: true}
}

// stop is accepted from any connection and lifted by none: the shell and the
// driving client are the same uid, so lifting needs a user-seat-only action.
func (d *Dispatcher) stop(_ ConnID, r Request, _ *Conn) Reply {
	if !d.stopped {
		d.stopped = true
		d.release()
		d.facts.publish("agent.stopped", true)
	}
	return Reply{ID: r.ID, OK: true}
}

// release lets go of every held key and button, so a stopped or departed
// client cannot leave a modifier or a drag stuck down.
func (d *Dispatcher) release() {
	for code := range d.heldKeys {
		_ = d.seat.Key(code, false)
	}
	for code := range d.heldButtons {
		_ = d.seat.Button(code, false)
	}
	clear(d.heldKeys)
	clear(d.heldButtons)
	d.facts.publish("agent.0.down", false)
}

func (d *Dispatcher) windows(_ ConnID, r Request, _ *Conn) Reply {
	ws, err := d.seat.Windows()
	if err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	return Reply{ID: r.ID, OK: true, Windows: ws}
}

func (d *Dispatcher) state(_ ConnID, r Request, _ *Conn) Reply {
	return Reply{ID: r.ID, OK: true, Facts: d.facts.snapshot()}
}

func (d *Dispatcher) subscribe(id ConnID, r Request, c *Conn) Reply {
	if c == nil {
		return fail(r.ID, CodeBadRequest, "subscribe needs a connection")
	}
	d.facts.subscribe(id, c)
	return Reply{ID: r.ID, OK: true}
}

func (d *Dispatcher) point(_ ConnID, r Request, _ *Conn) Reply {
	ws, err := d.seat.Windows()
	if err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	w, code := resolveTarget(r.Target, ws)
	if code != "" {
		return fail(r.ID, code, fmt.Sprintf("target %q", r.Target))
	}
	x, y := float64(w.X)+r.X, float64(w.Y)+r.Y
	if err := d.seat.Point(x, y); err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	d.facts.publish("agent.0.x", x)
	d.facts.publish("agent.0.y", y)
	d.facts.publish("agent.target", w.ID)
	return Reply{ID: r.ID, OK: true, Window: &w}
}

var buttonCodes = map[string]uint32{"left": 272, "right": 273, "middle": 274}

func (d *Dispatcher) button(_ ConnID, r Request, _ *Conn) Reply {
	code, ok := buttonCodes[r.Button]
	if !ok {
		return fail(r.ID, CodeBadRequest, fmt.Sprintf("no button %q", r.Button))
	}
	if err := d.seat.Button(code, r.Pressed); err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	if r.Pressed {
		d.heldButtons[code] = true
	} else {
		delete(d.heldButtons, code)
	}
	d.facts.publish("agent.0.down", len(d.heldButtons) > 0)
	return Reply{ID: r.ID, OK: true}
}

func (d *Dispatcher) key(_ ConnID, r Request, _ *Conn) Reply {
	code, ok := keyCodes[r.Key]
	if !ok {
		return fail(r.ID, CodeUnsupported, fmt.Sprintf("no key %q", r.Key))
	}
	if err := d.seat.Key(code, r.Pressed); err != nil {
		return fail(r.ID, CodeFailed, err.Error())
	}
	if r.Pressed {
		d.heldKeys[code] = true
	} else {
		delete(d.heldKeys, code)
	}
	return Reply{ID: r.ID, OK: true}
}

// typeText plans every keystroke before sending any, so text the US keymap
// cannot produce is refused without a partial effect.
func (d *Dispatcher) typeText(_ ConnID, r Request, _ *Conn) Reply {
	strokes, err := planText(r.Text)
	if err != nil {
		return fail(r.ID, CodeUnsupported, err.Error())
	}
	for _, s := range strokes {
		if err := d.stroke(s); err != nil {
			return fail(r.ID, CodeFailed, err.Error())
		}
	}
	return Reply{ID: r.ID, OK: true}
}

func (d *Dispatcher) stroke(s stroke) error {
	if s.shift {
		if err := d.seat.Key(keyShift, true); err != nil {
			return err
		}
	}
	if err := d.seat.Key(s.code, true); err != nil {
		return err
	}
	if err := d.seat.Key(s.code, false); err != nil {
		return err
	}
	if s.shift {
		return d.seat.Key(keyShift, false)
	}
	return nil
}

// resolveTarget picks the window a request names: id:N, pid:N or app-id:X.
// windows is most recently focused first, so an ambiguous app-id takes the
// window the user or the seat touched last; the reply names the one chosen.
func resolveTarget(target string, windows []Window) (Window, string) {
	kind, value, ok := strings.Cut(target, ":")
	if !ok || value == "" {
		return Window{}, CodeBadRequest
	}
	match := func(w Window) bool { return false }
	switch kind {
	case "id", "pid":
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return Window{}, CodeBadRequest
		}
		if kind == "id" {
			match = func(w Window) bool { return w.ID == n }
		} else {
			match = func(w Window) bool { return uint64(w.PID) == n }
		}
	case "app-id":
		match = func(w Window) bool { return w.AppID == value }
	default:
		return Window{}, CodeBadRequest
	}
	for _, w := range windows {
		if match(w) {
			return w, ""
		}
	}
	return Window{}, CodeUnknownTarget
}
