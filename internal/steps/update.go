// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The reasons of an update that does not take place.
const (
	nothingToUpdate = "nothing to update"
	notUpdated      = "you chose not to update"
	abortedRun      = "you aborted the run"
)

// nothingToMerge matches the line with which emerge --pretend says that it
// would merge nothing.
var nothingToMerge = regexp.MustCompile(`(?m)^Total: 0 packages\b`)

// updateStep updates the target, @world by default: it shows what emerge
// would do with --pretend, asks, then does it.
type updateStep struct {
	config config.Update
}

// Name implements pipeline.Step.
func (updateStep) Name() string { return "update" }

// Title implements pipeline.Step.
func (u updateStep) Title() string { return "update " + u.config.Target }

// Enabled implements pipeline.Step.
func (u updateStep) Enabled() bool { return u.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the update asks once
// the pretend has shown what it would do.
func (updateStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step. It does not run the pretend of emerge,
// which takes a while: it says what the step would run.
func (u updateStep) Pretend(_ context.Context, env pipeline.Env) ([]string, error) {
	return []string{
		strings.Join(u.command(env, true), " "),
		"then, once confirmed: " + strings.Join(u.command(env, false), " "),
	}, nil
}

// Run implements pipeline.Step. The pretend changes nothing, so it runs
// unprivileged; its output is shown as it comes, and read to know whether
// there is anything to update.
func (u updateStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	pretend := u.command(env, true)

	result, err := env.Runner.Run(ctx, runner.Command{Args: pretend, Mode: runner.StreamCapture, Root: false})
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if result.ExitCode != 0 {
		return pipeline.Fail(fmt.Sprintf("%s --pretend exited with status %d", pretend[0], result.ExitCode))
	}

	if nothingToMerge.MatchString(runner.StripEscapes(string(result.Output))) {
		return pipeline.Skip(nothingToUpdate)
	}

	if outcome, goOn := u.ask(ctx, env); !goOn {
		return outcome
	}

	command := u.command(env, false)

	result, err = env.Runner.Run(ctx, runner.Command{Args: command, Mode: runner.Stream, Root: true})
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if result.ExitCode != 0 {
		return pipeline.Fail(fmt.Sprintf("%s exited with status %d", command[0], result.ExitCode))
	}

	return pipeline.Success()
}

// ask asks whether to update the packages the pretend showed, and reports
// whether to; when not, it returns the step's outcome.
func (u updateStep) ask(ctx context.Context, env pipeline.Env) (pipeline.Outcome, bool) {
	update := ui.Action{Key: 'y', Name: "update them"}
	skip := ui.Action{Key: 's', Name: "skip the update"}
	abort := ui.Action{Key: 'a', Name: "abort the run"}

	action, err := env.Menu.Ask(ctx, "Update these packages?", update, skip, abort)

	switch {
	case errors.Is(err, ui.ErrAborted), err == nil && action == abort:
		return pipeline.Abort(abortedRun), false
	case err != nil:
		return pipeline.Fail(err.Error()), false
	case action == skip:
		return pipeline.Skip(notUpdated), false
	}

	return pipeline.Success(), true
}

// command returns the update's command line, with --pretend or not: the
// configured command and options, the colours of emergup, an --exclude for
// each excluded package, and the target.
func (u updateStep) command(env pipeline.Env, pretend bool) []string {
	command := slices.Concat(u.config.Command, u.config.Options)
	if pretend {
		command = append(command, "--pretend")
	}

	command = append(command, colorOption(env))

	for _, excluded := range u.config.Exclude {
		command = append(command, "--exclude", excluded)
	}

	return append(command, u.config.Target)
}

// colorOption returns the option that makes emerge write in colour when
// emergup does: its output reaches the terminal through a pipe, where it
// would not by itself.
func colorOption(env pipeline.Env) string {
	if env.Printer.Color {
		return "--color=y"
	}

	return "--color=n"
}
