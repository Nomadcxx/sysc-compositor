package wlr

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type pointerKind uint8

const (
	kindMouse pointerKind = iota
	kindTouchpad
	kindTrackpoint
	kindTrackball
)

func classifyPointer(tapFingers int, udev map[string]string) pointerKind {
	switch {
	case tapFingers > 0:
		return kindTouchpad
	case udev["ID_INPUT_POINTINGSTICK"] == "1":
		return kindTrackpoint
	case udev["ID_INPUT_TRACKBALL"] == "1":
		return kindTrackball
	default:
		return kindMouse
	}
}

func udevProperties(root, sysname string) map[string]string {
	properties := map[string]string{}
	if sysname == "" || sysname == "." || sysname == ".." || filepath.Base(sysname) != sysname {
		return properties
	}
	dev, err := os.ReadFile(filepath.Join(root, "sys/class/input", sysname, "dev"))
	if err != nil {
		return properties
	}
	number := strings.TrimSpace(string(dev))
	major, minor, ok := strings.Cut(number, ":")
	if !ok {
		return properties
	}
	if _, err := strconv.ParseUint(major, 10, 32); err != nil {
		return properties
	}
	if _, err := strconv.ParseUint(minor, 10, 32); err != nil {
		return properties
	}
	data, err := os.ReadFile(filepath.Join(root, "run/udev/data", "c"+number))
	if err != nil {
		return properties
	}
	for _, line := range strings.Split(string(data), "\n") {
		if entry, ok := strings.CutPrefix(line, "E:"); ok {
			if key, value, ok := strings.Cut(entry, "="); ok {
				properties[key] = value
			}
		}
	}
	return properties
}
