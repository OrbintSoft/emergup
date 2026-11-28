// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// newsState is where Portage records, for each repository, its news items
// relevant to the system: news-<repository>.unread names those not read
// yet. Portage writes the lists at every sync; everyone can read them.
const newsState = "var/lib/gentoo/news"

// The repository whose news items a row does not name.
const mainRepository = "gentoo"

// The reasons of a news step that reads no news.
const (
	noUnreadNews   = "no unread news items"
	newsLeftUnread = "you chose to leave them unread"
	newsUnderYes   = "left unread: --yes reads no news"
)

// newsStep lists the unread news items and asks whether to read them;
// once they are read, it asks whether to go on, so that the actions they
// ask for can be taken before the update.
type newsStep struct {
	config config.Step
}

// Name implements pipeline.Step.
func (newsStep) Name() string { return "news" }

// Title implements pipeline.Step.
func (newsStep) Title() string { return "read the unread news items" }

// Enabled implements pipeline.Step.
func (n newsStep) Enabled() bool { return n.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the step asks once it
// knows there are unread items, and skips without asking otherwise.
func (newsStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step. Listing the items only reads files,
// so the pretend lists them, and says what reading them would run.
func (n newsStep) Pretend(ctx context.Context, env pipeline.Env) ([]string, error) {
	items, err := listNews(ctx, env, false)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return []string{noUnreadNews}, nil
	}

	lines := make([]string, 0, len(items)+1)
	for _, item := range items {
		lines = append(lines, item.row())
	}

	run := "run: "
	if newsNeedsRoot(env) {
		run = "run as root: "
	}

	return append(lines, "would ask whether to read them, then "+run+strings.Join(n.config.Command, " ")), nil
}

// Run implements pipeline.Step. Nobody reads the items under --yes: they
// are listed, so that the log has them, and left unread.
func (n newsStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	items, err := listNews(ctx, env, false)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if len(items) == 0 {
		return pipeline.Skip(noUnreadNews)
	}

	showNews(env, items)

	if env.Menu.Yes {
		return pipeline.Skip(newsUnderYes)
	}

	read := ui.Action{Key: 'r', Name: "read them"}
	leave := ui.Action{Key: 's', Name: "go on, leaving them unread"}

	if outcome, goOn := confirm(ctx, env, "Read them?", read, leave, newsLeftUnread); !goOn {
		return outcome
	}

	if outcome := readNews(ctx, env, n.config, newsNeedsRoot(env)); outcome.Status != pipeline.Succeeded {
		return outcome
	}

	return goOnAfterReading(ctx, env)
}

// newsNeedsRoot reports whether reading the items needs root: marking them
// read writes Portage's record of them, which root and the portage group
// can write.
func newsNeedsRoot(env pipeline.Env) bool {
	return !env.CanWrite("/" + newsState)
}

// listNews returns the unread news items, and with withRead the read ones
// too, in the language of the invoking user.
func listNews(ctx context.Context, env pipeline.Env, withRead bool) ([]newsItem, error) {
	repositories, err := listRepositories(ctx, env.Runner)
	if err != nil {
		return nil, err
	}

	lcAll, _ := env.LookupEnv("LC_ALL")
	lang, _ := env.LookupEnv("LANG")

	return newsItems(env.Root, repositories, newsLanguages(lcAll, lang), withRead)
}

// showNews writes a row for each of items.
func showNews(env pipeline.Env, items []newsItem) {
	for _, item := range items {
		env.Printer.Row(ui.KindPlain, item.row())
	}
}

// readNews runs the command of step, which shows the unread items and
// marks them read, as root when root is true, and returns how it ended.
func readNews(ctx context.Context, env pipeline.Env, step config.Step, root bool) pipeline.Outcome {
	command := runner.Command{
		Args: step.Command, Mode: runner.Stream, Root: root, Env: nil, Watch: nil, Into: nil, Stdin: nil, Dir: "",
	}

	return runCommand(ctx, env, command, filepath.Base(step.Command[0]))
}

// goOnAfterReading asks whether to go on with the run, once the items are
// read: a news item may ask for actions before the update, which the user
// takes at the question. Ctrl-C and the end of the input stop the run.
func goOnAfterReading(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	goOn := ui.Action{Key: 'y', Name: "go on with the run"}
	abort := ui.Action{Key: 'a', Name: "abort the run"}

	action, err := env.Menu.Ask(ctx, "Go on with the run?", goOn, abort)

	switch {
	case errors.Is(err, ui.ErrAborted), err == nil && action == abort:
		return pipeline.Abort(abortedRun)
	case err != nil:
		return pipeline.Fail(err.Error())
	}

	return pipeline.Success()
}

// newsItem is a news item, as a row shows it.
type newsItem struct {
	repository string
	// name is the item's directory, which starts with the date it was
	// posted: 2026-11-01-foo-migration.
	name string
	// posted is the date the item gives, or what stands for it.
	posted string
	title  string
	// read is whether Portage lists it as read.
	read bool
}

// row returns the item's line in a list of news items, as eselect writes
// it: the date, then the title, after the repository's name when it is not
// gentoo, and followed by (read) when the item is read.
func (i newsItem) row() string {
	title := i.title
	if i.repository != mainRepository {
		title = "[" + i.repository + "] " + title
	}

	if i.read {
		title += " (read)"
	}

	return fmt.Sprintf("%-10s  %s", i.posted, title)
}

// newsLanguages returns the languages in which to read a news item, in
// order, as eselect chooses them from LC_ALL, or from LANG when LC_ALL is
// not set: the locale without its encoding and modifier (it_IT), its
// language alone (it), then English, which every item has.
func newsLanguages(lcAll, lang string) []string {
	locale := cmp.Or(lcAll, lang)
	if end := strings.IndexFunc(locale, func(r rune) bool { return !isLocaleLetter(r) }); end >= 0 {
		locale = locale[:end]
	}

	language, _, _ := strings.Cut(locale, "_")

	var languages []string
	if locale != language {
		languages = append(languages, locale)
	}

	switch language {
	case "", "C", "POSIX", "en":
	default:
		languages = append(languages, language)
	}

	return append(languages, "en")
}

// isLocaleLetter reports whether r can be part of the language and
// territory of a locale name.
func isLocaleLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
}

