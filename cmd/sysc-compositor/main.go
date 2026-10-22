// Command sysc-compositor is a Wayland compositor for the sysc desktop. With
// no subcommand it runs, nested inside an existing session; `msg` talks to a
// running one over the niri-ipc socket, and `validate` checks a config.
package main

import (
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/cli"
)

func main() {
	os.Exit(cli.Main("sysc-compositor", os.Args[1:], cli.Env{Getenv: os.Getenv, Stdout: os.Stdout, Stderr: os.Stderr}))
}
