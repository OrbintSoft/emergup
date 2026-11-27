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

// The reasons of a depclean that removes nothing.
const (
	nothingToRemove = "nothing to remove"
	notRemoved      = "you chose not to remove them"
)

// nothingToUnmerge matches the line with which emerge --depclean --pretend
// says that it would remove nothing.
var nothingToUnmerge = regexp.MustCompile(`(?m)^Number to remove:\s+0\s*$`)

// depcleanStep removes the packages nothing needs any more: it shows what
// emerge would remove with --pretend, asks, then removes them.
type depcleanStep struct {
	config config.Step
}

// Name implements pipeline.Step.
func (depcleanStep) Name() string { return "depclean" }

// Title implements pipeline.Step.
func (depcleanStep) Title() string { return "remove the packages no longer needed" }

// Enabled implements pipeline.Step.
func (d depcleanStep) Enabled() bool { return d.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the depclean asks
// once the pretend has shown what it would remove.
func (depcleanStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step.
func (d depcleanStep) Pretend(_ context.Context, env pipeline.Env) ([]string, error) {
	return d.work().lines(env), nil
}

// Run implements pipeline.Step.
func (d depcleanStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	return d.work().run(ctx, env)
}

// work is what the depclean does: --pretend, then emergup's question.
func (d depcleanStep) work() pretended {
	return pretended{
		command:     d.command,
		nothing:     nothingToUnmerge,
		nothingDone: nothingToRemove,
		question:    "Remove these packages?",
		goOn:        ui.Action{Key: 'y', Name: "remove them"},
		skip:        ui.Action{Key: 's', Name: "skip the depclean"},
		skipped:     notRemoved,
	}
}

// command returns the depclean's command line, with --pretend or not.
func (d depcleanStep) command(env pipeline.Env, pretend bool) []string {
	command := slices.Clone(d.config.Command)
	if pretend {
		command = append(command, pretendOption)
	}

	return append(command, colorOption(env))
}
