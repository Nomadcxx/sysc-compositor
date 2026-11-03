package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func firstRunEnv(t *testing.T) (func(string) string, string, string) {
	t.Helper()
	home := t.TempDir()
	env := map[string]string{"HOME": home}
	getenv := func(key string) string { return env[key] }
	return getenv, filepath.Join(home, ".config", "niri"), filepath.Join(home, ".config", "sysc-compositor")
}
func writeFirstRun(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func readFirstRun(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var importDate = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func TestLocateUsesSyscCompositorPaths(t *testing.T) {
	for _, tc := range []struct {
		name, flag string
		env        map[string]string
		want       Path
		user       string
	}{
		{"flag", "x.kdl", nil, Path{Named: "x.kdl"}, ""},
		{"environment", "", map[string]string{"SYSC_COMPOSITOR_CONFIG": "/a.kdl"}, Path{Named: "/a.kdl"}, ""},
		{"ignore Niri", "", map[string]string{"NIRI_CONFIG": "/n.kdl", "HOME": "/h"}, Path{Search: []string{"/h/.config/sysc-compositor/config.kdl", "/etc/sysc-compositor/config.kdl"}}, "/h/.config/sysc-compositor/config.kdl"},
		{"xdg", "", map[string]string{"XDG_CONFIG_HOME": "/x"}, Path{Search: []string{"/x/sysc-compositor/config.kdl", "/etc/sysc-compositor/config.kdl"}}, "/x/sysc-compositor/config.kdl"},
		{"relative xdg", "", map[string]string{"XDG_CONFIG_HOME": "rel", "HOME": "/h"}, Path{Search: []string{"/h/.config/sysc-compositor/config.kdl", "/etc/sysc-compositor/config.kdl"}}, "/h/.config/sysc-compositor/config.kdl"},
		{"no home", "", nil, Path{Search: []string{"/etc/sysc-compositor/config.kdl"}}, ""},
		{"relative xdg no home", "", map[string]string{"XDG_CONFIG_HOME": "rel"}, Path{Search: []string{"/etc/sysc-compositor/config.kdl"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			if got := Locate(tc.flag, getenv); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Locate = %+v want %+v", got, tc.want)
			}
			if got := UserPath(getenv); got != tc.user {
				t.Fatalf("UserPath = %q", got)
			}
		})
	}
	if got := NiriUserPath(func(k string) string {
		if k == "HOME" {
			return "/h"
		}
		return ""
	}); got != "/h/.config/niri/config.kdl" {
		t.Fatal(got)
	}
}
func TestFirstRunImportsNiriConfigAndIncludes(t *testing.T) {
	getenv, source, dest := firstRunEnv(t)
	files := map[string]string{"config.kdl": "// original\ninclude \"sysc-shell.kdl\"\ninclude \"sub/a.kdl\"\n", "sysc-shell.kdl": "layout { gaps 9; }\n", "sub/a.kdl": "include \"b.kdl\"\n", "sub/b.kdl": "input {}\n"}
	for name, text := range files {
		writeFirstRun(t, filepath.Join(source, name), []byte(text))
	}
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != filepath.Join(source, "config.kdl") || result.Wrote != filepath.Join(dest, "config.kdl") {
		t.Fatalf("%+v", result)
	}
	for name, text := range files {
		if got := string(readFirstRun(t, filepath.Join(source, name))); got != text {
			t.Fatalf("source %s changed", name)
		}
		want := text
		if name == "config.kdl" {
			want = "// Imported from " + filepath.Join(source, name) + " by sysc-compositor on 2026-11-02.\n" + text
		}
		if got := string(readFirstRun(t, filepath.Join(dest, name))); got != want {
			t.Fatalf("%s = %q want %q", name, got, want)
		}
	}
}
func TestFirstRunImportsOnlyOnce(t *testing.T) {
	getenv, source, dest := firstRunEnv(t)
	writeFirstRun(t, filepath.Join(source, "config.kdl"), []byte("input {}"))
	if _, err := PrepareUserConfig(Locate("", getenv), getenv, importDate); err != nil {
		t.Fatal(err)
	}
	before := readFirstRun(t, filepath.Join(dest, "config.kdl"))
	if err := os.Remove(filepath.Join(source, "config.kdl")); err != nil {
		t.Fatal(err)
	}
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil || !reflect.DeepEqual(result, FirstRun{}) {
		t.Fatalf("%+v %v", result, err)
	}
	if string(readFirstRun(t, filepath.Join(dest, "config.kdl"))) != string(before) {
		t.Fatal("second import changed config")
	}
}
func TestFirstRunKeepsExistingIncludeTargets(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "regular", true: "symlink"}[symlink], func(t *testing.T) {
			getenv, source, dest := firstRunEnv(t)
			writeFirstRun(t, filepath.Join(source, "config.kdl"), []byte("include \"sysc-shell.kdl\""))
			writeFirstRun(t, filepath.Join(source, "sysc-shell.kdl"), []byte("// source"))
			target := filepath.Join(dest, "sysc-shell.kdl")
			if symlink {
				outside := filepath.Join(t.TempDir(), "existing.kdl")
				writeFirstRun(t, outside, []byte("// shell"))
				if err := os.MkdirAll(dest, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFirstRun(t, target, []byte("// shell"))
			}
			result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
			if err != nil {
				t.Fatal(err)
			}
			if string(readFirstRun(t, target)) != "// shell" || !slices.Contains(result.Kept, target) {
				t.Fatalf("%+v", result)
			}
		})
	}
}
func TestFirstRunCopiesUnparseableNiriConfig(t *testing.T) {
	getenv, source, dest := firstRunEnv(t)
	writeFirstRun(t, filepath.Join(source, "config.kdl"), []byte("bogus-section {}\ninclude \"broken.kdl\"\n"))
	writeFirstRun(t, filepath.Join(source, "broken.kdl"), []byte("{{ broken"))
	if _, err := PrepareUserConfig(Locate("", getenv), getenv, importDate); err != nil {
		t.Fatal(err)
	}
	if string(readFirstRun(t, filepath.Join(dest, "broken.kdl"))) != "{{ broken" {
		t.Fatal("malformed include not copied")
	}
}
func TestFirstRunReportsOutsideAndMissingIncludes(t *testing.T) {
	getenv, source, _ := firstRunEnv(t)
	writeFirstRun(t, filepath.Join(source, "config.kdl"), []byte("include \"/etc/x.kdl\"\ninclude \"/etc/x.kdl\"\ninclude optional=true \"gone.kdl\"\n"))
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Outside, []string{"/etc/x.kdl"}) || !reflect.DeepEqual(result.Missing, []string{filepath.Join(source, "gone.kdl")}) {
		t.Fatalf("%+v", result)
	}
}
func TestFirstRunWritesDefaultWithoutNiriConfig(t *testing.T) {
	getenv, _, dest := firstRunEnv(t)
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil {
		t.Fatal(err)
	}
	if result.Wrote != filepath.Join(dest, "config.kdl") || result.Imported != "" || string(readFirstRun(t, result.Wrote)) != string(DefaultConfigKDL) {
		t.Fatalf("%+v", result)
	}
}
func TestFirstRunSkipsNamedConfigs(t *testing.T) {
	for _, named := range []bool{false, true} {
		getenv, _, dest := firstRunEnv(t)
		p := Path{Named: "/etc/greetd/g.kdl"}
		if !named {
			base := getenv
			getenv = func(k string) string {
				if k == "SYSC_COMPOSITOR_CONFIG" {
					return "/a.kdl"
				}
				return base(k)
			}
			p = Locate("", getenv)
		}
		result, err := PrepareUserConfig(p, getenv, importDate)
		if err != nil || !reflect.DeepEqual(result, FirstRun{}) {
			t.Fatalf("%+v %v", result, err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("created user config: %v", err)
		}
	}
	result, err := PrepareUserConfig(Path{}, func(string) string { return "" }, importDate)
	if err != nil || !reflect.DeepEqual(result, FirstRun{}) {
		t.Fatalf("no home %+v %v", result, err)
	}
}
func TestFirstRunKeepsExistingDanglingConfigSymlink(t *testing.T) {
	getenv, _, dest := firstRunEnv(t)
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dest, "config.kdl")
	if err := os.Symlink("missing.kdl", target); err != nil {
		t.Fatal(err)
	}
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil || !reflect.DeepEqual(result, FirstRun{}) {
		t.Fatalf("%+v %v", result, err)
	}
	if got, err := os.Readlink(target); err != nil || got != "missing.kdl" {
		t.Fatalf("%q %v", got, err)
	}
}
func TestFirstRunNeverWritesThroughEscapingIncludes(t *testing.T) {
	for _, name := range []string{"../outside.kdl", "sub/a.kdl"} {
		t.Run(name, func(t *testing.T) {
			getenv, source, dest := firstRunEnv(t)
			writeFirstRun(t, filepath.Join(source, "config.kdl"), []byte("include \""+name+"\"\n"))
			writeFirstRun(t, filepath.Join(source, name), []byte("// import"))
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.MkdirAll(outside, 0755); err != nil {
				t.Fatal(err)
			}
			if name == "sub/a.kdl" {
				if err := os.MkdirAll(dest, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dest, "sub")); err != nil {
					t.Fatal(err)
				}
			}
			_, _ = PrepareUserConfig(Locate("", getenv), getenv, importDate)
			if name == "sub/a.kdl" {
				if _, err := os.Stat(filepath.Join(outside, "a.kdl")); !os.IsNotExist(err) {
					t.Fatalf("wrote through symlink: %v", err)
				}
			} else if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "outside.kdl")); err == nil { // Source itself is the existing sibling; it must stay byte-identical.
				if string(readFirstRun(t, filepath.Join(filepath.Dir(dest), "outside.kdl"))) != "// import" {
					t.Fatal("outside source changed")
				}
			}
		})
	}
}
func TestShippedDefaultConfigIsValid(t *testing.T) {
	cfg, err := Parse("default-config.kdl", DefaultConfigKDL)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings: %v", cfg.Warnings)
	}
	// Every bind in the shipped default has to be one the compositor applies:
	// a bind whose action is not applied does nothing at all, and the user
	// never learns why.
	if len(cfg.UnappliedActions) != 0 {
		t.Errorf("the shipped default binds actions the compositor does not apply: %v", cfg.UnappliedActions)
	}
	text := strings.ToLower(string(DefaultConfigKDL))
	if strings.Contains(text, "niri") || strings.Contains(text, "spawn-at-startup") || strings.Contains(text, "spawn-sh-at-startup") {
		t.Fatal("default contains old identity or autostart")
	}
}
func TestBindsRunningNiriAreReported(t *testing.T) {
	cfg, err := Parse("binds.kdl", []byte(`binds { Mod+A { spawn "niri" "msg" "action" "quit"; }; Mod+B { spawn-sh "niri msg outputs"; }; Mod+C { spawn "foot"; }; Mod+D { spawn "/usr/bin/niri" "msg" "version"; }; }; spawn-at-startup "niri" "msg" "outputs"; spawn-sh-at-startup "niri msg version"`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Mod+A", "Mod+B", "Mod+D", "spawn-at-startup", "spawn-sh-at-startup"}
	if !reflect.DeepEqual(cfg.NiriCommandBinds, want) {
		t.Fatalf("got %v want %v", cfg.NiriCommandBinds, want)
	}
}

func TestFirstRunPublishesWithoutReplacingExistingFiles(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, symlink := range []bool{false, true} {
		name := "existing.kdl"
		if symlink {
			name = "link.kdl"
			if err := os.Symlink("existing.kdl", filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		} else {
			writeFirstRun(t, filepath.Join(dir, name), []byte("// keep"))
		}
		if err := publishImport(root, name, []byte("// replace")); !os.IsExist(err) {
			t.Fatalf("publish = %v, want existing-path error", err)
		}
		if string(readFirstRun(t, filepath.Join(dir, name))) != "// keep" {
			t.Fatal("overwrote existing destination")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("temporary files left: %v", entries)
	}
}
func TestBindsRunningNiriWarningTracksOverrides(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config.kdl")
	writeFirstRun(t, source, []byte("binds { Mod+A { spawn \"niri\"; }; }\ninclude \"override.kdl\"\n"))
	writeFirstRun(t, filepath.Join(dir, "override.kdl"), []byte("binds { Mod+A { spawn \"foot\"; }; }"))
	cfg, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.NiriCommandBinds) != 0 {
		t.Fatalf("warned about overridden bind: %v", cfg.NiriCommandBinds)
	}
}
func TestFirstRunCopiesRootWithSyntaxError(t *testing.T) {
	getenv, source, dest := firstRunEnv(t)
	original := []byte("input { broken {{")
	writeFirstRun(t, filepath.Join(source, "config.kdl"), original)
	result, err := PrepareUserConfig(Locate("", getenv), getenv, importDate)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported == "" || !strings.HasSuffix(string(readFirstRun(t, filepath.Join(dest, "config.kdl"))), string(original)) {
		t.Fatal("failed to copy malformed root")
	}
}
