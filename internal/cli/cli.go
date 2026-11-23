// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli is the emergup command line: subcommand dispatch, argument
// checking and the usage text. It is a package rather than the main package so
// that all of it can be exercised from tests; cmd/emergup holds nothing but the
// process's os.Exit.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/version"
)

// Exit statuses.
const (
	exitOK      = 0
	exitProblem = 1
	exitUsage   = 2
)

// usage is the help text; %[1]s is the name the program was invoked as.
const usage = `usage: %[1]s <command>

Drive the routine Gentoo update workflow.

commands:
  config check   validate the configuration files and report each problem
  config show    print the configuration in effect and where each value comes from
  version        print the emergup version (also --version)
  help           show this help (also -h, --help)
`

// Env is what an invocation reads and writes besides its arguments, given
// from outside so that every path is testable without touching the real
// standard streams or files.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// Root is the system's root file system, from which the configuration is
	// read.
	Root fs.FS
}

// Main runs one invocation and returns the process exit status.
func Main(ctx context.Context, env Env, invokedAs string, args []string) int {
	stdout, stderr := env.Stdout, env.Stderr
	prog := filepath.Base(invokedAs)

	if len(args) == 0 {
		printUsage(stderr, prog)

		return exitUsage
	}

	command, rest := args[0], args[1:]
	switch command {
	case "version", "--version":
		if len(rest) > 0 {
			return unexpectedArgument(stderr, prog, command, rest[0])
		}

		_, _ = fmt.Fprintf(stdout, "emergup %s\n", version.String())

		return exitOK
	case "help", "-h", "--help":
		if len(rest) > 0 {
			return unexpectedArgument(stderr, prog, command, rest[0])
		}

		printUsage(stdout, prog)

		return exitOK
	case "config":
		return configCommand(ctx, env, prog, rest)
	default:
		return unknownCommand(stderr, prog, command)
	}
}

// configCommand runs `config check` or `config show`.
func configCommand(ctx context.Context, env Env, prog string, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(env.Stderr, "%s config: missing subcommand, expected check or show\n\n", prog)
		printUsage(env.Stderr, prog)

		return exitUsage
	}

	subcommand, rest := args[0], args[1:]
	if subcommand != "check" && subcommand != "show" {
		_, _ = fmt.Fprintf(env.Stderr, "%s config: unknown subcommand %q\n\n", prog, subcommand)
		printUsage(env.Stderr, prog)

		return exitUsage
	}

	if len(rest) > 0 {
		return unexpectedArgument(env.Stderr, prog, "config "+subcommand, rest[0])
	}

	cfg, origins, err := config.Inspect(ctx, env.Root)
	if err != nil {
		return reportProblems(env.Stderr, err)
	}

	if subcommand == "show" {
		err := config.WriteTOML(env.Stdout, cfg, origins)
		if err != nil {
			_, _ = fmt.Fprintf(env.Stderr, "%s config show: %v\n", prog, err)

			return exitProblem
		}

		return exitOK
	}

	if len(origins.Files) == 0 {
		_, _ = fmt.Fprintln(env.Stdout, "No configuration file: the generic defaults apply.")
	}

	for _, file := range origins.Files {
		_, _ = fmt.Fprintf(env.Stdout, "Read %s\n", file)
	}

	_, _ = fmt.Fprintln(env.Stdout, "The configuration is valid.")

	return exitOK
}

// reportProblems writes each problem that err joins on its own line, then how
// many there are.
func reportProblems(stderr io.Writer, err error) int {
	problems := []error{err}

	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		problems = joined.Unwrap()
	}

	for _, problem := range problems {
		_, _ = fmt.Fprintln(stderr, problem)
	}

	noun := "problems"
	if len(problems) == 1 {
		noun = "problem"
	}

	_, _ = fmt.Fprintf(stderr, "%d %s in the configuration.\n", len(problems), noun)

	return exitProblem
}

func unknownCommand(stderr io.Writer, prog, command string) int {
	kind := "command"
	if strings.HasPrefix(command, "-") {
		kind = "option"
	}

	_, _ = fmt.Fprintf(stderr, "%s: unknown %s %q\n\n", prog, kind, command)
	printUsage(stderr, prog)

	return exitUsage
}

func unexpectedArgument(stderr io.Writer, prog, command, arg string) int {
	_, _ = fmt.Fprintf(stderr, "%s %s: unexpected argument %q\n\n", prog, strings.TrimLeft(command, "-"), arg)
	printUsage(stderr, prog)

	return exitUsage
}

func printUsage(w io.Writer, prog string) {
	_, _ = fmt.Fprintf(w, usage, prog)
}
