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

// onlyFor is the notice of the check backup, whose remedy is only for the
// exit statuses 1 and 2, when it fails otherwise.
const onlyFor = "==> the remedy of hook backup is only for exit status 1 or 2\n"

// checkFor returns the check of check(true), whose remedy is only for the
// exit statuses 1 and 2.
func checkFor() pipeline.Hook {
	backup := check(true)
	backup.RemedyStatus = []int{1, 2}

	return backup
}

func TestACheckOffersItsRemedyOnlyForItsStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer runnertest.Answer
		failed string
		offers bool
	}{
		{"a status of the list", runnertest.Answer{Result: runner.Result{ExitCode: 2}}, "exit status 2", true},
		{"another status", runnertest.Answer{Result: runner.Result{ExitCode: 3}}, "exit status 3", false},
		{"a signal", runnertest.Answer{Result: runner.Result{ExitCode: -1}}, "ended by a signal", false},
		{"a check that cannot start", runnertest.Answer{Err: errNotFound}, errNotFound.Error(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t)
			rig.yes = true
			rig.hooks = []pipeline.Hook{checkFor()}
			rig.runner.Script(tt.answer, succeeding(), succeeding())

			sync := rig.step(syncStep, time.Second, pipeline.Success())
			report := rig.run(t.Context(), sync)

			failed := "==> hook backup (before the run) failed: " + tt.failed + "\n"

			if tt.offers {
				assert.Contains(t, rig.out.String(), failed+remedyAsk+runTheRemedy+" (--yes)\n"+remedyLine)
				assert.Equal(t, append(ran(backupName), redoBackup, "run-backup"), rig.programs())
				assert.Equal(t, pipeline.RunFinished, report.Result)

				return
			}

			assert.Contains(t, rig.out.String(), failed+onlyFor+abortAsk+abortTheRun+" (--yes)\n")
			assert.Equal(t, ran(backupName), rig.programs(), "the remedy is not for that failure")
			assert.False(t, sync.ran)
			assert.Equal(t, pipeline.RunFailed, report.Result)
			assert.Equal(t, []pipeline.FailedHook{
				{Name: backupName, Place: runPlace, Reason: tt.failed, WentOn: false},
			}, report.FailedHooks)
		})
	}
}

func TestAFailureTheRemedyIsNotForAsksWhetherToGoOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		keys   []rune
		answer string
		result pipeline.Result
	}{
		{"Enter", []rune{ui.KeyEnter}, abortTheRun, pipeline.RunFailed},
		{"go on", []rune{'g', ui.KeyEnter}, goOnWithout, pipeline.RunFinished},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t, tt.keys...)
			rig.hooks = []pipeline.Hook{checkFor()}
			rig.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 3}})

			sync := rig.step(syncStep, time.Second, pipeline.Success())
			report := rig.run(t.Context(), sync)

			assert.Contains(t, rig.out.String(), onlyFor+abortAsk+tt.answer+"\n")
			assert.Equal(t, ran(backupName), rig.programs())
			assert.Equal(t, tt.result == pipeline.RunFinished, sync.ran)
			assert.Equal(t, tt.result, report.Result)
		})
	}
}

func TestACheckThatFailsOtherwiseAfterItsRemedyAsksWhetherToGoOn(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'y', ui.KeyEnter)
	rig.hooks = []pipeline.Hook{checkFor()}
	rig.runner.Script(failing(), succeeding(), runnertest.Answer{Result: runner.Result{ExitCode: 3}})

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), remedyLine+backupLine+
		"==> hook backup (before the run) failed: exit status 3\n"+onlyFor+abortAsk+abortTheRun+"\n")
	assert.Equal(t, append(ran(backupName), redoBackup, "run-backup"), rig.programs())
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestACheckWithoutStatusesOffersItsRemedyForAnyFailure(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.hooks = []pipeline.Hook{check(true)}
	rig.runner.Script(runnertest.Answer{Err: errNotFound}, succeeding(), succeeding())

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), remedyAsk+runTheRemedy+" (--yes)\n"+remedyLine)
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestPretendShowsForWhichStatusesACheckOffersItsRemedy(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true

	one, three := checkFor(), checkFor()
	one.Name, one.RemedyStatus = "one", []int{4}
	three.Name, three.RemedyStatus = "three", []int{1, 2, 3}
	rig.hooks = []pipeline.Hook{checkFor(), one, three}

	pipeline.Pretend(t.Context(), rig.env(), nil)

	assert.Equal(t, "    hook backup (before the run): run-backup; if it fails with exit status 1 or 2, offers to run as "+
		"root: redo-backup run; otherwise asks whether to go on\n"+
		"    hook one (before the run): run-backup; if it fails with exit status 4, offers to run as root: "+
		"redo-backup run; otherwise asks whether to go on\n"+
		"    hook three (before the run): run-backup; if it fails with exit status 1, 2 or 3, offers to run as "+
		"root: redo-backup run; otherwise asks whether to go on\n", rig.out.String())
}

