// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The news item that snapshot-news brings to the test repository, and how
// the news step shows it and asks about it.
const (
	newsItem       = "2026-11-01-emergup-test"
	newsTitle      = "Synthetic news of the test repository"
	newsRow        = "    2026-11-01  [emergup-test] " + newsTitle + "\n"
	newsHeading    = "==> [1/1] news: read the unread news items\n"
	readQuestion   = "==> Read them? [R/s/a/?] "
	goOnQuestion   = "==> Go on with the run? [Y/a/?] "
	noUnreadNews   = "==> news skipped: no unread news items\n"
	newsRecord     = "/var/lib/gentoo/news/news-emergup-test"
	readAndGoOn    = "r\ny\n"
	stoppedRun     = 130
	elevatedRead   = "sudo eselect news read new"
	unprivilegedIn = "eselect news read new"
)

// withNews leaves only the news step enabled and brings the news item of
// the test repository, which the sync marks unread.
func withNews(ctr *container) {
	ctr.t.Helper()

	ctr.dropIns("news-only")
	ctr.publishTree("snapshot-news")
	ctr.syncRepository()
}

// newsList returns the items of the test repository that Portage lists as
// kind, read or unread: none when it has no such list.
func (c *container) newsList(kind string) string {
	c.t.Helper()

	got := c.exec(root, "", "cat", newsRecord+"."+kind)
	if got.code != 0 {
		return ""
	}

	return strings.TrimSpace(got.stdout)
}

// ranCommand matches the line of the log of a run that says it ran
// command.
func ranCommand(command string) *regexp.Regexp {
	return regexp.MustCompile(`\n==> \S+ ` + regexp.QuoteMeta(command) + `\n`)
}

// NEWS-1: with no unread news item, the news step is skipped without a
// question.
func TestNEWS1_WithoutUnreadNewsTheStepAsksNothing(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("news-only")

	stdout := ctr.emergupRun("")
	assertInOrder(t, stdout, newsHeading, noUnreadNews)
	assert.NotContains(t, stdout, readQuestion)
}

// NEWS-1: the unread news items are listed; read, they are shown and
// marked read, and emergup asks whether to go on with the run.
func TestNEWS1_TheUnreadNewsAreListedReadThenTheRunAsks(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNews(ctr)

	read := ctr.emergupRun(readAndGoOn)
	assertInOrder(t, read,
		newsHeading, newsRow, readQuestion+"read them\n",
		newsTitle, "A news item of the test repository, which the news tests of emergup",
		goOnQuestion+"go on with the run\n", "==> news succeeded in ")
	assert.Equal(t, newsItem, ctr.newsList("read"), "the item is marked read")
	assert.Empty(t, ctr.newsList("unread"))
	assert.Regexp(t, ranCommand(elevatedRead), ctr.runLog(tester, read),
		"tester is not in the portage group: the read runs as root")

	again := ctr.emergupRun("")
	assert.Contains(t, again, noUnreadNews, "a read item is not listed again")
}

// NEWS-1: the unread news items can be left unread, and the run can be
// stopped at either question.
func TestNEWS1_TheNewsCanBeLeftUnreadOrStopTheRun(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNews(ctr)

	left := ctr.emergupRun("s\n")
	assertInOrder(t, left, newsRow, readQuestion+"go on, leaving them unread\n",
		"==> news skipped: you chose to leave them unread\n")
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")

	aborted := ctr.emergupInput("a\n", "run")
	assert.Equal(t, stoppedRun, aborted.code, "the exit status, with the output:\n%s%s", aborted.stdout, aborted.stderr)
	assert.Contains(t, aborted.stdout, readQuestion+"abort the run\n")
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")

	afterReading := ctr.emergupInput("r\na\n", "run")
	assert.Equal(t, stoppedRun, afterReading.code,
		"the exit status, with the output:\n%s%s", afterReading.stdout, afterReading.stderr)
	assertInOrder(t, afterReading.stdout, readQuestion+"read them\n", newsTitle, goOnQuestion+"abort the run\n")
	assert.Equal(t, newsItem, ctr.newsList("read"), "the item was read before the run stopped")
}

// NEWS-1: with `--yes` the unread news items are listed, in the log too,
// and left unread.
func TestNEWS1_UnderYesTheNewsAreListedAndLeftUnread(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNews(ctr)

	yes := ctr.emergupRun("", "--yes")
	assertInOrder(t, yes, newsHeading, newsRow, "==> news skipped: left unread: --yes reads no news\n")
	assert.NotContains(t, yes, readQuestion)
	assert.Contains(t, ctr.runLog(tester, yes), newsRow)
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")
}

