// SPDX-License-Identifier: GPL-3.0-or-later

// Package runnertest provides a fake runner.Runner for the tests of the code
// that runs commands.
package runnertest

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// Answer is what Fake returns for one command.
type Answer struct {
	Result runner.Result
	Err    error
}

// Fake is a runner.Runner that runs nothing: it records every command and
// answers each with the next scripted Answer, or with a successful Result
// once they are used up. A command run after its context ended is recorded
// and answered with the context's error. Its zero value is ready to use.
type Fake struct {
	mu       sync.Mutex
	answers  []Answer
	commands []runner.Command
}

// Script adds answers to give, in order, to the next commands.
func (f *Fake) Script(answers ...Answer) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.answers = append(f.answers, answers...)
}

// Commands returns the commands run so far, in order.
func (f *Fake) Commands() []runner.Command {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.commands)
}

// Run implements runner.Runner.
func (f *Fake) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.commands = append(f.commands, cmd)

	success := runner.Result{ExitCode: 0, Output: nil, Duration: 0}

	if err := ctx.Err(); err != nil {
		return success, fmt.Errorf("fake run: %w", err)
	}

	if len(f.answers) == 0 {
		return success, nil
	}

	answer := f.answers[0]
	f.answers = f.answers[1:]

	return answer.Result, answer.Err
}
