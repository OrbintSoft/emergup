// SPDX-License-Identifier: GPL-3.0-or-later

// Command emergup drives the routine Gentoo update workflow: sync, news, the
// @world update, rebuilds, config-file updates and depclean.
package main

import (
	"context"
	"os"

	"github.com/OrbintSoft/emergup/internal/cli"
)

// main is the process entry point and the program's only root context; all the
// behaviour lives in cli.Main, which is testable, while a test cannot observe
// os.Exit.
func main() {
	env := cli.Env{Stdout: os.Stdout, Stderr: os.Stderr, Root: os.DirFS("/")}
	os.Exit(cli.Main(context.Background(), env, os.Args[0], os.Args[1:]))
}
