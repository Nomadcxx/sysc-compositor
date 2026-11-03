package server

import (
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

// layoutSizeChange turns a size change from the wire into the one the layout
// works with. Niri counts proportions in percent, so the values carry over
// unchanged.
func layoutSizeChange(change ipc.SizeChange) layout.SizeChange {
	switch change.Kind {
	case ipc.SizeSetFixed:
		return layout.SizeChange{Fixed: int(change.Value)}
	case ipc.SizeAdjustFixed:
		return layout.SizeChange{Fixed: int(change.Value), Relative: true}
	case ipc.SizeAdjustProportion:
		return layout.SizeChange{Proportion: change.Value, Relative: true}
	default:
		return layout.SizeChange{Proportion: change.Value}
	}
}
