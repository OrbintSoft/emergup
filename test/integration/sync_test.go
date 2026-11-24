// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// row is the line the sync step shows for the test repository.
func row(reason string) string {
	return "    emergup-test  " + reason + "\n"
}

// syncRun runs emergup with only its sync step and the options args,
// which must succeed, and returns its standard output.
func syncRun(ctr *container, args ...string) string {
	ctr.t.Helper()

	got := ctr.emergup(append([]string{"run", "--yes"}, args...)...)
	require.Equal(ctr.t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	return got.stdout
}

// syncOutside syncs the test repository with emerge, as a sync done
// outside emergup, so that it was synced a moment ago.
func syncOutside(ctr *container) {
	ctr.t.Helper()

	ctr.publish("outside")
	ctr.root("emerge", "--sync", "emergup-test")
}

// SYNC-1: the sync step works on the repositories `emerge --sync` would
// sync, and syncs each one synced longer ago than the interval, 24 hours by
// default, saying when it was last synced.
func TestSYNC1_ARepositorySyncedLongAgoIsSynced(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.publish("new")

	stdout := syncRun(ctr)
	assert.Regexp(t, "\n"+row(`synced \d+h\d+m ago`), stdout, "the image synced the repository long ago")
	assert.NotContains(t, stdout, "\n    gentoo ", "the Gentoo tree has auto-sync off")
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// SYNC-1: a repository synced more recently than the interval is skipped;
// a sync done outside emergup counts.
func TestSYNC1_ARepositorySyncedRecentlyIsKept(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	syncOutside(ctr)
	ctr.publish("new")

	stdout := syncRun(ctr)
	assert.Contains(t, stdout, ", kept\n")
	assert.Contains(t, stdout, "every repository was synced less than 24h ago")
	assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")
}

// SYNC-1: the interval is configurable, and an interval of 0 syncs every
// time.
func TestSYNC1_TheIntervalIsConfigurable(t *testing.T) {
	t.Parallel()

	t.Run("longer than the last sync", func(t *testing.T) {
		t.Parallel()

		ctr := start(t)
		ctr.dropIns("sync-only")
		ctr.dropIns("long-interval")
		ctr.publish("new")

		stdout := syncRun(ctr)
		assert.Contains(t, stdout, ", kept\n")
		assert.False(t, ctr.exists(repository+"/new"), "the repository is not synced")
	})

	t.Run("zero", func(t *testing.T) {
		t.Parallel()

		ctr := start(t)
		ctr.dropIns("sync-only")
		ctr.dropIns("every-run")
		syncOutside(ctr)
		ctr.publish("new")

		stdout := syncRun(ctr)
		assert.Contains(t, stdout, row("synced on every run"))
		assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
	})
}

// SYNC-1: a repository whose last sync time cannot be determined is
// synced.
func TestSYNC1_ARepositoryWithoutItsLastSyncTimeIsSynced(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.dropIns("long-interval")
	ctr.root("rm", repository+timestamp)
	ctr.publish("new")

	stdout := syncRun(ctr)
	assert.Contains(t, stdout, row("last sync not known"))
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// SYNC-2: `emergup run --force-sync` syncs the repositories regardless of
// the interval.
func TestSYNC2_ForceSyncIgnoresTheInterval(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.dropIns("long-interval")
	ctr.publish("new")

	stdout := syncRun(ctr, "--force-sync")
	assert.Contains(t, stdout, row("--force-sync"))
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// SYNC-3: repositories listed in the configuration are synced on every run
// regardless of the interval.
func TestSYNC3_AnAlwaysSyncedRepositoryIgnoresTheInterval(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("sync-only")
	ctr.dropIns("long-interval")
	ctr.dropIns("always-sync")
	ctr.publish("new")

	stdout := syncRun(ctr)
	assert.Contains(t, stdout, row("always synced"))
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}

// SYNC-3: a repository listed in the configuration is synced even when
// Portage does not sync it by itself (`auto-sync = no`).
func TestSYNC3_AnAlwaysSyncedRepositoryIsSyncedWithAutoSyncOff(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.install("testdata/auto-sync-off/emergup-test.conf", "/etc/portage/repos.conf/emergup-test.conf")
	ctr.dropIns("sync-only")
	ctr.publish("new")

	assert.Contains(t, syncRun(ctr), "no repository to sync")
	assert.False(t, ctr.exists(repository+"/new"), "a repository with auto-sync off is not synced by itself")

	ctr.dropIns("always-sync")

	assert.Contains(t, syncRun(ctr), row("always synced"))
	assert.True(t, ctr.exists(repository+"/new"), "the repository is synced")
}
