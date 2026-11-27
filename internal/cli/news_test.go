// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// What `emergup news` shows and asks on the system of newNewsSession.
const (
	commandNews  = "news"
	optionAll    = "--all"
	unreadRow    = "    2026-11-01  Foo moves\n"
	readRow      = "    2026-10-01  Old news (read)\n"
	readQuestion = "==> Read them? [R/q/?] "
	newsRecord   = "/var/lib/gentoo/news"
	eselect      = "eselect"
)

// newsFile is a news item with its title and the date it was posted.
func newsFile(title, posted string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("Title: " + title + "\nPosted: " + posted +
		"\nNews-Item-Format: 2.0\n\nWhat to do.\n")}
}

// newNewsSession returns a session whose system has the gentoo repository,
// with an unread news item and a read one, and a keyboard that answers
// with keys; the files of its root are those of the map it returns.
func newNewsSession(t *testing.T, keys ...rune) (*session, fstest.MapFS) {
	t.Helper()

	sess := newSession(t, keys...)
	sess.runner.outputs[kindRepository] = "[gentoo]\nlocation = /var/db/repos/gentoo\nsync-type = rsync\n"
	root := fstest.MapFS{
		"var/lib/gentoo/news/news-gentoo.unread": {Data: []byte("2026-11-01-foo\n")},
		"var/lib/gentoo/news/news-gentoo.read":   {Data: []byte("2026-10-01-old\n")},
		"var/db/repos/gentoo/metadata/news/2026-11-01-foo/2026-11-01-foo.en.txt": newsFile(
			"Foo moves", "2026-11-01"),
		"var/db/repos/gentoo/metadata/news/2026-10-01-old/2026-10-01-old.en.txt": newsFile(
			"Old news", "2026-10-01"),
	}
	sess.root = root

	return sess, root
}

// readNews returns the command that shows the unread items and marks them
// read, after prefix, which elevates it.
func readNews(prefix ...string) runner.Command {
	return runner.Command{
		Args: append(prefix, eselect, "news", "read", "new"), Mode: runner.Stream, Root: len(prefix) > 0,
	}
}

func TestNewsListsTheUnreadItemsAndReadsThemThroughTheElevation(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t, ui.KeyEnter)

	code, stdout, stderr := sess.run(t, commandNews)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, unreadRow+readQuestion+"read them\n", stdout)
	assert.Equal(t, []runner.Command{listRepositories, readNews(sudo)}, sess.runner.Commands())
	assert.Empty(t, sess.runs.log.String(), "no run directory, no log")
}

func TestNewsReadsThemWithoutElevationForAUserWhoCanWriteTheRecord(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t, 'r')
	sess.system.programs = nil
	sess.system.writable = []string{newsRecord}

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code, "no elevation tool is needed:\n%s", stdout)
	assert.Equal(t, []runner.Command{listRepositories, readNews()}, sess.runner.Commands())
}

func TestNewsWithoutAnElevationToolSaysSoAfterTheRows(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t)
	sess.system.programs = nil
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 1, code)
	assert.Equal(t, unreadRow+"==> marking them read needs root: "+noTool+"\n", stdout, "no question")
	assert.Equal(t, []runner.Command{listRepositories}, sess.runner.Commands())
}

func TestNewsQuitsLeavingThemUnread(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t, 'q')

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code)
	assert.Equal(t, unreadRow+readQuestion+"quit, leaving them unread\n==> you chose to leave them unread\n", stdout)
	assert.Equal(t, []runner.Command{listRepositories}, sess.runner.Commands())
}

func TestNewsExitsWith130WhenItIsStopped(t *testing.T) {
	t.Parallel()

	for name, keys := range map[string][]rune{"Ctrl-C": {ui.KeyInterrupt}, "the end of the input": nil} {
		sess, _ := newNewsSession(t, keys...)

		code, stdout, _ := sess.run(t, commandNews)
		assert.Equal(t, 130, code, name)
		assert.Equal(t, unreadRow+readQuestion+"\n", stdout, name)
	}
}

