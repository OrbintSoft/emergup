// SPDX-License-Identifier: GPL-3.0-or-later

package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// exitedError is the error of a query that exited with a failure.
type exitedError struct {
	command string
	code    int
}

func (e exitedError) Error() string {
	return fmt.Sprintf("%s exited with status %d", e.command, e.code)
}

// Profile returns the profile of the system that every request has: the
// line of emerge --version, which names Portage, Python, the profile, the
// compiler, the C library, the kernel and the architecture.
func Profile(ctx context.Context, run runner.Runner) (Part, error) {
	text, err := query(ctx, run, "emerge", "--version")

	return Part{Title: "The system (emerge --version)", Text: text}, err
}

// Info returns the part the user can add with the whole configuration of
// Portage: emerge --info.
func Info(ctx context.Context, run runner.Runner) (Part, error) {
	text, err := query(ctx, run, "emerge", "--info")

	return Part{Title: "The configuration of Portage (emerge --info)", Text: text}, err
}

// Packages returns the part the user can add with the installed packages,
// one on each line, as installed lists them.
func Packages(installed []string) Part {
	return Part{Title: "The installed packages", Text: strings.Join(installed, "\n") + "\n"}
}

// query runs args, which changes nothing, unprivileged, and returns its
// output.
func query(ctx context.Context, run runner.Runner, args ...string) (string, error) {
	result, err := run.Run(ctx, runner.Command{
		Args: args, Mode: runner.Capture, Root: false, Env: nil, Watch: nil, Into: nil, Stdin: nil, Dir: "",
	})

	switch {
	case err != nil:
		return "", err //nolint:wrapcheck // The runner's error names the command.
	case result.ExitCode != 0:
		return "", exitedError{command: strings.Join(args, " "), code: result.ExitCode}
	}

	return string(result.Output), nil
}
