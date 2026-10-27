package wlr

import "errors"

const maxCaptureBytes = 64 << 20

func captureAllocation(width, height, stride int) (int, error) {
	if width <= 0 || height <= 0 || width > maxCaptureBytes/4 || stride <= 0 || stride/4 < width || height > maxCaptureBytes/stride {
		return 0, errors.New("wlr: invalid or oversized capture buffer (limit 64 MiB)")
	}
	return height * stride, nil
}
