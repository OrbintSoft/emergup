// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The lines of a failed step and of the question after it.
const (
	failedUpdate  = "==> update failed after 3s: emerge exited with status 1\n"
	failedPrompt  = "==> Run this step again? [r/s/a/?] "
	emergeFailed  = "emerge exited with status 1"
	answeredRun   = question + "run the step\n"
	notAnAnswer   = "==> Not an answer: press one of the keys in brackets, or ? for the list.\n"
	updateFailRow = "    update    failed         3s  emerge exited with status 1\n"
)

// failedThen returns a rig answering keys, and the steps sync, update and
// depclean, of which update fails, then ends with then when it runs again.
func failedThen(t *testing.T, then []pipeline.Outcome, keys ...rune) (*testRig, []*step) {
	t.Helper()

	rig := newRig(t, keys...)
	update := rig.step(updateStep, 3*time.Second, pipeline.Fail(emergeFailed))
	update.then = then

	return rig, []*step{
		rig.step(syncStep, 42*time.Second, pipeline.Success()),
		update,
		rig.step("depclean", time.Second, pipeline.Success()),
	}
}

// runSteps runs steps on rig.
func runSteps(t *testing.T, rig *testRig, steps []*step) pipeline.Report {
	t.Helper()

	all := make([]pipeline.Step, 0, len(steps))
	for _, step := range steps {
		all = append(all, step)
	}

	return rig.run(t.Context(), all...)
}

func TestAFailedStepCanBePassedOver(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 's', 'y')
	rig.hooks = []pipeline.Hook{hook("notify", pipeline.After, pipeline.RunStep)}

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), answeredRun+failedUpdate+failedPrompt+"go on with the next step\n")
	assert.Contains(t, rig.out.String(), summary+
		"    sync      succeeded     42s\n"+
		updateFailRow+
		"    depclean  succeeded      1s\n"+
		"==> The run failed after 46s.\n")
	assert.True(t, steps[2].ran, "the next step runs")
	assert.Equal(t, 1, steps[1].runs)
	assert.Equal(t, pipeline.RunFailed, report.Result, "a step passed over still fails the run")
	assert.Equal(t, []pipeline.Status{pipeline.Succeeded, pipeline.Failed, pipeline.Succeeded}, statuses(report))
	require.Len(t, rig.runner.Commands(), 1)
	assert.Contains(t, rig.runner.Commands()[0].Env, "EMERGUP_RESULT=failed")
}

func TestAFailedStepCanRunAgain(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, []pipeline.Outcome{pipeline.Success()}, 'y', 'y', 'r', 'y')
	rig.hooks = []pipeline.Hook{
		hook("snapshot", pipeline.Before, updateStep),
		hook("cleanup", pipeline.After, updateStep),
	}

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(),
		answeredRun+failedUpdate+failedPrompt+"run it again\n"+"==> update succeeded in 3s\n")
	assert.Contains(t, rig.out.String(), "    update    succeeded      6s\n", "the time of both runs")
	assert.Equal(t, 2, steps[1].runs)
	assert.Equal(t, 3, strings.Count(rig.out.String(), question), "its question is not asked again")
	assert.Equal(t, ran("snapshot", "cleanup"), rig.programs(), "its hooks run once, the after one at the end")
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_STEP_STATUS=succeeded")
	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.True(t, steps[2].ran)
}

func TestAStepThatFailsAgainIsAskedAboutAgain(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'r', 's', 'y')

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(),
		failedUpdate+failedPrompt+"run it again\n"+
			"==> update failed after 3s: emerge exited with status 1\n"+failedPrompt+"go on with the next step\n")
	assert.Contains(t, rig.out.String(), "    update    failed         6s  emerge exited with status 1\n")
	assert.Equal(t, 2, steps[1].runs)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestAFailedStepCanEndTheRun(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'a')
	rig.hooks = []pipeline.Hook{
		hook("cleanup", pipeline.After, updateStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedPrompt+"end the run\n")
	assert.Contains(t, rig.out.String(), "    depclean  not run\n"+"==> The run failed after 45s.\n")
	assert.False(t, steps[2].ran)
	assert.Equal(t, pipeline.RunFailed, report.Result, "it ends as a failure, not stopped")
	assert.Equal(t, ran("cleanup", "notify"), rig.programs(), "the hooks after the step and after the run still run")
	assert.Contains(t, rig.runner.Commands()[0].Env, "EMERGUP_STEP_STATUS=failed")
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_RESULT=failed")
}

func TestEnterIsNotAnAnswerAfterAFailedStep(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', ui.KeyEnter, 's', 'y')

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedPrompt+"\n"+notAnAnswer+failedPrompt+"go on with the next step\n")
	assert.Equal(t, 1, steps[1].runs)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestTheEndOfTheInputAfterAFailedStepStopsTheRun(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y')
	rig.hooks = []pipeline.Hook{
		hook("cleanup", pipeline.After, updateStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedUpdate+failedPrompt+"\n"+summary)
	assert.Contains(t, rig.out.String(), "==> The run was stopped after 45s.\n")
	assert.Equal(t, pipeline.RunStopped, report.Result)
	assert.Equal(t, []pipeline.Status{pipeline.Succeeded, pipeline.Failed, pipeline.NotRun}, statuses(report))
	assert.Empty(t, rig.programs(), "no hook starts once the run is stopped")
}

func TestAFailedStepUnderYesEndsTheRunWithoutAQuestion(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil)
	rig.yes = true
	rig.keys.silent = true

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), withYes+failedUpdate+summary)
	assert.NotContains(t, rig.out.String(), "again")
	assert.Equal(t, 1, steps[1].runs)
	assert.False(t, steps[2].ran)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestTheObserverIsToldOfEachRunOfAFailedStep(t *testing.T) {
	t.Parallel()

	rig, seen := watched(t, 'y', 'r', 'a')
	update := rig.step(updateStep, time.Second, pipeline.Fail(emergeFailed))
	update.then = []pipeline.Outcome{pipeline.Fail("emerge exited with status 2")}

	report := rig.run(t.Context(), update)

	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []string{
		startedUpdate,
		"failed update (emerge exited with status 1), after 0 hook commands",
		startedUpdate,
		"failed update (emerge exited with status 2), after 0 hook commands",
	}, seen.events)
}
