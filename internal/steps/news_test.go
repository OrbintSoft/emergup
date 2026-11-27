// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"os"
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

// What the news step shows, asks and runs.
const (
	newsName     = "news"
	eselect      = "eselect"
	newsRecord   = "/var/lib/gentoo/news"
	newsQuestion = "==> Read them? [R/s/a/?] "
	goOnQuestion = "==> Go on with the run? [Y/a/?] "
	twoNewsRows  = "    2026-09-01  Foo moves\n    2026-10-02  [guru] Bar changes\n"
	noUnreadNews = "no unread news items"
	newsRead     = newsQuestion + "read them\n"
	wentOn       = goOnQuestion + "go on with the run\n"
	abortedRun   = "you aborted the run"
)

// newsFile is a news item with its title and the date it was posted.
func newsFile(title, posted string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("Title: " + title + "\nPosted: " + posted +
		"\nNews-Item-Format: 2.0\n\nWhat to do.\n")}
}

// newsRun is a run of the news step on a system whose repositories are
// those of testdata/repos_config.txt, where an item of gentoo and one of
// guru are unread.
type newsRun struct {
	cfg      config.Step
	yes      bool
	writable bool
	env      map[string]string
	keys     keys
	root     fstest.MapFS
	runner   runnertest.Fake
	out      bytes.Buffer
	// checked are the paths whose writability the step asked about.
	checked []string
}

func newNewsRun(t *testing.T, keys ...rune) *newsRun {
	t.Helper()

	reposConfig, err := os.ReadFile("testdata/repos_config.txt")
	require.NoError(t, err)

	run := &newsRun{
		cfg: config.Default().Steps.News,
		env: map[string]string{},
		root: fstest.MapFS{
			"var/lib/gentoo/news/news-gentoo.unread": {Data: []byte("2026-09-01-foo\n")},
			"var/lib/gentoo/news/news-guru.unread":   {Data: []byte("2026-10-02-bar\n")},
			"var/db/repos/gentoo/metadata/news/2026-09-01-foo/2026-09-01-foo.en.txt": newsFile(
				"Foo moves", "2026-09-01"),
			"var/db/repos/gentoo/metadata/news/2026-09-01-foo/2026-09-01-foo.it.txt": newsFile(
				"Foo si sposta", "2026-09-01"),
			"var/db/repos/guru/metadata/news/2026-10-02-bar/2026-10-02-bar.en.txt": newsFile(
				"Bar changes", "2026-10-02"),
		},
	}
	run.keys.t = t
	run.keys.keys = keys
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: reposConfig}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *newsRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.News = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		if step.Name() == newsName {
			return step
		}
	}

	t.Fatal("no news step")

	return nil
}

func (r *newsRun) pipelineEnv() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: false, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: &r.keys, Yes: r.yes},
		Now:     func() time.Time { return now },
		Root:    r.root,
		LookupEnv: func(name string) (string, bool) {
			value, isSet := r.env[name]

			return value, isSet
		},
		CanWrite: func(name string) bool {
			r.checked = append(r.checked, name)

			return r.writable
		},
	}
}

// readNewsArgs returns the arguments of program that reads the unread items
// with options.
func readNewsArgs(program string, options ...string) []string {
	return append(append([]string{program, newsName, "read"}, options...), "new")
}

// readNews returns the command that shows the unread items and marks them
// read, elevated when root is true.
func readNews(root bool) runner.Command {
	return runner.Command{Args: readNewsArgs(eselect), Mode: runner.Stream, Root: root}
}

func TestNewsListsAsksReadsThenAsksWhetherToGoOn(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, ui.KeyEnter, ui.KeyEnter)

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, twoNewsRows+newsRead+wentOn, run.out.String())
	assert.Equal(t, []runner.Command{listRepositories, readNews(true)}, run.runner.Commands())
	assert.Equal(t, []string{newsRecord}, run.checked)
}

func TestNewsReadsThemWithoutElevationWhenTheUserCanWriteTheRecord(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, 'r', 'y')
	run.writable = true
	run.cfg.Command = readNewsArgs(eselect, "--raw")

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{listRepositories, {
		Args: readNewsArgs(eselect, "--raw"), Mode: runner.Stream, Root: false,
	}}, run.runner.Commands())
}

func TestNewsGoesOnLeavingThemUnread(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, 's')

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Skip("you chose to leave them unread"), outcome)
	assert.Equal(t, twoNewsRows+newsQuestion+"go on, leaving them unread\n", run.out.String())
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
}

func TestNewsStopsTheRunAtEitherQuestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keys     []rune
		out      string
		commands []runner.Command
	}{
		{"abort instead of reading", []rune{'a'}, newsQuestion + "abort the run\n", []runner.Command{listRepositories}},
		{"the end of the input instead of reading", nil, newsQuestion + "\n", []runner.Command{listRepositories}},
		{
			"abort after reading",
			[]rune{ui.KeyEnter, 'a'},
			newsRead + goOnQuestion + "abort the run\n",
			[]runner.Command{listRepositories, readNews(true)},
		},
		{
			"the end of the input after reading",
			[]rune{ui.KeyEnter},
			newsRead + goOnQuestion + "\n",
			[]runner.Command{listRepositories, readNews(true)},
		},
	}

	for _, test := range tests {
		run := newNewsRun(t, test.keys...)

		outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
		assert.Equal(t, pipeline.Abort(abortedRun), outcome, test.name)
		assert.Equal(t, twoNewsRows+test.out, run.out.String(), test.name)
		assert.Equal(t, test.commands, run.runner.Commands(), test.name)
	}
}

func TestNewsIsSkippedWithoutAQuestionWhenNothingIsUnread(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t)
	run.keys.silent = true
	run.root = fstest.MapFS{"var/lib/gentoo/news/news-gentoo.unread": {Data: []byte("\n")}}

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Skip(noUnreadNews), outcome)
	assert.Empty(t, run.out.String())
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
}

func TestNewsUnderYesListsThemAndLeavesThemUnread(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t)
	run.keys.silent = true
	run.yes = true

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Skip("left unread: --yes reads no news"), outcome)
	assert.Equal(t, twoNewsRows, run.out.String())
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
}

func TestNewsFailsWhenTheReadFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		program string
		answer  runnertest.Answer
		reason  string
	}{
		{"an exit status", eselect, runnertest.Answer{Result: runner.Result{ExitCode: 1}}, "eselect exited with status 1"},
		{
			"an exit status, the program by its path", "/usr/bin/eselect",
			runnertest.Answer{Result: runner.Result{ExitCode: 2}},
			"eselect exited with status 2",
		},
		{"a command that does not start", eselect, runnertest.Answer{Err: errCannotStart}, errCannotStart.Error()},
	}

	for _, test := range tests {
		run := newNewsRun(t, ui.KeyEnter)
		run.cfg.Command = readNewsArgs(test.program)
		run.runner.Script(test.answer)

		outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
		assert.Equal(t, pipeline.Fail(test.reason), outcome, test.name)
		assert.Equal(t, twoNewsRows+newsRead, run.out.String(), "no question after a failure: "+test.name)
	}
}

func TestNewsFailsWhenTheRepositoriesCannotBeListed(t *testing.T) {
	t.Parallel()

	run := &newsRun{cfg: config.Default().Steps.News, env: map[string]string{}}
	run.keys.t = t
	run.keys.silent = true
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})

	outcome := run.step(t).Run(t.Context(), run.pipelineEnv())
	assert.Equal(t, pipeline.Fail("could not list the repositories: portageq exited with status 1"), outcome)
	assert.Empty(t, run.out.String())
}

func TestNewsShowsTheItemsInTheLanguageOfTheUser(t *testing.T) {
	t.Parallel()

	for _, env := range []map[string]string{
		{"LANG": "it_IT.UTF-8"},
		{"LC_ALL": "it_IT.UTF-8", "LANG": "en_US.UTF-8"},
	} {
		run := newNewsRun(t, 's')
		run.env = env

		run.step(t).Run(t.Context(), run.pipelineEnv())
		assert.Contains(t, run.out.String(), "    2026-09-01  Foo si sposta\n    2026-10-02  [guru] Bar changes\n", env)
	}
}

func TestNewsConfirmsItself(t *testing.T) {
	t.Parallel()

	step := newNewsRun(t).step(t)
	assert.Equal(t, "read the unread news items", step.Title())

	confirms, isSelfConfirming := step.(pipeline.SelfConfirming)
	require.True(t, isSelfConfirming)
	assert.True(t, confirms.ConfirmsItself(), "no question when nothing is unread")
}

func TestNewsPretendListsThemAndSaysWhatItWouldRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		writable bool
		root     fstest.MapFS
		want     []string
	}{
		{
			"elevated", false, nil, []string{
				"2026-09-01  Foo moves", "2026-10-02  [guru] Bar changes",
				"would ask whether to read them, then run as root: eselect news read new",
			},
		},
		{
			"as the user", true, nil, []string{
				"2026-09-01  Foo moves", "2026-10-02  [guru] Bar changes",
				"would ask whether to read them, then run: eselect news read new",
			},
		},
		{"nothing unread", false, fstest.MapFS{}, []string{noUnreadNews}},
	}

	for _, test := range tests {
		run := newNewsRun(t)
		run.keys.silent = true
		run.writable = test.writable

		if test.root != nil {
			run.root = test.root
		}

		lines, err := run.step(t).Pretend(t.Context(), run.pipelineEnv())
		require.NoError(t, err, test.name)
		assert.Equal(t, test.want, lines, test.name)
		assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands(), "only the query: "+test.name)
		assert.Empty(t, run.out.String(), test.name)
	}
}
