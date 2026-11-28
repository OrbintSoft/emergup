// SPDX-License-Identifier: GPL-3.0-or-later

package report_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/report"
)

// The package of the tests that fails to build, as emerge names it, and
// the repository it comes from.
const (
	gentooRepo = "gentoo"
	failedCPV  = "app-misc/foo-1.2"
	failedLog  = "/var/tmp/portage/app-misc/foo-1.2/temp/build.log"
)

// failedBuild is what emerge writes, without colours, when the build of
// failedCPV fails in its compile phase.
const failedBuild = `>>> Compiling source in /var/tmp/portage/app-misc/foo-1.2/work ...
 * ERROR: app-misc/foo-1.2::gentoo failed (compile phase):
 *   emake failed
 * The complete build log is located at '` + failedLog + `'.

>>> Failed to emerge app-misc/foo-1.2, Log file:

>>>  '` + failedLog + `'

 * Messages for package app-misc/foo-1.2:

 * ERROR: app-misc/foo-1.2::gentoo failed (compile phase):
 *   emake failed
`

// watch writes text to a new Output, a line at a time as the runner does,
// and returns it.
func watch(t *testing.T, text string) *report.Output {
	t.Helper()

	output := report.NewOutput()
	for line := range strings.Lines(text) {
		_, err := io.WriteString(output, line)
		require.NoError(t, err)
	}

	return output
}

func TestOutputNamesThePackageThatFailed(t *testing.T) {
	t.Parallel()

	output := watch(t, failedBuild)
	assert.Equal(t, []report.Package{
		{CPV: failedCPV, Repository: gentooRepo, Phase: "compile", Log: failedLog},
	}, output.Packages())
}

func TestOutputNamesEveryPackageThatFailed(t *testing.T) {
	t.Parallel()

	output := watch(t, failedBuild+
		" * ERROR: dev-libs/bar-2.0::overlay failed (install phase):\n"+
		">>> Failed to emerge dev-libs/bar-2.0 to /mnt/root, Log file:\n"+
		">>>  '/var/log/portage/dev-libs:bar-2.0:20261128-101010.log.gz'\n"+
		">>> Failed to emerge sys-apps/baz-3.0\n"+
		" * ERROR: net-misc/qux-1.0::gentoo failed (pretend phase):\n")
	assert.Equal(t, []report.Package{
		{CPV: failedCPV, Repository: gentooRepo, Phase: "compile", Log: failedLog},
		{
			CPV: "dev-libs/bar-2.0", Repository: "overlay", Phase: "install",
			Log: "/var/log/portage/dev-libs:bar-2.0:20261128-101010.log.gz",
		},
		{CPV: "sys-apps/baz-3.0", Repository: "", Phase: "", Log: ""},
		{CPV: "net-misc/qux-1.0", Repository: gentooRepo, Phase: "pretend", Log: ""},
	}, output.Packages(), "in the order emerge names them; a failure without a log has none")
}

func TestOutputWaitsForTheLogOnlyUntilTheNextLine(t *testing.T) {
	t.Parallel()

	output := watch(t, ">>> Failed to emerge app-misc/foo-1.2, Log file:\n>>> Emerging (2 of 3) other\n"+
		">>>  '/not/its/log'\n")
	assert.Equal(t, []report.Package{{CPV: failedCPV, Repository: "", Phase: "", Log: ""}}, output.Packages())
}

func TestOutputNamesNoPackageWhenNoneFailed(t *testing.T) {
	t.Parallel()

	output := watch(t, ">>> Syncing repository 'gentoo'\nrsync: connection refused\n")
	assert.Empty(t, output.Packages())
	assert.Equal(t, []string{">>> Syncing repository 'gentoo'\n", "rsync: connection refused\n"}, output.Lines())
}

func TestOutputKeepsTheLastLines(t *testing.T) {
	t.Parallel()

	var text strings.Builder
	for line := range 1500 {
		_, _ = fmt.Fprintf(&text, "line %d\r\n", line)
	}

	lines := watch(t, text.String()).Lines()
	require.Len(t, lines, 1000)
	assert.Equal(t, "line 500\n", lines[0], "the carriage return goes")
	assert.Equal(t, "line 1499\n", lines[999])
}

func TestOutputTakesALineWrittenInPieces(t *testing.T) {
	t.Parallel()

	output := report.NewOutput()
	for _, piece := range []string{">>> Failed to emerge app-", "misc/foo-1.2\nnext\n", "last"} {
		_, err := io.WriteString(output, piece)
		require.NoError(t, err)
	}

	assert.Equal(t, []report.Package{{CPV: failedCPV, Repository: "", Phase: "", Log: ""}}, output.Packages())
	assert.Equal(t, []string{">>> Failed to emerge app-misc/foo-1.2\n", "next\n", "last\n"}, output.Lines(),
		"a last line without a newline gets one")
}
