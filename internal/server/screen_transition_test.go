package server

import (
	"errors"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

func TestScreenTransitionReplyFollowsTheSnapshot(t *testing.T) {
	delay := uint16(42)
	called := false
	h := niriHandler{screenTransition: func(got *uint16) error {
		called = true
		if got == nil || *got != delay {
			t.Error("delay lost")
		}
		return nil
	}}
	reply, after := h.action(ipc.Action{Name: "DoScreenTransition", DelayMS: &delay})
	if !called || after != nil || reply.Err != "" {
		t.Fatalf("snapshot not completed before reply: called=%t after=%t reply=%+v", called, after != nil, reply)
	}
	h.screenTransition = func(*uint16) error { return errors.New("capture failed") }
	reply, _ = h.action(ipc.Action{Name: "DoScreenTransition"})
	if reply.Err != "capture failed" {
		t.Fatalf("capture failure reply=%+v", reply)
	}
}