// The notices of a hook that runs only when someone answers the questions,
// and of a check whose remedy does, under --yes.
const (
	skippedUnderYes = "==> hook backup (before the run) skipped: it runs only when someone answers the questions\n"
	remedyAttended  = "==> the remedy of hook backup runs only when someone answers the questions\n"
)

func TestAnInteractiveOnlyHookIsSkippedUnderYes(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	attended := check(true)
	attended.InteractiveOnly = true
	rig.hooks = []pipeline.Hook{attended, hook("notify", pipeline.After, pipeline.RunStep)}

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.True(t, strings.HasPrefix(rig.out.String(), skippedUnderYes+"==> [1/1] sync: do sync\n"), rig.out.String())
	assert.Equal(t, ran("notify"), rig.programs(), "the check does not run")
	assert.True(t, sync.ran, "the run goes on")
	assert.Equal(t, pipeline.RunFinished, report.Result)
	assert.Empty(t, report.FailedHooks, "a hook skipped so has not failed")
}

func TestAnInteractiveOnlyHookRunsWhenSomeoneAnswers(t *testing.T) {
	t.Parallel()

	rig := newRig(t, ui.KeyEnter)

	attended := hook("snapshot", pipeline.Before, syncStep)
	attended.InteractiveOnly = true
	rig.hooks = []pipeline.Hook{attended}

	rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Equal(t, ran("snapshot"), rig.programs())
	assert.NotContains(t, rig.out.String(), "skipped: it runs only")
}

func TestARemedyThatRunsOnlyWhenSomeoneAnswersAbortsUnderYes(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	backup := check(true)
	backup.RemedyInteractiveOnly = true
	rig.hooks = []pipeline.Hook{backup}
	rig.runner.Script(failing())

	sync := rig.step(syncStep, time.Second, pipeline.Success())
	report := rig.run(t.Context(), sync)

	assert.Contains(t, rig.out.String(), backupLine+backupFailed+remedyAttended+abortAsk+abortTheRun+" (--yes)\n")
	assert.Equal(t, ran(backupName), rig.programs(), "the remedy does not run")
	assert.False(t, sync.ran)
	assert.Equal(t, pipeline.RunFailed, report.Result)
	assert.Equal(t, []pipeline.FailedHook{
		{Name: backupName, Place: runPlace, Reason: backupFailure, WentOn: false},
	}, report.FailedHooks)
}

func TestARemedyThatRunsOnlyWhenSomeoneAnswersIsOfferedThen(t *testing.T) {
	t.Parallel()

	rig := newRig(t, 'y', ui.KeyEnter)

	backup := check(true)
	backup.RemedyInteractiveOnly = true
	rig.hooks = []pipeline.Hook{backup}
	rig.runner.Script(failing(), succeeding(), succeeding())

	report := rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), backupFailed+remedyAsk+runTheRemedy+"\n"+remedyLine)
	assert.NotContains(t, rig.out.String(), remedyAttended)
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestAFailureTheRemedyIsNotForSaysOnlyThatUnderYes(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	backup := checkFor()
	backup.RemedyInteractiveOnly = true
	rig.hooks = []pipeline.Hook{backup}
	rig.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 3}})

	rig.run(t.Context(), rig.step(syncStep, time.Second, pipeline.Success()))

	assert.Contains(t, rig.out.String(), onlyFor+abortAsk)
	assert.NotContains(t, rig.out.String(), remedyAttended)
}

func TestPretendShowsWhatAHookWouldDoUnderYes(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true

	attended := hook("snapshot", pipeline.Before, pipeline.RunStep)
	attended.InteractiveOnly = true

	backup := check(true)
	backup.RemedyInteractiveOnly = true

	statuses := checkFor()
	statuses.Name, statuses.RemedyInteractiveOnly = "statuses", true

	both := check(true)
	both.Name, both.InteractiveOnly, both.RemedyInteractiveOnly = "both", true, true

	rig.hooks = []pipeline.Hook{attended, backup, statuses, both}

	pipeline.Pretend(t.Context(), rig.env(), nil)

	assert.Equal(t, "    hook snapshot (before the run): run-snapshot; skipped under --yes\n"+
		"    hook backup (before the run): run-backup; if it fails, offers to run as root: redo-backup run; "+
		"under --yes, offers no remedy and aborts\n"+
		"    hook statuses (before the run): run-backup; if it fails with exit status 1 or 2, offers to run as "+
		"root: redo-backup run; otherwise asks whether to go on; under --yes, offers no remedy and aborts\n"+
		"    hook both (before the run): run-backup; if it fails, offers to run as root: redo-backup run; "+
		"skipped under --yes\n", rig.out.String())
}