// newsItems returns the news items of the repositories, read from root:
// those Portage lists as unread for each, and with withRead those it lists
// as read too, with the title and date of each item in the first of
// languages it is written in. They are sorted by name, so by the date it
// starts with, as eselect lists them; items of the same name keep the
// order of the lists, then of the repositories. A repository without a
// list has no item in it.
func newsItems(root fs.FS, repositories []repository, languages []string, withRead bool) ([]newsItem, error) {
	lists := []string{"unread"}
	if withRead {
		lists = append(lists, "read")
	}

	var items []newsItem

	for _, list := range lists {
		for _, repo := range repositories {
			data, err := fs.ReadFile(root, path.Join(newsState, "news-"+repo.name+"."+list))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			if err != nil {
				return nil, fmt.Errorf("could not read the news items: %w", err)
			}

			for name := range strings.FieldsSeq(string(data)) {
				item := readNewsItem(root, repo, name, languages)
				item.read = list == "read"
				items = append(items, item)
			}
		}
	}

	slices.SortStableFunc(items, func(a, b newsItem) int { return strings.Compare(a.name, b.name) })

	return items, nil
}

// readNewsItem returns the item name of repo with the title and the date
// its file gives, in the first of languages it has. As eselect does, an
// item without a file is said to be removed, and one in a format other
// than 1 or 2 is named with its format.
func readNewsItem(root fs.FS, repo repository, name string, languages []string) newsItem {
	item := newsItem{
		repository: repo.name, name: name, posted: postedFromName(name), title: "(" + name + " - removed?)",
		read: false,
	}

	header, found := newsHeader(root, repo, name, languages)
	if !found {
		return item
	}

	format := header["News-Item-Format"]
	if !strings.HasPrefix(format, "1.") && !strings.HasPrefix(format, "2.") {
		item.posted = ""
		item.title = "(" + name + " - unknown format " + format + ")"

		return item
	}

	item.title = cmp.Or(header["Title"], "("+name+" - no title)")
	item.posted = cmp.Or(header["Posted"], item.posted)

	return item
}

// postedFromName returns the date the name of a news item starts with, or
// what stands for it when it does not start with one.
func postedFromName(name string) string {
	prefix := name[:min(len(name), len(time.DateOnly))]

	if _, err := time.Parse(time.DateOnly, prefix); err == nil {
		return prefix
	}

	return "(no date)"
}

// newsHeader returns the header of the item name of repo, in the first of
// languages it has, and whether it has one. A name that is not a directory
// of its own in the repository's news has none.
func newsHeader(root fs.FS, repo repository, name string, languages []string) (map[string]string, bool) {
	if !path.IsAbs(repo.location) || name == "." || name == ".." || strings.Contains(name, "/") {
		return nil, false
	}

	dir := path.Join(strings.TrimPrefix(path.Clean(repo.location), "/"), "metadata", "news", name)

	for _, language := range languages {
		data, err := fs.ReadFile(root, path.Join(dir, name+"."+language+".txt"))
		if err == nil {
			return parseNewsHeader(string(data)), true
		}
	}

	return nil, false
}

// parseNewsHeader returns the fields of the header of a news item: the
// "Key: value" lines up to the first empty one.
func parseNewsHeader(text string) map[string]string {
	header := map[string]string{}

	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			break
		}

		if key, value, isField := strings.Cut(line, ":"); isField {
			header[key] = strings.TrimSpace(value)
		}
	}

	return header
}
