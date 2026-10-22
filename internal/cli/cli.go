// Package cli is the command line shared by sysc-compositor and its niri
// drop-in (design §6): no subcommand runs the compositor, `msg` is a client
// of $NIRI_SOCKET speaking niri-ipc 26.4.0.
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/server"
	"github.com/Nomadcxx/sysc-compositor/internal/session"
)

// Env is what a command reads and writes, so tests can supply their own.
type Env struct {
	Getenv         func(string) string
	Stdout, Stderr io.Writer
}

// Main runs the command line and returns the exit status.
func Main(name string, args []string, env Env) int {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if len(args) > 0 {
		switch args[0] {
		case "msg":
			return msg(name, args[1:], env)
		case "validate":
			return validateConfig(name, args[1:], env)
		case "-V", "--version":
			fmt.Fprintln(env.Stdout, server.Version())
			return 0
		}
	}
	options, err := parseRunOptions(args)
	if err != nil {
		return usage(name, env, err.Error())
	}
	return run(name, options.configPath, options.session, env)
}

type runOptions struct {
	configPath string
	session    bool
}

func parseRunOptions(args []string) (runOptions, error) {
	var options runOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			options.session = true
		case "-c", "--config":
			if i+1 >= len(args) {
				return runOptions{}, fmt.Errorf("%s needs a path", args[i])
			}
			i++
			options.configPath = args[i]
		default:
			return runOptions{}, fmt.Errorf("unknown argument %s", args[i])
		}
	}
	return options, nil
}

// usage leads with the sysc-compositor names; niri is a compatibility alias
// that takes the same arguments (design §6).
func usage(name string, env Env, problem string) int {
	fmt.Fprintf(env.Stderr, "%s: %s\nusage: sysc-compositor [--session] [-c CONFIG] | sysc-compositor validate [-c CONFIG] | sysc-compositor msg [--json] REQUEST\n", name, problem)
	if name != "sysc-compositor" {
		fmt.Fprintf(env.Stderr, "%s is a compatibility alias of sysc-compositor and takes the same arguments\n", name)
	}
	return 2
}

func validateConfig(name string, args []string, env Env) int {
	flag := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return usage(name, env, args[i]+" needs a path")
			}
			i++
			flag = args[i]
		default:
			return usage(name, env, "unknown validate argument "+args[i])
		}
	}
	path := config.Locate(flag, env.Getenv)
	if path == "" {
		fmt.Fprintf(env.Stdout, "%s: built-in configuration is valid\n", name)
		return 0
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: config: %v\n", name, err)
		return 1
	}
	for _, section := range cfg.Warnings {
		fmt.Fprintf(env.Stderr, "%s: warning: config section `%s` is accepted but not applied yet\n", name, section)
	}
	if len(cfg.UnappliedActions) > 0 {
		fmt.Fprintf(env.Stderr, "%s: warning: bind actions are accepted but not applied yet: %s\n",
			name, strings.Join(cfg.UnappliedActions, ", "))
	}
	fmt.Fprintf(env.Stdout, "%s: configuration is valid: %s\n", name, path)
	return 0
}

// run starts the compositor. A config that fails to load is reported and the
// defaults are used, as niri does.
func run(name, configFlag string, sessionMode bool, env Env) int {
	// wlroots objects belong to the thread that created them.
	runtime.LockOSThread()
	path := config.Locate(configFlag, env.Getenv)
	loaded, err := config.LoadWithSources(path)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: config: %v; using the defaults\n", name, err)
		loaded.Config = config.Default()
	}
	s, err := server.New(loaded, err != nil)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", name, err)
		return 1
	}
	defer s.Destroy()
	if sessionMode {
		lifecycle, err := session.Activate(session.Environment{
			WaylandDisplay: s.SocketName(), NiriSocket: s.NiriSocket(),
			SyscSocket: s.ControlPath(), Display: env.Getenv("DISPLAY"),
		})
		if err != nil {
			fmt.Fprintf(env.Stderr, "%s: session: %v\n", name, err)
			return 1
		}
		defer lifecycle.Close()
		s.StartSessionDBus()
	}
	// The smoke scripts read these lines to reach the nested session.
	fmt.Fprintln(env.Stderr, "WAYLAND_DISPLAY="+s.SocketName())
	if p := s.NiriSocket(); p != "" {
		fmt.Fprintln(env.Stderr, "NIRI_SOCKET="+p)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		s.Terminate()
	}()
	if err := s.Run(); err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

