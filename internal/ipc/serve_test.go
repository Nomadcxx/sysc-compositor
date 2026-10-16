package ipc

import (
	"bufio"
	"net"
	"testing"
)

func TestServeConnAnswersEachLine(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	var after []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		ServeConn(server, func(r Request) (Reply, func()) {
			if r.Name == "Version" {
				return Reply{Ok: VersionResponse("v")}, nil
			}
			return Reply{Ok: Handled}, func() { after = append(after, r.Action.Name) }
		})
	}()
	read := bufio.NewScanner(client)
	for _, tc := range []struct{ line, want string }{
		{`"Version"`, `{"Ok":{"Version":"v"}}`},
		{`not json`, `{"Err":"error parsing request"}`},
		{`{"Action":{"Quit":{"skip_confirmation":true}}}`, `{"Ok":"Handled"}`},
	} {
		if _, err := client.Write([]byte(tc.line + "\n")); err != nil {
			t.Fatal(err)
		}
		if !read.Scan() {
			t.Fatalf("%s: no reply: %v", tc.line, read.Err())
		}
		if got := read.Text(); got != tc.want {
			t.Errorf("%s: reply %s, want %s", tc.line, got, tc.want)
		}
	}
	client.Close()
	<-done
	if len(after) != 1 || after[0] != "Quit" {
		t.Errorf("after-reply hooks ran for %v, want [Quit]", after)
	}
}
