package x11

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The helper consumes the inherited listening descriptor, as satellite does.
// It never starts Xwayland or connects to a compositor.
func TestSatelliteChild(t *testing.T) {
	if os.Getenv("SYSC_X11_TEST_CHILD") != "1" {
		return
	}
	for _, arg := range os.Args {
		if arg == "--test-listenfd-support" {
			os.Exit(0)
		}
	}
	f, err := os.OpenFile(os.Getenv("SYSC_X11_TEST_STARTS"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(2)
	}
	f.WriteString("start\n")
	f.Close()
	ln, err := net.FileListener(os.NewFile(3, "X11"))
	if err != nil {
		os.Exit(3)
	}
	conn, err := ln.Accept()
	if err != nil {
		os.Exit(4)
	}
	fmt.Fprintln(conn, os.Getenv("WAYLAND_DISPLAY")+"|"+os.Getenv("DISPLAY")+"|"+os.Getenv("NOTIFY_SOCKET")+"|"+os.Getenv("WAYLAND_SOCKET"))
	// Stay alive until the test closes the connection, so Close also proves
	// cancellation while a child is running.
	buf := make([]byte, 1)
	conn.Read(buf)
	os.Exit(0)
}

func TestSatelliteLifecycle(t *testing.T) {
	// AF_UNIX paths have a 108-byte ceiling; build TMPDIR can be much longer.
	dir, err := os.MkdirTemp("/var/tmp", "sysc-x11-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYSC_X11_TEST_BINARY", binary)
	t.Setenv("SYSC_X11_TEST_CHILD", "1")
	starts := filepath.Join(dir, "starts")
	t.Setenv("SYSC_X11_TEST_STARTS", starts)
	t.Setenv("DISPLAY", ":host")
	t.Setenv("WAYLAND_SOCKET", "3")
	t.Setenv("NOTIFY_SOCKET", "/host-notify")
	path := filepath.Join(dir, "satellite")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec \"$SYSC_X11_TEST_BINARY\" -test.run=^TestSatelliteChild$ -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s, err := open(path, "wayland-private", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := os.Stat(starts); !os.IsNotExist(err) {
		t.Fatalf("satellite started without an X11 connection: %v", err)
	}
	other, err := open(path, "wayland-other", dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Display() == other.Display() {
		t.Fatal("two owners reserved the same display")
	}
	otherLock := filepath.Join(dir, ".X"+strings.TrimPrefix(other.Display(), ":")+"-lock")
	if err := os.Rename(otherLock, otherLock+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherLock, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	other.Close()
	if data, _ := os.ReadFile(otherLock); string(data) != "replacement" {
		t.Fatal("shutdown removed a lock it no longer owned")
	}
	badPath := filepath.Join(dir, "broken-satellite")
	if err := os.WriteFile(badPath, []byte("#!/bin/sh\nprintf 'failed\\n' >> \"$SYSC_X11_TEST_STARTS\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s.SetPath(badPath)
	socketPath := filepath.Join(dir, ".X11-unix", "X"+strings.TrimPrefix(s.Display(), ":"))
	failed, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	failed.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := bufio.NewReader(failed).ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("failed activation did not drain the queued client: %v", err)
	}
	failed.Close()
	// With the first client drained, no new connection means no retry.
	time.Sleep(50 * time.Millisecond)
	if data, _ := os.ReadFile(starts); string(data) != "failed\n" {
		t.Fatalf("failed activation restarted without a new client: %q", data)
	}
	s.SetPath(path)
	connect := func() net.Conn {
		t.Helper()
		var last error
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			conn, err := net.DialTimeout("unix", filepath.Join(dir, ".X11-unix", "X"+strings.TrimPrefix(s.Display(), ":")), time.Second)
			if err != nil {
				last = err
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn.SetReadDeadline(time.Now().Add(time.Second))
			line, err := bufio.NewReader(conn).ReadString('\n')
			if err == nil {
				if line != "wayland-private|||\n" {
					t.Fatalf("child environment = %q", line)
				}
				return conn
			}
			last = err
			conn.Close()
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("satellite never accepted the pending client: %v", last)
		return nil
	}
	first := connect()
	first.Close()
	second := connect()
	s.Close()
	second.Close()
	s.Close()
	for _, name := range []string{filepath.Join(dir, ".X11-unix", "X0"), filepath.Join(dir, ".X0-lock")} {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			t.Errorf("owned path survived shutdown: %s (%v)", name, err)
		}
	}
	if data, _ := os.ReadFile(starts); strings.Count(string(data), "start\n") < 2 {
		t.Fatalf("no reactivation after child exit: %q", data)
	}
	unsafeDir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(unsafeDir, ".X11-unix")); err != nil {
		t.Fatal(err)
	}
	if bad, err := open(path, "w", unsafeDir); err == nil {
		bad.Close()
		t.Fatal("accepted a symlink as the shared X11 directory")
	}
}