// msg is `sysc-compositor msg` (niri's `niri msg`), for the requests built so far.
func msg(name string, args []string, env Env) int {
	asJSON := false
	if len(args) > 0 && (args[0] == "-j" || args[0] == "--json") {
		asJSON, args = true, args[1:]
	}
	req, err := parseMsg(args)
	if err != nil {
		return usage(name, env, "msg: "+err.Error())
	}
	if req.Name == "EventStream" {
		return streamEvents(name, env.Getenv("NIRI_SOCKET"), asJSON, env)
	}
	reply, err := send(env.Getenv("NIRI_SOCKET"), req)
	if err != nil {
		if req.Name != "Version" && shouldCheckVersion(err) {
			warnCompositorVersion(name, env.Getenv("NIRI_SOCKET"), env.Stderr)
		}
		fmt.Fprintf(env.Stderr, "Error: error communicating with %s: %v\n", name, err)
		return 1
	}
	if req.Name != "Version" && reply.Ok == nil {
		warnCompositorVersion(name, env.Getenv("NIRI_SOCKET"), env.Stderr)
	}
	if req.Name == "ReturnError" {
		if reply.Ok != nil {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: expected an error, got %s\n", reply.Ok)
			return 1
		}
		fmt.Fprintf(env.Stderr, "Error: %s returned an error: %s\n", name, reply.Err)
		return 1
	}
	if reply.Ok == nil {
		fmt.Fprintf(env.Stderr, "Error: %s returned an error: %s\n", name, reply.Err)
		return 1
	}
	if req.Name == "Version" {
		var v struct{ Version string }
		if err := json.Unmarshal(reply.Ok, &v); err != nil {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: %s\n", reply.Ok)
			return 1
		}
		if asJSON {
			out, _ := json.Marshal(map[string]string{"compositor": v.Version, "cli": server.Version()})
			fmt.Fprintln(env.Stdout, string(out))
		} else {
			if v.Version != server.Version() {
				printVersionMismatch(name, v.Version, server.Version(), env.Stderr)
			}
			fmt.Fprintf(env.Stdout, "Compositor version: %s\nCLI version:        %s\n", v.Version, server.Version())
		}
		return 0
	}
	if req.Name == "Action" {
		if string(reply.Ok) != `"Handled"` {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: %s\n", reply.Ok)
			return 1
		}
		return 0
	}
	if req.Name == "Output" {
		value, err := responseValue(reply.Ok, "OutputConfigChanged")
		if err != nil {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: %v\n", err)
			return 1
		}
		var result string
		if err := json.Unmarshal(value, &result); err != nil || (result != "Applied" && result != "OutputWasMissing") {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: %s\n", value)
			return 1
		}
		if asJSON {
			fmt.Fprintln(env.Stdout, string(value))
			return 0
		}
		if result == "OutputWasMissing" {
			fmt.Fprintf(env.Stdout, "Output %q is not connected.\n", req.Output.Name)
			fmt.Fprintln(env.Stdout, "The change will apply when it is connected.")
		}
		return 0
	}
	if isStateQuery(req.Name) {
		value, err := responseValue(reply.Ok, responseName(req.Name))
		if err != nil {
			fmt.Fprintf(env.Stderr, "Error: unexpected response: %v\n", err)
			return 1
		}
		if asJSON {
			fmt.Fprintln(env.Stdout, string(value))
		} else {
			var indented bytes.Buffer
			if err := json.Indent(&indented, value, "", "  "); err != nil {
				fmt.Fprintf(env.Stderr, "Error: formatting response: %v\n", err)
				return 1
			}
			fmt.Fprintln(env.Stdout, indented.String())
		}
	}
	return 0
}

func isStateQuery(name string) bool {
	switch name {
	case "Outputs", "Workspaces", "Windows", "Layers", "KeyboardLayouts", "FocusedOutput",
		"FocusedWindow", "PickWindow", "PickColor", "OverviewState", "Casts":
		return true
	default:
		return false
	}
}

