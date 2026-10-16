// Command niri is sysc-compositor under niri's name (design §6), so scripts
// that run `niri` or `niri msg` keep working.
package main

import (
	"os"

	"github.com/Nomadcxx/sysc-compositor/internal/cli"
)

func main() {
	os.Exit(cli.Main("niri", os.Args[1:], cli.Env{Getenv: os.Getenv, Stdout: os.Stdout, Stderr: os.Stderr}))
}
