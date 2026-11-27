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

	skipped := ctr.emergupRun("", "--yes")
	assert.Contains(t, skipped, "==> live-rebuild skipped: smart-live-rebuild is not installed\n")
	assert.NotContains(t, skipped, "? [Y/s/a/?]", "skipped without a question")
}

// The stand-in for smart-live-rebuild, and the file that makes it list the
// live package of the test repository, as if its upstream had changed.
const (
	smartLiveRebuild = "/usr/local/bin/smart-live-rebuild"
	upstreamChanged  = "/var/tmp/emergup-test-upstream"
	smartRan         = "/var/tmp/emergup-test-smart-ran"
)

// installSmartLiveRebuild installs the stand-in for smart-live-rebuild.
func (c *container) installSmartLiveRebuild() {
	c.t.Helper()

	c.install("testdata/smart-live-rebuild-script/smart-live-rebuild.sh", smartLiveRebuild)
	c.root("chmod", "0755", smartLiveRebuild)
}

// UPD-16: in the smart mode, smart-live-rebuild lists, as root, the live
// packages whose upstream has changed; emergup shows them, asks, and has
// emerge rebuild them, never from a binary package. With none, the step is
// skipped without a question.
func TestUPD16_TheSmartModeListsThePackagesThenAsks(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withLive(ctr)
	ctr.installSmartLiveRebuild()
	ctr.dropIns("smart-live-rebuild")
	builtLongAgo(ctr)

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, "    would list the live packages to rebuild, as root, with: "+smartLiveRebuild+
		" --pretend\n    and if there are any run: emerge --oneshot --color=n --usepkg-exclude-live=y followed by them\n")
	assert.False(t, ctr.exists(smartRan), "--pretend does not run smart-live-rebuild, which updates the clones")

	nothing := ctr.emergupRun("")
	assert.Contains(t, nothing, "==> live-rebuild skipped: smart-live-rebuild found nothing to rebuild\n")
	assert.NotContains(t, nothing, "Rebuild them?", "nothing to rebuild, nothing to ask")
	assert.True(t, ctr.exists(smartRan), "a run has smart-live-rebuild list the packages")

	ctr.root("touch", upstreamChanged)

	skipped := ctr.emergupRun("s\n")
	assert.Contains(t, skipped, "\n    emergup-test/live:0\n==> Rebuild them? [Y/s/a/?] skip the rebuild\n"+
		"==> live-rebuild skipped: you chose not to rebuild them\n")
	assert.Equal(t, longAgo, built(ctr), "the package is not rebuilt")

	rebuilt := ctr.emergupRun("y\n")
	assert.Contains(t, rebuilt, "==> Rebuild them? [Y/s/a/?] rebuild them\n")
	assert.Contains(t, rebuilt, "==> live-rebuild succeeded")
	assert.NotEqual(t, longAgo, built(ctr), "the package is rebuilt")
	assert.Contains(t, ctr.runLog(tester, rebuilt),
		" sudo emerge --oneshot --color=n --usepkg-exclude-live=y emergup-test/live:0\n",
		"emerge rebuilds the package listed, without checking the upstreams again")
}

// UPD-16: the interval mode, as every mode, lists the live packages it
// would rebuild before it asks.
func TestUPD16_TheIntervalModeListsThePackagesThenAsks(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withLive(ctr)
	builtLongAgo(ctr)

	skipped := ctr.emergupRun("s\n")
	assert.Regexp(t,
		`\n    emergup-test/live-9999  built \d+h\d+m ago\n==> Rebuild them\? \[Y/s/a/\?\] skip the rebuild\n`, skipped)
	assert.NotContains(t, skipped, "Run this step?", "the step asks itself, once it has listed the packages")
	assert.Equal(t, longAgo, built(ctr), "the package is not rebuilt")
}
