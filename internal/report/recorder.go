// SPDX-License-Identifier: GPL-3.0-or-later

package report

import (
	"context"
	"io"
	"sync"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// Ran is a command that ran: the command as a step gave it, how it ended,
// and its output.
type Ran struct {
	Command runner.Command
	Result  runner.Result
	// Err is why the command could not start or was stopped, nil otherwise.
	Err    error
	Output *Output
}

// Recorder is a runner.Runner that runs each command through Next,
// watching its output, and remembers the last one: when a step fails, that
// is the command that failed. A Terminal command gives it no output, as it
// gives none to emergup.
type Recorder struct {
	Next runner.Runner

	mu   sync.Mutex
	last Ran
	ran  bool
}

// Run implements runner.Runner.
func (r *Recorder) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	output := NewOutput()
	watched := cmd

	if cmd.Mode != runner.Terminal {
		watched.Watch = output
		if cmd.Watch != nil {
			watched.Watch = io.MultiWriter(cmd.Watch, output)
		}
	}

	result, err := r.Next.Run(ctx, watched)

	r.mu.Lock()
	r.last = Ran{Command: cmd, Result: result, Err: err, Output: output}
	r.ran = true
	r.mu.Unlock()

	return result, err //nolint:wrapcheck // The runner's own error, which the steps report as it is.
}

// Last returns the last command that ran, and whether one did.
func (r *Recorder) Last() (Ran, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.last, r.ran
}

// Forget forgets the last command, as when a step starts: what ran before
// it is not its own.
func (r *Recorder) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	var nothing Ran

	r.last, r.ran = nothing, false
}
