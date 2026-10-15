package wlr

/*
#cgo pkg-config: wlroots-0.20 wayland-server
#cgo CFLAGS: -DWLR_USE_UNSTABLE
*/
import "C"

const ABI = "0.20"

func init() { _ = C.int(0) }
