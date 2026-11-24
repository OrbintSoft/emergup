// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When the live package of the test repository was built, as Portage
// records it, and a time long before any test runs.
const (
	buildTime = "/var/db/pkg/emergup-test/live-9999/BUILD_TIME"
	longAgo   = "1767225600"
)

// withLive installs the live package of the test repository, and leaves
// only the live-package rebuild enabled.
func withLive(ctr *container) {
	ctr.t.Helper()

	ctr.publishTree("snapshot-live")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/live")
	ctr.dropIns("live-only")
}

// builtLongAgo makes Portage record that the live package was built long
// ago.
func builtLongAgo(ctr *container) {
	ctr.t.Helper()

	ctr.install("testdata/old-build/BUILD_TIME", buildTime)
}

// built returns when Portage records that the live package was built.
func built(ctr *container) string {
	ctr.t.Helper()

	got := ctr.exec(root, "", "cat", buildTime)
	require.Equal(ctr.t, 0, got.code, got.stderr)

	return strings.TrimSpace(got.stdout)
}

// UPD-9: `interval` rebuilds each live package built longer ago than the
// interval, 24 hours by default; `auto`, the default, is `interval` when
// smart-live-rebuild is not installed.
func TestUPD9_TheIntervalModeRebuildsTheLivePackagesBuiltLongAgo(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withLive(ctr)

	kept := ctr.emergupRun("", "--yes")
	assert.Contains(t, kept,
		"\n    smart-live-rebuild is not installed: the live packages built longer ago than 24h are rebuilt\n")
	assert.Regexp(t, `\n    emergup-test/live-9999  built (<1s|\d+s) ago, kept\n`, kept)
	assert.Contains(t, kept, "==> live-rebuild skipped: every live package was built less than 24h ago\n")

	builtLongAgo(ctr)

	rebuilt := ctr.emergupRun("", "--yes")
	assert.Regexp(t, `\n    emergup-test/live-9999  built \d+h\d+m ago\n`, rebuilt)
	assert.Contains(t, rebuilt, "==> live-rebuild succeeded")
	assert.NotEqual(t, longAgo, built(ctr), "the package is rebuilt")
}

// UPD-9: `always` rebuilds every live package.
func TestUPD9_TheAlwaysModeRebuildsEveryLivePackage(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withLive(ctr)
	ctr.dropIns("live-always")
	builtLongAgo(ctr)

	stdout := ctr.emergupRun("", "--yes")
	assert.Contains(t, stdout, "\n    emergup-test/live-9999  rebuilt on every run\n")
	assert.NotEqual(t, longAgo, built(ctr), "the package is rebuilt")

	again := ctr.emergupRun("", "--yes")
	assert.Contains(t, again, "\n    emergup-test/live-9999  rebuilt on every run\n", "even right after a rebuild")
	assert.Contains(t, again, "==> live-rebuild succeeded")
}

// UPD-4: an optional step whose tool is not installed is skipped with a
// notice instead of failing: here the `smart` mode without
// smart-live-rebuild.
func TestUPD4_TheSmartModeWithoutItsToolIsSkipped(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("live-only")
	ctr.dropIns("live-smart")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, "\n    smart-live-rebuild is not installed, so the step would be skipped\n")

	assert.Contains(t, ctr.emergupRun("", "--yes"), "==> live-rebuild skipped: smart-live-rebuild is not installed\n")
}
