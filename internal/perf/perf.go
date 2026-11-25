// Package perf serves opt-in, process-local benchmark diagnostics.
package perf

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime/metrics"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var active atomic.Pointer[Server]

// Server owns a private diagnostics socket. The disabled path creates nothing.
type Server struct {
	http                                              *http.Server
	done                                              chan struct{}
	snapshots, snapshotNS, outputFrames, sceneCommits atomic.Uint64
	sceneWorkNS, presented, discarded, zeroCopy       atomic.Uint64
	blurCacheRebuilds                                 atomic.Uint64
	inputDelay                                        [inputKinds]inputDelay
}

// InputKind names the libinput events whose delivery delay is recorded.
type InputKind int

const (
	InputMotion InputKind = iota
	InputButton
	InputAxis
	InputKey
	inputKinds
)

var inputKindNames = [inputKinds]string{"motion", "button", "axis", "key"}

// inputDelayBoundsMS are the inclusive upper bounds of the delay buckets; a
// last bucket counts everything above them. Timestamps have 1 ms resolution.
var inputDelayBoundsMS = [...]uint64{1, 2, 4, 8, 16, 20, 33, 50, 100, 250}

// inputSlowMS is the delay counted as a lag, like libinput's lag reports.
const inputSlowMS = 20

type inputDelay struct {
	count, sumMS, maxMS, slow atomic.Uint64
	buckets                   [len(inputDelayBoundsMS) + 1]atomic.Uint64
}

// Start enables diagnostics at path, or returns nil when path is empty.
// A private, same-owner directory prevents access during bind and chmod.
func Start(path string) (*Server, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("benchmark socket path must be absolute")
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("benchmark socket directory must be private and owned by uid %d", os.Getuid())
	}
	s := &Server{done: make(chan struct{})}
	if !active.CompareAndSwap(nil, s) {
		return nil, fmt.Errorf("benchmark diagnostics already enabled")
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		active.CompareAndSwap(s, nil)
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		active.CompareAndSwap(s, nil)
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/metrics", s.metrics)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { defer close(s.done); _ = s.http.Serve(ln) }()
	return s, nil
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	active.CompareAndSwap(s, nil)
	err := s.http.Close()
	<-s.done
	return err
}

func Enabled() bool { return active.Load() != nil }

// RecordSnapshot records wall duration, including any GC pause in the call.
func RecordSnapshot(start time.Time) {
	if s := active.Load(); s != nil {
		s.snapshots.Add(1)
		s.snapshotNS.Add(uint64(time.Since(start)))
	}
}

// OutputFrame counts output frame events and actual output commit sequences.
func OutputFrame(committed bool, elapsed time.Duration) {
	if s := active.Load(); s != nil {
		s.outputFrames.Add(1)
		s.sceneWorkNS.Add(uint64(elapsed))
		if committed {
			s.sceneCommits.Add(1)
		}
	}
}

// RecordBlurCacheRebuild counts backdrops refreshed during native repaint.
func RecordBlurCacheRebuild() {
	if s := active.Load(); s != nil {
		s.blurCacheRebuilds.Add(1)
	}
}

// RecordInputDelay records how long after eventMsec, a libinput timestamp on
// CLOCK_MONOTONIC, the compositor handled the event. A timestamp ahead of the
// clock or from long ago comes from another clock and is dropped.
func RecordInputDelay(kind InputKind, eventMsec uint32) {
	s := active.Load()
	if s == nil {
		return
	}
	var now unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &now) != nil {
		return
	}
	// The timestamp wraps with the clock's milliseconds, so compare modulo 2^32.
	delay := int32(uint32(now.Nano()/int64(time.Millisecond)) - eventMsec)
	if delay < 0 || delay > int32(time.Minute/time.Millisecond) {
		return
	}
	s.inputDelay[kind].record(uint64(delay))
}

func (d *inputDelay) record(ms uint64) {
	d.count.Add(1)
	d.sumMS.Add(ms)
	for prev := d.maxMS.Load(); ms > prev && !d.maxMS.CompareAndSwap(prev, ms); prev = d.maxMS.Load() {
	}
	if ms > inputSlowMS {
		d.slow.Add(1)
	}
	bucket := len(inputDelayBoundsMS)
	for i, bound := range inputDelayBoundsMS {
		if ms <= bound {
			bucket = i
			break
		}
	}
	d.buckets[bucket].Add(1)
}

func (d *inputDelay) snapshot() map[string]any {
	counts := make([]uint64, len(d.buckets))
	for i := range d.buckets {
		counts[i] = d.buckets[i].Load()
	}
	return map[string]any{"count": d.count.Load(), "sum_ms": d.sumMS.Load(), "max_ms": d.maxMS.Load(),
		"over_20ms": d.slow.Load(), "bounds_ms": inputDelayBoundsMS, "counts": counts}
}

// OutputPresent counts wlroots completion events. Zero-copy is the native
// presentation flag, not a guess based on a fullscreen application's state.
func OutputPresent(presented bool, flags uint32) {
	if s := active.Load(); s != nil {
		if !presented {
			s.discarded.Add(1)
			return
		}
		s.presented.Add(1)
		if flags&0x8 != 0 {
			s.zeroCopy.Add(1)
		}
	}
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	names := []string{"/gc/cycles/total:gc-cycles", "/gc/heap/live:bytes", "/gc/heap/allocs:bytes", "/gc/pauses:seconds", "/sched/latencies:seconds", "/memory/classes/heap/objects:bytes"}
	samples := make([]metrics.Sample, len(names))
	for i, name := range names {
		samples[i].Name = name
	}
	metrics.Read(samples)
	values := make(map[string]any, len(samples))
	for _, sample := range samples {
		switch sample.Value.Kind() {
		case metrics.KindUint64:
			values[sample.Name] = sample.Value.Uint64()
		case metrics.KindFloat64:
			values[sample.Name] = sample.Value.Float64()
		case metrics.KindFloat64Histogram:
			h := sample.Value.Float64Histogram()
			bounds := make([]any, len(h.Buckets))
			for i, v := range h.Buckets {
				switch {
				case math.IsInf(v, 1):
					bounds[i] = "+Inf"
				case math.IsInf(v, -1):
					bounds[i] = "-Inf"
				default:
					bounds[i] = v
				}
			}
			values[sample.Name] = map[string]any{"buckets": bounds, "counts": h.Counts}
		}
	}
	delays := make(map[string]any, inputKinds)
	for kind, name := range inputKindNames {
		delays[name] = s.inputDelay[kind].snapshot()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"input_delay": delays, "snapshots": s.snapshots.Load(), "snapshot_ns": s.snapshotNS.Load(), "output_frames": s.outputFrames.Load(), "scene_commits": s.sceneCommits.Load(), "scene_work_ns": s.sceneWorkNS.Load(), "blur_cache_rebuilds": s.blurCacheRebuilds.Load(), "presented": s.presented.Load(), "discarded": s.discarded.Load(), "zero_copy_presents": s.zeroCopy.Load(), "runtime": values})
}
