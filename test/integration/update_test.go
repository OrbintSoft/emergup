// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// withHeld installs emergup-test/held 1.0, adds it to the target set and
// excludes it from the update, then brings version 1.1 of it and of
// emergup-test/upgrade.
func withHeld(ctr *container) {
	ctr.t.Helper()

	ctr.publishTree("snapshot-held")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/held")
	ctr.install("testdata/set-with-held/emergup-test", "/etc/portage/sets/emergup-test")

	ctr.dropIns("update-only")
	ctr.dropIns("exclude-held")
	ctr.publishTree("snapshot-updates")
	ctr.syncRepository()
}

// UPD-10: before the update, emergup shows what emerge would do
// (`--pretend`) and asks for confirmation itself; with `--yes` it goes
// ahead without asking.
func TestUPD10_TheUpdateShowsWhatItWouldDoAndAsks(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("update-only")
	ctr.publishTree("snapshot-updates")
	ctr.syncRepository()

	skipped := ctr.emergupRun("s\n")
	assert.Contains(t, skipped, "[ebuild     U  ] emergup-test/upgrade-1.1::emergup-test [1.0::emergup-test]")
	assert.Contains(t, skipped, "==> Update these packages? [Y/s/a/?] skip the update\n")
	assert.Equal(t, "1.0", ctr.version("upgrade"), "the update is skipped")

	updated := ctr.emergupRun("", "--yes")
	assert.Contains(t, updated, "==> Update these packages? [Y/s/a/?] update them (--yes)\n")
	assert.Equal(t, "1.1", ctr.version("upgrade"), "the package is updated")
}

// EXC-1: packages listed in the configuration are excluded from the
// update.
func TestEXC1_AnExcludedPackageIsNotUpdated(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withHeld(ctr)

	stdout := ctr.emergupRun("", "--yes")
	assert.Contains(t, stdout, "emergup-test/upgrade-1.1::emergup-test")
	assert.NotContains(t, stdout, "emergup-test/held-1.1::emergup-test")
	assert.Equal(t, "1.1", ctr.version("upgrade"), "the other package is updated")
	assert.Equal(t, "1.0", ctr.version("held"), "the excluded package is not")
}

// EXC-2: excluded packages that have an update available are listed at the
// end of the run, once the update has updated or had nothing to update.
func TestEXC2_TheExcludedPackagesWithAnUpdateAreListed(t *testing.T) {
	t.Parallel()

	const held = "\n==> 1 excluded package has an update:\n    emergup-test/held-1.1\n"

	ctr := start(t)
	withHeld(ctr)

	updated := ctr.emergupRun("", "--yes")
	assert.Contains(t, updated, held, "after an update")

	unchanged := ctr.emergupRun("", "--yes")
	assert.Contains(t, unchanged, "==> update skipped: nothing to update\n")
	assert.Contains(t, unchanged, held, "with nothing to update")
}