func responseName(request string) string {
	switch request {
	case "PickWindow":
		return "PickedWindow"
	case "PickColor":
		return "PickedColor"
	default:
		return request
	}
}

func responseValue(raw json.RawMessage, name string) (json.RawMessage, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	value, ok := response[name]
	if !ok || len(response) != 1 {
		return nil, fmt.Errorf("expected %s, got %s", name, raw)
	}
	return value, nil
}

func streamEvents(name, socket string, asJSON bool, env Env) int {
	if socket == "" {
		fmt.Fprintf(env.Stderr, "Error: error communicating with %s: NIRI_SOCKET is not set, are you running this within niri?\n", name)
		return 1
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		fmt.Fprintf(env.Stderr, "Error: error communicating with %s: %v\n", name, err)
		return 1
	}
	defer conn.Close()
	if _, err := conn.Write(append(ipc.EncodeRequest(ipc.Request{Name: "EventStream"}), '\n')); err != nil {
		fmt.Fprintf(env.Stderr, "Error: sending EventStream: %v\n", err)
		return 1
	}
	reader := bufio.NewReaderSize(conn, ipc.MaxLine)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		fmt.Fprintf(env.Stderr, "Error: reading EventStream reply: %v\n", err)
		return 1
	}
	var reply ipc.Reply
	if err := json.Unmarshal(line, &reply); err != nil {
		fmt.Fprintf(env.Stderr, "Error: parsing EventStream reply: %v\n", err)
		return 1
	}
	if reply.Ok == nil {
		fmt.Fprintf(env.Stderr, "Error: %s returned an error: %s\n", name, reply.Err)
		return 1
	}
	if string(reply.Ok) != `"Handled"` {
		fmt.Fprintf(env.Stderr, "Error: unexpected EventStream reply: %s\n", reply.Ok)
		return 1
	}
	if !asJSON {
		fmt.Fprintln(env.Stdout, "Started reading events.")
	}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Fprintf(env.Stderr, "Error: error reading event from %s: %v\n", name, err)
			return 1
		}
		if _, err := env.Stdout.Write(line); err != nil {
			fmt.Fprintf(env.Stderr, "Error: writing event: %v\n", err)
			return 1
		}
	}
}

var (
	errNotBuilt     = errors.New("not built yet")
	errInvalidReply = errors.New("invalid IPC reply")
)

func parseMsg(args []string) (ipc.Request, error) {
	if len(args) == 0 {
		return ipc.Request{}, errors.New("a request is required")
	}
	switch args[0] {
	case "version":
		return ipc.Request{Name: "Version"}, onlyArgs(args[1:])
	case "outputs":
		return ipc.Request{Name: "Outputs"}, onlyArgs(args[1:])
	case "workspaces":
		return ipc.Request{Name: "Workspaces"}, onlyArgs(args[1:])
	case "windows":
		return ipc.Request{Name: "Windows"}, onlyArgs(args[1:])
	case "layers":
		return ipc.Request{Name: "Layers"}, onlyArgs(args[1:])
	case "keyboard-layouts":
		return ipc.Request{Name: "KeyboardLayouts"}, onlyArgs(args[1:])
	case "focused-output":
		return ipc.Request{Name: "FocusedOutput"}, onlyArgs(args[1:])
	case "focused-window":
		return ipc.Request{Name: "FocusedWindow"}, onlyArgs(args[1:])
	case "pick-window":
		return ipc.Request{Name: "PickWindow"}, onlyArgs(args[1:])
	case "pick-color":
		return ipc.Request{Name: "PickColor"}, onlyArgs(args[1:])
	case "event-stream":
		return ipc.Request{Name: "EventStream"}, onlyArgs(args[1:])
	case "request-error":
		return ipc.Request{Name: "ReturnError"}, onlyArgs(args[1:])
	case "overview-state":
		return ipc.Request{Name: "OverviewState"}, onlyArgs(args[1:])
	case "casts":
		return ipc.Request{Name: "Casts"}, onlyArgs(args[1:])
	case "output":
		request, err := parseOutput(args[1:])
		return request, err
	case "action":
		a, err := parseAction(args[1:])
		return ipc.Request{Name: "Action", Action: &a}, err
	}
	return ipc.Request{}, fmt.Errorf("%s: %w", args[0], errNotBuilt)
}

