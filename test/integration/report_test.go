// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The package that snapshot-broken brings, whose compile phase dies, and
// the names of its files in a failure report.
const (
	brokenFiles = "emergup-test_upgrade-1.2"
	brokenLog   = "/var/tmp/portage/emergup-test/upgrade-1.2/temp/build.log"
)

// reportAtTheEnd matches the end of a run that left a failure report: the
// line that says where it is, then the one that says where the log is.
var reportAtTheEnd = regexp.MustCompile(`\n==> The failure report is in (\S+)\n==> The log is in \S+\n$`)

// withBrokenUpdate leaves only the update enabled and brings the version
// of emergup-test/upgrade that does not build.
func withBrokenUpdate(ctr *container) {
	ctr.t.Helper()

	ctr.dropIns("update-only")
	ctr.publishTree("snapshot-broken")
	ctr.syncRepository()
}

// readAsTester returns the file at path, read as tester.
func (c *container) readAsTester(file string) string {
	c.t.Helper()

	read := c.exec(tester, "", "cat", file)
	require.Equal(c.t, 0, read.code, "%s is not read: %s", file, read.stderr)

	return read.stdout
}

// DIAG-1, DIAG-2: a step that fails leaves a failure report in the run
// directory, with the build log of the package that failed, copied as
// root and readable by the user, emerge --info and emerge -pqv for it;
// the end of the run says where it is.
func TestDIAG1_AFailedStepLeavesAFailureReport(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)

	got := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	found := reportAtTheEnd.FindStringSubmatch(got.stdout)
	require.NotNil(t, found, "the end of the run says where the report is, before the log:\n%s", got.stdout)

	dir := found[1]
	assert.Equal(t, path.Join(path.Dir(logPath(t, got.stdout)), "failure"), dir, "in the run directory")
	assert.NotContains(t, got.stdout, "not in the failure report", "everything is collected")

	listed := ctr.exec(tester, "", "ls", "-1", dir)
	require.Equal(t, 0, listed.code, listed.stderr)
	assert.ElementsMatch(t, []string{
		"summary.txt", "output.txt",
		brokenFiles + ".build.log", brokenFiles + ".emerge-info.txt", brokenFiles + ".emerge-pqv.txt",
	}, strings.Fields(listed.stdout))

	summary := ctr.readAsTester(dir + "/summary.txt")
	for _, line := range []string{
		"Step:     update (update @emergup-test)\n",
		"Failure:  emerge exited with status 1\n",
		"Command:  emerge --verbose --update --deep --newuse --verbose-conflicts --color=n @emergup-test, as root\n",
		"  emergup-test/upgrade-1.2::emergup-test, in its compile phase\n",
		"    build log      " + brokenLog + ", copied to " + brokenFiles + ".build.log\n",
	} {
		assert.Contains(t, summary, line)
	}

	assert.NotContains(t, summary, "Not collected")

	assert.Contains(t, ctr.readAsTester(dir+"/"+brokenFiles+".build.log"), "this version does not build",
		"the build log, which only Portage's user can read, copied for the user")
	assert.Contains(t, ctr.readAsTester(dir+"/"+brokenFiles+".emerge-info.txt"), "Portage 3.")
	assert.Contains(t, ctr.readAsTester(dir+"/"+brokenFiles+".emerge-pqv.txt"), "emergup-test/upgrade-1.2")
	assert.Contains(t, ctr.readAsTester(dir+"/output.txt"),
		">>> Failed to emerge emergup-test/upgrade-1.2, Log file:\n")

	owner := ctr.exec(root, "", "stat", "--format", "%U %a", dir, dir+"/"+brokenFiles+".build.log")
	require.Equal(t, 0, owner.code, owner.stderr)
	assert.Equal(t, "tester 700\ntester 600\n", owner.stdout, "the user's own, readable by nobody else")

	assert.Regexp(t, ranCommand("sudo cat -- "+brokenLog), ctr.runLog(tester, got.stdout),
		"the log is read through the elevation")
}

// DIAG-1: a run in which no step fails leaves no failure report, and
// neither does a hook that fails.
func TestDIAG1_OnlyAFailedStepLeavesAFailureReport(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.publish("new")

	finished := ctr.emergupRun("", "--yes")
	assert.NotContains(t, finished, "failure report")
	assert.False(t, ctr.exists(path.Join(path.Dir(logPath(t, finished)), "failure")))

	ctr.dropIns("hooks")
	ctr.dropIns("hook-fails")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assert.Contains(t, failed.stdout, "==> The hook snapshot (before sync) failed: exit status 3.\n")
	assert.NotContains(t, failed.stdout, "failure report")
	assert.False(t, ctr.exists(path.Join(path.Dir(logPath(t, failed.stdout)), "failure")))
}
