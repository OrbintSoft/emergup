// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Where the news items of the repositories of newsRepositories are, and
// where Portage lists those unread.
const (
	gentooNews    = "var/db/repos/gentoo/metadata/news/"
	overlayNews   = "var/db/repos/overlay/metadata/news/"
	gentooUnread  = "var/lib/gentoo/news/news-gentoo.unread"
	overlayUnread = "var/lib/gentoo/news/news-overlay.unread"
	overlayName   = "overlay"
)

// newsRepositories are the gentoo repository and an overlay.
func newsRepositories() []repository {
	return []repository{
		{name: mainRepository, location: gentooLocation, syncType: "rsync", autoSync: true},
		{name: overlayName, location: "/var/db/repos/overlay", syncType: gitSync, autoSync: true},
	}
}

// newsFile is a news item with its title and the date it was posted.
func newsFile(title, posted string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("Title: " + title + "\nAuthor: A Developer <dev@example.org>\n" +
		"Posted: " + posted + "\nRevision: 1\nNews-Item-Format: 2.0\n\nTitle: not a header\n")}
}

// lines returns a file of the lines.
func lines(text string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(text)}
}

func TestUnreadNewsListsTheUnreadItemsOfEveryRepositoryByName(t *testing.T) {
	t.Parallel()

	root := fstest.MapFS{
		gentooUnread:                           lines("2026-10-02-bar\n2026-09-01-foo\n"),
		overlayUnread:                          lines("2026-09-15-baz\n"),
		"var/lib/gentoo/news/news-gentoo.read": lines("2026-01-01-old\n"),
		gentooNews + "2026-10-02-bar/2026-10-02-bar.en.txt":  newsFile("Bar changes", "2026-10-02"),
		gentooNews + "2026-09-01-foo/2026-09-01-foo.en.txt":  newsFile("Foo moves", "2026-09-01"),
		gentooNews + "2026-01-01-old/2026-01-01-old.en.txt":  newsFile("Old news", "2026-01-01"),
		overlayNews + "2026-09-15-baz/2026-09-15-baz.en.txt": newsFile("Baz: a new slot", "2026-09-16"),
	}

	items, err := unreadNews(root, newsRepositories(), []string{"en"})
	require.NoError(t, err)
	assert.Equal(t, []newsItem{
		{repository: mainRepository, name: "2026-09-01-foo", posted: "2026-09-01", title: "Foo moves"},
		{repository: overlayName, name: "2026-09-15-baz", posted: "2026-09-16", title: "Baz: a new slot"},
		{repository: mainRepository, name: "2026-10-02-bar", posted: "2026-10-02", title: "Bar changes"},
	}, items)

	rows := make([]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, item.row())
	}

	assert.Equal(t, []string{
		"2026-09-01  Foo moves",
		"2026-09-16  [overlay] Baz: a new slot",
		"2026-10-02  Bar changes",
	}, rows)
}

func TestUnreadNewsReadsAnItemInTheFirstLanguageItHas(t *testing.T) {
	t.Parallel()

	root := fstest.MapFS{
		gentooUnread: lines("2026-09-01-foo 2026-09-02-bar\n"),
		gentooNews + "2026-09-01-foo/2026-09-01-foo.en.txt": newsFile("Foo moves", "2026-09-01"),
		gentooNews + "2026-09-01-foo/2026-09-01-foo.it.txt": newsFile("Foo si sposta", "2026-09-01"),
		gentooNews + "2026-09-02-bar/2026-09-02-bar.en.txt": newsFile("Bar changes", "2026-09-02"),
	}

	items, err := unreadNews(root, newsRepositories(), []string{"it_IT", "it", "en"})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "Foo si sposta", items[0].title)
	assert.Equal(t, "Bar changes", items[1].title)
}

func TestUnreadNewsFindsNoneWithoutUnreadItems(t *testing.T) {
	t.Parallel()

	for name, root := range map[string]fstest.MapFS{
		"no news at all":  {},
		"empty lists":     {gentooUnread: lines(""), overlayUnread: lines("\n")},
		"only read items": {"var/lib/gentoo/news/news-gentoo.read": lines("2026-09-01-foo\n")},
	} {
		items, err := unreadNews(root, newsRepositories(), []string{"en"})
		require.NoError(t, err, name)
		assert.Empty(t, items, name)
	}
}

