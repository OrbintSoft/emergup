// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The lines of a check named backup before the run, with an elevated
// remedy.
const (
	backupLine    = "==> hook backup (before the run): run-backup\n"
	backupFailed  = "==> hook backup (before the run) failed: exit status 1\n"
	remedyAsk     = "==> Run the remedy and check again? [Y/g/a/?] "
	remedyLine    = "==> remedy of hook backup (before the run), as root: redo-backup run\n"
	abortAsk      = "==> Abort the run? [A/g/?] "
	runTheRemedy  = "run the remedy and check again"
	goOnWithout   = "go on without it"
	abortTheRun   = "abort the run"
	redoBackup    = "redo-backup"
	backupName    = "backup"
	backupFailure = "exit status 1"
	runPlace      = "before the run"
)

// check returns a check named backup before the run, with an elevated
// remedy, or with none when remedy is false.
func check(remedy bool) pipeline.Hook {
	backup := hook(backupName, pipeline.Before, pipeline.RunStep)
	backup.Asks = true

	if remedy {
		backup.Remedy, backup.RemedyRoot = []string{redoBackup, "run"}, true
	}

	return backup
}

func TestACheckThatPassesAsksNothing(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{check(true)}

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Equal(t, ran(backupName), rig.programs())
	assert.NotContains(t, rig.out.String(), remedyAsk)
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestAFailedCheckRunsItsRemedyAndChecksAgain(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'y', ui.KeyEnter)
	rig.hooks = []pipeline.Hook{check(true)}
	rig.runner.Script(failing(), succeeding(), succeeding())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.True(t, strings.HasPrefix(rig.out.String(), backupLine+backupFailed+remedyAsk+runTheRemedy+"\n"+
		remedyLine+backupLine+"==> [1/1] sync: do sync\n"), rig.out.String())
	assert.True(t, sync.ran)
	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.Empty(t, report.FailedHooks, "a check that passes after its remedy has not failed")

	commands := rig.runner.Commands()
	require.Len(t, commands, 3)
	assert.Equal(t, []string{redoBackup, "run"}, commands[1].Args)
	assert.True(t, commands[1].Root, "the remedy runs as root")
	assert.False(t, commands[0].Root, "the check runs as the invoking user")
	assert.Equal(t, commands[0].Env, commands[1].Env, "the remedy gets the variables of its hook")
}

func TestWithYesTheRemedyRunsOnce(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{check(true), hook("notify", pipeline.After, pipeline.RunStep)}
	rig.runner.Script(failing(), succeeding(), failing())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.Contains(t, rig.out.String(), backupFailed+remedyAsk+runTheRemedy+" (--yes)\n"+remedyLine+backupLine+
		"==> hook backup (before the run) failed: exit status 1, after its remedy\n")
	assert.False(t, sync.ran)
	assert.Equal(t, append(ran(backupName), redoBackup, "run-backup", "run-notify"), rig.programs())
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: backupName, Place: runPlace, Reason: "exit status 1, after its remedy", WentOn: false},
	}, report.FailedHooks)
}

func TestACheckThatFailsAgainAsksAgain(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'y', 'g', ui.KeyEnter)
	rig.hooks = []pipeline.Hook{check(true)}
	rig.runner.Script(failing(), succeeding(), failing())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.Contains(t, rig.out.String(), remedyLine+backupLine+backupFailed+remedyAsk+goOnWithout+"\n")
	assert.True(t, sync.ran, "the run goes on without the check")
	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: backupName, Place: runPlace, Reason: backupFailure, WentOn: true},
	}, report.FailedHooks)
	assert.Contains(t, rig.out.String(), "==> The hook backup (before the run) failed: exit status 1; the run went on.\n")
}

