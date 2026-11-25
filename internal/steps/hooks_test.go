// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/steps"
)

// notify is the name and the program of a hook.
const notify = "notify"

func TestHooksAreTheEnabledOnesInTheOrderOfTheirNames(t *testing.T) {
	t.Parallel()

	hooks := steps.Hooks(config.Hooks{
		"snapshot": {Enabled: true, At: "before-update", Command: []string{"snap", "pre"}, OnFailure: config.HookStop},
		notify:     {Enabled: true, At: "after-run", Command: []string{notify}, OnFailure: config.HookContinue},
		"off":      {Enabled: false, At: "before-run", Command: []string{"off"}, OnFailure: config.HookStop},
		"cleanup":  {Enabled: true, At: "after-live-rebuild", Command: []string{"clean"}, OnFailure: config.HookStop},
	})

	assert.Equal(t, []pipeline.Hook{
		{Name: "cleanup", When: pipeline.After, Step: liveName, Command: []string{"clean"}, GoesOn: false},
		{Name: notify, When: pipeline.After, Step: pipeline.RunStep, Command: []string{notify}, GoesOn: true},
		{Name: "snapshot", When: pipeline.Before, Step: "update", Command: []string{"snap", "pre"}, GoesOn: false},
	}, hooks)
}

func TestHooksKeepWhatAHookRunsAsRootAndAsks(t *testing.T) {
	t.Parallel()

	hooks := steps.Hooks(config.Hooks{
		"backup": {
			Enabled: true, At: "before-run", Command: []string{"fresh"}, Elevated: true,
			OnFailure: config.HookAsk, Remedy: []string{"redo-backup", "run"}, RemedyElevated: true,
		},
	})

	assert.Equal(t, []pipeline.Hook{{
		Name: "backup", When: pipeline.Before, Step: pipeline.RunStep, Command: []string{"fresh"}, Root: true,
		GoesOn: false, Asks: true, Remedy: []string{"redo-backup", "run"}, RemedyRoot: true,
	}}, hooks)
}

func TestHooksWithoutAny(t *testing.T) {
	t.Parallel()

	assert.Empty(t, steps.Hooks(config.Default().Hooks))
}

// TestTheHookPlacesNameTheStepsOfTheRun keeps the places the configuration
// writes in step with the names the pipeline gives the steps and the run.
func TestTheHookPlacesNameTheStepsOfTheRun(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, len(config.StepNames()))
	for _, step := range steps.BuiltIn(config.Default(), steps.Options{ForceSync: false}) {
		names = append(names, step.Name())
	}

	assert.Equal(t, config.StepNames(), names)
	assert.Equal(t, pipeline.RunStep, config.RunName)
	assert.Equal(t, pipeline.Before, config.HookPlace("before-run").When())
	assert.Equal(t, pipeline.After, config.HookPlace("after-run").When())
}
