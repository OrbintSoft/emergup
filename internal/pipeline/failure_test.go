// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"context"
	"errors"
	"fmt"
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

// errNoAnswer is the error of an explanation that did not come.
var errNoAnswer = errors.New("no answer from ask-ai: it exited with status 3")

// explained records the failures a rig's explain is asked about, and
// answers each with err.
func explained(rig *testRig, err error) *[]string {
	asked := &[]string{}

	rig.explain = func(_ context.Context, step pipeline.Step, report pipeline.StepReport) error {
		*asked = append(*asked, fmt.Sprintf("%s: %s, %s", step.Name(), report.Status, report.Reason))

		return err
	}

	return asked
}

// explainPrompt is the question after a failed step that can be explained.
const explainPrompt = "==> Run this step again? [r/s/a/e/?] "

func TestAFailedStepCanBeExplained(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'e', 'a')
	asked := explained(rig, nil)

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedUpdate+explainPrompt+"explain the failure\n"+
		explainPrompt+"end the run\n", "then the same question again")
	assert.Equal(t, []string{"update: failed, " + emergeFailed}, *asked)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestAnExplanationThatFailsIsSaid(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'e', 's', 'y')
	explained(rig, errNoAnswer)

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), explainPrompt+"explain the failure\n"+
		"==> "+errNoAnswer.Error()+"\n"+
		explainPrompt+"go on with the next step\n", "a notice, not a second failure")
	assert.True(t, steps[2].ran)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestAnAbortedExplanationStopsTheRun(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'e')
	explained(rig, fmt.Errorf("asking whether to send it: %w", ui.ErrAborted))

	report := runSteps(t, rig, steps)

	assert.Equal(t, pipeline.RunStopped, report.Result)
	assert.NotContains(t, rig.out.String(), "asking whether", "an abort is no failure to tell")
}

func TestWithoutAnExplainerTheFailureHasNoExplanation(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'e', 'a')

	runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedPrompt+"\n"+notAnAnswer+failedPrompt+"end the run\n",
		"e is not an answer")
}

// chatted records the failures a rig's chat is asked about, and answers
// each with err.
func chatted(rig *testRig, err error) *[]string {
	asked := &[]string{}

	rig.chat = func(_ context.Context, step pipeline.Step, report pipeline.StepReport) error {
		*asked = append(*asked, fmt.Sprintf("%s: %s, %s", step.Name(), report.Status, report.Reason))

		return err
	}

	return asked
}

func TestAFailedStepCanBeChattedAbout(t *testing.T) {
	t.Parallel()

	const chatPrompt = "==> Run this step again? [r/s/a/e/c/?] "

	rig, steps := failedThen(t, nil, 'y', 'y', 'c', 'e', 'a')
	chats := chatted(rig, nil)
	explanations := explained(rig, nil)

	report := runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), failedUpdate+chatPrompt+"chat about the failure\n"+
		chatPrompt+"explain the failure\n"+chatPrompt+"end the run\n", "the same question after each")
	assert.Equal(t, []string{"update: failed, " + emergeFailed}, *chats)
	assert.Equal(t, []string{"update: failed, " + emergeFailed}, *explanations)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestAChatThatFailsIsSaid(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'c', 'a')
	chatted(rig, errNoAnswer)

	runSteps(t, rig, steps)

	assert.Contains(t, rig.out.String(), "==> Run this step again? [r/s/a/c/?] chat about the failure\n"+
		"==> "+errNoAnswer.Error()+"\n", "without an explainer, no e; the failure of the chat is a notice")
}

func TestAnAbortedChatStopsTheRun(t *testing.T) {
	t.Parallel()

	rig, steps := failedThen(t, nil, 'y', 'y', 'c', 'a')
	chatted(rig, fmt.Errorf("asking whether to send it: %w", ui.ErrAborted))

	assert.Equal(t, pipeline.RunStopped, runSteps(t, rig, steps).Result)
}
