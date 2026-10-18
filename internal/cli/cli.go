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
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/ipc"
	"github.com/Nomadcxx/sysc-compositor/internal/server"
)

// Env is what a command reads and writes, so tests can supply their own.
type Env struct {
	Getenv         func(string) string
	Stdout, Stderr io.Writer
}

// Main runs the command line and returns the exit status.
func Main(name string, args []string, env Env) int {
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
	configPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return usage(name, env, "-c needs a path")
			}
			i++
			configPath = args[i]
		default:
			return usage(name, env, "unknown argument "+args[i])
		}
	}
	return run(name, configPath, env)
}

func usage(name string, env Env, problem string) int {
	fmt.Fprintf(env.Stderr, "%s: %s\nusage: %s [-c CONFIG] | %s validate [-c CONFIG] | %s msg [--json] REQUEST\n", name, problem, name, name, name)
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
func run(name, configFlag string, env Env) int {
	// wlroots objects belong to the thread that created them.
	runtime.LockOSThread()
	path := config.Locate(configFlag, env.Getenv)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: config: %v; using the defaults\n", name, err)
		cfg = config.Default()
	}
	s, err := server.New(cfg, err != nil)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", name, err)
		return 1
	}
	defer s.Destroy()
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

// msg is `niri msg`, for the requests built so far.
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
		fmt.Fprintf(env.Stderr, "Error: error communicating with %s: %v\n", name, err)
		return 1
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
			fmt.Fprintf(env.Stdout, "Compositor version: %s\nCLI version:        %s\n", v.Version, server.Version())
		}
		return 0
	}
	if isStateQuery(req.Name) {
		value, err := responseValue(reply.Ok, req.Name)
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
	case "Outputs", "Workspaces", "Windows", "Layers", "FocusedOutput":
		return true
	default:
		return false
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

var errNotBuilt = errors.New("not built yet")

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
	case "focused-output":
		return ipc.Request{Name: "FocusedOutput"}, onlyArgs(args[1:])
	case "event-stream":
		return ipc.Request{Name: "EventStream"}, onlyArgs(args[1:])
	case "request-error":
		return ipc.Request{Name: "ReturnError"}, onlyArgs(args[1:])
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

// parseAction follows niri's clap rules: spawn and spawn-sh take their
// command after `--`; quit takes -s/--skip-confirmation.
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
	}
	return ipc.Action{}, fmt.Errorf("action %s: %w", args[0], errNotBuilt)
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
	return reply, err
}
