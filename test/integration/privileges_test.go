// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The sudo of the image, and where a test puts it to take it off the PATH.
const (
	sudo       = "/usr/bin/sudo"
	hiddenSudo = "/root/sudo"
)

// firstHeading starts the heading of a run's first step.
const firstHeading = "==> [1/"

// withoutSudo takes sudo off the PATH: the image has no other elevation
// tool.
func withoutSudo(ctr *container) {
	ctr.t.Helper()

	ctr.root("mv", sudo, hiddenSudo)
}

// PRIV-2: the elevation method is detected automatically or set in the
// configuration; without one, a run stops before its first step.
func TestPRIV2_TheElevationMethodIsDetectedOrConfigured(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.publish("elevated")

	synced := ctr.emergupRun("", "--yes")
	assert.Regexp(t, `\n==> \S+ sudo emerge --sync --color=n emergup-test\n`,
		ctr.runLog(tester, synced), "the sync runs through sudo")
	assert.True(t, ctr.exists(repository+"/elevated"), "the repository is synced")

	withoutSudo(ctr)

	none := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, none.code, "the exit status, with the output:\n%s%s", none.stdout, none.stderr)
	assert.Contains(t, none.stdout,
		"==> no elevation tool found: install sudo, doas or run0, or run emergup as root\n")
	assert.NotContains(t, none.stdout, firstHeading, "no step starts")

	ctr.root("mv", hiddenSudo, sudo)
	ctr.dropIns("doas")

	configured := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, configured.code, "the exit status, with the output:\n%s%s", configured.stdout, configured.stderr)
	assert.Contains(t, configured.stdout, "==> elevation method doas is not installed\n", "sudo is not taken instead")
	assert.NotContains(t, configured.stdout, firstHeading, "no step starts")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout,
		"==> elevation method doas is not installed; a run would stop before its first step\n")
	assert.Contains(t, pretend.stdout, "==> [1/1] sync: sync the repositories\n")
}

// PRIV-4: started as root, emergup runs the commands directly and says so
// once, at the start.
func TestPRIV4_AsRootTheCommandsRunDirectly(t *testing.T) {
	t.Parallel()

	const notice = "==> Running as root: the commands run without sudo, doas or run0, " +
		"and AI assistance is off.\n"

	ctr := start(t)
	withoutSudo(ctr)
	ctr.dropIns("sync-only")
	ctr.publish("as-root")

	run := ctr.emergupAs(root, "", "run", "--yes")
	assert.Equal(t, 0, run.code, "the exit status, with the output:\n%s%s", run.stdout, run.stderr)
	assert.True(t, strings.HasPrefix(run.stdout, notice+firstHeading), "the notice comes first:\n%s", run.stdout)
	assert.Equal(t, 1, strings.Count(run.stdout, notice), "the notice is given once")
	assert.Regexp(t, `\n==> \S+ emerge --sync --color=n emergup-test\n`,
		ctr.runLog(root, run.stdout), "the sync runs as it is")
	assert.True(t, ctr.exists(repository+"/as-root"), "the repository is synced")

	pretend := ctr.emergupAs(root, "", "run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.True(t, strings.HasPrefix(pretend.stdout, notice+firstHeading), "the notice comes first:\n%s", pretend.stdout)
	assert.Equal(t, 1, strings.Count(pretend.stdout, notice), "the notice is given once")
}
