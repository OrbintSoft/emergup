// SPDX-License-Identifier: GPL-3.0-or-later

package ai

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// errNoAnswer is the error of a backend that gave no answer.
var errNoAnswer = errors.New("no answer")

// Command is the backend that runs a program as the invoking user: the
// request is its standard input, the answer its standard output.
type Command struct {
	// Runner runs the program; it never runs it as root.
	Runner runner.Runner
	// Args holds the program, then its first arguments.
	Args []string
}

// Name implements Backend: the name of the program.
func (c Command) Name() string {
	return path.Base(c.Args[0])
}

// Explain implements Backend: the program's output is shown and logged as
// it comes.
func (c Command) Explain(ctx context.Context, _ *Conversation, request Request) error {
	result, err := c.Runner.Run(ctx, runner.Command{
		Args: c.Args, Mode: runner.Stream, Root: false, Env: nil, Watch: nil, Into: nil,
		Stdin: strings.NewReader(request.Text()), Dir: "",
	})

	switch {
	case err != nil:
		return fmt.Errorf("%w from %s: %w", errNoAnswer, c.Name(), err)
	case result.ExitCode != 0:
		return fmt.Errorf("%w from %s: it exited with status %d", errNoAnswer, c.Name(), result.ExitCode)
	}

	return nil
}
