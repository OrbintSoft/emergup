// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escape starts the terminal's escape sequences, colours among them.
const escape = "\x1b"

// UPD-13: on a terminal emergup's lines have the colour of their kind and
// emerge keeps its own colours, while the log has none; NO_COLOR and
// TERM=dumb turn every colour off.
func TestUPD13_OnATerminalTheOutputIsColoured(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("update-and-preserved")
	ctr.publishTree("snapshot-updates")
	ctr.syncRepository()

	coloured := ctr.emergupTerminal(nil, "run", "--yes")
	require.Equal(t, 0, coloured.code, "the exit status, with the output:\n%s", coloured.stdout)

	for kind, line := range map[string]string{
		"heading": "\x1b[1;34m==> [1/2] update: update @emergup-test\x1b[0m\r\n",
		"prompt":  "\x1b[1;36m==> Update these packages? [Y/s/a/?]\x1b[0m update them (--yes)\r\n",
		"success": "\x1b[32m==> update succeeded in ",
		"skip":    "\x1b[33m==> preserved-rebuild skipped: no preserved libraries\x1b[0m\r\n",
		"emerge":  "\x1b[32;01memergup-test/upgrade-1.1::emergup-test\x1b[39;49;00m",
	} {
		assert.Contains(t, coloured.stdout, line, kind)
	}

	log := ctr.runLog(tester, coloured.stdout)
	assert.NotContains(t, log, escape, "the log has no colours")
	assert.Contains(t, log, "\n==> update succeeded in ")
	assert.Contains(t, log, "] emergup-test/upgrade-1.1::emergup-test [1.0::emergup-test] ", "emerge's lines")

	for _, off := range []string{"NO_COLOR=1", "TERM=dumb"} {
		plain := ctr.emergupTerminal([]string{off}, "run", "--yes")
		require.Equal(t, 0, plain.code, "%s, with the output:\n%s", off, plain.stdout)
		assert.Contains(t, plain.stdout, "==> [1/2] update: update @emergup-test\r\n", off)
		assert.NotContains(t, plain.stdout, escape, "%s turns every colour off", off)
	}

	ctr.dropIns("doas")

	failed := ctr.emergupTerminal(nil, "run", "--yes")
	assert.Equal(t, 1, failed.code, "the exit status, with the output:\n%s", failed.stdout)
	assert.Contains(t, failed.stdout, "\x1b[1;31m==> elevation method doas is not installed\x1b[0m\r\n", "failure")

	pretend := ctr.emergupTerminal(nil, "run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s", pretend.stdout)
	assert.Contains(t, pretend.stdout,
		"\x1b[35m==> elevation method doas is not installed; a run would stop before its first step\x1b[0m\r\n",
		"notice")
}

// The run directories of tester, by default.
const runsDir = stateDir + "/runs"

// runDir matches the name of a run directory.
var runDir = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d-\d\d-\d\d(-\d+)?$`)

// UPD-15: each run keeps a log of what it showed, the questions and their
// answers, and every command with its output, in a directory of its own
// under the state directory; the end of the run says where.
func TestUPD15_EachRunKeepsALogOfWhatItShowed(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.publish("logged")

	stdout := ctr.emergupRun("y\n")

	path := logPath(t, stdout)
	assert.Equal(t, runsDir, filepath.Dir(filepath.Dir(path)), "the run directory is in the state directory")
	assert.Regexp(t, runDir, filepath.Base(filepath.Dir(path)))
	assert.Equal(t, "emergup.log", filepath.Base(path))

	log := ctr.runLog(tester, stdout)
	assert.Contains(t, log, "==> [1/1] sync: sync the repositories\n==> Run this step? [Y/s/a/?] run the step\n",
		"the question and its answer")
	assert.Regexp(t, `\n==> \S+ sudo emerge --sync --color=n emergup-test\n`, log, "the command")
	assert.Contains(t, log, "\n==> exit status 0 after ", "how the command ended")

	for line := range strings.Lines(stdout) {
		assert.Contains(t, log, line, "every line the run showed")
	}

	const state = "/home/tester/elsewhere"

	moved := ctr.exec(tester, "s\n", "env", "XDG_STATE_HOME="+state, "emergup", "run")
	require.Equal(t, 0, moved.code, "the exit status, with the output:\n%s%s", moved.stdout, moved.stderr)
	assert.Equal(t, state+"/emergup/runs", filepath.Dir(filepath.Dir(logPath(t, moved.stdout))), "XDG_STATE_HOME")
	assert.Contains(t, ctr.runLog(tester, moved.stdout), "==> sync skipped: you chose to skip it\n")
}

// runDirs returns the names of the run directories of tester, in order.
func runDirs(ctr *container) []string {
	ctr.t.Helper()

	listed := ctr.exec(tester, "", "ls", "-1", runsDir)
	require.Equal(ctr.t, 0, listed.code, "the run directories are not listed: %s", listed.stderr)

	return strings.Fields(listed.stdout)
}

// runOnce runs emergup with only a skipped sync and returns the name of its
// run directory.
func runOnce(ctr *container) string {
	ctr.t.Helper()

	return filepath.Base(filepath.Dir(logPath(ctr.t, ctr.emergupRun("s\n"))))
}

// UPD-15: the 10 latest run directories are kept, or as many as `[runs]
// keep` says; 0 keeps them all.
func TestUPD15_TheLatestRunDirectoriesAreKept(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")

	// The directories of eleven older runs.
	older := make([]string, 0, 11)
	for second := 1; second <= 11; second++ {
		older = append(older, fmt.Sprintf("2026-01-01T00-00-%02d", second))
	}

	made := ctr.exec(tester, "", append([]string{"mkdir", "-p"}, prefixed(runsDir+"/", older)...)...)
	require.Equal(t, 0, made.code, "the older run directories are not made: %s", made.stderr)

	latest := runOnce(ctr)
	assert.Equal(t, append(slices.Clone(older[2:]), latest), runDirs(ctr), "the default keeps 10")

	ctr.dropIns("keep-two")

	next := runOnce(ctr)
	assert.Equal(t, []string{latest, next}, runDirs(ctr), "keep = 2")

	ctr.dropIns("keep-all")

	kept := make([]string, 0, 5)
	kept = append(kept, latest, next)

	for range 3 {
		kept = append(kept, runOnce(ctr))
	}

	assert.Equal(t, kept, runDirs(ctr), "keep = 0")
}

// prefixed returns each of names after prefix.
func prefixed(prefix string, names []string) []string {
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, prefix+name)
	}

	return paths
}
