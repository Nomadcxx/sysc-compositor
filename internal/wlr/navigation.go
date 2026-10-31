package wlr

import (
	"strings"
	"unicode"

	"github.com/Nomadcxx/sysc-compositor/internal/bind"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
)

// navigationKind says how a navigation action reaches the scene: focus
// actions move keyboard focus, moves only rearrange, and crossing actions
// may change the focused output.
type navigationKind uint8

const (
	navigationFocus navigationKind = iota
	navigationMove
	navigationCrossing
)

type navigationAction struct {
	kind navigationKind
	run  func(m *wm.Model, index int, workspaceID uint64) bool
}

func plainNavigation(kind navigationKind, action func(*wm.Model) bool) navigationAction {
	return navigationAction{kind, func(m *wm.Model, _ int, _ uint64) bool { return action(m) }}
}

// navigationActions is M15 part 1: Niri's remaining navigation and
// workspace actions, by bind name. Binds and IPC both run them here.
var navigationActions = map[string]navigationAction{
	bind.ActionFocusWorkspaceDown:     plainNavigation(navigationFocus, (*wm.Model).FocusWorkspaceDown),
	bind.ActionFocusWorkspaceUp:       plainNavigation(navigationFocus, (*wm.Model).FocusWorkspaceUp),
	bind.ActionFocusWorkspacePrevious: plainNavigation(navigationFocus, (*wm.Model).FocusWorkspacePrevious),
	bind.ActionMoveWorkspaceDown:      plainNavigation(navigationFocus, (*wm.Model).MoveWorkspaceDown),
	bind.ActionMoveWorkspaceUp:        plainNavigation(navigationFocus, (*wm.Model).MoveWorkspaceUp),
	bind.ActionMoveWorkspaceToIndex: {navigationFocus, func(m *wm.Model, index int, workspaceID uint64) bool {
		return m.MoveWorkspaceToIndex(index, workspaceID)
	}},
	bind.ActionFocusColumn: {navigationFocus, func(m *wm.Model, index int, _ uint64) bool {
		return m.FocusColumn(index)
	}},
	bind.ActionFocusColumnRightOrFirst:     plainNavigation(navigationFocus, (*wm.Model).FocusColumnRightOrFirst),
	bind.ActionFocusColumnLeftOrLast:       plainNavigation(navigationFocus, (*wm.Model).FocusColumnLeftOrLast),
	bind.ActionFocusWindowOrMonitorUp:      plainNavigation(navigationCrossing, (*wm.Model).FocusWindowOrMonitorUp),
	bind.ActionFocusWindowOrMonitorDown:    plainNavigation(navigationCrossing, (*wm.Model).FocusWindowOrMonitorDown),
	bind.ActionFocusColumnOrMonitorLeft:    plainNavigation(navigationCrossing, (*wm.Model).FocusColumnOrMonitorLeft),
	bind.ActionFocusColumnOrMonitorRight:   plainNavigation(navigationCrossing, (*wm.Model).FocusColumnOrMonitorRight),
	bind.ActionFocusWindowDownOrColumnLeft: plainNavigation(navigationFocus, (*wm.Model).FocusWindowDownOrColumnLeft),
	bind.ActionFocusWindowDownOrColumnRight: plainNavigation(navigationFocus,
		(*wm.Model).FocusWindowDownOrColumnRight),
	bind.ActionFocusWindowUpOrColumnLeft:  plainNavigation(navigationFocus, (*wm.Model).FocusWindowUpOrColumnLeft),
	bind.ActionFocusWindowUpOrColumnRight: plainNavigation(navigationFocus, (*wm.Model).FocusWindowUpOrColumnRight),
	bind.ActionFocusWindowOrWorkspaceDown: plainNavigation(navigationFocus, (*wm.Model).FocusWindowOrWorkspaceDown),
	bind.ActionFocusWindowOrWorkspaceUp:   plainNavigation(navigationFocus, (*wm.Model).FocusWindowOrWorkspaceUp),
	bind.ActionFocusWindowTop:             plainNavigation(navigationFocus, (*wm.Model).FocusWindowTop),
	bind.ActionFocusWindowBottom:          plainNavigation(navigationFocus, (*wm.Model).FocusWindowBottom),
	bind.ActionFocusWindowDownOrTop:       plainNavigation(navigationFocus, (*wm.Model).FocusWindowDownOrTop),
	bind.ActionFocusWindowUpOrBottom:      plainNavigation(navigationFocus, (*wm.Model).FocusWindowUpOrBottom),
	bind.ActionMoveColumnLeftOrToMonitorLeft: plainNavigation(navigationCrossing,
		(*wm.Model).MoveColumnLeftOrToMonitorLeft),
	bind.ActionMoveColumnRightOrToMonitorRight: plainNavigation(navigationCrossing,
		(*wm.Model).MoveColumnRightOrToMonitorRight),
	bind.ActionMoveColumnToIndex: {navigationMove, func(m *wm.Model, index int, _ uint64) bool {
		return m.MoveColumnToIndex(index)
	}},
	bind.ActionMoveWindowDownOrToWorkspaceDown: plainNavigation(navigationFocus,
		(*wm.Model).MoveWindowDownOrToWorkspaceDown),
	bind.ActionMoveWindowUpOrToWorkspaceUp: plainNavigation(navigationFocus, (*wm.Model).MoveWindowUpOrToWorkspaceUp),
	bind.ActionSwapWindowLeft:              plainNavigation(navigationMove, (*wm.Model).SwapWindowLeft),
	bind.ActionSwapWindowRight:             plainNavigation(navigationMove, (*wm.Model).SwapWindowRight),
}

// NavigationAction runs one of these actions by its niri-ipc name (for
// example FocusWorkspaceDown) and reports whether the name is one of them.
// workspaceID is MoveWorkspaceToIndex's resolved reference, 0 for the
// focused output's active workspace.
func (c *Compositor) NavigationAction(ipcName string, index int, workspaceID uint64) bool {
	action, ok := navigationActions[kebabName(ipcName)]
	if ok {
		c.runNavigation(action, index, workspaceID)
	}
	return ok
}

func (c *Compositor) runNavigation(action navigationAction, index int, workspaceID uint64) {
	if c.model == nil {
		return
	}
	output := c.model.Snapshot().FocusedOutput
	if !action.run(c.model, index, workspaceID) {
		return
	}
	c.applyWorkspaceModel()
	if action.kind == navigationMove {
		c.notifyWindowStateChanged()
		return
	}
	c.activateWorkspaceFocus()
	if focused := c.model.Snapshot().FocusedOutput; action.kind == navigationCrossing && focused != output {
		c.warpCursorToOutput(focused)
	}
}

// kebabName turns a niri-ipc action name into its KDL bind name.
func kebabName(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
