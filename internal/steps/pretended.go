// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// pretended is the work of a step that shows what emerge would do with
// --pretend, asks, then does it: the update and the depclean. emergup asks,
// never emerge's --ask.
type pretended struct {
	// command returns the step's command line, with --pretend or not.
	command func(env pipeline.Env, pretend bool) []string
	// nothing matches the output of a pretend with nothing to do, which
	// skips the step saying nothingDone.
	nothing     *regexp.Regexp
	nothingDone string
	// question is asked with goOn as the usual action and skip, which skips
	// the step saying skipped.
	question string
	goOn     ui.Action
	skip     ui.Action
	skipped  string
}

// run runs the pretend, unprivileged since it changes nothing, its output
// shown as it comes and read; asks; then runs the command, elevated.
func (p pretended) run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	pretend := p.command(env, true)

	result, err := env.Runner.Run(ctx, runner.Command{Args: pretend, Mode: runner.StreamCapture, Root: false, Env: nil})
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if result.ExitCode != 0 {
		return pipeline.Fail(fmt.Sprintf("%s --pretend exited with status %d", pretend[0], result.ExitCode))
	}

	if p.nothing.MatchString(runner.StripEscapes(string(result.Output))) {
		return pipeline.Skip(p.nothingDone)
	}

	if outcome, goOn := confirm(ctx, env, p.question, p.goOn, p.skip, p.skipped); !goOn {
		return outcome
	}

	command := p.command(env, false)

	return runElevated(ctx, env, command, command[0])
}

// lines says what run would run, without running the pretend, which takes
// a while.
func (p pretended) lines(env pipeline.Env) []string {
	return []string{
		strings.Join(p.command(env, true), " "),
		"then, once confirmed: " + strings.Join(p.command(env, false), " "),
	}
}
