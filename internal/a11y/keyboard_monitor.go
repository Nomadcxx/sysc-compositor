// Package a11y owns the compositor's assistive-technology state: the
// org.freedesktop.a11y.KeyboardMonitor grab rules, without D-Bus or wlroots.
package a11y

import (
	"slices"
	"sync"
	"time"
)

// Decision is what the compositor does with a key after the monitor has
// seen it (Niri's KbMonBlock).
type Decision int

const (
	// Pass handles the key normally.
	Pass Decision = iota
	// ModifierFirstPress blocks a grabbed modifier's press or release. It
	// must not change XKB state either, so a single tap of an Orca key such
	// as Caps Lock leaves the lock alone.
	ModifierFirstPress
	// Block keeps the key from binds and clients; XKB state still follows it.
	Block
)

// Key is one key event as XKB saw it before the event changed its state.
type Key struct {
	Time     time.Duration
	Released bool
	Mods     uint32 // effective XKB modifier mask
	Keysym   uint32
	Unichar  uint32
	Keycode  uint16
}

type caller struct {
	watched, grabbed bool
	modifiers        map[uint32]bool
	keystrokes       [][2]uint32 // keysym, modifier mask
}

// Monitor keeps every caller's grabs and the keys they currently swallow. It
// follows Niri 26.04's src/dbus/freedesktop_a11y.rs, which cites KWin and
// Mutter for the same interface. It is safe for concurrent use.
type Monitor struct {
	mu          sync.Mutex
	callers     map[string]*caller
	grabbedMods map[uint32]bool
	lastPress   map[uint32]time.Duration
	suppressed  map[uint32]bool
}

func NewMonitor() *Monitor {
	return &Monitor{callers: map[string]*caller{}, grabbedMods: map[uint32]bool{},
		lastPress: map[uint32]time.Duration{}, suppressed: map[uint32]bool{}}
}

func (m *Monitor) entry(name string) *caller {
	c := m.callers[name]
	if c == nil {
		c = &caller{}
		m.callers[name] = c
	}
	return c
}

// Grab sends every key to name and keeps it from the compositor.
func (m *Monitor) Grab(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(name).grabbed = true
}

// Ungrab reverses Grab; the caller's key grabs and watch stay.
func (m *Monitor) Ungrab(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.callers[name]; c != nil {
		c.grabbed = false
	}
}

// Watch sends every key to name while the compositor still handles it.
func (m *Monitor) Watch(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(name).watched = true
}

// Unwatch reverses Watch; the caller's key grabs stay.
func (m *Monitor) Unwatch(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.callers[name]; c != nil {
		c.watched = false
	}
}

// SetKeyGrabs replaces name's grabbed modifier keysyms and its
// (keysym, modifier mask) keystrokes.
func (m *Monitor) SetKeyGrabs(name string, modifiers []uint32, keystrokes [][2]uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.entry(name)
	c.modifiers = map[uint32]bool{}
	for _, sym := range modifiers {
		c.modifiers[sym] = true
	}
	c.keystrokes = slices.Clone(keystrokes)
	m.rebuildGrabbedMods()
}

// Forget drops everything name asked for, as when it leaves the bus.
func (m *Monitor) Forget(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.callers, name)
	m.rebuildGrabbedMods()
}

func (m *Monitor) rebuildGrabbedMods() {
	clear(m.grabbedMods)
	for _, c := range m.callers {
		for sym := range c.modifiers {
			m.grabbedMods[sym] = true
		}
	}
}

// ProcessKey decides what happens to key and names, in order, the callers to
// send it to. repeatDelay is the window in which a grabbed modifier's second
// press passes through.
func (m *Monitor) ProcessKey(key Key, repeatDelay time.Duration) (Decision, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var to []string
	for name, c := range m.callers {
		if c.watched || c.grabs(m.suppressed, key) {
			to = append(to, name)
		}
	}
	slices.Sort(to)

	if m.grabbedMods[key.Keysym] {
		if key.Released {
			// The release of a second press that was handled normally.
			if !m.suppressed[key.Keysym] {
				return Pass, to
			}
		} else {
			last := m.lastPress[key.Keysym]
			m.lastPress[key.Keysym] = key.Time
			if key.Time <= last+repeatDelay {
				return Pass, to
			}
		}
	}

	block := false
	switch {
	case key.Released:
		block = m.suppressed[key.Keysym]
		delete(m.suppressed, key.Keysym)
	case m.suppressed[key.Keysym]:
		// Already down, from another keyboard.
		block = true
	default:
		for _, c := range m.callers {
			if c.grabs(m.suppressed, key) {
				m.suppressed[key.Keysym] = true
				block = true
				break
			}
		}
	}
	switch {
	case !block:
		return Pass, to
	case m.grabbedMods[key.Keysym]:
		return ModifierFirstPress, to
	default:
		return Block, to
	}
}

func (c *caller) grabs(suppressed map[uint32]bool, key Key) bool {
	if c.grabbed {
		return true
	}
	for sym := range c.modifiers {
		if sym == key.Keysym || suppressed[sym] {
			return true
		}
	}
	for _, stroke := range c.keystrokes {
		if stroke[0] == key.Keysym && stroke[1] == key.Mods {
			return true
		}
	}
	return false
}
