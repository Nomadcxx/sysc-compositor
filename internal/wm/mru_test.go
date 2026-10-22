package wm

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func mruIDs(windows []MRUWindow) []uint64 {
	ids := make([]uint64, len(windows))
	for i, w := range windows {
		ids[i] = w.ID
	}
	return ids
}

// verifyMRU is Niri's WindowMru::verify_invariants.
func verifyMRU(t *testing.T, m *MRU, history []string) {
	t.Helper()
	listed := m.Listed()
	id, ok := m.Current()
	if ok && !slices.ContainsFunc(listed, func(w MRUWindow) bool { return w.ID == id }) {
		t.Fatalf("after %v: selection %d is not listed in %v", history, id, mruIDs(listed))
	}
	if !ok && len(listed) != 0 {
		t.Fatalf("after %v: no selection, but %v are listed", history, mruIDs(listed))
	}
}

func TestMRUOrdersByFocusStampNewestFirst(t *testing.T) {
	m := NewMRU([]MRUWindow{
		{ID: 1, Stamp: 10 * time.Second, Stamped: true},
		{ID: 2},
		{ID: 3, Stamp: 30 * time.Second, Stamped: true},
		{ID: 4},
		{ID: 5, Stamp: 20 * time.Second, Stamped: true},
	})
	if got := mruIDs(m.Listed()); !slices.Equal(got, []uint64{3, 5, 1, 2, 4}) {
		t.Fatalf("order = %v", got)
	}
	if id, ok := m.Current(); !ok || id != 3 {
		t.Fatalf("selection = %d, %v; want the newest", id, ok)
	}
	m.Backward()
	if id, _ := m.Current(); id != 4 {
		t.Errorf("backward from the first wrapped to %d, want the last", id)
	}
	m.Forward()
	m.Forward()
	if id, _ := m.Current(); id != 5 {
		t.Errorf("forward = %d, want 5", id)
	}
	m.Last()
	m.Forward()
	if id, _ := m.Current(); id != 3 {
		t.Errorf("forward from the last wrapped to %d, want the first", id)
	}
}

// Niri's remove_last_window_out_of_two.
func TestMRURemovingTheLastOfTwoKeepsASelection(t *testing.T) {
	m := NewMRU([]MRUWindow{{ID: 1}, {ID: 2}})
	m.Backward()
	m.Remove(2)
	verifyMRU(t, m, []string{"backward", "remove 2"})
	if id, ok := m.Current(); !ok || id != 1 {
		t.Fatalf("selection = %d, %v; want 1", id, ok)
	}
	m.Remove(1)
	if _, ok := m.Current(); ok || !m.Empty() {
		t.Fatal("removing the last window left a selection")
	}
}

func TestMRUScopeAndFilterKeepTheNearestSelection(t *testing.T) {
	foot, kitty := "foot", "kitty"
	m := NewMRU([]MRUWindow{
		{ID: 1, Stamp: 5, Stamped: true, OnOutput: true, OnWorkspace: true, AppID: &foot},
		{ID: 2, Stamp: 4, Stamped: true, OnOutput: true, AppID: &kitty},
		{ID: 3, Stamp: 3, Stamped: true, AppID: &foot},
		{ID: 4, Stamp: 2, Stamped: true, OnOutput: true, OnWorkspace: true, AppID: &kitty},
		{ID: 5, Stamp: 1, Stamped: true},
	})
	m.SetCurrent(3)
	if !m.SetScope(MRUScopeOutput) {
		t.Fatal("changing the scope reported no change")
	}
	// Window 3 is on another output: the nearest listed window before it.
	if id, _ := m.Current(); id != 2 {
		t.Errorf("output scope selected %d, want 2", id)
	}
	if got := mruIDs(m.Listed()); !slices.Equal(got, []uint64{1, 2, 4}) {
		t.Errorf("output scope lists %v", got)
	}
	if !m.SetAppIDFilter(true) {
		t.Fatal("filtering by the selection's app id reported no change")
	}
	if got := mruIDs(m.Listed()); !slices.Equal(got, []uint64{2, 4}) {
		t.Errorf("kitty on the output lists %v", got)
	}
	m.CycleScope() // Output wraps to All.
	if m.Scope() != MRUScopeAll {
		t.Errorf("cycling from output gave scope %v", m.Scope())
	}
	m.CycleScope()
	if m.Scope() != MRUScopeWorkspace {
		t.Errorf("cycling from all gave scope %v", m.Scope())
	}
	if id, _ := m.Current(); id != 4 {
		t.Errorf("kitty on the workspace selected %d, want 4", id)
	}
	if m.SetScope(MRUScopeWorkspace) {
		t.Error("setting the same scope reported a change")
	}
	m.SetAppIDFilter(false)
	m.SetScope(MRUScopeAll)
	m.SetCurrent(5)
	if m.SetAppIDFilter(true) || m.FilteredByAppID() {
		t.Error("a window without an app id set the filter")
	}
	m.SetScope(MRUScopeWorkspace)
	// Window 5 is past the workspace's last window, which is selected.
	if id, _ := m.Current(); id != 4 {
		t.Errorf("workspace scope from the last window selected %d, want 4", id)
	}
}

// The randomized operations of Niri's random_operations_dont_panic, with
// its invariant checked after every one.
func TestMRURandomOperationsKeepTheSelectionListed(t *testing.T) {
	appIDs := []*string{nil, new(string), new(string)}
	*appIDs[1], *appIDs[2] = "app-1", "app-2"
	for seed := range uint64(2000) {
		r := rand.New(rand.NewPCG(seed, 0x6d7275))
		windows := make([]MRUWindow, 1+r.IntN(9))
		for i := range windows {
			windows[i] = MRUWindow{
				ID: uint64(i + 1), Stamp: time.Duration(r.IntN(4)), Stamped: r.IntN(2) == 0,
				OnOutput: r.IntN(2) == 0, OnWorkspace: r.IntN(2) == 0, AppID: appIDs[r.IntN(3)],
			}
		}
		m := NewMRU(windows)
		history := []string{fmt.Sprintf("seed %d", seed)}
		verifyMRU(t, m, history)
		for range r.IntN(40) {
			var op string
			switch r.IntN(7) {
			case 0:
				op = "forward"
				m.Forward()
			case 1:
				op = "backward"
				m.Backward()
			case 2:
				op = "first"
				m.First()
			case 3:
				op = "last"
				m.Last()
			case 4:
				scope := MRUScope(r.IntN(3))
				op = fmt.Sprintf("scope %d", scope)
				m.SetScope(scope)
			case 5:
				on := r.IntN(2) == 0
				op = fmt.Sprintf("filter %v", on)
				m.SetAppIDFilter(on)
			case 6:
				id := uint64(1 + r.IntN(9))
				op = fmt.Sprintf("remove %d", id)
				m.Remove(id)
			}
			history = append(history, op)
			verifyMRU(t, m, history)
		}
	}
}
