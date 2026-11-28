// SPDX-License-Identifier: GPL-3.0-or-later

// Command emergup drives the routine Gentoo update workflow: sync, news, the
// @world update, rebuilds, config-file updates and depclean.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OrbintSoft/emergup/internal/cli"
	"github.com/OrbintSoft/emergup/internal/elevation"
	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/rundir"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// main is the process entry point and the program's only root context; all the
// behaviour lives in cli.Main, which is testable, while a test cannot observe
// os.Exit. main gives it the real system: the standard streams, the root file
// system, the commands and the clock.
//
// SIGINT and SIGTERM end the root context instead of the process, so that a
// running command can be stopped cleanly; the terminal also gives a Ctrl-C to
// the command itself.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	store := rundir.Store{Getenv: os.Getenv}
	env := cli.Env{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Root:     os.DirFS("/"),
		Keyboard: ui.NewKeyboard(os.Stdin),
		NewRunner: func(log io.Writer) runner.Runner {
			return &runner.Exec{
				Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
				Log: log, GracePeriod: 0, Now: nil,
			}
		},
		Runs:   store,
		System: elevation.OS{},
		Lock:   store,
		Color:  ui.ColorEnabled(ui.IsTerminal(os.Stdout), os.Getenv),
		Now:    time.Now,
		OpenReport: func(path string) (cli.ReportDir, error) {
			return report.OpenDir(path)
		},
	}
	code := cli.Main(ctx, env, os.Args[0], os.Args[1:])

	stop()
	os.Exit(code)
}
