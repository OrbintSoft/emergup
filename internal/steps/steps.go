// SPDX-License-Identifier: GPL-3.0-or-later

// Package steps holds the built-in steps of an update, in the order a run
// performs them, as the configuration sets them up.
package steps

import (
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
)

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
		depcleanStep{config: built.Depclean},
	}
}
