// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bytes"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configuration files of the origin tests, as shown to the user.
const (
	shownMain   = "/etc/emergup/config.toml"
	shownDropIn = "/etc/emergup/config.d/10.toml"
)

func TestInspectSaysWhereEachValueComesFrom(t *testing.T) {
	t.Parallel()

	_, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file("[steps.update]\ntarget = \"@system\"\nexclude = [\"app-misc/a\"]\n"),
		dropIn("10.toml"): file("[steps.update]\ntarget = \"@world\"\nexclude = [\"app-misc/b\"]\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{shownMain, shownDropIn}, origins.Files)
	assert.Equal(t, shownDropIn, origins.Of("steps.update.target"))
	assert.Empty(t, origins.Of("steps.sync.min-interval"))
	assert.Equal(t, shownMain, origins.OfEntry(keyExclude, pkgA))
	assert.Equal(t, shownDropIn, origins.OfEntry(keyExclude, pkgB))
}

func TestInspectFollowsKeysOfInlineTables(t *testing.T) {
	t.Parallel()

	_, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile: file("[steps]\ndepclean = { enabled = false }\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, shownMain, origins.Of("steps.depclean.enabled"))
	assert.Empty(t, origins.Of("steps.depclean.command"))
}

func TestInspectForgetsTheOriginOfRemovedEntries(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          excludes(pkgA, pkgB),
		dropIn("10.toml"): excludes("-*", pkgA),
	})
	require.NoError(t, err)
	assert.Equal(t, Entries{pkgA}, cfg.Steps.Update.Exclude)
	assert.Equal(t, shownDropIn, origins.OfEntry(keyExclude, pkgA))
	assert.Empty(t, origins.OfEntry(keyExclude, pkgB))
}

func TestInspectWithoutFiles(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{})
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)
	assert.Empty(t, origins.Files)
}

func TestDurationMarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		want     string
	}{
		{24 * time.Hour, "24h"},
		{90 * time.Minute, "1h30m"},
		{90 * time.Second, "1m30s"},
		{2*time.Hour + 5*time.Second, "2h0m5s"},
		{0, "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			text, err := Duration(tt.duration).MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(text))
		})
	}
}

func TestNamedValuesMarshalText(t *testing.T) {
	t.Parallel()

	text, err := ElevationRun0.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "run0", string(text))

	text, err = LiveRebuildInterval.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "interval", string(text))
}

// shown returns what WriteTOML writes for cfg and origins.
func shown(t *testing.T, cfg Config, origins Origins) string {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, WriteTOML(&out, cfg, origins))

	return out.String()
}

func TestWriteTOMLWritesTheDefaults(t *testing.T) {
	t.Parallel()

	out := shown(t, Default(), Origins{})

	assert.Contains(t, out, "[elevation]\nmethod = \"auto\"  # default\n\n[steps.sync]\n")
	assert.Contains(t, out, "[steps.sync]\nenabled = true  # default\ncommand = [\"emerge\", \"--sync\"]  # default\n"+
		"min-interval = \"24h\"  # default\nalways-sync = []  # default\n")
	assert.Contains(t, out, "[steps.live-rebuild]\nenabled = true  # default\n")
	assert.Contains(t, out, "mode = \"auto\"  # default\ninterval = \"24h\"  # default\n"+
		"smart-command = [\"smart-live-rebuild\"]  # default\n")
	assert.NotContains(t, out, "[steps]\n", "a table without values of its own has no header")
	assert.True(t, len(out) > 0 && out[len(out)-1] == '\n' && out[len(out)-2] != '\n', "ends with exactly one newline")
}

func TestWriteTOMLNamesTheOriginOfEachValue(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file("[steps.update]\ntarget = \"@system\"\nexclude = [\"app-misc/a\"]\n"),
		dropIn("10.toml"): file("[steps.update]\nexclude = [\"app-misc/b\"]\n\n[steps.sync]\nalways-sync = [\"overlay\"]\n"),
	})
	require.NoError(t, err)

	out := shown(t, cfg, origins)
	assert.Contains(t, out, "target = \"@system\"  # "+shownMain+"\n")
	assert.Contains(t, out, "exclude = [\n  \"app-misc/a\",  # "+shownMain+"\n  \"app-misc/b\",  # "+shownDropIn+"\n]\n")
	assert.Contains(t, out, "always-sync = [\"overlay\"]  # "+shownDropIn+"\n")
	assert.Contains(t, out, "min-interval = \"24h\"  # default\n")
}

func TestWriteTOMLNamesWhoEmptiedAList(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          excludes(pkgA),
		dropIn("10.toml"): excludes("-*"),
	})
	require.NoError(t, err)
	assert.Contains(t, shown(t, cfg, origins), "exclude = []  # "+shownDropIn+"\n")
}

func TestWriteTOMLReadsBackToTheSameConfiguration(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile: file("[elevation]\nmethod = \"doas\"\n\n[steps.update]\n" +
			"options = [\"--with=\\\"quoted\\\"\", \"back\\\\slash\", \"tab\\there\", \"café\"]\n" +
			"exclude = [\"app-misc/a\"]\n\n[steps.live-rebuild]\nmode = \"interval\"\ninterval = \"1h30m\"\n"),
		dropIn("10.toml"): file("[steps.update]\nexclude = [\"app-misc/b\"]\n\n[steps.depclean]\nenabled = false\n"),
	})
	require.NoError(t, err)

	back, err := Load(t.Context(), fstest.MapFS{MainFile: file(shown(t, cfg, origins))})
	require.NoError(t, err)
	assert.Equal(t, cfg, back)
}
