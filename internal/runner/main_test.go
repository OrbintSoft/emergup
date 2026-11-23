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
//	echo-stdin   copies standard input to standard output
//	wait-term    writes "ready", waits for SIGTERM, writes "terminated", exits 3
//	ignore-term  ignores SIGTERM, writes "ready" and waits for a minute
//	exit:N       exits with status N
func child(ops []string) int {
	for _, operation := range ops {
		name, arg, _ := strings.Cut(operation, ":")

		switch name {
		case "out":
			_, _ = fmt.Fprintln(os.Stdout, arg)
		case "err":
			_, _ = fmt.Fprintln(os.Stderr, arg)
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
