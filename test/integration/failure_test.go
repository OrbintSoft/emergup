// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The question after a failed step and the update's own, as a run without
// a terminal writes them, and the summary row of the update that fails.
const (
	againPrompt  = "==> Run this step again? [r/s/a/?] "
	updatePrompt = "==> Update these packages? [Y/s/a/?] "
	failedRow    = `update +failed +` + took + `  emerge exited with status 1`
)

// withHooksAroundTheUpdate installs the hook of the tests before and after
// the update and after the run.
func withHooksAroundTheUpdate(ctr *container) {
	ctr.t.Helper()

	ctr.installHook()
	ctr.dropIns("failure-hooks")
}

// UPD-17, DIAG-1, DIAG-2: after the update fails, running it again runs it
// again, asking its own question again but without running the hook
// before it again; a second failure leaves a second report; ending the run
// then fails it, and the hooks after the update and after the run each
// run once, told that it failed.
func TestUPD17_AFailedStepCanRunAgain(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withHooksAroundTheUpdate(ctr)

	got := ctr.emergupInput("y\nr\ny\na\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assert.Equal(t, 2, strings.Count(got.stdout, updatePrompt), "the update asks again:\n%s", got.stdout)
	assert.Equal(t, 2, strings.Count(got.stdout, "\n==> update failed after "))
	assertInOrder(t, got.stdout, againPrompt+"run it again\n", againPrompt+"end the run\n")
	assert.Regexp(t, summaryRow(failedRow), got.stdout)

	run := path.Dir(logPath(t, got.stdout))
	assert.Contains(t, got.stdout, "\n==> The failure report is in "+run+"/failure\n"+
		"==> The failure report is in "+run+"/failure-2\n"+
		"==> The log is in ", "a line for each report, in order, before the log")

	for _, dir := range []string{"failure", "failure-2"} {
		assert.Contains(t, ctr.readAsTester(run+"/"+dir+"/summary.txt"), "Step:     update (update @emergup-test)\n",
			"the report in %s", dir)
	}

	ran := ctr.hooksRan()
	require.Equal(t, []string{"snapshot before update", "cleanup after update", "last after run"}, places(ran),
		"the hook before the update runs once, those after once, at the end")
	assert.Contains(t, ran[1], " status=failed ")
	assert.Contains(t, ran[2], " result=failed ")
}

// UPD-17: a failed update passed over leaves the run going on with the
// next step, and the run fails.
func TestUPD17_AFailedStepCanBePassedOver(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("update-and-preserved")
	ctr.publishTree("snapshot-broken")
	ctr.syncRepository()

	got := ctr.emergupInput("y\ns\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assert.Contains(t, got.stdout, againPrompt+"go on with the next step\n")
	assert.Regexp(t, summaryRow(failedRow), got.stdout)
	assert.Regexp(t, summaryRow(`preserved-rebuild +skipped +`+took+`  no preserved libraries`), got.stdout,
		"the next step ran")
	assert.Contains(t, got.stdout, "\n==> The run failed after ")
	assert.Equal(t, 1, strings.Count(got.stdout, "==> The failure report is in "))
}

// UPD-17: Enter is not an answer to the question after a failed step, and
// the end of the input there stops the run, no hook starting after it.
func TestUPD17_TheEndOfTheInputAfterAFailedStepStopsTheRun(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withHooksAroundTheUpdate(ctr)

	got := ctr.emergupInput("y\n\n", "run")
	assert.Equal(t, 130, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assert.Contains(t, got.stdout, againPrompt+"\n"+
		"==> Not an answer: press one of the keys in brackets, or ? for the list.\n"+
		againPrompt+"\n")
	assert.Contains(t, got.stdout, "\n==> The run was stopped after ")
	assert.Equal(t, []string{"snapshot before update"}, places(ctr.hooksRan()))
}
