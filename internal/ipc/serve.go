package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
)

// MaxLine bounds one request line. Niri reads to the newline; a line this
// long is no request this wire defines.
const MaxLine = 64 << 10

// Handler answers one decoded request. The returned func, when not nil, runs
// after the reply is written: Quit uses it so the reply is not lost to the
// shutdown it causes.
type Handler func(Request) (Reply, func())

// ServeConn answers requests on one connection until it closes, any number
// per connection, as Niri does (src/ipc/server.rs).
func ServeConn(c net.Conn, h Handler) {
	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, 4096), MaxLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var reply Reply
		var after func()
		if r, err := DecodeRequest(line); err != nil {
			reply = Reply{Err: err.Error()}
		} else {
			reply, after = h(r)
		}
		out, _ := json.Marshal(reply)
		if _, err := c.Write(append(out, '\n')); err != nil {
			return
		}
		if after != nil {
			after()
		}
	}
}
