package wlr

/*
#cgo pkg-config: wlroots-0.20 scenefx-0.5
#cgo CFLAGS: -DWLR_USE_UNSTABLE
#include <stdlib.h>
#include <wlr/backend.h>
#include <wlr/backend/multi.h>
#include <wlr/backend/drm.h>
#include <wlr/backend/session.h>
#include <scenefx/render/fx_renderer/fx_renderer.h>

struct sysc_debug_backend_list {
	struct wlr_backend **items;
	size_t count;
};
static void sysc_debug_backend_collect(struct wlr_backend *backend, void *data) {
	struct sysc_debug_backend_list *list = data;
	if (wlr_backend_is_multi(backend)) {
		wlr_multi_for_each_backend(backend, sysc_debug_backend_collect, data);
		return;
	}
	if (!wlr_backend_is_drm(backend)) {
		return;
	}
	struct wlr_backend **items = realloc(list->items, (list->count + 1) * sizeof(*items));
	if (items == NULL) {
		return;
	}
	list->items = items;
	list->items[list->count++] = backend;
}
static struct sysc_debug_backend_list sysc_debug_backend_list(struct wlr_backend *backend) {
	struct sysc_debug_backend_list list = {0};
	sysc_debug_backend_collect(backend, &list);
	return list;
}
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unsafe"
)

func filterDebugDRMDevices(candidates, ignored []string) ([]string, error) {
	var selected []string
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			return nil, fmt.Errorf("DRM device %s: %w", candidate, err)
		}
		excluded := false
		for _, path := range ignored {
			other, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("ignored DRM device %s: %w", path, err)
			}
			if os.SameFile(info, other) {
				excluded = true
				break
			}
		}
		if !excluded {
			selected = append(selected, candidate)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no DRM devices remain after ignore-drm-device")
	}
	return selected, nil
}

// The explicit native GPU list disables wlroots' unfiltered hotplug monitor.
// initializeDebug installs the corresponding filtered session monitor.
func (c *Compositor) createConfiguredBackend() error {
	previous, set := os.LookupEnv("WLR_DRM_DEVICES")
	backends := strings.Split(os.Getenv("WLR_BACKENDS"), ",")
	needsDRM := slices.Contains(backends, "drm") || os.Getenv("WLR_BACKENDS") == "" && os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == ""
	if needsDRM && len(c.debugConfig.IgnoredDRMDevices) > 0 {
		var candidates []string
		if set {
			candidates = strings.Split(previous, ":")
		} else {
			candidates, _ = filepath.Glob("/dev/dri/card[0-9]*")
		}
		selected, err := filterDebugDRMDevices(candidates, c.debugConfig.IgnoredDRMDevices)
		if err != nil {
			return err
		}
		if err = os.Setenv("WLR_DRM_DEVICES", strings.Join(selected, ":")); err != nil {
			return err
		}
		defer func() {
			if set {
				_ = os.Setenv("WLR_DRM_DEVICES", previous)
			} else {
				_ = os.Unsetenv("WLR_DRM_DEVICES")
			}
		}()
		c.debugRuntime.filteredHotplug = true
	}
	c.backend = C.wlr_backend_autocreate(c.loop, &c.session)
	if c.backend == nil {
		return fmt.Errorf("wlr_backend_autocreate failed")
	}
	return nil
}

func (c *Compositor) createConfiguredRenderer() unsafe.Pointer {
	path := c.debugConfig.RenderDRMDevice
	if path == nil {
		return createSceneFXRenderer(unsafe.Pointer(c.backend))
	}
	fd, err := syscall.Open(*path, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysc-compositor: render DRM device: %v\n", err)
		return nil
	}
	defer syscall.Close(fd)
	var stat syscall.Stat_t
	if err = syscall.Fstat(fd, &stat); err != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFCHR {
		fmt.Fprintln(os.Stderr, "sysc-compositor: render-drm-device must name a DRM character device")
		return nil
	}
	// SceneFX creates its EGL device while this descriptor is open and owns
	// the resulting device/context, not the borrowed fd.
	return unsafe.Pointer(C.fx_renderer_create_with_drm_fd(C.int(fd)))
}

func (c *Compositor) debugBackendIgnored(backend *C.struct_wlr_backend) bool {
	fd := int(C.wlr_backend_get_drm_fd(backend))
	if fd < 0 {
		return false
	}
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil {
		return false
	}
	for _, path := range c.debugConfig.IgnoredDRMDevices {
		var other syscall.Stat_t
		if syscall.Stat(path, &other) == nil && other.Mode&syscall.S_IFMT == syscall.S_IFCHR && other.Rdev == stat.Rdev {
			return true
		}
	}
	return false
}

func (c *Compositor) addDebugDRMDevice(path string) {
	if c.session == nil || !c.debugRuntime.filteredHotplug {
		return
	}
	if _, err := filterDebugDRMDevices([]string{path}, c.debugConfig.IgnoredDRMDevices); err != nil {
		return
	}
	devices := C.sysc_debug_backend_list(c.backend)
	defer C.free(unsafe.Pointer(devices.items))
	var primary *C.struct_wlr_backend
	if devices.count > 0 {
		primary = *devices.items
	}
	var wanted syscall.Stat_t
	if syscall.Stat(path, &wanted) != nil {
		return
	}
	for _, backend := range unsafe.Slice(devices.items, int(devices.count)) {
		var current syscall.Stat_t
		if syscall.Fstat(int(C.wlr_backend_get_drm_fd(backend)), &current) == nil && current.Rdev == wanted.Rdev {
			return
		}
	}
	name := C.CString(path)
	defer C.free(unsafe.Pointer(name))
	device := C.wlr_session_open_file(c.session, name)
	if device == nil {
		return
	}
	backend := C.wlr_drm_backend_create(c.session, device, primary)
	if backend == nil {
		C.wlr_session_close_file(c.session, device)
		return
	}
	if !bool(C.wlr_multi_backend_add(c.backend, backend)) || !bool(C.wlr_backend_start(backend)) {
		C.wlr_backend_destroy(backend)
	}
}
