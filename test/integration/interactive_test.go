// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// checkPretend is the line --pretend writes for the check of the check
// drop-in, before what it would do under --yes.
const checkPretend = "    hook backup (before the run): /usr/local/bin/emergup-test-hook need:" + backupFile +
	"; if it fails, offers to run as root: touch " + backupFile

// remedyThenStep answers yes to the question of the failed check, which
// runs its remedy, then to the question of the step.
const remedyThenStep = "y\n" + "y\n"

// HOOK-5: a hook that runs only when someone answers the questions is
// skipped under --yes, with a notice, and the run goes on; when someone
// answers, it runs.
func TestHOOK5_AnInteractiveOnlyHookIsSkippedUnderYes(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.dropIns("check-interactive-only")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, checkPretend+"; skipped under --yes\n")

	skipped := ctr.emergupRun("", "--yes")
	assertInOrder(t, skipped,
		"==> hook backup (before the run) skipped: it runs only when someone answers the questions\n",
		"==> [1/1] sync: sync the repositories\n", "==> The run finished in ")
	assert.NotContains(t, skipped, checkLine, "the check does not run")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
	assert.Equal(t, []string{ranLast}, places(ctr.hooksRan()))

	answered := ctr.emergupRun(remedyThenStep)
	assert.Contains(t, answered, checkLine+checkFailed+remedyAsk+"run the remedy and check again\n"+remedyRunLine)
	assert.True(t, ctr.exists(backupFile), "the remedy runs")
}

// HOOK-5: a check whose remedy runs only when someone answers the
// questions still runs under --yes, but its failure aborts the run rather
// than start the remedy; when someone answers, the remedy is offered.
func TestHOOK5_ARemedyThatRunsOnlyWhenSomeoneAnswersAbortsUnderYes(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.dropIns("check-remedy-attended")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, checkPretend+"; under --yes, offers no remedy and aborts\n")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout, checkLine+checkFailed+
		"==> the remedy of hook backup runs only when someone answers the questions\n"+
		"==> Abort the run? [A/g/?] abort the run (--yes)\n")
	assert.False(t, ctr.exists(backupFile), "the remedy does not run")
	assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")
	assert.Equal(t, []string{ranCheck, ranLast}, places(ctr.hooksRan()), "the hooks after the run still run")

	answered := ctr.emergupRun(remedyThenStep)
	assert.Contains(t, answered, checkFailed+remedyAsk+"run the remedy and check again\n"+remedyRunLine+checkLine)
	assert.True(t, ctr.exists(backupFile), "the remedy runs")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}
