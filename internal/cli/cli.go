// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli is the emergup command line: subcommand dispatch, argument
// checking and the usage text. It is a package rather than the main package so
// that all of it can be exercised from tests; cmd/emergup holds nothing but the
// process's os.Exit.
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/emergup/internal/version"
)

// Exit statuses.
const (
	exitOK    = 0
	exitUsage = 2
)

// usage is the help text; %[1]s is the name the program was invoked as.
const usage = `usage: %[1]s <command>

Drive the routine Gentoo update workflow.

commands:
  version   print the emergup version (also --version)
  help      show this help (also -h, --help)
`

// Main runs one invocation and returns the process exit status. Output goes to
// the given writers, so every path is testable without touching real stdio.
func Main(_ context.Context, stdout, stderr io.Writer, invokedAs string, args []string) int {
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
	default:
		kind := "command"
		if strings.HasPrefix(command, "-") {
			kind = "option"
		}
		_, _ = fmt.Fprintf(stderr, "%s: unknown %s %q\n\n", prog, kind, command)
		printUsage(stderr, prog)
		return exitUsage
	}
}

func unexpectedArgument(stderr io.Writer, prog, command, arg string) int {
	_, _ = fmt.Fprintf(stderr, "%s %s: unexpected argument %q\n\n", prog, strings.TrimLeft(command, "-"), arg)
	printUsage(stderr, prog)
	return exitUsage
}

func printUsage(w io.Writer, prog string) {
	_, _ = fmt.Fprintf(w, usage, prog)
}
