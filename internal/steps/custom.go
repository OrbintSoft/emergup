// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
)

// All returns the steps of a run that cfg sets up: the built-in steps, in
// the order of the [steps] section, each followed by the custom steps after
// it in the order of their names. A custom step turned off is among them, as
// a built-in step turned off is.
func All(cfg config.Config, options Options) []pipeline.Step {
	builtIn := BuiltIn(cfg, options)
	names := slices.Sorted(maps.Keys(cfg.CustomSteps))
	all := make([]pipeline.Step, 0, len(builtIn)+len(names))

	for _, step := range builtIn {
		all = append(all, step)

		for _, name := range names {
			if cfg.CustomSteps[name].After == step.Name() {
				all = append(all, customStep{name: name, config: cfg.CustomSteps[name]})
			}
		}
	}

	return all
}

// customStep runs a command that the configuration adds to the steps of a
// run.
type customStep struct {
	name   string
	config config.CustomStep
}

// Name implements pipeline.Step.
func (c customStep) Name() string { return c.name }

// Title implements pipeline.Step: the configured title, or one that names
// the program.
func (c customStep) Title() string {
	if c.config.Title != "" {
		return c.config.Title
	}

	return "run " + c.program()
}

// Enabled implements pipeline.Step.
func (c customStep) Enabled() bool { return c.config.Enabled }

// Unavailable implements pipeline.MayBeUnavailable: an optional step whose
// program is not installed cannot run. One that is not optional runs, and
// fails, so that a mistyped program does not go unnoticed.
func (c customStep) Unavailable(env pipeline.Env) string {
	if !c.config.Optional {
		return ""
	}

	if _, err := env.LookPath(c.program()); err != nil {
		return c.program() + " is not installed"
	}

	return ""
}

// Pretend implements pipeline.Step.
func (c customStep) Pretend(_ context.Context, env pipeline.Env) ([]string, error) {
	if reason := c.Unavailable(env); reason != "" {
		return []string{reason + ", so the step would be skipped"}, nil
	}

	run := "would run: "
	if c.config.Elevated {
		run = "would run as root: "
	}

	return []string{run + strings.Join(c.config.Command, " ")}, nil
}

// Run implements pipeline.Step.
func (c customStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	command := runner.Command{
		Args: slices.Clone(c.config.Command), Mode: runner.Stream, Root: c.config.Elevated, Env: nil,
		Watch: nil, Into: nil, Stdin: nil, Dir: "",
	}

	return runCommand(ctx, env, command, c.program())
}

// program returns the program the step starts, or "" when it has none.
func (c customStep) program() string {
	if len(c.config.Command) == 0 {
		return ""
	}

	return c.config.Command[0]
}
