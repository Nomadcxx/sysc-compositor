package a11y

import (
	"slices"
	"testing"
	"time"
)

const (
	symA        = 0x61
	symH        = 0x68
	symShiftL   = 0xffe1
	symCapsLock = 0xffe5
	symInsert   = 0xff63
	maskShift   = 1
	maskControl = 4
)

// step is one call on the monitor. Only key steps check a decision and the
// callers that receive the key.
type step struct {
	call     func(*Monitor)
	key      *Key
	decision Decision
	to       []string
}

func press(ms int, sym, mods uint32) *Key {
	return &Key{Time: time.Duration(ms) * time.Millisecond, Keysym: sym, Mods: mods}
}

func release(ms int, sym, mods uint32) *Key {
	k := press(ms, sym, mods)
	k.Released = true
	return k
}

// TestKeyboardMonitorGrabsLikeNiri replays Niri 26.04's
// src/dbus/freedesktop_a11y.rs decisions: who hears a key, which keys the
// compositor swallows, and the double-tap escape for a grabbed modifier.
func TestKeyboardMonitorGrabsLikeNiri(t *testing.T) {
	const delay = 600 * time.Millisecond
	orca := func(m *Monitor) { m.SetKeyGrabs(":1.5", []uint32{symCapsLock}, [][2]uint32{{symH, maskControl}}) }
	tests := []struct {
		name  string
		steps []step
	}{
		{"nobody listening passes keys", []step{
			{key: press(1000, symA, 0), decision: Pass},
		}},
		{"a watcher hears keys that still pass", []step{
			{call: func(m *Monitor) { m.Watch(":1.7") }},
			{key: press(1000, symA, 0), decision: Pass, to: []string{":1.7"}},
			{key: release(1010, symA, 0), decision: Pass, to: []string{":1.7"}},
			{call: func(m *Monitor) { m.Unwatch(":1.7") }},
			{key: press(1020, symA, 0), decision: Pass},
		}},
		{"a full grab blocks a key and its release", []step{
			{call: func(m *Monitor) { m.Grab(":1.5"); m.Watch(":1.7") }},
			{key: press(1000, symA, 0), decision: Block, to: []string{":1.5", ":1.7"}},
			{key: release(1010, symA, 0), decision: Block, to: []string{":1.5", ":1.7"}},
			{call: func(m *Monitor) { m.Ungrab(":1.5") }},
			{key: press(1020, symA, 0), decision: Pass, to: []string{":1.7"}},
		}},
		{"an exact keystroke is grabbed, other masks pass", []step{
			{call: orca},
			{key: press(1000, symH, maskControl), decision: Block, to: []string{":1.5"}},
			{key: release(1010, symH, maskControl), decision: Block, to: []string{":1.5"}},
			{key: press(1020, symH, maskControl|maskShift), decision: Pass},
			{key: press(1030, symH, 0), decision: Pass},
		}},
		{"keys under a held grabbed modifier are grabbed", []step{
			{call: orca},
			{key: press(1000, symCapsLock, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
			{key: press(1100, symA, 0), decision: Block, to: []string{":1.5"}},
			{key: release(1110, symA, 0), decision: Block, to: []string{":1.5"}},
			{key: release(1200, symCapsLock, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
			{key: press(1300, symA, 0), decision: Pass},
		}},
		{"a grabbed modifier tapped twice passes, then blocks after the delay", []step{
			{call: orca},
			{key: press(1000, symCapsLock, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
			{key: release(1050, symCapsLock, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
			{key: press(1300, symCapsLock, 0), decision: Pass, to: []string{":1.5"}},
			{key: release(1350, symCapsLock, 0), decision: Pass, to: []string{":1.5"}},
			{key: press(2000, symCapsLock, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
		}},
		{"a second keyboard's press of a suppressed key is blocked", []step{
			{call: func(m *Monitor) { m.SetKeyGrabs(":1.5", nil, [][2]uint32{{symA, 0}}) }},
			{key: press(1000, symA, 0), decision: Block, to: []string{":1.5"}},
			{call: func(m *Monitor) { m.SetKeyGrabs(":1.5", nil, nil) }},
			{key: press(1010, symA, 0), decision: Block},
			{key: release(1020, symA, 0), decision: Block},
			{key: release(1030, symA, 0), decision: Pass},
		}},
		{"a departed caller's grabs are gone", []step{
			{call: orca},
			{call: func(m *Monitor) { m.SetKeyGrabs(":1.9", []uint32{symInsert}, nil) }},
			{call: func(m *Monitor) { m.Forget(":1.5") }},
			{key: press(1000, symCapsLock, 0), decision: Pass},
			{key: press(1100, symH, maskControl), decision: Pass},
			{key: press(1200, symInsert, 0), decision: ModifierFirstPress, to: []string{":1.9"}},
		}},
		{"new key grabs replace the previous ones", []step{
			{call: orca},
			{call: func(m *Monitor) { m.SetKeyGrabs(":1.5", []uint32{symInsert}, nil) }},
			{key: press(1000, symCapsLock, 0), decision: Pass},
			{key: press(1100, symH, maskControl), decision: Pass},
			{key: press(1200, symInsert, 0), decision: ModifierFirstPress, to: []string{":1.5"}},
		}},
		{"ungrab and unwatch of an unknown caller do nothing", []step{
			{call: func(m *Monitor) { m.Ungrab(":1.3"); m.Unwatch(":1.3") }},
			{key: press(1000, symA, 0), decision: Pass},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor()
			for i, s := range tt.steps {
				if s.call != nil {
					s.call(m)
					continue
				}
				decision, to := m.ProcessKey(*s.key, delay)
				if decision != s.decision || !slices.Equal(to, s.to) {
					t.Fatalf("step %d (keysym %#x released=%v at %v): got %v to %q, want %v to %q",
						i, s.key.Keysym, s.key.Released, s.key.Time, decision, to, s.decision, s.to)
				}
			}
		})
	}
}
