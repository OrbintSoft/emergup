// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

// The steps of the hook tests.
const (
	syncStep   = "sync"
	updateStep = "update"
)

// hook returns a hook named name at when and step, whose command is
// "run-<name>".
func hook(name, when, step string) pipeline.Hook {
	return pipeline.Hook{Name: name, When: when, Step: step, Command: []string{"run-" + name}, GoesOn: false}
}

// ran returns the programs of the hooks named names, in order.
func ran(names ...string) []string {
	programs := make([]string, 0, len(names))
	for _, name := range names {
		programs = append(programs, "run-"+name)
	}

	return programs
}

// programs returns the program of each command the rig's runner ran.
func (rig *testRig) programs() []string {
	commands := rig.runner.Commands()
	programs := make([]string, 0, len(commands))

	for _, command := range commands {
		programs = append(programs, command.Args[0])
	}

	return programs
}

// failing is the answer of a hook that exits with status 1.
func failing() runnertest.Answer {
	return runnertest.Answer{Result: runner.Result{ExitCode: 1, Output: nil, Duration: 0}, Err: nil}
}

// succeeding is the answer of a hook that succeeds.
func succeeding() runnertest.Answer {
	return runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: nil, Duration: 0}, Err: nil}
}

func TestRunRunsTheHooksAtTheirPlaces(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("first", pipeline.Before, pipeline.RunStep),
		hook("snapshot", pipeline.Before, updateStep),
		hook("check", pipeline.After, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
		hook("cleanup", pipeline.After, updateStep),
	}

	report := rig.run(t.Context(),
		rig.step(syncStep, time.Second, pipeline.Success()),
		rig.step(updateStep, time.Minute, pipeline.Success()),
	)

	assert.Equal(t, "==> hook first (before the run): run-first\n"+
		"==> [1/2] sync: do sync\n"+withYes+
		"==> sync succeeded in 1s\n"+
		"==> hook check (after sync): run-check\n"+
		"==> [2/2] update: do update\n"+
		"==> hook snapshot (before update): run-snapshot\n"+withYes+
		"==> update succeeded in 1m00s\n"+
		"==> hook cleanup (after update): run-cleanup\n"+
		"==> hook notify (after the run): run-notify\n"+
		summary+
		"    sync    succeeded      1s\n"+
		"    update  succeeded   1m00s\n"+
		"==> The run finished in 1m01s.\n", rig.out.String())

	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.Empty(t, report.FailedHooks)
	assert.Equal(t, ran("first", "check", "snapshot", "cleanup", "notify"), rig.programs())
}

func TestEachHookGetsTheContextOfTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hookEnv = []string{"EMERGUP_RUN_DIR=/runs/1", "EMERGUP_LOG=/runs/1/emergup.log"}
	rig.hooks = []pipeline.Hook{
		hook("first", pipeline.Before, pipeline.RunStep),
		hook("snapshot", pipeline.Before, updateStep),
		hook("cleanup", pipeline.After, updateStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	rig.run(t.Context(), rig.step(updateStep, time.Minute, pipeline.Skip("nothing to update")))

	commands := rig.runner.Commands()
	require.Len(t, commands, 4)

	base := []string{"EMERGUP_RUN_DIR=/runs/1", "EMERGUP_LOG=/runs/1/emergup.log"}
	assert.Equal(t, append(base, "EMERGUP_HOOK=first", "EMERGUP_WHEN=before", "EMERGUP_STEP=run"), commands[0].Env)
	assert.Equal(t, append(base, "EMERGUP_HOOK=snapshot", "EMERGUP_WHEN=before", "EMERGUP_STEP=update"),
		commands[1].Env)
	assert.Equal(t, append(base, "EMERGUP_HOOK=cleanup", "EMERGUP_WHEN=after", "EMERGUP_STEP=update",
		"EMERGUP_STEP_STATUS=skipped"), commands[2].Env)
	assert.Equal(t, append(base, "EMERGUP_HOOK=notify", "EMERGUP_WHEN=after", "EMERGUP_STEP=run",
		"EMERGUP_RESULT=finished"), commands[3].Env)

	for _, command := range commands {
		assert.False(t, command.Root, "a hook runs as the invoking user")
		assert.Equal(t, runner.Stream, command.Mode, "a hook's output is shown and logged")
	}
}

// TestHookVariablesNamesEveryVariableOfAHook keeps the names that the
// configuration of doas is told to keep in step with what a hook gets.
func TestHookVariablesNamesEveryVariableOfAHook(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hookEnv = pipeline.RunDirEnv("/runs/1", "/runs/1/emergup.log")
	rig.hooks = []pipeline.Hook{
		hook("first", pipeline.Before, pipeline.RunStep),
		hook("cleanup", pipeline.After, updateStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	rig.run(t.Context(), rig.step(updateStep, time.Minute, pipeline.Success()))

	var names []string

	for _, command := range rig.runner.Commands() {
		for _, variable := range command.Env {
			name, _, _ := strings.Cut(variable, "=")
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}

	assert.ElementsMatch(t, pipeline.HookVariables(), names)
}

func TestAStepsHooksRunAroundItsQuestion(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 's')
	rig.hooks = []pipeline.Hook{hook("snapshot", pipeline.Before, updateStep), hook("cleanup", pipeline.After, updateStep)}

	update := rig.step(updateStep, time.Minute, pipeline.Success())
	rig.run(t.Context(), update)

	assert.Contains(t, rig.out.String(), "==> [1/1] update: do update\n"+
		"==> hook snapshot (before update): run-snapshot\n"+question+"skip the step\n"+
		"==> update skipped: you chose to skip it\n"+
		"==> hook cleanup (after update): run-cleanup\n")
	assert.False(t, update.ran)
	assert.Equal(t, ran("snapshot", "cleanup"), rig.programs())
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_STEP_STATUS=skipped")
}

func TestADisabledStepRunsNoHook(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{hook("snapshot", pipeline.Before, updateStep), hook("cleanup", pipeline.After, updateStep)}

	update := rig.step(updateStep, time.Minute, pipeline.Success())
	update.disabled = true

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()), update)

	assert.Empty(t, rig.programs())
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestTheHooksAfterAFailedStepRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("cleanup", pipeline.After, syncStep), hook("notify", pipeline.After, pipeline.RunStep),
	}

	report := rig.run(t.Context(),
		rig.step(syncStep, time.Second, pipeline.Fail("exit status 1")),
		rig.step(updateStep, time.Minute, pipeline.Success()),
	)

	assert.Equal(t, ran("cleanup", "notify"), rig.programs())
	assert.Contains(t, rig.runner.Commands()[0].Env, "EMERGUP_STEP_STATUS=failed")
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_RESULT=failed")
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []pipeline.Status{pipeline.Failed, pipeline.NotRun}, statuses(report))
}

func TestABeforeHookThatFailsStopsTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("snapshot", pipeline.Before, updateStep),
		hook("second", pipeline.Before, updateStep),
		hook("cleanup", pipeline.After, updateStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}
	rig.runner.Script(failing())

	update := rig.step(updateStep, time.Minute, pipeline.Success())
	report := rig.run(t.Context(), update, rig.step("depclean", time.Second, pipeline.Success()))

	assert.False(t, update.ran)
	assert.Equal(t, ran("snapshot", "notify"), rig.programs(),
		"the hooks after a failed before hook do not run, but those after the run do")
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_RESULT=failed")
	assert.Equal(t, "==> [1/2] update: do update\n"+
		"==> hook snapshot (before update): run-snapshot\n"+
		"==> hook snapshot (before update) failed: exit status 1\n"+
		"==> hook notify (after the run): run-notify\n"+
		summary+
		"    update    not run\n"+
		"    depclean  not run\n"+
		"==> The hook snapshot (before update) failed: exit status 1.\n"+
		"==> The run failed after <1s.\n", rig.out.String())

	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: "snapshot", Place: "before update", Reason: "exit status 1", WentOn: false},
	}, report.FailedHooks)
}

