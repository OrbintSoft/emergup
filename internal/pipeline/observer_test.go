// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

// startedUpdate is what the observer of the tests is told when the update
// starts.
const startedUpdate = "started update"

// observer records what a run tells it, with how many hook commands had
// run by then, so that a test sees when it was told.
type observer struct {
	runner *runnertest.Fake
	events []string
}

func (o *observer) Started(step pipeline.Step) {
	o.events = append(o.events, "started "+step.Name())
}

func (o *observer) Failed(_ context.Context, step pipeline.Step, report pipeline.StepReport) {
	o.events = append(o.events,
		fmt.Sprintf("failed %s (%s), after %d hook commands", report.Name, report.Reason, len(o.runner.Commands())))

	if step.Name() != report.Name {
		o.events = append(o.events, "told of another step: "+step.Name())
	}
}

// watched returns a rig whose run tells an observer how its steps go.
func watched(t *testing.T, keys ...rune) (*testRig, *observer) {
	t.Helper()

	rig := newRig(t, keys...)
	seen := &observer{runner: &rig.runner, events: nil}
	rig.observer = seen

	return rig, seen
}

func TestRunTellsTheObserverOfEachStepItRunsAndOfOneThatFails(t *testing.T) {
	t.Parallel()

	rig, seen := watched(t)
	rig.yes = true
	rig.keys.silent = true
	rig.hooks = []pipeline.Hook{
		hook("check", pipeline.Before, updateStep),
		hook("notify", pipeline.After, updateStep),
	}

	report := rig.run(t.Context(),
		rig.step(syncStep, time.Second, pipeline.Success()),
		rig.step(updateStep, time.Second, pipeline.Fail("emerge exited with status 1")),
		rig.step("depclean", time.Second, pipeline.Success()),
	)
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []string{
		"started sync",
		startedUpdate,
		"failed update (emerge exited with status 1), after 1 hook commands",
	}, seen.events, "told of the failure after the hook before the step, before the one after it")
	assert.Len(t, rig.runner.Commands(), 2, "the hook after the step still runs")
}

func TestRunTellsTheObserverOnlyOfTheStepsItRuns(t *testing.T) {
	t.Parallel()

	rig, seen := watched(t, 's')

	unavailable := rig.step("haskell", time.Second, pipeline.Success())
	unavailable.unavailable = "its program is not installed"

	disabled := rig.step("live-rebuild", time.Second, pipeline.Success())
	disabled.disabled = true

	rig.run(t.Context(),
		rig.step(syncStep, time.Second, pipeline.Success()),
		unavailable,
		disabled,
		withConfirms(rig.step(updateStep, time.Second, pipeline.Skip("nothing to update"))),
	)
	assert.Equal(t, []string{startedUpdate}, seen.events,
		"not the step skipped at its question, nor one that cannot run or is disabled; a skip is no failure")
}

// withConfirms returns step, which asks its own question.
func withConfirms(step *step) *step {
	step.confirms = true

	return step
}

func TestRunDoesNotTellTheObserverOfAStepThatWasStopped(t *testing.T) {
	t.Parallel()

	rig, seen := watched(t)
	rig.yes = true
	rig.keys.silent = true

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	interrupted := rig.step(updateStep, time.Second, pipeline.Fail("emerge exited with status 130"))
	interrupted.interrupt = cancel

	report := rig.run(ctx, interrupted)
	assert.Equal(t, pipeline.RunStopped, report.Result)
	assert.Equal(t, []string{startedUpdate}, seen.events, "a step stopped by Ctrl-C has not failed")
}

func TestPretendTellsTheObserverNothing(t *testing.T) {
	t.Parallel()

	rig, seen := watched(t)
	rig.keys.silent = true

	pipeline.Pretend(t.Context(), rig.env(), []pipeline.Step{rig.step(updateStep, 0, pipeline.Fail("never run"))})
	assert.Empty(t, seen.events)
}