func TestNewsSaysWhenThereIsNothingUnread(t *testing.T) {
	t.Parallel()

	sess, root := newNewsSession(t)
	delete(root, "var/lib/gentoo/news/news-gentoo.unread")

	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> no unread news items\n", stdout)
}

func TestNewsAllListsEveryItemAndAsksNothing(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t)
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandNews, optionAll)
	assert.Equal(t, 0, code)
	assert.Equal(t, readRow+unreadRow, stdout)
	assert.Equal(t, []runner.Command{listRepositories}, sess.runner.Commands())

	empty := newSession(t)
	empty.keys.silent = true

	code, stdout, _ = empty.run(t, commandNews, optionAll)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> no news items\n", stdout)
}

func TestNewsAsRootSaysSoOnceAndReadsThemDirectly(t *testing.T) {
	t.Parallel()

	sess, _ := newNewsSession(t, ui.KeyEnter)
	sess.system = system{programs: nil, euid: 0, writable: []string{newsRecord}}

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code)
	assert.Equal(t, runningAsRoot+unreadRow+readQuestion+"read them\n", stdout)
	assert.Equal(t, []runner.Command{listRepositories, readNews()}, sess.runner.Commands())
}

func TestNewsFollowsTheConfiguration(t *testing.T) {
	t.Parallel()

	sess, root := newNewsSession(t, ui.KeyEnter)
	root[configFile] = &fstest.MapFile{Data: []byte(
		"[elevation]\nkeep-env = [\"EDITOR\"]\n\n[steps.news]\nenabled = false\n" +
			"command = [\"eselect\", \"news\", \"read\", \"--raw\", \"new\"]\n",
	)}
	sess.system.env = map[string]string{"EDITOR": "vi"}

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code, stdout)
	assert.Equal(t, []runner.Command{listRepositories, {
		Args: []string{sudo, "--preserve-env=EDITOR", eselect, "news", "read", "--raw", "new"},
		Mode: runner.Stream, Root: true,
	}}, sess.runner.Commands(), "the step's command, kept variables too, though the step is off for runs")
}

func TestNewsThroughDoasSaysWhatItMustKeep(t *testing.T) {
	t.Parallel()

	sess, root := newNewsSession(t, ui.KeyEnter)
	root[configFile] = &fstest.MapFile{Data: []byte(doasMethod + doasKeep)}
	sess.system.programs = []string{doas}

	code, stdout, _ := sess.run(t, commandNews)
	assert.Equal(t, 0, code, stdout)
	assert.Equal(t, unreadRow+doasNotice(false)+readQuestion+"read them\n", stdout)
	assert.Equal(t, []runner.Command{listRepositories, readNews(doas)}, sess.runner.Commands())
}

func TestNewsWithAProblemInTheConfiguration(t *testing.T) {
	t.Parallel()

	sess, root := newNewsSession(t)
	root[configFile] = &fstest.MapFile{Data: []byte(invalidSyncKey)}
	sess.keys.silent = true

	code, stdout, stderr := sess.run(t, commandNews)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "1 problem in the configuration.\n")
}

func TestNewsRejectsWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"--bogus", "everything"} {
		sess, _ := newNewsSession(t)
		sess.keys.silent = true

		code, stdout, stderr := sess.run(t, commandNews, arg)
		assert.Equal(t, 2, code, arg)
		assert.Empty(t, stdout, arg)
		assert.Contains(t, stderr, arg, arg)
		assert.Contains(t, stderr, usageLine, arg)
	}
}

func TestHelpNamesTheNewsCommand(t *testing.T) {
	t.Parallel()

	_, stdout, _ := newSession(t).run(t, "help")
	assert.Contains(t, stdout, "\n  news           list the unread news items and offer to read them\n"+
		"    --all          list every news item, those read too, and ask nothing\n")
}
