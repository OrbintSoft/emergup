// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The lines of the check of the check drop-in.
const (
	backupFile    = "/var/tmp/emergup-test-backup"
	checkLine     = "==> hook backup (before the run): /usr/local/bin/emergup-test-hook need:" + backupFile + "\n"
	checkFailed   = "==> hook backup (before the run) failed: exit status 1\n"
	remedyAsk     = "==> Run the remedy and check again? [Y/g/a/?] "
	remedyRunLine = "==> remedy of hook backup (before the run), as root: touch " + backupFile + "\n"
	ranCheck      = "backup before run"
)

// owner returns the user that owns the file at path in the container.
func (c *container) owner(path string) string {
	c.t.Helper()

	got := c.exec(root, "", "stat", "--format=%U", path)
	require.Equal(c.t, 0, got.code, "%s: %s", path, got.stderr)

	return strings.TrimSpace(got.stdout)
}

// HOOK-2: a check that fails asks whether to run its remedy and check
// again; --yes runs it. The remedy runs as root, configured so (PRIV-3).
func TestHOOK2_AFailedCheckRunsItsRemedyAndChecksAgain(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, "    hook backup (before the run): /usr/local/bin/emergup-test-hook need:"+
		backupFile+"; if it fails, offers to run as root: touch "+backupFile+"\n")

	stdout := ctr.emergupRun("", "--yes")
	assertInOrder(t, stdout,
		checkLine+checkFailed+remedyAsk+"run the remedy and check again (--yes)\n"+remedyRunLine+checkLine,
		"==> [1/1] sync: sync the repositories\n", "==> The run finished in ")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
	assert.Equal(t, root, ctr.owner(backupFile), "the remedy runs as root")
	assert.Equal(t, []string{ranCheck, ranCheck, ranLast}, places(ctr.hooksRan()))

	again := ctr.emergupRun("", "--yes")
	assert.NotContains(t, again, remedyAsk, "a check that passes asks nothing")
}

// HOOK-2: with --yes the remedy runs once: a check that fails again after
// it fails the run, the hooks after the run still running.
func TestHOOK2_WithYesTheRemedyRunsOnce(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.dropIns("check-useless-remedy")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout,
		"==> hook backup (before the run) failed: exit status 1, after its remedy\n")
	assert.Regexp(t, summaryRow(`sync +not run`), failed.stdout)
	assert.Contains(t, failed.stdout, "==> The hook backup (before the run) failed: exit status 1, after its remedy.\n")

	ran := ctr.hooksRan()
	assert.Equal(t, []string{ranCheck, ranCheck, ranLast}, places(ran))
	assert.Contains(t, ran[len(ran)-1], " result=failed ")
}

// HOOK-2: at the question of a failed check the user can go on without it,
// or abort the run, which fails it.
func TestHOOK2_AFailedCheckCanBeOverriddenOrAbort(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.publish("new")

	aborted := ctr.emergupInput("a\n", "run")
	assert.Equal(t, 1, aborted.code, "the exit status, with the output:\n%s%s", aborted.stdout, aborted.stderr)
	assert.Contains(t, aborted.stdout, checkFailed+remedyAsk+"abort the run\n")
	assert.Contains(t, aborted.stdout, "==> The run failed after ")
	assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")
	assert.False(t, ctr.exists(backupFile), "the remedy does not run")
	assert.Contains(t, ctr.hooksRan()[1], "last after run status= result=failed ", "the hook after the run runs")

	wentOn := ctr.emergupRun("g\ny\n")
	assert.Contains(t, wentOn, checkFailed+remedyAsk+"go on without it\n")
	assert.Contains(t, wentOn, "==> The hook backup (before the run) failed: exit status 1; the run went on.\n")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
	assert.False(t, ctr.exists(backupFile), "the remedy does not run")
}

// HOOK-2: without a remedy, a failed check offers only to go on or to
// abort, and --yes aborts.
func TestHOOK2_ACheckWithoutARemedyAbortsWithYes(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.dropIns("check-no-remedy")

	pretend := ctr.emergup("run", "--pretend")
	assert.Contains(t, pretend.stdout, "; if it fails, asks whether to go on\n")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout, checkFailed+"==> Abort the run? [A/g/?] abort the run (--yes)\n")
	assert.Regexp(t, summaryRow(`sync +not run`), failed.stdout)
}

// HOOK-2: a check that lists the exit statuses its remedy is for offers it
// only for those: failing with another, as when the backup disk is
// missing, it says so, and --yes aborts without running the remedy.
func TestHOOK2_ACheckOffersItsRemedyOnlyForItsStatuses(t *testing.T) {
	t.Parallel()

	const (
		disk  = "/var/tmp/emergup-test-disk"
		check = "hook backup (before the run): /usr/local/bin/emergup-test-hook mounted:" + disk +
			" need:" + backupFile
	)

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("check")
	ctr.dropIns("check-remedy-status")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, "    "+check+"; if it fails with exit status 1, offers to run as root: touch "+
		backupFile+"; otherwise asks whether to go on\n")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout, "==> "+check+"\n==> hook backup (before the run) failed: exit status 2\n"+
		"==> the remedy of hook backup is only for exit status 1\n==> Abort the run? [A/g/?] abort the run (--yes)\n")
	assert.False(t, ctr.exists(backupFile), "the remedy does not run")
	assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")

	ctr.root("touch", disk)

	remedied := ctr.emergupRun("", "--yes")
	assertInOrder(t, remedied, "==> "+check+"\n"+checkFailed+remedyAsk+"run the remedy and check again (--yes)\n"+
		remedyRunLine+"==> "+check+"\n", "==> [1/1] sync: sync the repositories\n")
	assert.True(t, ctr.exists(backupFile), "the remedy runs")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// PRIV-3: a hook runs as the invoking user, unless it is configured to be
// elevated.
func TestPRIV3_AHookRunsAsRootOnlyWhenConfiguredTo(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.dropIns("elevated-hook")

	pretend := ctr.emergup("run", "--pretend")
	assert.Contains(t, pretend.stdout,
		"    hook elevated (before the run), as root: touch /var/tmp/emergup-test-elevated\n"+
			"    hook plain (before the run): touch /var/tmp/emergup-test-plain\n")

	ctr.emergupRun("", "--yes")
	assert.Equal(t, root, ctr.owner("/var/tmp/emergup-test-elevated"))
	assert.Equal(t, tester, ctr.owner("/var/tmp/emergup-test-plain"))
}
