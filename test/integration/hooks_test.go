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

// hookRecord is the file the hook of the tests appends its lines to.
const hookRecord = "/tmp/emergup-test-hooks"

// The hooks of the hooks drop-in, by their name and place, as places
// returns them.
const (
	ranFirst    = "first before run"
	ranSnapshot = "snapshot before sync"
	ranSync1    = "sync-1 after sync"
	ranSync2    = "sync-2 after sync"
	ranLast     = "last after run"
)

// installHook installs the hook of the tests as
// /usr/local/bin/emergup-test-hook, which the hooks drop-ins run.
func (c *container) installHook() {
	c.t.Helper()

	const target = "/usr/local/bin/emergup-test-hook"

	c.install("testdata/hook-script/emergup-test-hook.sh", target)
	c.root("chmod", "0755", target)
}

// hooksRan returns the lines the hook of the tests has recorded, one for
// each time it ran, in order.
func (c *container) hooksRan() []string {
	c.t.Helper()

	if !c.exists(hookRecord) {
		return nil
	}

	got := c.exec(root, "", "cat", hookRecord)
	require.Equal(c.t, 0, got.code, "%s cannot be read: %s", hookRecord, got.stderr)

	return strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
}

// places returns the name and the place of each line of the hook record
// lines: "first before run".
func places(lines []string) []string {
	got := make([]string, 0, len(lines))

	for _, line := range lines {
		fields := strings.Fields(line)
		got = append(got, strings.Join(fields[:min(3, len(fields))], " "))
	}

	return got
}

// hookLine runs the hook of the tests at the place of a hooks drop-in, as
// the pretend and a run show it.
func hookLine(name, place string) string {
	return "hook " + name + " (" + place + "): /usr/local/bin/emergup-test-hook\n"
}

// HOOK-1: hooks run before and after the whole run and before and after
// any step, those at the same place in the order of their names, and the
// pretend shows them without running them. A hook runs as the invoking
// user (PRIV-3).
func TestHOOK1_HooksRunAroundTheRunAndEachStep(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("hooks")
	ctr.publish("new")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assertInOrder(t, pretend.stdout,
		"    "+hookLine("first", "before the run"), "==> [1/1] sync: sync the repositories\n",
		"    "+hookLine("snapshot", "before sync"), "    emerge --sync",
		"    "+hookLine("sync-1", "after sync"), "    "+hookLine("sync-2", "after sync"),
		"    "+hookLine("last", "after the run"))
	assert.NotContains(t, pretend.stdout, "hook update", "the hooks of a disabled step do not run")
	assert.Empty(t, ctr.hooksRan(), "the pretend runs no hook")

	stdout := ctr.emergupRun("", "--yes")
	assertInOrder(t, stdout,
		"==> "+hookLine("first", "before the run"), "==> [1/1] sync: sync the repositories\n",
		"==> "+hookLine("snapshot", "before sync"), "==> Run this step?", "==> sync succeeded in ",
		"==> "+hookLine("sync-1", "after sync"), "==> "+hookLine("sync-2", "after sync"),
		"==> "+hookLine("last", "after the run"), "==> Summary\n", "==> The run finished in ")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")

	ran := ctr.hooksRan()
	assert.Equal(t, []string{
		ranFirst, ranSnapshot, ranSync1, ranSync2, ranLast,
	}, places(ran))

	for _, line := range ran {
		assert.Contains(t, line, " user="+tester+" ", "a hook runs as the invoking user")
	}
}

// HOOK-1: a hook that fails stops the run as a failed step does, the
// hooks after the run still running, unless it is configured to go on.
func TestHOOK1_AHookThatFailsStopsTheRun(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("hooks")
	ctr.dropIns("hook-fails")
	ctr.publish("new")

	failed := ctr.emergup("run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s%s", failed.stdout, failed.stderr)
	assertInOrder(t, failed.stdout,
		"==> hook snapshot (before sync): /usr/local/bin/emergup-test-hook exit:3\n"+
			"==> hook snapshot (before sync) failed: exit status 3\n",
		"==> "+hookLine("last", "after the run"), "==> Summary\n")
	assert.Regexp(t, summaryRow(`sync +not run`), failed.stdout)
	assert.Contains(t, failed.stdout, "==> The hook snapshot (before sync) failed: exit status 3.\n"+
		"==> The run failed after ")
	assert.NotContains(t, failed.stdout, "Run this step?", "the step is not asked about")
	assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")

	ran := ctr.hooksRan()
	assert.Equal(t, []string{ranFirst, ranSnapshot, ranLast}, places(ran))
	assert.Contains(t, ran[len(ran)-1], " result=failed ")

	ctr.dropIns("hook-goes-on")

	wentOn := ctr.emergupRun("", "--yes")
	assertInOrder(t, wentOn,
		"==> hook snapshot (before sync) failed: exit status 3; the run goes on\n", "==> sync succeeded in ",
		"==> The hook snapshot (before sync) failed: exit status 3; the run went on.\n", "==> The run finished in ")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// HOOK-1: a configuration file can turn off a hook another one adds.
func TestHOOK1_ADropInTurnsOffAHook(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("hooks")
	ctr.dropIns("hook-off")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.NotContains(t, pretend.stdout, "hook sync-1")
	assert.Contains(t, pretend.stdout, hookLine("sync-2", "after sync"))

	ctr.emergupRun("", "--yes")
	assert.Equal(t, []string{ranFirst, ranSnapshot, ranSync2, ranLast},
		places(ctr.hooksRan()))
}

// HOOK-4: hooks receive the run context in environment variables: the
// hook, the step and whether before or after it, the run directory and its
// log, and how the step or the run ended.
func TestHOOK4_HooksReceiveTheRunContext(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("hooks")

	log := logPath(t, ctr.emergupRun("", "--yes"))
	context := " dir=" + path.Dir(log) + " log=" + log

	assert.Equal(t, []string{
		"first before run status= result= user=" + tester + context,
		"snapshot before sync status= result= user=" + tester + context,
		"sync-1 after sync status=succeeded result= user=" + tester + context,
		"sync-2 after sync status=succeeded result= user=" + tester + context,
		"last after run status= result=finished user=" + tester + context,
	}, ctr.hooksRan())

	// A step skipped at its question.
	ctr.root("rm", hookRecord)
	ctr.emergupRun("s\n")

	ran := ctr.hooksRan()
	require.Len(t, ran, 5)
	assert.True(t, strings.HasPrefix(ran[2], "sync-1 after sync status=skipped "), ran[2])
}
