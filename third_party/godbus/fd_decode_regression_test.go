//go:build !windows && !solaris

package dbus

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestReceivedDescriptorContainersAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      func(UnixFD) []any
		signature string
		count     uint32
		wantError bool
	}{
		{"array", func(fd UnixFD) []any { return []any{[]UnixFD{fd, fd}} }, "ah", 2, false},
		{"nested-array", func(fd UnixFD) []any { return []any{MakeVariant([][]UnixFD{{fd}})} }, "v", 1, false},
		{"dictionary-key", func(fd UnixFD) []any { return []any{map[UnixFD]Variant{fd: MakeVariant(uint32(1))}} }, "a{hv}", 1, false},
		{"unreferenced", func(fd UnixFD) []any { return []any{uint32(99)} }, "u", 1, false},
		{"decode-error", func(fd UnixFD) []any { return nil }, "h", 1, true},
		{"wrong-count", func(fd UnixFD) []any { return []any{fd} }, "h", 2, true},
		{"invalid-index", func(fd UnixFD) []any { return []any{[]UnixFDIndex{3}} }, "ah", 1, true},
		{"duplicate-key", func(fd UnixFD) []any { return []any{map[string]UnixFD{"x": fd, "y": fd}} }, "a{sh}", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connections, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			open := func(fd int) *net.UnixConn {
				file := os.NewFile(uintptr(fd), "wire")
				conn, err := net.FileConn(file)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
				return conn.(*net.UnixConn)
			}
			sender, receiver := open(connections[0]), open(connections[1])
			defer sender.Close()
			defer receiver.Close()
			pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.Close(pair[1])
			message := &Message{Type: TypeMethodCall, serial: 1, Headers: map[HeaderField]Variant{
				FieldPath: MakeVariant(ObjectPath("/test")), FieldInterface: MakeVariant("org.test.FDs"), FieldMember: MakeVariant("Call"),
				FieldSignature: MakeVariant(ParseSignatureMust(tc.signature)), FieldUnixFDs: MakeVariant(tc.count),
			}, Body: tc.body(UnixFD(pair[0]))}
			var wire bytes.Buffer
			rights, err := message.EncodeToWithFDs(&wire, binary.LittleEndian)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "duplicate-key" {
				payload := wire.Bytes()
				i := bytes.LastIndex(payload, []byte{1, 0, 0, 0, 'y', 0})
				if i < 0 {
					t.Fatal("dictionary key missing in encoded payload")
				}
				payload[i+4] = 'x'
			}
			if len(rights) == 0 {
				rights = []int{pair[0]}
			}
			if _, _, err = sender.WriteMsgUnix(wire.Bytes(), syscall.UnixRights(rights...), nil); err != nil {
				t.Fatal(err)
			}
			syscall.Close(pair[0])
			transport := newUnixTransportFromConn(receiver)
			got, err := transport.ReadMessage()
			if (err != nil) != tc.wantError {
				t.Fatalf("ReadMessage error = %v", err)
			}
			if err == nil {
				if len(got.Body) != 1 {
					t.Fatalf("decoded body = %#v, want one complete argument", got.Body)
				}
				switch tc.name {
				case "array":
					for _, fd := range got.Body[0].([]UnixFD) {
						syscall.Close(int(fd))
					}
				case "nested-array":
					for _, fds := range got.Body[0].(Variant).Value().([][]UnixFD) {
						for _, fd := range fds {
							syscall.Close(int(fd))
						}
					}
				case "dictionary-key":
					for fd := range got.Body[0].(map[UnixFD]Variant) {
						syscall.Close(int(fd))
					}
				}
			}
			syscall.SetNonblock(pair[1], true)
			var byte [1]byte
			if n, err := syscall.Read(pair[1], byte[:]); n != 0 || err != nil {
				t.Fatalf("received descriptor leaked: read=%d err=%v", n, err)
			}
		})
	}
}