func TestAHookBeforeTheRunThatFailsRunsNoStep(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true
	rig.hooks = []pipeline.Hook{
		hook("check", pipeline.Before, pipeline.RunStep),
		hook("second", pipeline.Before, pipeline.RunStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}
	rig.runner.Script(failing())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.False(t, sync.ran)
	assert.NotContains(t, rig.out.String(), "sync: do sync", "no step is announced")
	assert.Equal(t, ran("check", "notify"), rig.programs())
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestEveryAfterHookRunsWhenOneFails(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("check", pipeline.After, syncStep),
		hook("second", pipeline.After, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
		hook("last", pipeline.After, pipeline.RunStep),
	}
	rig.runner.Script(failing(), succeeding(), failing())

	update := rig.step(updateStep, time.Minute, pipeline.Success())
	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()), update)

	assert.False(t, update.ran, "a failed after hook stops the run before the next step")
	assert.Equal(t, ran("check", "second", "notify", "last"), rig.programs())
	assert.Contains(t, rig.runner.Commands()[2].Env, "EMERGUP_RESULT=failed")
	assert.Equal(t, []pipeline.Status{pipeline.Succeeded, pipeline.NotRun}, statuses(report),
		"the step keeps its own status")
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Len(t, report.FailedHooks, 2)
}

func TestAHookAfterTheRunThatFailsFailsTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{hook("notify", pipeline.After, pipeline.RunStep)}
	rig.runner.Script(failing())

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Contains(t, rig.out.String(), "==> The hook notify (after the run) failed: exit status 1.\n"+
		"==> The run failed after 1s.\n")
}

func TestAHookThatGoesOnOnlyShowsANotice(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	optional := hook("snapshot", pipeline.Before, updateStep)
	optional.GoesOn = true
	rig.hooks = []pipeline.Hook{optional, hook("second", pipeline.Before, updateStep)}
	rig.runner.Script(failing())

	update := rig.step(updateStep, time.Minute, pipeline.Success())
	report := rig.run(t.Context(), update)

	assert.True(t, update.ran)
	assert.Equal(t, ran("snapshot", "second"), rig.programs())
	assert.Contains(t, rig.out.String(),
		"==> hook snapshot (before update) failed: exit status 1; the run goes on\n"+
			"==> hook second (before update): run-second\n")
	assert.Contains(t, rig.out.String(), "==> The hook snapshot (before update) failed: exit status 1; the run went on.\n"+
		"==> The run finished in 1m00s.\n")
	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: "snapshot", Place: "before update", Reason: "exit status 1", WentOn: true},
	}, report.FailedHooks)
}

// errNotFound is the error of a hook whose program cannot start.
var errNotFound = errors.New("executable file not found in $PATH")

func TestTheReasonAHookFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer runnertest.Answer
		reason string
	}{
		{"exit status", runnertest.Answer{Result: runner.Result{ExitCode: 3}}, "exit status 3"},
		{"signal", runnertest.Answer{Result: runner.Result{ExitCode: -1}}, "ended by a signal"},
		{"cannot start", runnertest.Answer{Err: errNotFound}, errNotFound.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t)
			rig.yes = true
			rig.hooks = []pipeline.Hook{hook("check", pipeline.Before, pipeline.RunStep)}
			rig.runner.Script(tt.answer)

			report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

			require.Len(t, report.FailedHooks, 1)
			assert.Equal(t, tt.reason, report.FailedHooks[0].Reason)
			assert.Contains(t, rig.out.String(), "==> hook check (before the run) failed: "+tt.reason+"\n")
		})
	}
}

