// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// CLI-1: `emergup version` (or `--version`) prints the emergup version.
func TestCLI1_VersionPrintsTheVersion(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	want := result{stdout: "emergup " + setting(t, versionVariable) + "\n", stderr: "", code: 0}

	for _, arg := range []string{"version", "--version"} {
		assert.Equal(t, want, ctr.emergup(arg), arg)
	}
}
