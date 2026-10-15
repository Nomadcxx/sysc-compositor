// Command sysc-compositor is a nested Wayland compositor for the sysc desktop.
// It runs inside an existing session and serves its own socket.
package main

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Nomadcxx/sysc-compositor/internal/server"
)

func main() {
	// wlroots objects belong to the thread that created them. Lock before any
	// wlroots call and never hand one to another thread.
	runtime.LockOSThread()

	s, err := server.New()
	if err != nil {
		os.Stderr.WriteString("sysc-compositor: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer s.Destroy()

	// The smoke script reads this line to spawn clients against the nested socket.
	os.Stderr.WriteString("WAYLAND_DISPLAY=" + s.SocketName() + "\n")

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		s.Terminate()
	}()

	if err := s.Run(); err != nil {
		os.Stderr.WriteString("sysc-compositor: " + err.Error() + "\n")
		os.Exit(1)
	}
}
