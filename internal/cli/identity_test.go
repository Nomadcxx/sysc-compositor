package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserVisibleTextNeverSaysNiri(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	if err := os.WriteFile(path, []byte("input {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--bad-flag"}, {"msg", "version"}, {"validate", "-c", path}} {
		var out, errout bytes.Buffer
		Main("sysc-compositor", args, Env{Getenv: func(string) string { return "" }, Stdout: &out, Stderr: &errout})
		assertOwnIdentity(t, strings.ReplaceAll(out.String()+errout.String(), path, "config.kdl"))
	}
	if err := filepath.WalkDir("../../share", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil {
			assertOwnIdentity(t, string(b))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func assertOwnIdentity(t *testing.T, s string) {
	t.Helper()
	s = strings.ReplaceAll(s, "NIRI_SOCKET", "")
	s = strings.ReplaceAll(s, "niri-ipc", "")
	if strings.Contains(strings.ToLower(s), "niri") {
		t.Errorf("foreign identity: %s", s)
	}
}

func TestValidateReportsNiriCommandBinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.kdl")
	if err := os.WriteFile(path, []byte(`binds { Mod+A { spawn "niri" "msg" "outputs"; }; }`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := Main("sysc-compositor", []string{"validate", "-c", path}, Env{Getenv: func(string) string { return "" }, Stdout: &out, Stderr: &errout}); code != 0 {
		t.Fatalf("validate=%d: %s", code, errout.String())
	}
	if !strings.Contains(errout.String(), "use `sysc-compositor msg`: Mod+A") {
		t.Fatalf("no migration warning: %s", errout.String())
	}
}
func TestValidateDoesNotPrepareUserConfig(t *testing.T) {
	home := t.TempDir()
	var out, errout bytes.Buffer
	env := Env{Getenv: func(key string) string {
		if key == "HOME" {
			return home
		}
		return ""
	}, Stdout: &out, Stderr: &errout}
	Main("sysc-compositor", []string{"validate"}, env)
	if _, err := os.Stat(filepath.Join(home, ".config", "sysc-compositor")); !os.IsNotExist(err) {
		t.Fatalf("validate created config directory: %v", err)
	}
}
