// Command niri is a compatibility alias of sysc-compositor (design §6), so
// the greeter config and scripts that run `niri`, `niri msg` or
// `niri validate` keep working. New docs and scripts use sysc-compositor.
package main

import (
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/cli"
)

func main() {
	os.Exit(cli.Main("niri", os.Args[1:], cli.Env{Getenv: os.Getenv, Stdout: os.Stdout, Stderr: os.Stderr}))
}