func onlyArgs(rest []string) error {
	if len(rest) != 0 {
		return fmt.Errorf("unexpected argument %q", rest[0])
	}
	return nil
}

// parseAction follows niri's CLI shape for the actions built so far.
func parseAction(args []string) (ipc.Action, error) {
	if len(args) == 0 {
		return ipc.Action{}, errors.New("an action is required")
	}
	afterDash := func(rest []string) ([]string, error) {
		if len(rest) < 2 || rest[0] != "--" {
			return nil, errors.New("the command goes after `--`")
		}
		return rest[1:], nil
	}
	switch args[0] {
	case "quit":
		a := ipc.Action{Name: "Quit"}
		for _, f := range args[1:] {
			if f != "-s" && f != "--skip-confirmation" {
				return a, fmt.Errorf("quit: unexpected argument %q", f)
			}
			a.SkipConfirmation = true
		}
		return a, nil
	case "spawn":
		command, err := afterDash(args[1:])
		return ipc.Action{Name: "Spawn", Command: command}, err
	case "spawn-sh":
		command, err := afterDash(args[1:])
		if err == nil && len(command) != 1 {
			err = errors.New("spawn-sh takes one command string")
		}
		if err != nil {
			return ipc.Action{}, err
		}
		return ipc.Action{Name: "SpawnSh", ShellCommand: command[0]}, nil
	case "switch-layout":
		if len(args) == 2 {
			switch args[1] {
			case "next":
				return ipc.Action{Name: "SwitchLayout", Layout: "Next"}, nil
			case "prev":
				return ipc.Action{Name: "SwitchLayout", Layout: "Prev"}, nil
			default:
				index, err := parseUint8(args[1])
				if err == nil {
					return ipc.Action{Name: "SwitchLayout", Layout: "Index", LayoutIndex: &index}, nil
				}
			}
		}
		return ipc.Action{}, errors.New("switch-layout expects `next`, `prev`, or a layout index")
	case "load-config-file":
		var path *string
		if len(args) > 1 {
			if args[1] != "--path" || len(args) != 3 || args[2] == "" {
				return ipc.Action{}, errors.New("load-config-file accepts only --path PATH")
			}
			path = &args[2]
		}
		return ipc.Action{Name: "LoadConfigFile", ConfigPath: path}, nil
	case "focus-window":
		id, err := parseWindowID(args[1:], true)
		return ipc.Action{Name: "FocusWindow", WindowID: id}, err
	case "close-window":
		id, err := parseWindowID(args[1:], false)
		return ipc.Action{Name: "CloseWindow", WindowID: id}, err
	case "focus-workspace":
		if len(args) != 2 {
			return ipc.Action{}, errors.New("focus-workspace needs an index or name")
		}
		reference, err := parseWorkspaceReference(args[1])
		return ipc.Action{Name: "FocusWorkspace", Workspace: reference}, err
	}
	return ipc.Action{}, fmt.Errorf("action %s: %w", args[0], errNotBuilt)
}

func parseUint8(value string) (uint8, error) {
	index, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", value)
	}
	return uint8(index), nil
}

func parseOutput(args []string) (ipc.Request, error) {
	if len(args) < 2 || args[0] == "" {
		return ipc.Request{}, errors.New("output needs a name and action")
	}
	action, err := parseOutputAction(args[1], args[2:])
	if err != nil {
		return ipc.Request{}, err
	}
	return ipc.Request{Name: "Output", Output: &ipc.OutputRequest{Name: args[0], Action: action}}, nil
}

