package perf

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchmarkDiagnostics(t *testing.T) {
	if s, err := Start(""); err != nil || s != nil || Enabled() {
		t.Fatalf("disabled diagnostics: %v %v", s, err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "perf.sock")
	s, err := Start(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions: %v %v", info, err)
	}
	RecordSnapshot(time.Now().Add(-time.Millisecond))
	OutputFrame(true, time.Millisecond)
	OutputFrame(false, time.Millisecond)
	OutputPresent(true, 0x9)
	OutputPresent(false, 0)
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) { return net.Dial("unix", path) },
	}}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://localhost/metrics")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Snapshots    uint64         `json:"snapshots"`
		SnapshotNS   uint64         `json:"snapshot_ns"`
		OutputFrames uint64         `json:"output_frames"`
		SceneCommits uint64         `json:"scene_commits"`
		SceneWorkNS  uint64         `json:"scene_work_ns"`
		Presented    uint64         `json:"presented"`
		Discarded    uint64         `json:"discarded"`
		ZeroCopy     uint64         `json:"zero_copy_presents"`
		Runtime      map[string]any `json:"runtime"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if got.Snapshots != 1 || got.SnapshotNS < 1e6 || got.OutputFrames != 2 || got.SceneCommits != 1 || len(got.Runtime) == 0 {
		t.Fatalf("metrics: %+v", got)
	}
	if got.SceneWorkNS != 2e6 || got.Presented != 1 || got.Discarded != 1 || got.ZeroCopy != 1 {
		t.Fatalf("presentation counters: %+v", got)
	}
	response, err = client.Get("http://localhost/debug/pprof/goroutine?debug=1")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(body) == 0 {
		t.Fatalf("pprof response: %v %s", err, body)
	}
	if _, err := Start(path); err == nil {
		t.Fatal("accepted duplicate diagnostics server")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("counters still enabled after close")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket survived close: %v", err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(path); err == nil {
		t.Fatal("accepted publicly accessible socket directory")
	}
}
