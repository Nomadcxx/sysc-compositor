package wlr

import (
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestDumpSceneLabelsLiveViews(t *testing.T) {
	c := newNested(t)
	runTouchClients(t, c, 1, func(_ []*testclient.ToplevelClient, views []*toplevelView) {
		onCompositor(t, c, func() {
			var out strings.Builder
			c.DumpScene(&out)
			dump := out.String()
			t.Log(dump)
			for _, want := range []string{"scene dump: views=1", "<layer:windows>", "<windowAnimationTree>", "surface"} {
				if !strings.Contains(dump, want) {
					t.Errorf("dump lacks %q", want)
				}
			}
			if !strings.Contains(dump, "<view ") {
				t.Error("dump does not label the live view")
			}
		})
	})
}
