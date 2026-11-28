// SPDX-License-Identifier: GPL-3.0-or-later

package report_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

// The names and contents of the report of failedBuild.
const (
	runLog     = "/home/user/.local/state/emergup/runs/2026-11-28T10-00-00/emergup.log"
	buildLog   = "app-misc_foo-1.2.build.log"
	emergeInfo = "app-misc_foo-1.2.emerge-info.txt"
	emergePqv  = "app-misc_foo-1.2.emerge-pqv.txt"
	summary    = "summary.txt"
	outputFile = "output.txt"
)

// The errors of the fake files.
var (
	errDisk   = errors.New("no space left on device")
	errExists = errors.New("file exists")
)

// files is a fake report.Files that keeps the files it creates; a name in
// broken cannot be created.
type files struct {
	created map[string]*bytes.Buffer
	order   []string
	broken  map[string]bool
}

func newFiles() *files {
	return &files{created: map[string]*bytes.Buffer{}, broken: map[string]bool{}}
}

type closer struct{ *bytes.Buffer }

func (closer) Close() error { return nil }

func (f *files) Create(name string) (io.WriteCloser, error) {
	if f.broken[name] {
		return nil, errDisk
	}

	if _, exists := f.created[name]; exists {
		return nil, fmt.Errorf("%w: %s", errExists, name)
	}

	f.created[name] = &bytes.Buffer{}
	f.order = append(f.order, name)

	return closer{f.created[name]}, nil
}

// content returns what was written to the file name.
func (f *files) content(t *testing.T, name string) string {
	t.Helper()

	buffer, exists := f.created[name]
	require.True(t, exists, "%s is not in the report, which has %v", name, f.order)

	return buffer.String()
}

// failedUpdate is the failure of the update whose command wrote
// failedBuild.
func failedUpdate(t *testing.T) report.Failure {
	t.Helper()

	return report.Failure{
		Step: "update", Title: "update @world", Reason: "emerge exited with status 1",
		Ran: report.Ran{
			Command: update(), Result: runner.Result{ExitCode: 1}, Err: nil, Output: watch(t, failedBuild),
		},
		RanAny: true,
		Log:    runLog,
	}
}

// queries returns the arguments, the mode and whether it ran as root, of
// each of commands.
func queries(commands []runner.Command) []string {
	described := make([]string, 0, len(commands))

	for _, command := range commands {
		root := ""
		if command.Root {
			root = " (root)"
		}

		saved := ""
		if command.Mode == runner.Save && command.Into != nil {
			saved = " (saved)"
		}

		described = append(described, strings.Join(command.Args, " ")+root+saved)
	}

	return described
}

func TestWriteCollectsTheReportOfAFailedPackage(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(
		runnertest.Answer{Result: runner.Result{Output: []byte("the build log\n")}},
		runnertest.Answer{Result: runner.Result{Output: []byte("Portage 3.0.83\n")}},
		runnertest.Answer{Result: runner.Result{Output: []byte("[ebuild     U  ] app-misc/foo-1.2\n")}},
	)

	report := newFiles()

	problems := writeReport(t, report, &fake, failedUpdate(t))
	assert.Empty(t, problems)

	assert.Equal(t, []string{
		"cat -- " + failedLog + " (root) (saved)",
		"emerge --info =app-misc/foo-1.2::gentoo (saved)",
		"emerge -pqv =app-misc/foo-1.2::gentoo (saved)",
	}, queries(fake.Commands()), "the log is read as root, the queries run unprivileged")

	assert.Equal(t, "the build log\n", report.content(t, buildLog))
	assert.Equal(t, "Portage 3.0.83\n", report.content(t, emergeInfo))
	assert.Equal(t, "[ebuild     U  ] app-misc/foo-1.2\n", report.content(t, emergePqv))
	assert.Equal(t, failedBuild, report.content(t, outputFile))
	assert.Equal(t, `The failure report of an emergup run.

Step:     update (update @world)
Failure:  emerge exited with status 1
Command:  emerge --update @world, as root
Run log:  `+runLog+`

The packages that failed:
  app-misc/foo-1.2::gentoo, in its compile phase
    build log      `+failedLog+`, copied to `+buildLog+`
    emerge --info  `+emergeInfo+`
    emerge -pqv    `+emergePqv+`

The end of the command's output, its last 1000 lines at most, without colours,
is in output.txt; the run log has all of it.
`, report.content(t, summary))
	assert.Equal(t, summary, report.order[len(report.order)-1], "the summary comes last, once all is known")
}

// writeReport writes the report of failure into files with run, and
// returns what could not be collected.
func writeReport(t *testing.T, files report.Files, run runner.Runner, failure report.Failure) []string {
	t.Helper()

	return report.Write(t.Context(), files, run, failure)
}