func TestNoHookRunsOnceTheRunIsAborted(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'a')
	rig.hooks = []pipeline.Hook{
		hook("snapshot", pipeline.Before, syncStep),
		hook("cleanup", pipeline.After, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Equal(t, ran("snapshot"), rig.programs())
	assert.Equal(t, pipeline.RunStopped, report.Result)
}

func TestNoHookRunsOnceTheRunIsInterrupted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("cleanup", pipeline.After, syncStep), hook("notify", pipeline.After, pipeline.RunStep),
	}

	sync := rig.step(syncStep, time.Second, pipeline.Fail("interrupted"))
	sync.interrupt = cancel

	report := rig.run(ctx, sync)

	assert.Empty(t, rig.programs())
	assert.Equal(t, pipeline.RunStopped, report.Result)
}

// interruptingRunner ends the run's context while a command runs, as a
// Ctrl-C during a hook would.
type interruptingRunner struct {
	runnertest.Fake

	cancel context.CancelFunc
}

// Run records cmd, ends the context and returns its error, as a runner
// does for a command whose context ends while it runs.
func (r *interruptingRunner) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	result, err := r.Fake.Run(ctx, cmd)
	if err != nil {
		return result, fmt.Errorf("interrupting: %w", err)
	}

	r.cancel()

	return runner.Result{ExitCode: -1, Output: nil, Duration: 0}, fmt.Errorf("interrupting: %w", ctx.Err())
}

func TestAHookInterruptedStopsTheRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{
		hook("snapshot", pipeline.Before, syncStep),
		hook("second", pipeline.Before, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	interrupting := &interruptingRunner{cancel: cancel}
	env := rig.env()
	env.Runner = interrupting

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := pipeline.Run(ctx, env, []pipeline.Step{sync})

	assert.False(t, sync.ran)
	assert.Len(t, interrupting.Commands(), 1, "neither the next hook nor the one after the run starts")
	assert.Empty(t, report.FailedHooks, "an interrupted hook has not failed")
	assert.Equal(t, pipeline.RunStopped, report.Result)
}

func TestNoHookStartsOnceTheContextHasEnded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{hook("first", pipeline.Before, pipeline.RunStep)}

	report := rig.run(ctx, rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Empty(t, rig.programs())
	assert.Equal(t, pipeline.RunStopped, report.Result)
}

func TestAStepWhoseQuestionCannotBeAskedRunsNoAfterHook(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.err = errBroken
	rig.hooks = []pipeline.Hook{
		hook("snapshot", pipeline.Before, syncStep),
		hook("cleanup", pipeline.After, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.False(t, sync.ran)
	assert.Equal(t, ran("snapshot", "notify"), rig.programs())
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestPretendShowsTheHooksAtTheirPlaces(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true
	rig.hooks = []pipeline.Hook{
		hook("first", pipeline.Before, pipeline.RunStep),
		hook("snapshot", pipeline.Before, updateStep),
		hook("cleanup", pipeline.After, updateStep),
		hook("check", pipeline.After, syncStep),
		hook("notify", pipeline.After, pipeline.RunStep),
	}

	sync := rig.step(syncStep, 0, pipeline.Success())
	sync.disabled = true
	update := rig.step(updateStep, 0, pipeline.Success())
	update.pretend = []string{"emerge --update @world"}

	result := pipeline.Pretend(t.Context(), rig.env(), []pipeline.Step{sync, update})

	assert.Equal(t, "    hook first (before the run): run-first\n"+
		"==> sync is disabled in the configuration\n"+
		"==> [1/1] update: do update\n"+
		"    hook snapshot (before update): run-snapshot\n"+
		"    emerge --update @world\n"+
		"    hook cleanup (after update): run-cleanup\n"+
		"    hook notify (after the run): run-notify\n", rig.out.String())
	assert.Empty(t, rig.programs(), "the pretend runs no hook")
	assert.Equal(t, pipeline.RunFinished, result)
}

func TestResultString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "finished", pipeline.RunFinished.String())
	assert.Equal(t, "failed", pipeline.RunFailed.String())
	assert.Equal(t, "stopped", pipeline.RunStopped.String())
	assert.Equal(t, "Result(7)", pipeline.Result(7).String())
}