func TestUnreadNewsShowsTheItemsItCannotRead(t *testing.T) {
	t.Parallel()

	root := fstest.MapFS{
		gentooUnread: lines("2026-09-01-gone\n2026-09-02-untitled\nundated\n2026-09-03-future\n" +
			"2026-09-04-unposted\n../secret\n"),
		gentooNews + "2026-09-02-untitled/2026-09-02-untitled.en.txt": lines(
			"Posted: 2026-09-02\nNews-Item-Format: 2.0\n\nTitle: in the body\n"),
		gentooNews + "2026-09-03-future/2026-09-03-future.en.txt": lines(
			"Title: Future\nPosted: 2026-09-03\nNews-Item-Format: 3.0\n\nBody\n"),
		gentooNews + "2026-09-04-unposted/2026-09-04-unposted.en.txt": lines(
			"Title: No date\nNews-Item-Format: 1.0\n\nBody\n"),
		"var/db/repos/gentoo/metadata/secret.en.txt": newsFile("Secret", "2026-09-05"),
		overlayUnread: lines("2026-09-06-elsewhere\n"),
		"relative/overlay/metadata/news/2026-09-06-elsewhere/2026-09-06-elsewhere.en.txt": newsFile(
			"Read from the wrong place", "2026-09-06"),
	}
	repositories := []repository{
		newsRepositories()[0],
		{name: overlayName, location: "relative/overlay", syncType: "", autoSync: true},
	}

	items, err := unreadNews(root, repositories, []string{"en"})
	require.NoError(t, err)
	assert.Equal(t, []newsItem{
		{repository: mainRepository, name: "../secret", posted: "(no date)", title: "(../secret - removed?)"},
		{repository: mainRepository, name: "2026-09-01-gone", posted: "2026-09-01", title: "(2026-09-01-gone - removed?)"},
		{
			repository: mainRepository, name: "2026-09-02-untitled", posted: "2026-09-02",
			title: "(2026-09-02-untitled - no title)",
		},
		{
			repository: mainRepository, name: "2026-09-03-future", posted: "",
			title: "(2026-09-03-future - unknown format 3.0)",
		},
		{repository: mainRepository, name: "2026-09-04-unposted", posted: "2026-09-04", title: "No date"},
		{
			repository: overlayName, name: "2026-09-06-elsewhere", posted: "2026-09-06",
			title: "(2026-09-06-elsewhere - removed?)",
		},
		{repository: mainRepository, name: "undated", posted: "(no date)", title: "(undated - removed?)"},
	}, items)
	assert.Equal(t, "            (2026-09-03-future - unknown format 3.0)", items[3].row())
}

func TestUnreadNewsFailsWhenAListCannotBeRead(t *testing.T) {
	t.Parallel()

	root := fstest.MapFS{gentooUnread + "/not-a-file": lines("")}

	_, err := unreadNews(root, newsRepositories(), []string{"en"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), gentooUnread)
}

func TestNewsLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		lcAll, lang string
		want        []string
	}{
		{"", "", []string{"en"}},
		{"", "C", []string{"en"}},
		{"", "POSIX", []string{"en"}},
		{"", "C.UTF-8", []string{"en"}},
		{"", "en", []string{"en"}},
		{"", "en_GB.UTF-8", []string{"en_GB", "en"}},
		{"", "fr", []string{"fr", "en"}},
		{"", "it_IT.UTF-8", []string{"it_IT", "it", "en"}},
		{"", "sr_RS@latin", []string{"sr_RS", "sr", "en"}},
		{"de_DE.UTF-8", "it_IT.UTF-8", []string{"de_DE", "de", "en"}},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, newsLanguages(test.lcAll, test.lang), "LC_ALL=%q LANG=%q", test.lcAll, test.lang)
	}
}
