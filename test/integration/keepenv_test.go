// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PRIV-5: the commands run as root keep the variables the configuration
// lists, and only those, which sudo would reset. HOOK-4: a hook run as root
// gets the context of the run all the same.
func TestPRIV5_TheCommandsRunAsRootKeepTheVariablesListed(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.installHook()
	ctr.dropIns("sync-only")
	ctr.dropIns("keep-env")

	got := ctr.emergupWith([]string{"EMERGUP_TEST_KEPT=kept", "EMERGUP_TEST_DROPPED=dropped"}, "run", "--yes")
	require.Equal(t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)
	assert.Contains(t, got.stdout, "==> hook rooted (before the run), as root: /usr/local/bin/emergup-test-hook "+
		"show:EMERGUP_TEST_KEPT show:EMERGUP_TEST_DROPPED\nEMERGUP_TEST_KEPT=kept\nEMERGUP_TEST_DROPPED is unset\n")

	ran := ctr.hooksRan()
	require.Len(t, ran, 1)
	assert.Regexp(t, `^rooted before run status= result= user=root `+
		`dir=(/home/tester/\.local/state/emergup/runs/[^ ]+) log=/home/tester/\.local/state/emergup/runs/[^ ]+/emergup\.log$`,
		ran[0])
}
