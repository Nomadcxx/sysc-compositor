package server

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
)

func TestNiriPickerRequestsReturnTheirCorrelatedResult(t *testing.T) {
	h := niriHandler{
		pickWindow: func(context.Context) (*ipc.Window, error) { return &ipc.Window{ID: 7}, nil },
		pickColor:  func(context.Context) ([3]float64, error) { return [3]float64{.25, .5, .75}, nil },
	}
	reply, after := h.handle(ipc.Request{Name: "PickWindow"})
	if reply.Err != "" || after != nil {
		t.Fatalf("window reply=%+v after=%v", reply, after != nil)
	}
	var picked struct{ PickedWindow *ipc.Window }
	if err := json.Unmarshal(reply.Ok, &picked); err != nil || picked.PickedWindow == nil || picked.PickedWindow.ID != 7 {
		t.Fatalf("window result=%s err=%v", reply.Ok, err)
	}
	reply, after = h.handle(ipc.Request{Name: "PickColor"})
	if reply.Err != "" || after != nil || string(reply.Ok) != `{"PickedColor":{"rgb":[0.25,0.5,0.75]}}` {
		t.Fatalf("color reply=%+v after=%v", reply, after != nil)
	}
	h.pickWindow = func(context.Context) (*ipc.Window, error) { return nil, nil }
	reply, _ = h.handle(ipc.Request{Name: "PickWindow"})
	if string(reply.Ok) != `{"PickedWindow":null}` {
		t.Fatalf("empty pick=%s", reply.Ok)
	}
}

func TestNiriPickerDisconnectCancelsTheRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	started, cancelled, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := niriHandler{pickWindow: func(ctx context.Context) (*ipc.Window, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}}
	go func() {
		defer close(stopped)
		c, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer c.Close()
		h.serve(c)
	}()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Write([]byte("\"PickWindow\"\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("picker never started")
	}
	client.Close()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect left the picker running")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnected request did not finish")
	}
}
