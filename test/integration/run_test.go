// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// UPD-6: `--pretend` shows the steps and the commands that would run,
// without running them.
func TestUPD6_PretendShowsTheStepsWithoutRunningThem(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	// A snapshot that a sync would bring.
	ctr.publish("pretend")

	got := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	for _, line := range []string{
		"==> [1/6] sync: sync the repositories",
		"    emerge --sync --color=n emergup-test",
		"==> [2/6] update: update @emergup-test",
		"==> [3/6] live-rebuild: rebuild the live packages",
		"==> [4/6] preserved-rebuild: rebuild the packages that use preserved libraries",
		"==> [5/6] config-files: update the configuration files",
		"==> [6/6] depclean: remove the packages no longer needed",
	} {
		assert.Contains(t, got.stdout, line+"\n")
	}

	assert.False(t, ctr.exists(repository+"/pretend"), "the repository is not synced")
}
