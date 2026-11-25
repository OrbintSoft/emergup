// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configuration file of emergup-test/conf, and the update of it that
// Portage leaves next to it when the installed one was edited.
const (
	confFile    = "/etc/emergup-test.conf"
	pendingConf = "/etc/._cfg0000_emergup-test.conf"
)

// rootOutput runs args as root, which must succeed, and returns their
// standard output without the last line break.
func rootOutput(ctr *container, args ...string) string {
	ctr.t.Helper()

	got := ctr.exec(root, "", args...)
	require.Equal(ctr.t, 0, got.code, "%v failed: %s", args, got.stderr)

	return strings.TrimSpace(got.stdout)
}

// preservedLibraries returns the libraries Portage preserves, as it lists
// them, or "" when it preserves none.
func preservedLibraries(ctr *container) string {
	ctr.t.Helper()

	return strings.TrimSpace(ctr.exec(root, "", "portageq", "list_preserved_libs", "/").stdout)
}

// withPreservedLibrary installs a program linked against version 1 of a
// library, then version 2 of the library, whose soname differs: Portage
// keeps version 1, the program still needing it.
func withPreservedLibrary(ctr *container) {
	ctr.t.Helper()

	ctr.publishTree("snapshot-libs-1")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/app")

	ctr.publishTree("snapshot-libs-2")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "--oneshot", "emergup-test/lib")
}

// withPendingConf installs emergup-test/conf, edits its configuration file
// as an administrator would, and updates the package, which leaves the new
// configuration file pending.
func withPendingConf(ctr *container) {
	ctr.t.Helper()

	ctr.publishTree("snapshot-conf-1")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/conf")
	ctr.install("testdata/edited-conf/emergup-test.conf", confFile)

	ctr.publishTree("snapshot-conf-2")
	ctr.syncRepository()
	ctr.root("emerge", "--quiet", "emergup-test/conf")
	require.True(ctr.t, ctr.exists(pendingConf), "the new configuration file is pending")

	ctr.dropIns("config-files-only")
}

// UPD-5: the preserved-library rebuild runs only when Portage reports
// preserved libraries.
func TestUPD5_ThePreservedLibrariesAreRebuiltOnlyWhenThereAreSome(t *testing.T) {
	t.Parallel()

	const (
		listed   = "\n    emergup-test/lib-2  /usr/lib64/libemergup-test.so.1\n"
		skipped  = "==> preserved-rebuild skipped: no preserved libraries\n"
		question = "==> Rebuild the packages that use them? [Y/s/a/?] "
	)

	ctr := start(t)
	ctr.dropIns("preserved-only")

	nothing := ctr.emergupRun("")
	assert.Contains(t, nothing, skipped)
	assert.NotContains(t, nothing, question, "with nothing to rebuild, nothing is asked")

	withPreservedLibrary(ctr)
	require.Equal(t, "emergup-test/lib-2 /usr/lib64/libemergup-test.so.1", preservedLibraries(ctr))

	kept := ctr.emergupRun("s\n")
	assert.Contains(t, kept, listed)
	assert.Contains(t, kept, question+"skip the rebuild\n")
	assert.NotEmpty(t, preservedLibraries(ctr), "the rebuild is skipped")

	rebuilt := ctr.emergupRun("", "--yes")
	assert.Contains(t, rebuilt, listed)
	assert.Contains(t, rebuilt, "==> preserved-rebuild succeeded")
	assert.Empty(t, preservedLibraries(ctr), "the program is rebuilt against version 2")

	assert.Contains(t, ctr.emergupRun("", "--yes"), skipped)
}

// ETC-1: after the update, pending config-file updates (`._cfg*` files) are
// listed. dispatch-conf, the default tool, is interactive, so a run with
// `--yes` does not start it.
func TestETC1_ThePendingConfigurationFilesAreListed(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withPendingConf(ctr)

	stdout := ctr.emergupRun("", "--yes")
	assert.Contains(t, stdout, "\n    "+confFile+"\n")
	assert.Contains(t, stdout, "==> config-files skipped: dispatch-conf is interactive: run it to update them\n")
	assert.True(t, ctr.exists(pendingConf), "the new configuration file is still pending")
}

// ETC-3: the step can delegate to an external tool, here etc-update, which
// emergup runs as root once the user confirms.
func TestETC3_TheUpdateIsDelegatedToTheConfiguredTool(t *testing.T) {
	t.Parallel()

	const question = "==> Update them with etc-update? [Y/s/a/?] "

	ctr := start(t)
	withPendingConf(ctr)
	ctr.dropIns("etc-update")

	assert.Contains(t, ctr.emergupRun("s\n"), question+"skip the update\n")
	assert.Equal(t, "edited", rootOutput(ctr, "cat", confFile), "the update is skipped")

	updated := ctr.emergupRun("y\n")
	assert.Contains(t, updated, question+"update them\n")
	assert.Contains(t, updated, "==> config-files succeeded")
	assert.Equal(t, "1.1", rootOutput(ctr, "cat", confFile), "the new configuration file is taken")
	assert.False(t, ctr.exists(pendingConf), "nothing is pending")

	assert.Contains(t, ctr.emergupRun("", "--yes"), "==> config-files skipped: no configuration file to update\n")
}

// UPD-10: before the depclean, emergup shows what emerge would do
// (`--pretend`) and asks for confirmation itself; with `--yes` it goes
// ahead without asking.
func TestUPD10_TheDepcleanShowsWhatItWouldDoAndAsks(t *testing.T) {
	t.Parallel()

	const question = "==> Remove these packages? [Y/s/a/?] "

	ctr := start(t)
	ctr.publishTree("snapshot-held")
	ctr.syncRepository()
	// Installed without being added to the world set: nothing needs it.
	ctr.root("emerge", "--quiet", "--oneshot", "emergup-test/held")
	ctr.dropIns("depclean-only")

	kept := ctr.emergupRun("s\n")
	assert.Contains(t, kept, ">>> These are the packages that would be unmerged:\n")
	assert.Contains(t, kept, "\n emergup-test/held\n")
	assert.Contains(t, kept, question+"skip the depclean\n")
	assert.Equal(t, "1.0", ctr.version("held"), "the depclean is skipped")

	removed := ctr.emergupRun("", "--yes")
	assert.Contains(t, removed, question+"remove them (--yes)\n")
	assert.False(t, ctr.exists("/usr/share/emergup-test/held"), "the package is removed")
	assert.True(t, ctr.exists("/usr/share/emergup-test/upgrade"), "a package of the world set is kept")

	assert.Contains(t, ctr.emergupRun("", "--yes"), "==> depclean skipped: nothing to remove\n")
}
