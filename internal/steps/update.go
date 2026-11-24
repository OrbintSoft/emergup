// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"regexp"
	"slices"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The reasons of an update that does not take place.
const (
	nothingToUpdate = "nothing to update"
	notUpdated      = "you chose not to update"
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

// Pretend implements pipeline.Step.
func (u updateStep) Pretend(_ context.Context, env pipeline.Env) ([]string, error) {
	return u.work().lines(env), nil
}

// Run implements pipeline.Step.
func (u updateStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	return u.work().run(ctx, env)
}

// work is what the update does: --pretend, then emergup's question.
func (u updateStep) work() pretended {
	return pretended{
		command:     u.command,
		nothing:     nothingToMerge,
		nothingDone: nothingToUpdate,
		question:    "Update these packages?",
		goOn:        ui.Action{Key: 'y', Name: "update them"},
		skip:        ui.Action{Key: 's', Name: "skip the update"},
		skipped:     notUpdated,
	}
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
