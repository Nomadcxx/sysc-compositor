package wm

import (
	"cmp"
	"slices"
	"time"
)

// MRUScope is the windows the window switcher lists (Niri's MruScope).
type MRUScope uint8

const (
	MRUScopeAll MRUScope = iota
	MRUScopeOutput
	MRUScopeWorkspace
)

// mruScopeCycle is the order the switcher's s key cycles scopes in.
var mruScopeCycle = [...]MRUScope{MRUScopeAll, MRUScopeWorkspace, MRUScopeOutput}

// MRUWindow is one window as the switcher sees it when it opens.
type MRUWindow struct {
	ID uint64
	// Stamp is the focus timestamp, when Stamped.
	Stamp   time.Duration
	Stamped bool
	// OnOutput and OnWorkspace say whether the window is on the switcher's
	// output, and on that output's active workspace.
	OnOutput, OnWorkspace bool
	AppID                 *string
}

// MRU is Niri's WindowMru: the windows, most recently focused first, with a
// scope, an optional app-id filter and the selected window. The selection is
// always one of the listed windows, and there is none only when no window is
// listed.
type MRU struct {
	windows    []MRUWindow
	current    uint64
	hasCurrent bool
	scope      MRUScope
	appID      *string
}

// NewMRU orders windows newest stamp first; unstamped windows go last, and
// ties keep the given order. The newest is selected, in scope All.
func NewMRU(windows []MRUWindow) *MRU {
	m := &MRU{windows: slices.Clone(windows)}
	slices.SortStableFunc(m.windows, func(a, b MRUWindow) int {
		if a.Stamped != b.Stamped {
			if a.Stamped {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Stamp, a.Stamp)
	})
	if len(m.windows) > 0 {
		m.current, m.hasCurrent = m.windows[0].ID, true
	}
	return m
}

// Empty reports whether there are no windows at all, listed or not.
func (m *MRU) Empty() bool { return len(m.windows) == 0 }

// Current is the selected window.
func (m *MRU) Current() (uint64, bool) { return m.current, m.hasCurrent }

func (m *MRU) Scope() MRUScope { return m.scope }

// FilteredByAppID reports whether only the selection's app id is listed.
func (m *MRU) FilteredByAppID() bool { return m.appID != nil }

// Windows is every window, listed or not, in order.
func (m *MRU) Windows() []MRUWindow { return slices.Clone(m.windows) }

// Listed is the windows in scope and filter, in order.
func (m *MRU) Listed() []MRUWindow {
	var listed []MRUWindow
	for _, w := range m.windows {
		if m.matches(w) {
			listed = append(listed, w)
		}
	}
	return listed
}

func (m *MRU) matches(w MRUWindow) bool {
	switch m.scope {
	case MRUScopeOutput:
		if !w.OnOutput {
			return false
		}
	case MRUScopeWorkspace:
		if !w.OnWorkspace {
			return false
		}
	}
	return m.appID == nil || (w.AppID != nil && *w.AppID == *m.appID)
}

// Forward selects the next listed window, wrapping to the first.
func (m *MRU) Forward() { m.step(1) }

// Backward selects the previous listed window, wrapping to the last.
func (m *MRU) Backward() { m.step(-1) }

func (m *MRU) step(by int) {
	if !m.hasCurrent {
		return
	}
	listed := m.Listed()
	i := slices.IndexFunc(listed, func(w MRUWindow) bool { return w.ID == m.current })
	m.current = listed[(i+by+len(listed))%len(listed)].ID
}

func (m *MRU) First() { m.selectListed(0) }

func (m *MRU) Last() { m.selectListed(-1) }

func (m *MRU) selectListed(i int) {
	listed := m.Listed()
	if len(listed) == 0 {
		m.hasCurrent = false
		return
	}
	if i < 0 {
		i = len(listed) - 1
	}
	m.current, m.hasCurrent = listed[i].ID, true
}

// SetCurrent selects id if it is listed.
func (m *MRU) SetCurrent(id uint64) {
	if slices.ContainsFunc(m.Listed(), func(w MRUWindow) bool { return w.ID == id }) {
		m.current, m.hasCurrent = id, true
	}
}

// SetScope changes the scope and reports whether it changed. The selection
// stays, or moves to the nearest listed window before it, else the first.
func (m *MRU) SetScope(scope MRUScope) bool {
	if m.scope == scope {
		return false
	}
	m.relist(func() { m.scope = scope })
	return true
}

// CycleScope moves to the next scope in Niri's cycle: All, Workspace, Output.
// It reports whether the scope changed, which it always does.
func (m *MRU) CycleScope() bool {
	i := slices.Index(mruScopeCycle[:], m.scope)
	return m.SetScope(mruScopeCycle[(i+1)%len(mruScopeCycle)])
}

// SetAppIDFilter lists only the selection's app id, or every app id, and
// reports whether the filter changed. Filtering needs a selection with an
// app id. The selection moves as SetScope moves it.
func (m *MRU) SetAppIDFilter(on bool) bool {
	if on == (m.appID != nil) {
		return false
	}
	if !on {
		m.relist(func() { m.appID = nil })
		return true
	}
	if !m.hasCurrent {
		return false
	}
	appID := m.windows[m.index(m.current)].AppID
	if appID == nil {
		return false
	}
	m.relist(func() { m.appID = appID })
	return true
}

// relist applies change to the scope or filter, then selects the current
// window, else the last listed window before it, else the first listed.
func (m *MRU) relist(change func()) {
	current := -1
	if m.hasCurrent {
		current = m.index(m.current)
	}
	change()
	m.hasCurrent = false
	for i, w := range m.windows {
		if !m.matches(w) {
			continue
		}
		if !m.hasCurrent || i <= current {
			m.current, m.hasCurrent = w.ID, true
		}
		if i >= current {
			break
		}
	}
}

// Remove drops a window that closed. A selected window passes the selection
// on to the next listed one.
func (m *MRU) Remove(id uint64) {
	i := m.index(id)
	if i < 0 {
		return
	}
	if m.hasCurrent && m.current == id {
		m.Forward()
		if m.current == id {
			m.hasCurrent = false
		}
	}
	m.windows = slices.Delete(m.windows, i, i+1)
}

func (m *MRU) index(id uint64) int {
	return slices.IndexFunc(m.windows, func(w MRUWindow) bool { return w.ID == id })
}
