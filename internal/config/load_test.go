// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"context"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// file is a configuration file with the given content.
func file(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

// dropIn is the path of a drop-in file of DropInDir.
func dropIn(name string) string {
	return path.Join(DropInDir, name)
}

func TestLoadWithoutConfigurationFiles(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{})
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)
}

func TestLoadMainFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[steps.update]\ntarget = \"@system\"\n\n[steps.depclean]\nenabled = false\n"),
	})
	require.NoError(t, err)

	want := Default()
	want.Steps.Update.Target = "@system"
	want.Steps.Depclean.Enabled = false
	assert.Equal(t, want, cfg)
}

func TestLoadOnlyDropIns(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-update.toml"): file("[steps.update]\ntarget = \"@system\"\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, "@system", cfg.Steps.Update.Target)
}

func TestLoadReadsDropInsInLexicalOrderAfterTheMainFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile:                 file("[steps.update]\ntarget = \"@main\"\n"),
		dropIn("20-second.toml"): file("[steps.update]\ntarget = \"@second\"\n"),
		dropIn("10-first.toml"):  file("[steps.update]\ntarget = \"@first\"\noptions = [\"--first\"]\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, "@second", cfg.Steps.Update.Target)
	assert.Equal(t, []string{"--first"}, cfg.Steps.Update.Options)
}

func TestLoadReplacesCommandLines(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile:                  file("[steps.sync]\ncommand = [\"emaint\", \"sync\"]\n"),
		dropIn("10-options.toml"): file("[steps.update]\noptions = [\"--update\", \"--deep\"]\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"emaint", "sync"}, cfg.Steps.Sync.Command)
	assert.Equal(t, []string{"--update", "--deep"}, cfg.Steps.Update.Options)
}

// Synthetic package atoms for the exclusion lists.
const (
	pkgA = "app-misc/a"
	pkgB = "app-misc/b"
	pkgC = "app-misc/c"
)

// excludes is a configuration file that sets the exclusion list to entries.
func excludes(entries ...string) *fstest.MapFile {
	quoted := make([]string, len(entries))
	for i, entry := range entries {
		quoted[i] = strconv.Quote(entry)
	}

	return file("[steps.update]\nexclude = [" + strings.Join(quoted, ", ") + "]\n")
}

func TestLoadAddsUpEntries(t *testing.T) {
	t.Parallel()

	noList := file("[steps.update]\ntarget = \"@system\"\n")

	tests := []struct {
		name   string
		main   *fstest.MapFile
		dropIn *fstest.MapFile
		want   Entries
	}{
		{"each file adds its entries", excludes(pkgA), excludes(pkgB), Entries{pkgA, pkgB}},
		{"an entry already there is not repeated", excludes(pkgA), excludes(pkgA, pkgB), Entries{pkgA, pkgB}},
		{"-entry removes an earlier entry", excludes(pkgA, pkgB), excludes("-"+pkgA, pkgC), Entries{pkgB, pkgC}},
		{"-* clears the earlier entries", excludes(pkgA, pkgB), excludes("-*", pkgC), Entries{pkgC}},
		{"a file that does not set the list keeps it", excludes(pkgA), noList, Entries{pkgA}},
		{"an empty list adds nothing", excludes(pkgA), excludes(), Entries{pkgA}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(t.Context(), fstest.MapFS{MainFile: tt.main, dropIn("10.toml"): tt.dropIn})
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Steps.Update.Exclude)
		})
	}
}

func TestLoadAddsUpEveryEntryList(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile:          file("[steps.sync]\nalways-sync = [\"overlay-a\"]\n"),
		dropIn("10.toml"): file("[steps.sync]\nalways-sync = [\"overlay-b\"]\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, Entries{"overlay-a", "overlay-b"}, cfg.Steps.Sync.AlwaysSync)
}

// TestEntryListsCoversEveryEntriesField guards the list of fields that add up
// across files: a new Entries field that entryLists misses would be replaced
// by a later file instead.
func TestEntryListsCoversEveryEntriesField(t *testing.T) {
	t.Parallel()

	var cfg Config
	assert.Len(t, cfg.entryLists(), countEntriesFields(reflect.TypeFor[Config]()))
}

func countEntriesFields(typ reflect.Type) int {
	count := 0

	for field := range typ.Fields() {
		switch {
		case field.Type == reflect.TypeFor[Entries]():
			count++
		case field.Type.Kind() == reflect.Struct:
			count += countEntriesFields(field.Type)
		}
	}

	return count
}

func TestLoadIgnoresWhatIsNotADropIn(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		dropIn("README"):             file("not toml"),
		dropIn("10-old.toml.bak"):    file("not toml"),
		dropIn("20-dir.toml/inside"): file("not toml"),
	})
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)
}

func TestLoadDecodesValues(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[elevation]\nmethod = \"doas\"\n\n" +
			"[steps.sync]\nmin-interval = \"12h\"\n\n" +
			"[steps.live-rebuild]\nmode = \"smart\"\ninterval = \"90m\"\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, ElevationDoas, cfg.Elevation.Method)
	assert.Equal(t, Duration(12*time.Hour), cfg.Steps.Sync.MinInterval)
	assert.Equal(t, LiveRebuildSmart, cfg.Steps.LiveRebuild.Mode)
	assert.Equal(t, Duration(90*time.Minute), cfg.Steps.LiveRebuild.Interval)
}

func TestLoadStopsWhenTheContextIsDone(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Load(ctx, fstest.MapFS{MainFile: file("[steps.depclean]\nenabled = false\n")})
	require.ErrorIs(t, err, context.Canceled)
}
