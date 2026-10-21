package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherObservesAtomicConfigReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.kdl")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewWatcher([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	tmp := filepath.Join(dir, "config.tmp")
	if err := os.WriteFile(tmp, []byte("input {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	waitForConfigChange(t, watcher)
}

func TestWatcherObservesCreationOfMissingOptionalInclude(t *testing.T) {
	path := filepath.Join(t.TempDir(), "optional.kdl")
	watcher, err := NewWatcher([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	if err := os.WriteFile(path, []byte("input {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForConfigChange(t, watcher)
}

func TestWatcherFindsParentWhenOptionalIncludeDirectoryIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "optional", "config.kdl")
	watcher, err := NewWatcher([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	waitForConfigChange(t, watcher)
	if err := watcher.Update([]string{path}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("input {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForConfigChange(t, watcher)
}

func TestWatcherRetainsSharedDirectoryWhenReplacingSourceSet(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	aliasDir := filepath.Join(root, "alias")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Fatal(err)
	}
	rootConfig := filepath.Join(realDir, "root.kdl")
	include := filepath.Join(aliasDir, "include.kdl")
	for _, path := range []string{rootConfig, include} {
		if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	watcher, err := NewWatcher([]string{rootConfig, include})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Update([]string{include}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(include, []byte("input {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForConfigChange(t, watcher)
}

func waitForConfigChange(t *testing.T, watcher *Watcher) {
	t.Helper()
	select {
	case <-watcher.Events():
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for config change")
	}
}
