// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The reasons of a preserved rebuild that does not take place.
const (
	noPreservedLibraries = "no preserved libraries"
	notRebuilt           = "you chose not to rebuild them"
)

// noneFound is the exit status with which portageq says it found nothing.
const noneFound = 1

// preservedStep rebuilds the packages that use the libraries Portage
// preserved during an update, when there are any.
type preservedStep struct {
	config config.Step
}

// Name implements pipeline.Step.
func (preservedStep) Name() string { return "preserved-rebuild" }

// Title implements pipeline.Step.
func (preservedStep) Title() string { return "rebuild the packages that use preserved libraries" }

// Enabled implements pipeline.Step.
func (p preservedStep) Enabled() bool { return p.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the step asks once it
// knows there is something to rebuild, and skips without asking otherwise.
func (preservedStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step.
func (p preservedStep) Pretend(ctx context.Context, env pipeline.Env) ([]string, error) {
	libraries, err := preservedLibraries(ctx, env.Runner)
	if err != nil {
		return nil, err
	}

	if len(libraries) == 0 {
		return []string{noPreservedLibraries}, nil
	}

	lines := make([]string, 0, len(libraries)+1)
	for _, row := range rows(libraries) {
		lines = append(lines, row.text)
	}

	return append(lines, strings.Join(p.command(env), " ")), nil
}

// Run implements pipeline.Step.
func (p preservedStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	libraries, err := preservedLibraries(ctx, env.Runner)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if len(libraries) == 0 {
		return pipeline.Skip(noPreservedLibraries)
	}

	for _, row := range rows(libraries) {
		env.Printer.Row(ui.KindPlain, row.text)
	}

	rebuild := ui.Action{Key: 'y', Name: "rebuild them"}
	skip := ui.Action{Key: 's', Name: "skip the rebuild"}

	outcome, goOn := confirm(ctx, env, "Rebuild the packages that use them?", rebuild, skip, notRebuilt)
	if !goOn {
		return outcome
	}

	command := p.command(env)

	return runElevated(ctx, env, command, command[0])
}

// command returns the command line of the rebuild.
func (p preservedStep) command(env pipeline.Env) []string {
	return append(slices.Clone(p.config.Command), colorOption(env))
}

// preservedLibraries asks Portage which libraries it preserved, as
// decisions that name the package and the library; the query changes
// nothing, so it runs unprivileged.
func preservedLibraries(ctx context.Context, run runner.Runner) ([]decision, error) {
	command := []string{portageq, "list_preserved_libs", "/"}

	result, err := run.Run(ctx, runner.Command{Args: command, Mode: runner.Capture, Root: false})
	if err != nil {
		return nil, fmt.Errorf("could not list the preserved libraries: %w", err)
	}

	switch result.ExitCode {
	case 0:
	case noneFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("could not list the preserved libraries: %w",
			exitError{command: command[0], code: result.ExitCode})
	}

	return parsePreservedLibraries(string(result.Output)), nil
}

// parsePreservedLibraries reads the lines of `portageq list_preserved_libs`:
// a package, with or without a colon after it, then its libraries.
func parsePreservedLibraries(text string) []decision {
	var libraries []decision

	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		owner := strings.TrimSuffix(fields[0], ":")
		for _, library := range fields[1:] {
			libraries = append(libraries, decision{name: owner, act: true, reason: library})
		}
	}

	return libraries
}
