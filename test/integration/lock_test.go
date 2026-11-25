// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The state directory of tester, and the lock in it.
const (
	stateDir = "/home/tester/.local/state/emergup"
	lockFile = stateDir + "/run.lock"
)

// UPD-8: while a run of a user is active, another run of that user refuses
// to start and says which process holds the run; a pretend, or a run of
// another user, goes ahead.
func TestUPD8_OnlyOneRunOfAUserIsActive(t *testing.T) {
	t.Parallel()

	const (
		question = "==> Run this step? [Y/s/a/?] "
		skipped  = "==> sync skipped: you chose to skip it\n"
	)

	ctr := start(t)
	ctr.dropIns("sync-only")

	first := ctr.emergupBackground("run")
	first.waitFor(question)

	second := ctr.emergup("run")
	assert.Equal(t, 1, second.code, "the exit status, with the output:\n%s%s", second.stdout, second.stderr)

	refusal := regexp.MustCompile(`^==> another emergup run of this user is active: process (\d+)\n$`).
		FindStringSubmatch(second.stdout)
	require.NotNil(t, refusal, "the refusal:\n%s", second.stdout)
	assert.Equal(t, rootOutput(ctr, "pgrep", "--uid", tester, "--exact", program), refusal[1],
		"the process of the first run")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)

	byRoot := ctr.emergupAs(root, "s\n", "run")
	assert.Equal(t, 0, byRoot.code, "the exit status, with the output:\n%s%s", byRoot.stdout, byRoot.stderr)
	assert.Contains(t, byRoot.stdout, skipped)

	ended := first.finish("s\n")
	assert.Equal(t, 0, ended.code, "the exit status, with the output:\n%s", ended.stdout)
	assert.Contains(t, ended.stdout, question+"skip the step\n")

	assert.Contains(t, ctr.emergupRun("s\n"), skipped, "the lock is free again")
}

// UPD-8: emergup does not use a lock that another user could change.
func TestUPD8_TheLockIsNotUsedWhenAnotherUserCouldChangeIt(t *testing.T) {
	t.Parallel()

	for _, unsafe := range []struct {
		name string
		// prepare makes the lock unsafe, run as user.
		user    string
		prepare []string
		problem string
	}{
		{
			name:    "a state directory others can write",
			user:    tester,
			prepare: []string{"chmod", "0770", stateDir},
			problem: "the state directory is not safe: " + stateDir + " can be changed by other users",
		},
		{
			name:    "a lock of another user",
			user:    root,
			prepare: []string{"install", "--mode=0666", "/dev/null", lockFile},
			problem: "the lock is not safe: " + lockFile + " belongs to another user",
		},
	} {
		t.Run(unsafe.name, func(t *testing.T) {
			t.Parallel()

			ctr := start(t)
			ctr.dropIns("sync-only")

			made := ctr.exec(tester, "", "mkdir", "-p", stateDir)
			require.Equal(t, 0, made.code, "the state directory is not made: %s", made.stderr)

			done := ctr.exec(unsafe.user, "", unsafe.prepare...)
			require.Equal(t, 0, done.code, "%v failed: %s", unsafe.prepare, done.stderr)

			got := ctr.emergupInput("y\n", "run")
			assert.Equal(t, result{stdout: "==> " + unsafe.problem + "\n", stderr: "", code: 1}, got)
		})
	}
}
