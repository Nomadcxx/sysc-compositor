package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE

#include <wlr/types/wlr_seat.h>
*/
import "C"

import (
	"strconv"
	"unsafe"
)

// Seat capability bits, re-exported so callers can assert against the probe
// without cgo. cgo is not available in _test.go files, and a test that has to
// reach into the package internals to check a public result is a test that
// breaks when the internals move. They stay untyped so no cgo type leaks into
// the package's exported surface.
const (
	CKeyboard = 1 << iota
	CPointer
	CTouch
)

// SeatProbeResult is what the probe observed, kept as plain data so the
// finding can be recorded in a design and compared against a later wlroots.
type SeatProbeResult struct {
	// Seats is how many seats the compositor was able to hold at once.
	Seats int
	// Distinct reports whether each seat is a separate object rather than the
	// same one handed back under a second name.
	Distinct bool
	// Capabilities is the advertised bitmask per seat, in creation order.
	Capabilities []uint32
	// Isolated reports whether clearing one seat left the others untouched.
	Isolated bool
}

// ProbeSeats answers the question the Tier 3 seat work is gated on: whether
// wlroots 0.20 can hold more than one seat, and whether the state on each seat
// is independent.
//
// extra is how many seats to create beyond the first, so the agent design's
// "two extra seats" is a value to change rather than a rewrite. It is
// deliberately not defaulted: a probe that silently picks its own number is a
// probe that answers a question nobody asked.
//
// The seats this creates are deliberately not stored anywhere. They exist to
// be observed and then handed back to wlroots by the display teardown, and
// keeping them would make the probe the owner of compositor state it does not
// own.
func ProbeSeats(display unsafe.Pointer, extra int) SeatProbeResult {
	result := SeatProbeResult{Seats: 1 + extra, Distinct: true, Isolated: false}
	if extra < 1 {
		return result
	}

	dpy := (*C.struct_wl_display)(display)
	seats := make([]*C.struct_wlr_seat, 0, result.Seats)
	for i := 0; i < result.Seats; i++ {
		seat := C.wlr_seat_create(dpy, C.CString("seat"+strconv.Itoa(i)))
		if seat == nil {
			result.Seats = len(seats)
			break
		}
		for _, other := range seats {
			if unsafe.Pointer(other) == unsafe.Pointer(seat) {
				result.Distinct = false
			}
		}
		seats = append(seats, seat)
	}

	// Give each seat a different capability set, then read it all back. If the
	// state were shared the reads would agree; that is the whole test.
	wants := []uint32{
		CKeyboard,
		CPointer,
		CKeyboard | CPointer,
		CTouch,
	}
	for i, seat := range seats {
		C.wlr_seat_set_capabilities(seat, C.uint32_t(wants[i%len(wants)]))
	}
	for _, seat := range seats {
		result.Capabilities = append(result.Capabilities, uint32(seat.capabilities))
	}

	// Clear every seat but the first. If clearing one reaches the others, the
	// seats are not independent and an agent seat would be able to take the
	// user's input away.
	before := uint32(seats[0].capabilities)
	for i := 1; i < len(seats); i++ {
		C.wlr_seat_set_capabilities(seats[i], 0)
	}
	result.Isolated = before == uint32(seats[0].capabilities)

	return result
}
