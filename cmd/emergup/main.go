// SPDX-License-Identifier: GPL-3.0-or-later

// Command emergup drives the routine Gentoo update workflow: sync, news, the
// @world update, rebuilds, config-file updates and depclean.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/OrbintSoft/emergup/internal/cli"
)

// main is the process entry point and the program's only root context; all the
// behaviour lives in cli.Main, which is testable, while a test cannot observe
// os.Exit.
//
// SIGINT and SIGTERM end the root context instead of the process, so that a
// running command can be stopped cleanly; the terminal also gives a Ctrl-C to
// the command itself.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	env := cli.Env{Stdout: os.Stdout, Stderr: os.Stderr, Root: os.DirFS("/")}
	code := cli.Main(ctx, env, os.Args[0], os.Args[1:])

	stop()
	os.Exit(code)
}
