// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// gitSync is the sync type of a git repository.
const gitSync = "git"

func TestParseReposConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []repository
	}{
		{
			"a section for each repository",
			"[DEFAULT]\nmain-repo = gentoo\n\n[gentoo]\nlocation = /var/db/repos/gentoo\nsync-type = rsync\n" +
				"[local]\nlocation = /var/db/repos/local\n",
			[]repository{
				{name: "gentoo", location: "/var/db/repos/gentoo", syncType: "rsync", autoSync: true},
				{name: "local", location: "/var/db/repos/local", syncType: "", autoSync: true},
			},
		},
		{
			"a key of DEFAULT that a section does not set",
			"[DEFAULT]\nauto-sync = no\nsync-type = git\n\n[guru]\nlocation = /g\n[other]\nauto-sync = yes\nlocation = /o\n",
			[]repository{
				{name: "guru", location: "/g", syncType: gitSync, autoSync: false},
				{name: "other", location: "/o", syncType: gitSync, autoSync: true},
			},
		},
		{
			"spaces, comments, values with an equals sign and on more lines",
			"# a comment\n; another\n[gentoo]\nlocation=/var/db/repos/gentoo  \n" +
				"sync-uri = https://example.org/?a=b\nsync-git-env = A=1\n\tB=2\nsync-type = git\n",
			[]repository{{name: "gentoo", location: "/var/db/repos/gentoo", syncType: gitSync, autoSync: true}},
		},
		{
			"nothing", "", []repository{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseReposConfig(tt.text))
		})
	}
}

func TestParseReposConfigReadsAutoSyncAsPortageDoes(t *testing.T) {
	t.Parallel()

	values := map[string]bool{
		"yes": true, "Yes": true, "true": true, "TRUE": true, "": true,
		"no": false, "No": false, "false": false, "FALSE": false,
	}
	for value, want := range values {
		repositories := parseReposConfig("[r]\nauto-sync = " + value + "\n")
		assert.Equal(t, []repository{{name: "r", location: "", syncType: "", autoSync: want}}, repositories,
			"auto-sync = %q", value)
	}
}