func TestARemedyThatFailsChecksAgain(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'y', ui.KeyEnter)
	rig.hooks = []pipeline.Hook{check(true)}
	rig.runner.Script(failing(), runnertest.Answer{Result: runner.Result{ExitCode: 2}}, succeeding())

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), remedyLine+"==> the remedy of hook backup failed: exit status 2\n"+backupLine)
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestAbortingAtACheckFailsTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'a')
	rig.hooks = []pipeline.Hook{check(true), hook("notify", pipeline.After, pipeline.RunStep)}
	rig.runner.Script(failing())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.Contains(t, rig.out.String(), backupFailed+remedyAsk+abortTheRun+"\n")
	assert.False(t, sync.ran)
	assert.Equal(t, ran(backupName, "notify"), rig.programs(), "the hooks after the run still run")
	assert.Contains(t, rig.runner.Commands()[1].Env, "EMERGUP_RESULT=failed")
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: backupName, Place: runPlace, Reason: backupFailure, WentOn: false},
	}, report.FailedHooks)
}

func TestCtrlCAtACheckStopsTheRun(t *testing.T) {
	t.Parallel()

	for _, keys := range [][]rune{{ui.KeyInterrupt}, nil} {
		rig := newRig(t, keys...)
		rig.hooks = []pipeline.Hook{check(true), hook("notify", pipeline.After, pipeline.RunStep)}
		rig.runner.Script(failing())

		report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

		assert.Equal(t, ran(backupName), rig.programs(), "no hook starts once the run is stopped")
		assert.Equal(t, pipeline.RunStopped, report.Result)
		assert.Empty(t, report.FailedHooks)
	}
}

func TestACheckWithoutARemedyAbortsByDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		keys   []rune
		yes    bool
		answer string
		result pipeline.Result
	}{
		{"Enter", []rune{ui.KeyEnter}, false, abortTheRun, pipeline.RunFailed},
		{"--yes", nil, true, abortTheRun + " (--yes)", pipeline.RunFailed},
		{"go on", []rune{'g', ui.KeyEnter}, false, goOnWithout, pipeline.RunFinished},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t, tt.keys...)
			rig.yes = tt.yes
			rig.hooks = []pipeline.Hook{check(false)}
			rig.runner.Script(failing())

			sync := rig.step(syncStep, time.Second, pipeline.Success())
			report := rig.run(t.Context(), sync)

			assert.Contains(t, rig.out.String(), backupFailed+abortAsk+tt.answer+"\n")
			assert.Equal(t, ran(backupName), rig.programs(), "there is no remedy to run")
			assert.Equal(t, tt.result == pipeline.RunFinished, sync.ran)
			assert.Equal(t, tt.result, report.Result)
		})
	}
}

func TestACheckWhoseQuestionCannotBeAskedFailsTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.err = errBroken
	rig.hooks = []pipeline.Hook{check(true)}
	rig.runner.Script(failing())

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.NotContains(t, rig.out.String(), question, "the run does not reach the step")
	require.Len(t, report.FailedHooks, 1)
	assert.False(t, report.FailedHooks[0].WentOn)
}

func TestAnElevatedHookRunsAsRoot(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	elevated := hook("snapshot", pipeline.Before, syncStep)
	elevated.Root = true
	rig.hooks = []pipeline.Hook{elevated}

	rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), "==> hook snapshot (before sync), as root: run-snapshot\n")
	require.Len(t, rig.runner.Commands(), 1)
	assert.True(t, rig.runner.Commands()[0].Root)
}

func TestPretendShowsWhatACheckWouldOffer(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true

	elevated := hook("snapshot", pipeline.After, pipeline.RunStep)
	elevated.Root = true
	rig.hooks = []pipeline.Hook{check(true), check(false), elevated}
	rig.hooks[1].Name = "plain"

	pipeline.Pretend(t.Context(), rig.env(), nil)

	assert.Equal(t, "    hook backup (before the run): run-backup; if it fails, offers to run as root: redo-backup run\n"+
		"    hook plain (before the run): run-backup; if it fails, asks whether to go on\n"+
		"    hook snapshot (after the run), as root: run-snapshot\n", rig.out.String())
	assert.Empty(t, rig.programs())
}
