// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// What the config-files step asks and writes.
const (
	configFilesName     = "config-files"
	configFilesQuestion = "==> Update them with dispatch-conf? [Y/s/a/?] "
	pendingRows         = "    /etc/conf.d/bar\n    /etc/foo.conf\n    /usr/share/gnupg/qualified.txt\n"
	findPattern         = "._cfg????_*"
)

// The commands of the config-files step.
var (
	//nolint:gochecknoglobals // The query every test expects.
	queryProtected = runner.Command{
		Args: []string{portageq, "envvar", "CONFIG_PROTECT", "CONFIG_PROTECT_MASK"}, Mode: runner.Capture, Root: false,
	}
	//nolint:gochecknoglobals // The search in the protected directories.
	findInDirs = runner.Command{
		Args: []string{"find", "/etc", "-name", findPattern, "-print"}, Mode: runner.Capture, Root: true,
	}
	//nolint:gochecknoglobals // The search next to the protected files.
	findNextToFiles = runner.Command{
		Args: []string{"find", "/usr/share/gnupg", "-maxdepth", "1", "-name", findPattern, "-print"},
		Mode: runner.Capture, Root: true,
	}
	//nolint:gochecknoglobals // The tool, attached to the terminal.
	dispatchConf = runner.Command{Args: []string{"dispatch-conf"}, Mode: runner.Terminal, Root: true}
)

// configFilesRun is a run of the config-files step where /etc and the
// file /usr/share/gnupg/qualified.txt are protected, /usr/share/missing
// is protected but does not exist, and /etc/env.d is masked.
type configFilesRun struct {
	cfg    config.Step
	yes    bool
	keys   keys
	runner runnertest.Fake
	out    bytes.Buffer
}

// answers are what portageq and the two finds print.
func protectedAnswers(inDirs, nextToFiles string) []runnertest.Answer {
	return []runnertest.Answer{
		{Result: runner.Result{ExitCode: 0, Output: []byte(
			"/etc /usr/share/gnupg/qualified.txt /usr/share/missing\n/etc/ca-certificates.conf /etc/env.d\n",
		)}},
		{Result: runner.Result{ExitCode: 0, Output: []byte(inDirs)}},
		{Result: runner.Result{ExitCode: 0, Output: []byte(nextToFiles)}},
	}
}

// newConfigFilesRun returns a run where three files are pending, and some
// that Portage does not count: a backup, an editor's copy, a masked one, a
// file next to a protected one but of another name.
func newConfigFilesRun(t *testing.T, keys ...rune) *configFilesRun {
	t.Helper()

	run := &configFilesRun{cfg: config.Default().Steps.ConfigFiles}
	run.keys.t = t
	run.keys.keys = keys
	run.runner.Script(protectedAnswers(
		"/etc/._cfg0000_foo.conf\n/etc/conf.d/._cfg0001_bar\n/etc/env.d/._cfg0000_99editor\n"+
			"/etc/._cfg0000_old~\n/etc/._cfg0002_saved.BAK\n/etc/._cfgabcd_notanumber\n",
		"/usr/share/gnupg/._cfg0000_qualified.txt\n/usr/share/gnupg/._cfg0000_other\n",
	)...)

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *configFilesRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.ConfigFiles = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		if step.Name() == configFilesName {
			return step
		}
	}

	t.Fatal("no config-files step")

	return nil
}

func (r *configFilesRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: false, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: &r.keys, Yes: r.yes},
		Now:     func() time.Time { return now },
		Root: fstest.MapFS{
			"etc/hostname":                  {Data: []byte("host\n")},
			"usr/share/gnupg/qualified.txt": {Data: []byte("\n")},
		},
		LookPath: nil,
	}
}

func TestConfigFilesListsAsksAndRunsTheTool(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t, ui.KeyEnter)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, pendingRows+configFilesQuestion+"update them\n", run.out.String(),
		"the files each pending update would replace")
	assert.Equal(t, []runner.Command{queryProtected, findInDirs, findNextToFiles, dispatchConf},
		run.runner.Commands(), "the searches elevated but read-only; the tool attached to the terminal")
}

