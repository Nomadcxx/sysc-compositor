package unixsock

import (
	"errors"
	"testing"
)

func TestPeerDecision(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		peer, own uint32
		err       error
		want      bool
	}{
		{"same uid", 1000, 1000, nil, true},
		{"other uid", 1001, 1000, nil, false},
		{"root is not the user", 0, 1000, nil, false},
		{"credentials unreadable", 1000, 1000, errors.New("EINVAL"), false},
	} {
		if got := acceptPeer(tc.peer, tc.own, tc.err); got != tc.want {
			t.Errorf("%s: acceptPeer = %v, want %v", tc.name, got, tc.want)
		}
	}
}
