// Package config reads niri's KDL config (KDL v1, as niri 26.04's knuffel
// parses it). The sections built so far are decoded; every other section
// niri knows is accepted and not applied yet; a node niri does not know is an
// error, as it is in niri.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sblinch/kdl-go"
	"github.com/sblinch/kdl-go/document"
)

// Config is the part of niri's config the compositor applies.
type Config struct {
	SpawnAtStartup   [][]string
	SpawnShAtStartup []string
	Warnings         []string
	UnappliedActions []string
	// Environment is applied to spawned commands; a nil value unsets.
	Environment map[string]*string
	Keyboard    Keyboard
}

// Keyboard is input.keyboard: the xkb names and the repeat clients use.
type Keyboard struct {
	Rules, Model, Layout, Variant, Options, File string
	RepeatDelay, RepeatRate                      int
}

// Default is what niri runs with when no config applies.
func Default() *Config {
	return &Config{Environment: map[string]*string{},
		Keyboard: Keyboard{RepeatDelay: 600, RepeatRate: 25}}
}

// topLevel is every top-level node niri 26.04 decodes (niri-config/src/lib.rs).
var topLevel = map[string]bool{
	"input": true, "output": true, "spawn-at-startup": true, "spawn-sh-at-startup": true,
	"layout": true, "prefer-no-csd": true, "cursor": true, "screenshot-path": true,
	"clipboard": true, "hotkey-overlay": true, "config-notification": true, "animations": true,
	"blur": true, "gestures": true, "overview": true, "environment": true,
	"xwayland-satellite": true, "window-rule": true, "layer-rule": true, "binds": true,
	"switch-events": true, "debug": true, "workspace": true, "recent-windows": true,
	"include": true,
}

