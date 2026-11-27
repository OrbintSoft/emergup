// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// What `emergup news` asks.
const (
	commandQuestion = "==> Read them? [R/q/?] "
	oldNewsRow      = "    2026-08-01  Old news (read)\n"
)

// errNoElevation is why a system has no way to run a command as root.
var errNoElevation = errors.New("no elevation tool found: install sudo, doas or run0, or run emergup as root")

// news runs `emergup news` on the system of the run, with --all when all
// is true.
func (r *newsRun) news(t *testing.T, all bool) pipeline.Outcome {
	t.Helper()

	return steps.News(t.Context(), r.pipelineEnv(), steps.NewsOptions{
		Config: r.cfg,
		All:    all,
		Elevated: func() (runner.Runner, error) {
			r.elevations++
			if r.noElevation != nil {
				return nil, r.noElevation
			}

			return &r.elevated, nil
		},
	})
}

// withOldNews adds an item of gentoo already read.
func (r *newsRun) withOldNews() {
	r.root["var/lib/gentoo/news/news-gentoo.read"] = &fstest.MapFile{Data: []byte("2026-08-01-old\n")}
	r.root["var/db/repos/gentoo/metadata/news/2026-08-01-old/2026-08-01-old.en.txt"] = newsFile(
		"Old news", "2026-08-01")
}

func TestNewsCommandListsAsksAndReadsThemAsRoot(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, ui.KeyEnter)

	outcome := run.news(t, false)
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, twoNewsRows+commandQuestion+"read them\n", run.out.String(), "no question after reading")
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
	assert.Equal(t, []runner.Command{readNewsAsRoot()}, run.elevated.Commands(), "the read runs as root")
	assert.Equal(t, 1, run.elevations)
}

func TestNewsCommandReadsThemAsAUserWhoCanWriteTheRecord(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, 'r')
	run.writable = true
	run.cfg.Command = readNewsArgs(eselect, "--raw")

	outcome := run.news(t, false)
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{listRepositories, {
		Args: readNewsArgs(eselect, "--raw"), Mode: runner.Stream, Root: false,
	}}, run.runner.Commands())
	assert.Zero(t, run.elevations, "no elevation is asked for")
}

func TestNewsCommandQuitsLeavingThemUnread(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, 'q')

	outcome := run.news(t, false)
	assert.Equal(t, pipeline.Skip("you chose to leave them unread"), outcome)
	assert.Equal(t, twoNewsRows+commandQuestion+"quit, leaving them unread\n", run.out.String())
	assert.Empty(t, run.elevated.Commands())
}

func TestNewsCommandStopsAtCtrlCOrTheEndOfTheInput(t *testing.T) {
	t.Parallel()

	for name, keys := range map[string][]rune{"Ctrl-C": {ui.KeyInterrupt}, "the end of the input": nil} {
		run := newNewsRun(t, keys...)

		outcome := run.news(t, false)
		assert.Equal(t, pipeline.Abort(abortedRun), outcome, name)
		assert.Empty(t, run.elevated.Commands(), name)
	}
}

func TestNewsCommandWithoutAWayToRunAsRootSaysSoWithoutAQuestion(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t)
	run.keys.silent = true
	run.noElevation = errNoElevation

	outcome := run.news(t, false)
	assert.Equal(t, pipeline.Fail(errNoElevation.Error()), outcome)
	assert.Equal(t, twoNewsRows, run.out.String(), "the rows, then no question")
}

func TestNewsCommandSaysWhenThereIsNothingToList(t *testing.T) {
	t.Parallel()

	unread := newNewsRun(t)
	unread.keys.silent = true
	unread.root = fstest.MapFS{}
	assert.Equal(t, pipeline.Skip(noUnreadNews), unread.news(t, false))
	assert.Empty(t, unread.out.String())
	assert.Zero(t, unread.elevations)

	all := newNewsRun(t)
	all.keys.silent = true
	all.root = fstest.MapFS{}
	assert.Equal(t, pipeline.Skip("no news items"), all.news(t, true))
	assert.Empty(t, all.out.String())
}

func TestNewsCommandAllListsEveryItemAndAsksNothing(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t)
	run.keys.silent = true
	run.withOldNews()

	outcome := run.news(t, true)
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, oldNewsRow+twoNewsRows, run.out.String())
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
	assert.Zero(t, run.elevations)

	readOnly := newNewsRun(t)
	readOnly.keys.silent = true
	readOnly.root = fstest.MapFS{}
	readOnly.withOldNews()
	assert.Equal(t, pipeline.Success(), readOnly.news(t, true), "the read ones are listed with none unread")
	assert.Equal(t, oldNewsRow, readOnly.out.String())
}

func TestNewsCommandListsOnlyTheUnreadOnesWithoutAll(t *testing.T) {
	t.Parallel()

	run := newNewsRun(t, 'q')
	run.withOldNews()

	run.news(t, false)
	assert.NotContains(t, run.out.String(), "Old news")
}

func TestNewsCommandFails(t *testing.T) {
	t.Parallel()

	read := newNewsRun(t, ui.KeyEnter)
	read.elevated.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})
	assert.Equal(t, pipeline.Fail("eselect exited with status 1"), read.news(t, false), "when the read fails")

	list := &newsRun{cfg: read.cfg, env: map[string]string{}}
	list.keys.t = t
	list.keys.silent = true
	list.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})
	assert.Equal(t, pipeline.Fail("could not list the repositories: portageq exited with status 1"), list.news(t, true),
		"when the repositories cannot be listed")
}