func TestConfigFilesConfirmsItself(t *testing.T) {
	t.Parallel()

	confirms, isSelfConfirming := newConfigFilesRun(t).step(t).(pipeline.SelfConfirming)
	require.True(t, isSelfConfirming)
	assert.True(t, confirms.ConfirmsItself())
}

func TestConfigFilesSkipsWhenNothingIsPending(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t)
	run.keys.silent = true
	run.runner = runnertest.Fake{}
	run.runner.Script(protectedAnswers("/etc/._cfg0000_old~\n", "")...)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("no configuration file to update"), outcome, "without a question")
	assert.Empty(t, run.out.String())
	assert.Len(t, run.runner.Commands(), 3)
}

func TestConfigFilesWithYesDoesNotRunAnInteractiveTool(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t)
	run.yes = true
	run.keys.silent = true

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("dispatch-conf is interactive: run it to update them"), outcome)
	assert.Equal(t, pendingRows, run.out.String(), "the pending files are listed")
	assert.Len(t, run.runner.Commands(), 3, "the tool does not run")
}

func TestConfigFilesAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keys    []rune
		outcome pipeline.Outcome
	}{
		{"skip the update", []rune{'s'}, pipeline.Skip("you chose not to update them")},
		{caseAbort, []rune{'a'}, pipeline.Abort("you aborted the run")},
		{caseEndOfInput, nil, pipeline.Abort("you aborted the run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newConfigFilesRun(t, tt.keys...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, tt.outcome, outcome)
			assert.Len(t, run.runner.Commands(), 3, "the tool does not run")
		})
	}
}

func TestConfigFilesRunsTheConfiguredTool(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t, ui.KeyEnter)
	run.cfg.Command = []string{"/usr/bin/etc-update"}

	run.step(t).Run(t.Context(), run.env())
	assert.Contains(t, run.out.String(), "==> Update them with etc-update? [Y/s/a/?] ")
	require.Len(t, run.runner.Commands(), 4)
	assert.Equal(t, runner.Command{Args: []string{"/usr/bin/etc-update"}, Mode: runner.Terminal, Root: true},
		run.runner.Commands()[3])
}

func TestConfigFilesSearchesOnlyTheDirectories(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t, ui.KeyEnter)
	run.runner = runnertest.Fake{}
	run.runner.Script(
		runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: []byte("/etc\n\n")}},
		runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: []byte("/etc/._cfg0000_foo.conf\n")}},
	)

	run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, []runner.Command{queryProtected, findInDirs, dispatchConf}, run.runner.Commands(),
		"no search next to files when no file is protected")
}

func TestConfigFilesFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []runnertest.Answer
		want    string
	}{
		{
			whenPortageqFails,
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 1}}},
			"could not find the protected configuration: portageq exited with status 1",
		},
		{
			"when find fails",
			[]runnertest.Answer{protectedAnswers("", "")[0], {Result: runner.Result{ExitCode: 1}}},
			"could not look for the configuration files to update: find exited with status 1",
		},
		{
			"when the tool fails",
			append(protectedAnswers("/etc/._cfg0000_foo.conf\n", ""), runnertest.Answer{Result: runner.Result{ExitCode: 1}}),
			"dispatch-conf exited with status 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newConfigFilesRun(t, ui.KeyEnter)
			run.runner = runnertest.Fake{}
			run.runner.Script(tt.answers...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Fail(tt.want), outcome)
		})
	}
}

func TestConfigFilesPretendElevatesNothing(t *testing.T) {
	t.Parallel()

	run := newConfigFilesRun(t)
	run.keys.silent = true

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{"would look for the pending configuration files, as root, then run: dispatch-conf"}, lines)
	assert.Empty(t, run.runner.Commands())
}
