package wlr

import (
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

var rePreferredScale = regexp.MustCompile(`wp_fractional_scale_v1#\d+\.preferred_scale\((\d+)\)`)

// TestFractionalScaleFollowsOutput: a client on an output at scale s is told
// s*120, which is how the protocol spells fractional scales; the drawn pointer
// is loaded at the same scale so it is not blurred.
func TestFractionalScaleFollowsOutput(t *testing.T) {
	for _, tc := range []struct {
		scale float64
		want  string
	}{
		{1, "120"},
		{1.5, "180"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			c := newNested(t)
			if err := c.setOutputScale(tc.scale); err != nil {
				t.Fatal(err)
			}
			var got []string
			lines := runClient(t, c, []string{"foot"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
				return rePreferredScale.MatchString(line)
			})
			for _, line := range lines {
				if m := rePreferredScale.FindStringSubmatch(line); m != nil {
					got = append(got, m[1])
				}
			}
			if len(got) == 0 {
				t.Fatal("foot never received a preferred_scale")
			}
			if got[len(got)-1] != tc.want {
				t.Errorf("preferred_scale %v, want %s", got, tc.want)
			}
			if !c.cursorLoadedAt(tc.scale) {
				t.Errorf("no pointer image loaded at scale %v", tc.scale)
			}
		})
	}
}

// TestDmabufClientMaps: a client gets the scene's dmabuf feedback, and a
// client that allocates its buffers on the GPU maps a window from them.
// weston-simple-dmabuf-feedback asks for fullscreen and cannot draw at the
// 0x0 size a compositor without fullscreen (M6) sends, so it only proves the
// feedback; weston-simple-dmabuf-egl draws a fixed-size window.
func TestDmabufClientMaps(t *testing.T) {
	t.Run("feedback", func(t *testing.T) {
		c := newNested(t)
		var feedback bool
		lines := runClient(t, c, []string{"weston-simple-dmabuf-feedback"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
			feedback = feedback || strings.Contains(line, "zwp_linux_dmabuf_feedback_v1#") && strings.HasSuffix(line, ".done()")
			return feedback
		})
		if !feedback {
			t.Fatalf("no dmabuf feedback reached the client:\n%s", tail(lines, 20))
		}
	})
	t.Run("maps", func(t *testing.T) {
		c := newNested(t)
		var mapped atomic.Bool
		c.viewMapped = func(*toplevelView) {
			mapped.Store(true)
			c.Terminate()
		}
		var dmabuf bool
		lines := runClient(t, c, []string{"weston-simple-dmabuf-egl"}, []string{"WAYLAND_DEBUG=client"}, func(line string) bool {
			dmabuf = dmabuf || strings.Contains(line, "zwp_linux_buffer_params_v1#") && strings.Contains(line, "create")
			return false
		})
		if !dmabuf {
			t.Fatalf("the client created no dmabuf buffer:\n%s", tail(lines, 20))
		}
		if !mapped.Load() {
			t.Fatalf("the client never mapped:\n%s", tail(lines, 20))
		}
	})
}

func tail(lines []string, n int) string {
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