// Parse decodes one config file's text. name is used in errors.
func Parse(name string, src []byte) (*Config, error) {
	doc, err := kdl.Parse(strings.NewReader(string(src)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	root, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve config path: %w", name, err)
	}
	stack := map[string]bool{filepath.Clean(root): true}
	nodes, err := expandIncludes(root, doc.Nodes, stack, 0)
	if err != nil {
		return nil, err
	}
	return decode(name, nodes)
}

func decode(name string, nodes []*document.Node) (*Config, error) {
	c := Default()
	for _, n := range nodes {
		node := nodeName(n)
		if !topLevel[node] {
			return nil, fmt.Errorf("%s: unexpected node `%s`", name, node)
		}
		var err error
		switch node {
		case "include":
			err = errors.New("include must be loaded from a config path")
		case "spawn-at-startup":
			var argv []string
			if argv, err = stringArgs(n, 1, -1); err == nil {
				c.SpawnAtStartup = append(c.SpawnAtStartup, argv)
			}
		case "spawn-sh-at-startup":
			var argv []string
			if argv, err = stringArgs(n, 1, 1); err == nil {
				c.SpawnShAtStartup = append(c.SpawnShAtStartup, argv[0])
			}
		case "environment":
			err = c.environment(n)
		case "input":
			err = c.input(n)
		case "binds":
			if !slices.Contains(c.Warnings, node) {
				c.Warnings = append(c.Warnings, node)
			}
			for _, binding := range n.Children {
				for _, action := range binding.Children {
					name := nodeName(action)
					if !slices.Contains(c.UnappliedActions, name) {
						c.UnappliedActions = append(c.UnappliedActions, name)
					}
				}
			}
		default:
			if !slices.Contains(c.Warnings, node) {
				c.Warnings = append(c.Warnings, node)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", name, node, err)
		}
	}
	return c, nil
}

const maxIncludeDepth = 255

func expandIncludes(name string, nodes []*document.Node, stack map[string]bool, depth int) ([]*document.Node, error) {
	var expanded []*document.Node
	for _, node := range nodes {
		if nodeName(node) != "include" {
			expanded = append(expanded, node)
			continue
		}
		paths, err := stringArgs(node, 1, 1)
		if err != nil {
			return nil, fmt.Errorf("%s: include: %w", name, err)
		}
		if len(node.Children) != 0 {
			return nil, fmt.Errorf("%s: include: unexpected child node", name)
		}
		optional := false
		for key, value := range node.Properties {
			if key != "optional" {
				return nil, fmt.Errorf("%s: include: unexpected property `%s`", name, key)
			}
			var ok bool
			optional, ok = value.Value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s: include: `optional` must be boolean", name)
			}
		}
		path, err := resolveIncludePath(filepath.Dir(name), paths[0])
		if err != nil {
			return nil, fmt.Errorf("%s: include %q: %w", name, paths[0], err)
		}
		if depth >= maxIncludeDepth {
			return nil, fmt.Errorf("%s: include %q: recursion limit (%d) reached", name, paths[0], maxIncludeDepth)
		}
		if stack[path] {
			return nil, fmt.Errorf("%s: include %q: recursive include", name, paths[0])
		}
		content, err := os.ReadFile(path)
		if err != nil {
			if optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("%s: include %q: read %s: %w", name, paths[0], path, err)
		}
		doc, err := kdl.Parse(strings.NewReader(string(content)))
		if err != nil {
			return nil, fmt.Errorf("%s: include %q: %w", name, paths[0], err)
		}
		stack[path] = true
		children, expandErr := expandIncludes(path, doc.Nodes, stack, depth+1)
		delete(stack, path)
		if expandErr != nil {
			return nil, expandErr
		}
		expanded = append(expanded, children...)
	}
	return expanded, nil
}

func resolveIncludePath(base, path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		rest := strings.TrimPrefix(path, "~")
		rest = strings.TrimPrefix(rest, string(filepath.Separator))
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (c *Config) environment(n *document.Node) error {
	for _, v := range n.Children {
		if len(v.Arguments) != 1 || len(v.Children) != 0 {
			return fmt.Errorf("`%s` takes one value or null", nodeName(v))
		}
		switch val := v.Arguments[0].Value.(type) {
		case nil:
			c.Environment[nodeName(v)] = nil
		case string:
			c.Environment[nodeName(v)] = &val
		default:
			return fmt.Errorf("`%s` must be a string or null", nodeName(v))
		}
	}
	return nil
}

// input decodes input.keyboard. Its other children (mouse, touchpad, …) are
// accepted and not applied yet.
func (c *Config) input(n *document.Node) error {
	for _, kb := range n.Children {
		if nodeName(kb) != "keyboard" {
			continue
		}
		for _, k := range kb.Children {
			var err error
			switch nodeName(k) {
			case "xkb":
				err = c.xkb(k)
			case "repeat-delay":
				c.Keyboard.RepeatDelay, err = intArg(k, 0, 1<<16-1)
			case "repeat-rate":
				c.Keyboard.RepeatRate, err = intArg(k, 0, 255)
			}
			if err != nil {
				return fmt.Errorf("keyboard: %w", err)
			}
		}
	}
	return nil
}

func (c *Config) xkb(n *document.Node) error {
	fields := map[string]*string{
		"rules": &c.Keyboard.Rules, "model": &c.Keyboard.Model, "layout": &c.Keyboard.Layout,
		"variant": &c.Keyboard.Variant, "options": &c.Keyboard.Options, "file": &c.Keyboard.File,
	}
	for _, f := range n.Children {
		dst, ok := fields[nodeName(f)]
		if !ok {
			return fmt.Errorf("xkb: unexpected node `%s`", nodeName(f))
		}
		v, err := stringArgs(f, 1, 1)
		if err != nil {
			return fmt.Errorf("xkb %s: %w", nodeName(f), err)
		}
		*dst = v[0]
	}
	return nil
}

func nodeName(n *document.Node) string { return n.Name.ValueString() }

// stringArgs reads a node's string arguments, between min and max of them
// (max < 0 is unbounded).
func stringArgs(n *document.Node, min, max int) ([]string, error) {
	if len(n.Arguments) < min || (max >= 0 && len(n.Arguments) > max) {
		return nil, fmt.Errorf("wrong number of arguments (%d)", len(n.Arguments))
	}
	out := make([]string, 0, len(n.Arguments))
	for _, a := range n.Arguments {
		s, ok := a.Value.(string)
		if !ok {
			return nil, fmt.Errorf("argument %v is not a string", a.Value)
		}
		out = append(out, s)
	}
	return out, nil
}

func intArg(n *document.Node, min, max int64) (int, error) {
	if len(n.Arguments) != 1 {
		return 0, fmt.Errorf("`%s` takes one number", nodeName(n))
	}
	v, ok := n.Arguments[0].Value.(int64)
	if !ok || v < min || v > max {
		return 0, fmt.Errorf("`%s` must be a whole number from %d to %d", nodeName(n), min, max)
	}
	return int(v), nil
}

// Locate is the config path niri would read: the -c flag, then $NIRI_CONFIG
// (empty means unset), then the user's and the system config. It returns ""
// when no file exists and none was named.
func Locate(flag string, getenv func(string) string) string {
	if flag != "" {
		return flag
	}
	if p := getenv("NIRI_CONFIG"); p != "" {
		return p
	}
	var candidates []string
	if x := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		candidates = append(candidates, filepath.Join(x, "niri", "config.kdl"))
	} else if h := getenv("HOME"); h != "" {
		candidates = append(candidates, filepath.Join(h, ".config", "niri", "config.kdl"))
	}
	candidates = append(candidates, "/etc/niri/config.kdl")
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Load reads and parses path; "" is the default config.
func Load(path string) (*Config, error) {
	if path == "" {
		return Default(), nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, src)
}