func parseOutputAction(name string, args []string) (json.RawMessage, error) {
	variant := map[string]string{
		"off": "Off", "on": "On", "mode": "Mode", "custom-mode": "CustomMode", "modeline": "Modeline",
		"scale": "Scale", "transform": "Transform", "position": "Position", "vrr": "Vrr",
	}[name]
	unit := func() (json.RawMessage, error) {
		if err := onlyArgs(args); err != nil {
			return nil, err
		}
		return json.Marshal(variant)
	}
	object := func(body any) (json.RawMessage, error) {
		return json.Marshal(map[string]any{variant: body})
	}
	switch name {
	case "off", "on":
		return unit()
	case "mode", "custom-mode":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s needs one mode", name)
		}
		mode, err := parseConfiguredMode(args[0])
		if err != nil {
			return nil, err
		}
		if name == "mode" {
			var toSet any = "Automatic"
			if mode != nil {
				toSet = map[string]any{"Specific": mode}
			}
			return object(map[string]any{"mode": toSet})
		}
		if mode == nil {
			return nil, errors.New("custom-mode does not accept auto")
		}
		return object(map[string]any{"mode": mode})
	case "modeline":
		if len(args) != 11 {
			return nil, errors.New("modeline needs CLOCK HDISPLAY HSYNC-START HSYNC-END HTOTAL VDISPLAY VSYNC-START VSYNC-END VTOTAL HSYNC-POLARITY VSYNC-POLARITY")
		}
		clock, err := parseFiniteFloat(args[0], "clock")
		if err != nil {
			return nil, err
		}
		fields := []string{"hdisplay", "hsync_start", "hsync_end", "htotal", "vdisplay", "vsync_start", "vsync_end", "vtotal"}
		values := make([]uint16, len(fields))
		for i, field := range fields {
			values[i], err = parseUint16(args[i+1], field)
			if err != nil {
				return nil, err
			}
		}
		hsync := map[string]string{"+hsync": "PHSync", "-hsync": "NHSync"}[args[9]]
		vsync := map[string]string{"+vsync": "PVSync", "-vsync": "NVSync"}[args[10]]
		if hsync == "" || vsync == "" {
			return nil, errors.New("modeline polarity must be +hsync, -hsync, +vsync, or -vsync")
		}
		return object(map[string]any{"clock": clock, "hdisplay": values[0], "hsync_start": values[1],
			"hsync_end": values[2], "htotal": values[3], "vdisplay": values[4], "vsync_start": values[5],
			"vsync_end": values[6], "vtotal": values[7], "hsync_polarity": hsync, "vsync_polarity": vsync})
	case "scale":
		if len(args) != 1 {
			return nil, errors.New("scale needs auto or a factor")
		}
		var scale any = "Automatic"
		if args[0] != "auto" {
			value, err := parseFiniteFloat(args[0], "scale")
			if err != nil {
				return nil, err
			}
			scale = map[string]float64{"Specific": value}
		}
		return object(map[string]any{"scale": scale})
	case "transform":
		if len(args) != 1 {
			return nil, errors.New("transform needs one transform")
		}
		transforms := map[string]string{
			"normal": "Normal", "90": "90", "180": "180", "270": "270", "flipped": "Flipped",
			"flipped-90": "Flipped90", "flipped-180": "Flipped180", "flipped-270": "Flipped270",
		}
		value, ok := transforms[args[0]]
		if !ok {
			return nil, fmt.Errorf("invalid transform %q", args[0])
		}
		return object(map[string]any{"transform": value})
	case "position":
		if len(args) == 1 && args[0] == "auto" {
			return object(map[string]any{"position": "Automatic"})
		}
		if len(args) != 3 || args[0] != "set" {
			return nil, errors.New("position expects `auto` or `set X Y`")
		}
		x, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid position X %q", args[1])
		}
		y, err := strconv.ParseInt(args[2], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid position Y %q", args[2])
		}
		position := map[string]any{"Specific": map[string]int32{"x": int32(x), "y": int32(y)}}
		return object(map[string]any{"position": position})
	case "vrr":
		if len(args) < 1 || len(args) > 2 {
			return nil, errors.New("vrr needs ON|OFF and optional --on-demand")
		}
		var enabled bool
		switch strings.ToLower(args[0]) {
		case "on", "true", "yes", "y", "t", "1":
			enabled = true
		case "off", "false", "no", "n", "f", "0":
		default:
			return nil, fmt.Errorf("invalid vrr value %q", args[0])
		}
		onDemand := false
		if len(args) == 2 {
			if args[1] != "--on-demand" {
				return nil, fmt.Errorf("vrr: unexpected argument %q", args[1])
			}
			onDemand = true
		}
		return object(map[string]any{"vrr": enabled, "on_demand": onDemand})
	default:
		return nil, fmt.Errorf("unknown output action %q", name)
	}
}

