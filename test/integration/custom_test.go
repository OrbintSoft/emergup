// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The files that the custom steps of the custom-steps drop-in create.
const (
	customRootFile  = "/var/tmp/emergup-test-custom-root"
	customPlainFile = "/var/tmp/emergup-test-custom-plain"
)

// The custom step of the custom-step-missing drop-in, whose program is not
// installed.
const (
	missingHeading = "==> [1/1] missing: run emergup-test-not-installed\n"
	notInstalled   = "emergup-test-not-installed is not installed"
)

// HOOK-3: custom steps run after their built-in step, those after the same
// step in the order of their names, each announced, asked about and listed
// in the summary like a built-in step, with hooks of their own; one runs as
// root only when it is configured to (PRIV-3). The pretend shows them
// without running them.
func TestHOOK3_CustomStepsRunAfterTheirBuiltInStep(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("custom-steps")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assertInOrder(t, pretend.stdout,
		"==> [1/3] sync: sync the repositories\n",
		"==> [2/3] a-elevated: run touch\n    would run as root: touch "+customRootFile+"\n",
		"==> [3/3] b-plain: touch a file as the invoking user\n",
		"    "+hookLine("before-plain", "before b-plain"), "    would run: touch "+customPlainFile+"\n",
		"    "+hookLine("after-plain", "after b-plain"))
	assert.False(t, ctr.exists(customRootFile), "the pretend runs no custom step")
	assert.False(t, ctr.exists(customPlainFile), "the pretend runs no custom step")
	assert.Empty(t, ctr.hooksRan(), "the pretend runs no hook")

	// An answer for each of the three questions.
	stdout := ctr.emergupRun("\n\n\n")
	assertInOrder(t, stdout,
		"==> sync succeeded in ",
		"==> [2/3] a-elevated: run touch\n==> Run this step?", "==> a-elevated succeeded in ",
		"==> [3/3] b-plain: touch a file as the invoking user\n",
		"==> "+hookLine("before-plain", "before b-plain"), "==> Run this step?", "==> b-plain succeeded in ",
		"==> "+hookLine("after-plain", "after b-plain"), "==> Summary\n", "==> The run finished in ")
	assert.Regexp(t, summaryRow(`a-elevated +succeeded +\S+`), stdout)
	assert.Regexp(t, summaryRow(`b-plain +succeeded +\S+`), stdout)

	assert.Equal(t, root, ctr.owner(customRootFile), "the custom step configured to be elevated runs as root")
	assert.Equal(t, tester, ctr.owner(customPlainFile), "a custom step runs as the invoking user")

	ran := ctr.hooksRan()
	assert.Equal(t, []string{"before-plain before b-plain", "after-plain after b-plain"}, places(ran))
	require.Len(t, ran, 2)
	assert.Contains(t, ran[1], " status=succeeded ")
}

// UPD-4: an optional custom step whose program is not installed is skipped
// with a notice, without its question; one that is not optional fails
// instead. A custom step runs after its built-in step even when the
// built-in step is disabled.
func TestUPD4_AnOptionalCustomStepWithoutItsProgramIsSkipped(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("all-disabled")
	ctr.dropIns("custom-step-missing")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, missingHeading+"    "+notInstalled+", so the step would be skipped\n")

	// No input: a question would end the run.
	skipped := ctr.emergupRun("")
	assert.Contains(t, skipped, missingHeading+"==> missing skipped: "+notInstalled+"\n")
	assert.NotContains(t, skipped, "Run this step?")
	assert.Regexp(t, summaryRow(`missing +skipped .*`+notInstalled), skipped)

	ctr.dropIns("custom-step-required")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout, missingHeading+"==> Run this step? [Y/s/a/?] run the step (--yes)\n")
	assert.Contains(t, failed.stdout, "==> missing failed after ")
	assert.Contains(t, failed.stdout, `exec: "emergup-test-not-installed": executable file not found in $PATH`)
	assert.Contains(t, failed.stdout, "==> The run failed after ")
}
