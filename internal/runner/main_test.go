// SPDX-License-Identifier: GPL-3.0-or-later

package runner_test

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// childMarker, as the first argument of the test binary, makes it a child
// process for the tests instead of running them, so the tests start no
// program of the system.
const childMarker = "-emergup-test-child"

// The escape sequences the child writes.
const (
	greenEscape = "\x1b[32m"
	boldEscape  = "\x1b[1m"
	resetEscape = "\x1b[0m"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == childMarker {
		os.Exit(child(os.Args[2:]))
	}

	os.Exit(m.Run())
}

// child runs the operations ops in order and returns the exit status:
//
//	out:TEXT     writes TEXT and a newline to standard output
//	err:TEXT     writes TEXT and a newline to standard error
//	green:TEXT   writes TEXT in green and a newline to standard output
//	bold:TEXT    writes TEXT in bold and a newline to standard error
//	echo-stdin   copies standard input to standard output
//	wait-term    writes "ready", waits for SIGTERM, writes "terminated", exits 3
//	ignore-term  ignores SIGTERM, writes "ready" and waits for a minute
//	exit:N       exits with status N
func child(ops []string) int {
	writes := map[string]func(text string){
		"out":   func(text string) { _, _ = fmt.Fprintln(os.Stdout, text) },
		"err":   func(text string) { _, _ = fmt.Fprintln(os.Stderr, text) },
		"green": func(text string) { _, _ = fmt.Fprintln(os.Stdout, greenEscape+text+resetEscape) },
		"bold":  func(text string) { _, _ = fmt.Fprintln(os.Stderr, boldEscape+text+resetEscape) },
	}

	for _, operation := range ops {
		name, arg, _ := strings.Cut(operation, ":")

		if write, isWrite := writes[name]; isWrite {
			write(arg)

			continue
		}

		switch name {
		case "echo-stdin":
			_, _ = io.Copy(os.Stdout, os.Stdin)
		case "wait-term":
			terms := make(chan os.Signal, 1)
			signal.Notify(terms, syscall.SIGTERM)

			_, _ = fmt.Fprintln(os.Stdout, "ready")

			<-terms

			_, _ = fmt.Fprintln(os.Stdout, "terminated")

			return 3
		case "ignore-term":
			signal.Ignore(syscall.SIGTERM)

			_, _ = fmt.Fprintln(os.Stdout, "ready")

			time.Sleep(time.Minute)
		case "exit":
			code, err := strconv.Atoi(arg)
			if err != nil {
				return 125
			}

			return code
		default:
			_, _ = fmt.Fprintf(os.Stderr, "unknown operation %q\n", operation)

			return 125
		}
	}

	return 0
}