func parseConfiguredMode(value string) (any, error) {
	if value == "auto" {
		return nil, nil
	}
	parts := strings.SplitN(value, "@", 2)
	dimensions := strings.SplitN(parts[0], "x", 2)
	if len(dimensions) != 2 {
		return nil, fmt.Errorf("invalid mode %q: expected WIDTHxHEIGHT[@HZ]", value)
	}
	width, err := parseUint16(dimensions[0], "mode width")
	if err != nil {
		return nil, err
	}
	height, err := parseUint16(dimensions[1], "mode height")
	if err != nil {
		return nil, err
	}
	var refresh any
	if len(parts) == 2 {
		refresh, err = parseFiniteFloat(parts[1], "refresh rate")
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"width": width, "height": height, "refresh": refresh}, nil
}

func parseUint16(value, field string) (uint16, error) {
	n, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", field, value)
	}
	return uint16(n), nil
}

func parseFiniteFloat(value, field string) (float64, error) {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid %s %q", field, value)
	}
	return n, nil
}

func parseWindowID(args []string, required bool) (*uint64, error) {
	if len(args) == 0 && !required {
		return nil, nil
	}
	if len(args) != 2 || args[0] != "--id" {
		return nil, errors.New("expected --id <window-id>")
	}
	id, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid window ID %q", args[1])
	}
	return &id, nil
}

func parseWorkspaceReference(value string) (*ipc.WorkspaceReference, error) {
	if value == "" {
		return nil, errors.New("workspace reference cannot be empty")
	}
	if index, err := strconv.ParseInt(value, 10, 32); err == nil {
		if index < 0 || index > 255 {
			return nil, errors.New("workspace index must be between 0 and 255")
		}
		value := uint8(index)
		return &ipc.WorkspaceReference{Index: &value}, nil
	}
	return &ipc.WorkspaceReference{Name: &value}, nil
}

// send writes one request and reads its reply.
func send(socket string, r ipc.Request) (ipc.Reply, error) {
	if socket == "" {
		// niri-ipc's own message (socket.rs).
		return ipc.Reply{}, errors.New("NIRI_SOCKET is not set, are you running this within niri?")
	}
	c, err := net.Dial("unix", socket)
	if err != nil {
		return ipc.Reply{}, err
	}
	defer c.Close()
	if _, err := c.Write(append(ipc.EncodeRequest(r), '\n')); err != nil {
		return ipc.Reply{}, err
	}
	line, err := bufio.NewReaderSize(c, ipc.MaxLine).ReadBytes('\n')
	if err != nil {
		return ipc.Reply{}, err
	}
	var reply ipc.Reply
	err = json.Unmarshal(line, &reply)
	if err != nil {
		return ipc.Reply{}, fmt.Errorf("%w: %v", errInvalidReply, err)
	}
	return reply, nil
}

func shouldCheckVersion(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errInvalidReply)
}

func warnCompositorVersion(name, socket string, stderr io.Writer) {
	reply, err := send(socket, ipc.Request{Name: "Version"})
	if err == nil && reply.Ok != nil {
		value, valueErr := responseValue(reply.Ok, "Version")
		var compositor string
		if valueErr == nil {
			_ = json.Unmarshal(value, &compositor)
		}
		if compositor != "" {
			if compositor != server.Version() {
				printVersionMismatch(name, compositor, server.Version(), stderr)
			}
			return
		}
	}
	fmt.Fprintln(stderr, "Unable to get the running niri compositor version.")
	fmt.Fprintln(stderr, "Did you forget to restart niri after an update?")
	fmt.Fprintln(stderr)
}

func printVersionMismatch(name, compositor, cli string, stderr io.Writer) {
	fmt.Fprintf(stderr, "Running niri compositor has a different version from the %s CLI:\n", name)
	fmt.Fprintf(stderr, "Compositor version: %s\n", compositor)
	fmt.Fprintf(stderr, "CLI version:        %s\n", cli)
	fmt.Fprintln(stderr, "Did you forget to restart niri after an update?")
	fmt.Fprintln(stderr)
}