// NEWS-1: a user who can write Portage's record of the news read, as a
// member of the portage group can, reads them without elevation.
func TestNEWS1_AMemberOfThePortageGroupReadsThemWithoutElevation(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNews(ctr)
	ctr.root("usermod", "--append", "--groups", "portage", tester)

	read := ctr.emergupRun(readAndGoOn)
	log := ctr.runLog(tester, read)
	assert.Regexp(t, ranCommand(unprivilegedIn), log)
	assert.NotContains(t, log, elevatedRead)
	assert.Equal(t, newsItem, ctr.newsList("read"), "the item is marked read")
}

// NEWS-1: `--pretend` lists the unread news items and says what would run,
// and as whom, without reading them.
func TestNEWS1_PretendListsTheNewsAndReadsNone(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNews(ctr)

	got := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)
	assertInOrder(t, got.stdout, newsHeading, newsRow,
		"    would ask whether to read them, then run as root: eselect news read new\n")
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")
}

// The question of `emergup news`, and the row of the item once read.
const (
	commandQuestion = "==> Read them? [R/q/?] "
	readNewsRow     = "    2026-11-01  [emergup-test] " + newsTitle + " (read)\n"
)

// withNewsForTheCommand brings the news item of the test repository, which
// the sync marks unread, with every step of a run disabled: the command
// does not depend on the news step of runs.
func withNewsForTheCommand(ctr *container) {
	ctr.t.Helper()

	ctr.dropIns("all-disabled")
	ctr.publishTree("snapshot-news")
	ctr.syncRepository()
}

// NEWS-2: `emergup news` lists the unread news items and reads them, as
// root for a user outside the portage group, marking them read; `--all`
// lists every item, those read marked, and asks nothing.
func TestNEWS2_TheCommandListsTheUnreadNewsAndReadsThem(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNewsForTheCommand(ctr)

	before := ctr.emergup("news", "--all")
	assert.Equal(t, 0, before.code, "--all asks nothing, with the output:\n%s%s", before.stdout, before.stderr)
	assert.Contains(t, before.stdout, newsRow, "unread, not marked")
	assert.NotContains(t, before.stdout, commandQuestion)

	read := ctr.emergupInput("r\n", "news")
	assert.Equal(t, 0, read.code, "the exit status, with the output:\n%s%s", read.stdout, read.stderr)
	assertInOrder(t, read.stdout, newsRow, commandQuestion+"read them\n", newsTitle,
		"A news item of the test repository, which the news tests of emergup")
	assert.NotContains(t, read.stdout, goOnQuestion, "no run to go on with")
	assert.Equal(t, newsItem, ctr.newsList("read"), "the item is marked read, which needs root for tester")
	assert.Empty(t, ctr.newsList("unread"))

	again := ctr.emergup("news")
	assert.Equal(t, 0, again.code)
	assert.Equal(t, "==> no unread news items\n", again.stdout)

	after := ctr.emergup("news", "--all")
	assert.Equal(t, 0, after.code)
	assert.Contains(t, after.stdout, readNewsRow)
}

// NEWS-2: `emergup news` can leave the items unread, and exits with 130
// at the end of the input.
func TestNEWS2_TheCommandLeavesThemUnreadOrStops(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNewsForTheCommand(ctr)

	left := ctr.emergupInput("q\n", "news")
	assert.Equal(t, 0, left.code, "the exit status, with the output:\n%s%s", left.stdout, left.stderr)
	assert.Contains(t, left.stdout, commandQuestion+"quit, leaving them unread\n==> you chose to leave them unread\n")
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")

	stopped := ctr.emergup("news")
	assert.Equal(t, stoppedRun, stopped.code, "the exit status, with the output:\n%s%s", stopped.stdout, stopped.stderr)
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")
}

// NEWS-2: a member of the portage group reads the items without an
// elevation tool; a user who needs one and has none is told so, and is
// not asked.
func TestNEWS2_TheCommandElevatesOnlyWhenItMust(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withNewsForTheCommand(ctr)
	withoutSudo(ctr)

	refused := ctr.emergupInput("r\n", "news")
	assert.Equal(t, 1, refused.code, "the exit status, with the output:\n%s%s", refused.stdout, refused.stderr)
	assertInOrder(t, refused.stdout, newsRow, "==> marking them read needs root: no elevation tool found: "+
		"install sudo, doas or run0, or run emergup as root\n")
	assert.NotContains(t, refused.stdout, commandQuestion)
	assert.Equal(t, newsItem, ctr.newsList("unread"), "the item stays unread")

	ctr.root("usermod", "--append", "--groups", "portage", tester)

	read := ctr.emergupInput("r\n", "news")
	assert.Equal(t, 0, read.code, "the exit status, with the output:\n%s%s", read.stdout, read.stderr)
	assert.Equal(t, newsItem, ctr.newsList("read"), "the item is marked read, without sudo")
}
