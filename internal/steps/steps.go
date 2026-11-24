// SPDX-License-Identifier: GPL-3.0-or-later

// Package steps holds the built-in steps of an update, in the order a run
// performs them, as the configuration sets them up.
package steps

import (
	"context"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
)

// notAvailable is what a step whose work is not written yet says.
const notAvailable = "not available yet in this version of emergup"

// Options are the options of a run that change what the steps do.
type Options struct {
	// ForceSync syncs every repository, whatever its last sync.
	ForceSync bool
}

// BuiltIn returns the built-in steps that cfg sets up, in the order of the
// [steps] section of the configuration.
func BuiltIn(cfg config.Config, options Options) []pipeline.Step {
	built := cfg.Steps

	return []pipeline.Step{
		syncStep{config: built.Sync, force: options.ForceSync},
		updateStep{config: built.Update},
		liveStep{config: built.LiveRebuild},
		preservedStep{config: built.PreservedRebuild},
		configFilesStep{config: built.ConfigFiles},
		placeholder{name: "depclean", title: "remove the packages no longer needed", enabled: built.Depclean.Enabled},
	}
}

// placeholder is a built-in step whose work is not written yet: it runs
// nothing and is skipped.
type placeholder struct {
	name    string
	title   string
	enabled bool
}

// Name implements pipeline.Step.
func (p placeholder) Name() string { return p.name }

// Title implements pipeline.Step.
func (p placeholder) Title() string { return p.title }

// Enabled implements pipeline.Step.
func (p placeholder) Enabled() bool { return p.enabled }

// Pretend implements pipeline.Step.
func (placeholder) Pretend(context.Context, pipeline.Env) ([]string, error) {
	return []string{notAvailable}, nil
}

// Run implements pipeline.Step.
func (placeholder) Run(context.Context, pipeline.Env) pipeline.Outcome {
	return pipeline.Skip(notAvailable)
}
