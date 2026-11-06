package wlr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClassifyPointerFollowsNiri(t *testing.T) {
	for _, tc := range []struct {
		fingers int
		props   map[string]string
		want    pointerKind
	}{
		{2, map[string]string{"ID_INPUT_POINTINGSTICK": "1"}, kindTouchpad},
		{0, map[string]string{"ID_INPUT_POINTINGSTICK": "1"}, kindTrackpoint},
		{0, map[string]string{"ID_INPUT_TRACKBALL": "1"}, kindTrackball},
		{0, nil, kindMouse},
	} {
		if got := classifyPointer(tc.fingers, tc.props); got != tc.want {
			t.Errorf("classify(%d,%v)=%v want %v", tc.fingers, tc.props, got, tc.want)
		}
	}
}

func TestUdevPropertiesReadsTheDatabase(t *testing.T) {
	root := t.TempDir()
	for path, body := range map[string]string{"sys/class/input/event7/dev": "13:71\n", "run/udev/data/c13:71": "E:ID_INPUT_POINTINGSTICK=1\nG:seat\n"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := udevProperties(root, "event7"); !reflect.DeepEqual(got, map[string]string{"ID_INPUT_POINTINGSTICK": "1"}) {
		t.Fatalf("properties=%v", got)
	}
	for _, name := range []string{"event8", "../event7", "/event7"} {
		if got := udevProperties(root, name); len(got) != 0 {
			t.Errorf("properties(%q)=%v", name, got)
		}
	}
}
