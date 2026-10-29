package wlr

import (
	"strings"
	"testing"
)

// newHeadless builds a compositor on wlroots' headless backend with one
// output. It opens no window on the host, and unlike the nested backend it
// takes DRM syncobj timelines whatever the host compositor offers.
func newHeadless(t *testing.T) *Compositor {
	t.Helper()
	t.Setenv("WLR_BACKENDS", "headless")
	t.Setenv("WLR_HEADLESS_OUTPUTS", "1")
	return newNested(t)
}

// TestExplicitSyncAdvertisedOnlyWithTimelineSupport: clients are offered
// linux-drm-syncobj exactly when the renderer and backend can honour it, so a
// client on a backend without timelines keeps implicit sync.
func TestExplicitSyncAdvertisedOnlyWithTimelineSupport(t *testing.T) {
	c := newNested(t)
	supported := timelineSupportForTest(c)
	globals, _ := advertised(runClient(t, c, []string{"wayland-info"}, nil, nil))
	if _, ok := globals["wp_linux_drm_syncobj_manager_v1"]; ok != supported {
		t.Fatalf("wp_linux_drm_syncobj_manager_v1 advertised=%v, timeline support=%v", ok, supported)
	}
}

// TestExplicitSyncVulkanClientPresentsAndReleases: Mesa's Vulkan WSI submits
// every frame with an acquire and a release point. Its swapchain holds a few
// images, so it reaches a hundred presents only if the compositor waits for
// each acquire point and signals the release points. A window screenshot
// samples the synced buffer through its own acquire wait.
func TestExplicitSyncVulkanClientPresentsAndReleases(t *testing.T) {
	c := newHeadless(t)
	if !timelineSupportForTest(c) {
		t.Skip("needs a renderer and backend with DRM syncobj timelines")
	}
	var shot error
	shotTaken := false
	c.viewMapped = func(view *toplevelView) {
		if shotTaken {
			return
		}
		shotTaken = true
		g := view.geometry()
		_, shot = c.renderSurfaceTree(view.toplevel.base.surface, g.x, g.y, 1, g.w, g.h)
	}
	acquires, releases := 0, 0
	lines := runClient(t, c, []string{"vkcube", "--wsi", "wayland", "--c", "300"},
		[]string{"WAYLAND_DEBUG=client"}, func(line string) bool {
			if !strings.Contains(line, " -> wp_linux_drm_syncobj_surface_v1#") {
				return false
			}
			if strings.Contains(line, ".set_acquire_point(") {
				acquires++
			}
			if strings.Contains(line, ".set_release_point(") {
				releases++
			}
			return releases >= 100
		})
	if acquires < 100 || releases < 100 {
		t.Fatalf("vkcube sent %d acquire and %d release points, want 100 presents:\n%s",
			acquires, releases, tail(lines, 20))
	}
	if !shotTaken {
		t.Fatal("vkcube's window never mapped")
	}
	if shot != nil {
		t.Errorf("window screenshot of a synced buffer failed: %v", shot)
	}
}
