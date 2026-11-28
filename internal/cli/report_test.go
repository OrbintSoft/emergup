// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/cli"
	"github.com/OrbintSoft/emergup/internal/runner"
)

// The failure of the update of the tests: the package that fails, its log,
// and what emerge shows.
const (
	failedCPV   = "app-misc/foo-1.2"
	failedLog   = "/var/tmp/portage/app-misc/foo-1.2/temp/build.log"
	reportDir   = runDir + "/failure"
	reportLine  = "==> The failure report is in " + reportDir + "\n"
	failedBuild = " * ERROR: app-misc/foo-1.2::gentoo failed (compile phase):\n" +
		">>> Failed to emerge app-misc/foo-1.2, Log file:\n>>>  '" + failedLog + "'\n"
	realUpdate = "sudo emerge"
)

// The errors of the fake report directories.
var (
	errNoReport = errors.New("permission denied")
	errExists   = errors.New("file exists")
)

// reports is where a session's failure reports go: in memory, each
// directory opened once; err, when set, keeps any from opening.
type reports struct {
	opened []string
	dirs   map[string]*memoryReport
	err    error
}

//nolint:ireturn // What cli.Env asks for.
func (r *reports) open(path string) (cli.ReportDir, error) {
	if r.err != nil {
		return nil, r.err
	}

	r.opened = append(r.opened, path)
	if r.dirs == nil {
		r.dirs = map[string]*memoryReport{}
	}

	dir := &memoryReport{files: map[string]*bytes.Buffer{}, closed: false}
	r.dirs[path] = dir

	return dir, nil
}

// memoryReport is a report's directory in memory.
type memoryReport struct {
	files  map[string]*bytes.Buffer
	closed bool
}

func (d *memoryReport) Create(name string) (io.WriteCloser, error) {
	if _, exists := d.files[name]; exists {
		return nil, fmt.Errorf("%w: %s", errExists, name)
	}

	d.files[name] = &bytes.Buffer{}

	return nopCloser{d.files[name]}, nil
}

func (d *memoryReport) Close() error {
	d.closed = true

	return nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// withFailedUpdate makes the update of sess fail as failedBuild shows,
// with a build log and the queries of the report answered.
func withFailedUpdate(sess *session) {
	sess.runner.outputs[kindPretend] = "Total: 1 package (1 upgrade)\n"
	sess.runner.codes = map[string]int{realUpdate: 1}
	sess.runner.shown = map[string]string{realUpdate: failedBuild}
	sess.runner.outputs["sudo cat"] = "the build log\n"
	sess.runner.outputs["emerge --info"] = "Portage 3.0.83\n"
	sess.runner.outputs["emerge -pqv"] = "[ebuild     U  ] app-misc/foo-1.2\n"
}

// savedCommands returns the arguments of the Save commands of sess, and
// whether each ran as root.
func savedCommands(sess *session) []string {
	var saved []string

	for _, command := range sess.runner.Commands() {
		if command.Mode == runner.Save {
			saved = append(saved, fmt.Sprintf("%v root=%t", command.Args, command.Root))
		}
	}

	return saved
}

func TestRunWritesAFailureReportWhenAStepFails(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	withFailedUpdate(sess)

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "==> update failed after <1s: emerge exited with status 1\n")
	assert.Contains(t, stdout, "\n==> The run failed after <1s.\n"+reportLine+logLine,
		"the report comes after the summary, before the log, which stays last")

	require.Equal(t, []string{reportDir}, sess.reports.opened)

	dir := sess.reports.dirs[reportDir]
	assert.True(t, dir.closed)
	assert.Contains(t, dir.files["summary.txt"].String(), "Step:     update (update @world)\n"+
		"Failure:  emerge exited with status 1\n")
	assert.Contains(t, dir.files["summary.txt"].String(), "Run log:  "+runDir+"/emergup.log\n")
	assert.Equal(t, failedBuild, dir.files["output.txt"].String())
	assert.Equal(t, "the build log\n", dir.files["app-misc_foo-1.2.build.log"].String())
	assert.Equal(t, "Portage 3.0.83\n", dir.files["app-misc_foo-1.2.emerge-info.txt"].String())

	assert.Equal(t, []string{
		"[sudo cat -- " + failedLog + "] root=true",
		"[emerge --info =app-misc/foo-1.2::gentoo] root=false",
		"[emerge -pqv =app-misc/foo-1.2::gentoo] root=false",
	}, savedCommands(sess), "the log through the elevation, the queries unprivileged")
}

func TestRunSaysWhatTheFailureReportLacks(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	withFailedUpdate(sess)
	sess.runner.codes["sudo cat"] = 1

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "==> update failed after <1s: emerge exited with status 1\n"+
		"==> not in the failure report: the build log of "+failedCPV+" could not be copied: cat exited with status 1\n",
		"said right after the failure")
	assert.Contains(t, stdout, reportLine+logLine)
}

func TestRunWithoutAPlaceForTheFailureReport(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	withFailedUpdate(sess)
	sess.reports.err = errNoReport

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code, "the run failed, not a second time")
	assert.Contains(t, stdout, "==> update failed after <1s: emerge exited with status 1\n"+
		"==> no failure report: "+errNoReport.Error()+"\n")
	assert.NotContains(t, stdout, "The failure report is in")
	assert.Empty(t, savedCommands(sess))
}

func TestRunWritesNoFailureReportWithoutAFailedStep(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.Empty(t, sess.reports.opened)
	assert.NotContains(t, stdout, "failure report")
}

func TestRunWritesNoFailureReportForAFailedHook(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	sess.root = fstest.MapFS{configFile: {Data: []byte(
		"[hooks.check]\nat = \"before-update\"\ncommand = [\"/usr/local/bin/check\"]\n",
	)}}
	sess.runner.codes = map[string]int{"/usr/local/bin/check": 1}

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "The hook check (before update) failed")
	assert.Empty(t, sess.reports.opened, "a hook is the user's own command, its output is in the log")
}

func TestPretendWritesNoFailureReport(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	withFailedUpdate(sess)

	code, _, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.Empty(t, sess.reports.opened)
}
