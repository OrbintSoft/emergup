// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// took matches a time as emergup writes it: "<1s", "42s", "12m03s",
// "2h03m".
const took = `(<1s|\d+s|\d+m\d\ds|\d+h\d\dm)`

// summaryRow matches the row of the summary that pattern matches.
func summaryRow(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^    ` + pattern + `$`)
}

// summaryRun is a run of TestUPD7_TheSummaryListsHowEachStepEnded: the
// answers to its questions, and how it ends.
type summaryRun struct {
	name    string
	answers []string
	code    int
	// rows match the rows of the summary, the disabled live-package
	// rebuild apart.
	rows []string
	end  string
}

// summaryRuns returns the runs of TestUPD7_TheSummaryListsHowEachStepEnded,
// one after the other.
func summaryRuns() []summaryRun {
	return []summaryRun{
		{
			name:    "stopped",
			answers: []string{"y", "a"},
			code:    130,
			rows: []string{
				`sync +succeeded +` + took,
				`update +stopped +` + took + `  you aborted the run`,
				`preserved-rebuild +not run`,
				`config-files +not run`,
				`depclean +not run`,
			},
			end: "\n==> The run was stopped after ",
		},
		{
			name:    "failed",
			answers: []string{"s", "y"},
			code:    1,
			rows: []string{
				`sync +skipped +you chose to skip it`,
				`update +failed +` + took + `  emerge exited with status 1`,
				`preserved-rebuild +not run`,
				`config-files +not run`,
				`depclean +not run`,
			},
			end: "\n==> The run failed after ",
		},
		{
			name:    "finished",
			answers: []string{"s", "s"},
			code:    0,
			rows: []string{
				`sync +skipped +you chose to skip it`,
				`update +skipped +` + took + `  you chose not to update`,
				`preserved-rebuild +skipped +` + took + `  no preserved libraries`,
				`config-files +skipped +` + took + `  no configuration file to update`,
				`depclean +skipped +` + took + `  nothing to remove`,
			},
			end: "\n==> The run finished in ",
		},
	}
}

// UPD-7: the summary lists how each step ended, and says how the run
// ended.
func TestUPD7_TheSummaryListsHowEachStepEnded(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("no-live")
	// A version of emergup-test/upgrade that fails to build.
	ctr.publishTree("snapshot-broken")

	for _, run := range summaryRuns() {
		got := ctr.emergupInput(strings.Join(run.answers, "\n")+"\n", "run")
		assert.Equal(t, run.code, got.code, "%s, with the output:\n%s%s", run.name, got.stdout, got.stderr)
		assert.Contains(t, got.stdout, "\n==> Summary\n", run.name)

		for _, row := range append(run.rows, `live-rebuild +disabled`) {
			assert.Regexp(t, summaryRow(row), got.stdout, run.name)
		}

		assert.Contains(t, got.stdout, run.end, run.name)
	}

	assert.Equal(t, "1.0", ctr.version("upgrade"), "the broken version is not installed")
}

// seconds returns the time emergup wrote as text, "<1s" being 0.
func seconds(t *testing.T, text string) time.Duration {
	t.Helper()

	if text == "<1s" {
		return 0
	}

	duration, err := time.ParseDuration(text)
	require.NoError(t, err)

	return duration
}

// UPD-12: each step says how long it took when it ends, and the summary
// lists the time of every step and of the whole run.
func TestUPD12_TheTimeOfEachStepAndOfTheRunIsShown(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("update-only")
	ctr.publishTree("snapshot-updates")
	ctr.syncRepository()

	stdout := ctr.emergupRun("", "--yes")

	ended := regexp.MustCompile(`\n==> update succeeded in ` + took + `\n`).FindStringSubmatch(stdout)
	require.NotNil(t, ended, "the end of the update:\n%s", stdout)

	// A pretend, then the update: emerge works out the dependencies twice.
	step := seconds(t, ended[1])
	assert.GreaterOrEqual(t, step, time.Second, "the time of the update")

	assert.Regexp(t, summaryRow(`update +succeeded +`+regexp.QuoteMeta(ended[1])), stdout,
		"the summary has the time of the update")

	finished := regexp.MustCompile(`\n==> The run finished in ` + took + `\.\n`).FindStringSubmatch(stdout)
	require.NotNil(t, finished, "the end of the run:\n%s", stdout)
	assert.GreaterOrEqual(t, seconds(t, finished[1]), step, "the time of the run")
}
