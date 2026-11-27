// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stepHeadings returns the headings of the steps of a run with the image's
// configuration, in order.
func stepHeadings() []string {
	return []string{
		"==> [1/6] sync: sync the repositories\n",
		"==> [2/6] update: update @emergup-test\n",
		"==> [3/6] live-rebuild: rebuild the live packages\n",
		"==> [4/6] preserved-rebuild: rebuild the packages that use preserved libraries\n",
		"==> [5/6] config-files: update the configuration files\n",
		"==> [6/6] depclean: remove the packages no longer needed\n",
	}
}

// stepNames returns the names of the steps, as the configuration and the
// summary give them.
func stepNames() []string {
	return []string{"sync", "update", "live-rebuild", "preserved-rebuild", "config-files", "depclean"}
}

// UPD-1: one command runs the whole update, step after step. The news get
// a step of their own with the news features.
func TestUPD1_OneCommandRunsTheWholeUpdate(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	// A package that nothing needs, for the depclean.
	ctr.publishTree("snapshot-held")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "--oneshot", "emergup-test/held")
	// An upgrade, for the sync to bring and the update to install. The
	// sync of the set-up is recent, which would skip emergup's.
	ctr.publishTree("snapshot-updates")

	stdout := ctr.emergupRun("", "--yes", "--force-sync")
	assertInOrder(t, stdout, append(stepHeadings(), "==> Summary\n", "==> The run finished in ")...)

	for _, row := range []string{
		`sync +succeeded +` + took,
		`update +succeeded +` + took,
		`live-rebuild +skipped +` + took + `  no live package installed`,
		`preserved-rebuild +skipped +` + took + `  no preserved libraries`,
		`config-files +skipped +` + took + `  no configuration file to update`,
		`depclean +succeeded +` + took,
	} {
		assert.Regexp(t, summaryRow(row), stdout)
	}

	assert.Equal(t, "1.1", ctr.version("upgrade"), "the upgrade is synced and installed")
	assert.False(t, ctr.exists("/usr/share/emergup-test/held"), "the package nothing needs is removed")
}

// UPD-2: each step is announced, and emergup asks before running it; with
// `--yes` it takes the usual answer to every question and says so.
func TestUPD2_EachStepIsAnnouncedThenConfirmed(t *testing.T) {
	t.Parallel()

	const (
		syncHeading    = "==> [1/2] sync: sync the repositories\n"
		updateHeading  = "==> [2/2] update: update @emergup-test\n"
		runQuestion    = "==> Run this step? [Y/s/a/?] "
		updateQuestion = "==> Update these packages? [Y/s/a/?] "
	)

	ctr := start(t)
	ctr.dropIns("sync-and-update")
	ctr.publishTree("snapshot-updates")

	asked := ctr.emergupRun("y\ns\n")
	assertInOrder(t, asked,
		syncHeading, runQuestion+"run the step\n", ">>> Syncing repository 'emergup-test'", "==> sync succeeded in ",
		updateHeading, "] emergup-test/upgrade-1.1::emergup-test [1.0::emergup-test]",
		updateQuestion+"skip the update\n", "==> update skipped: you chose not to update\n")
	assert.Equal(t, "1.0", ctr.version("upgrade"), "the update is skipped")

	// Without input, a question that waited for an answer would stop the
	// run.
	yes := ctr.emergupRun("", "--yes")
	assertInOrder(t, yes,
		syncHeading, runQuestion+"run the step (--yes)\n",
		updateHeading, updateQuestion+"update them (--yes)\n", "==> update succeeded in ")
	assert.Equal(t, "1.1", ctr.version("upgrade"), "the update runs")
}

// UPD-3: every step can be disabled in the configuration.
func TestUPD3_EveryStepCanBeDisabled(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("all-disabled")

	stdout := ctr.emergupRun("")
	assert.NotContains(t, stdout, firstHeading, "no step runs")
	assert.Contains(t, stdout, "\n==> The run finished in ")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)

	for _, step := range stepNames() {
		assert.Regexp(t, summaryRow(step+` +disabled`), stdout)
		assert.Contains(t, pretend.stdout, "==> "+step+" is disabled in the configuration\n")
	}
}

// UPD-3: the command and options of every step can be changed in the
// configuration.
func TestUPD3_TheCommandOfEachStepCanBeChanged(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	// A live package, rebuilt on every run, so that the live-package
	// rebuild has a command to run.
	ctr.publishTree("snapshot-live")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/live")
	ctr.dropIns("live-always")
	ctr.dropIns("commands")

	pretend := ctr.emergup("run", "--pretend", "--force-sync")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)

	for _, line := range []string{
		"    emerge --sync --quiet --color=n emergup-test\n",
		"    emerge --quiet-build --update --newuse --pretend --color=n @emergup-test\n",
		"    then, once confirmed: emerge --quiet-build --update --newuse --color=n @emergup-test\n",
		"    emerge --oneshot --quiet-build --color=n --usepkg-exclude-live=y =emergup-test/live-9999\n",
		" run: emerge --quiet-build @preserved-rebuild --color=n\n",
		" then run: etc-update --automode -5\n",
		"    emerge --depclean --with-bdeps=y --pretend --color=n\n",
	} {
		assert.Contains(t, pretend.stdout, line)
	}

	ctr.publishTree("snapshot-updates")

	log := ctr.runLog(tester, ctr.emergupRun("", "--yes", "--force-sync"))
	for _, command := range []string{
		"sudo emerge --sync --quiet --color=n emergup-test",
		"emerge --quiet-build --update --newuse --pretend --color=n @emergup-test",
		"sudo emerge --quiet-build --update --newuse --color=n @emergup-test",
		"sudo emerge --oneshot --quiet-build --color=n --usepkg-exclude-live=y =emergup-test/live-9999",
		"emerge --depclean --with-bdeps=y --pretend --color=n",
	} {
		assert.Regexp(t, `\n==> \S+ `+regexp.QuoteMeta(command)+`\n`, log, "the command that ran")
	}

	assert.Equal(t, "1.1", ctr.version("upgrade"), "the update runs")
}

// UPD-6: `--pretend` shows the steps and the commands that would run,
// without running them.
func TestUPD6_PretendShowsTheStepsWithoutRunningThem(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	// A snapshot that a sync would bring.
	ctr.publish("pretend")

	got := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)
	assertInOrder(t, got.stdout, stepHeadings()...)
	assert.Contains(t, got.stdout, "\n    emerge --sync --color=n emergup-test\n")

	assert.False(t, ctr.exists(repository+"/pretend"), "the repository is not synced")
}
