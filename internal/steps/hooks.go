// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"maps"
	"slices"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
)

// Hooks returns the hooks of the configuration that are enabled, in the
// order of their names, the order in which the pipeline runs the hooks of a
// place.
func Hooks(hooks config.Hooks) []pipeline.Hook {
	names := slices.Sorted(maps.Keys(hooks))
	enabled := make([]pipeline.Hook, 0, len(names))

	for _, name := range names {
		hook := hooks[name]
		if !hook.Enabled {
			continue
		}

		enabled = append(enabled, pipeline.Hook{
			Name:    name,
			When:    hook.At.When(),
			Step:    hook.At.Step(),
			Command: slices.Clone(hook.Command),
			GoesOn:  hook.OnFailure == config.HookContinue,
		})
	}

	return enabled
}