func TestWriteSaysWhatItCouldNotCollect(t *testing.T) {
	t.Parallel()

	failure := failedUpdate(t)
	failure.Ran.Output = watch(t, failedBuild+">>> Failed to emerge dev-libs/bar-2.0\n")

	var fake runnertest.Fake

	fake.Script(
		runnertest.Answer{Result: runner.Result{ExitCode: 1}},
		runnertest.Answer{Err: errScripted},
		runnertest.Answer{},
		runnertest.Answer{},
		runnertest.Answer{},
	)

	files := newFiles()
	files.broken[emergePqv] = true

	problems := writeReport(t, files, &fake, failure)
	assert.Equal(t, []string{
		"the build log of app-misc/foo-1.2 could not be copied: cat exited with status 1",
		"emerge --info for app-misc/foo-1.2 could not run: " + errScripted.Error(),
		"could not write " + emergePqv + ": " + errDisk.Error(),
		"emerge named no build log for dev-libs/bar-2.0",
	}, problems)
	assert.Equal(t, []string{
		"cat -- " + failedLog + " (root) (saved)",
		"emerge --info =app-misc/foo-1.2::gentoo (saved)",
		"emerge --info =dev-libs/bar-2.0 (saved)",
		"emerge -pqv =dev-libs/bar-2.0 (saved)",
	}, queries(fake.Commands()), "a query without its file is not run; a package without a repository is named alone")

	assert.Contains(t, files.content(t, summary), "\nNot collected:\n"+
		"  the build log of app-misc/foo-1.2 could not be copied: cat exited with status 1\n"+
		"  emerge --info for app-misc/foo-1.2 could not run: "+errScripted.Error()+"\n"+
		"  could not write "+emergePqv+": "+errDisk.Error()+"\n"+
		"  emerge named no build log for dev-libs/bar-2.0\n")
	assert.Contains(t, files.content(t, summary), "  dev-libs/bar-2.0\n    build log      not named by emerge\n")
}

func TestWriteKeepsTheSuffixOfACompressedLog(t *testing.T) {
	t.Parallel()

	const log = "/var/log/portage/app-misc:foo-1.2:20261128-101010.log.gz"

	failure := failedUpdate(t)
	failure.Ran.Output = watch(t, ">>> Failed to emerge app-misc/foo-1.2, Log file:\n>>>  '"+log+"'\n")

	var fake runnertest.Fake

	files := newFiles()
	assert.Empty(t, writeReport(t, files, &fake, failure))
	assert.Equal(t, "cat -- "+log+" (root) (saved)", queries(fake.Commands())[0])
	assert.Contains(t, files.order, "app-misc_foo-1.2.build.log.gz", "copied as it is")
}

func TestWriteNamesTheFilesOfAPackageSafely(t *testing.T) {
	t.Parallel()

	failure := failedUpdate(t)
	failure.Ran.Output = watch(t, ">>> Failed to emerge ../../etc/x\n")

	var fake runnertest.Fake

	files := newFiles()
	writeReport(t, files, &fake, failure)
	assert.Contains(t, files.order, ".._.._etc_x.emerge-info.txt", "no file outside the report")
}

func TestWriteWithoutAFailedPackage(t *testing.T) {
	t.Parallel()

	failure := report.Failure{
		Step: "sync", Title: "sync the repositories", Reason: "emerge exited with status 1",
		Ran: report.Ran{
			Command: runner.Command{Args: []string{"emerge", "--sync", "gentoo"}, Mode: runner.Stream, Root: true},
			Result:  runner.Result{ExitCode: 1}, Err: nil, Output: watch(t, "rsync: connection refused\n"),
		},
		RanAny: true, Log: runLog,
	}

	var fake runnertest.Fake

	files := newFiles()
	assert.Empty(t, writeReport(t, files, &fake, failure))
	assert.Empty(t, fake.Commands())
	assert.Equal(t, []string{outputFile, summary}, files.order)
	assert.Equal(t, "rsync: connection refused\n", files.content(t, outputFile))
	assert.Contains(t, files.content(t, summary), "\nemerge named no package that failed.\n")
}

func TestWriteWithoutACommand(t *testing.T) {
	t.Parallel()

	failure := report.Failure{
		Step: "news", Title: "read the unread news items", Reason: "could not read the news items: permission denied",
		Ran: report.Ran{}, RanAny: false, Log: runLog,
	}

	var fake runnertest.Fake

	files := newFiles()
	assert.Empty(t, writeReport(t, files, &fake, failure))
	assert.Equal(t, []string{summary}, files.order)
	assert.Contains(t, files.content(t, summary), "Command:  none: the step failed before it ran one\n")
	assert.NotContains(t, files.content(t, summary), outputFile)
}

func TestWriteSaysWhyTheCommandStopped(t *testing.T) {
	t.Parallel()

	failure := failedUpdate(t)
	failure.Ran.Output = watch(t, "")
	failure.Ran.Err = errScripted

	var fake runnertest.Fake

	files := newFiles()
	writeReport(t, files, &fake, failure)
	assert.Contains(t, files.content(t, summary), "Command:  emerge --update @world, as root\n"+
		"Stopped:  "+errScripted.Error()+"\n")
}

func TestWriteSaysWhenTheSummaryCannotBeWritten(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	files := newFiles()
	files.broken[summary] = true

	failure := failedUpdate(t)
	failure.Ran.Output = watch(t, "something failed\n")

	assert.Equal(t, []string{"could not write " + summary + ": " + errDisk.Error()},
		writeReport(t, files, &fake, failure))
}
