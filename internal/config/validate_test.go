// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenFile is the drop-in the validation tests write their content to.
const brokenFile = "/etc/emergup/config.d/10-broken.toml"

// Values that several cases share.
const (
	keyExclude     = "steps.update.exclude"
	keyMinInterval = "steps.sync.min-interval"
	keyMode        = "steps.live-rebuild.mode"
	unknownKey     = "unknown key"
	emptyCommand   = "empty command"
	otherFile      = "/f.toml"
)

// problems returns every *Error that err joins, in order.
func problems(t *testing.T, err error) []*Error {
	t.Helper()
	require.Error(t, err)

	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		var single *Error
		require.ErrorAs(t, err, &single)

		return []*Error{single}
	}

	list := make([]*Error, 0, len(joined.Unwrap()))

	for _, each := range joined.Unwrap() {
		var problem *Error
		require.ErrorAs(t, each, &problem)

		list = append(list, problem)
	}

	return list
}

func TestErrorFormat(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/f.toml:4:1: steps.sync.bogus: unknown key",
		(&Error{File: otherFile, Line: 4, Column: 1, Key: "steps.sync.bogus", Err: message(unknownKey)}).Error())
	assert.Equal(t, "/f.toml:2:5: expected ']'",
		(&Error{File: otherFile, Line: 2, Column: 5, Err: message("expected ']'")}).Error())
	assert.Equal(t, "/f.toml: permission denied",
		(&Error{File: otherFile, Err: message("permission denied")}).Error())
}

//nolint:funlen // one row per kind of problem, easier to read as one table
func TestLoadReportsWhereEachProblemIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		column  int
		key     string
		want    string
	}{
		{
			unknownKey, "[steps.sync]\nenabled = true\nbogus = 1\n",
			3, 1, "steps.sync.bogus", unknownKey,
		},
		{
			"unknown table", "[steps.nope]\nx = 1\n",
			1, 2, "steps.nope", unknownKey,
		},
		{
			"unknown key in an inline table", "[steps]\ndepclean = { enabled = false, bogus = 1 }\n",
			2, 31, "steps.depclean.bogus", unknownKey,
		},
		{
			"string for a boolean", "[steps.depclean]\nenabled = \"yes\"\n",
			2, 11, "steps.depclean.enabled", "expected true or false, found a string",
		},
		{
			"list for a string", "[steps.update]\ntarget = [\"@world\"]\n",
			2, 10, "steps.update.target", "expected a string, found a list",
		},
		{
			"string for a list", "[steps.update]\nexclude = \"app-misc/a\"\n",
			2, 11, keyExclude, "expected a list of strings, found a string",
		},
		{
			"integer for a duration", "[steps.sync]\nmin-interval = 5\n",
			2, 1, keyMinInterval, `expected a duration such as "24h", found an integer`,
		},
		{
			"integer for a name", "[steps.live-rebuild]\nmode = 1\n",
			2, 1, keyMode, "expected one of auto, smart, interval, always, found an integer",
		},
		{
			"invalid duration", "[steps.sync]\nmin-interval = \"1d\"\n",
			2, 16, keyMinInterval, `invalid duration "1d"`,
		},
		{
			"negative duration", "[steps.sync]\nmin-interval = \"-1h\"\n",
			2, 16, keyMinInterval, `negative duration "-1h"`,
		},
		{
			"unknown name", "[steps.live-rebuild]\nmode = \"smrt\"\n",
			2, 8, keyMode, `unknown live-rebuild mode "smrt"`,
		},
		{
			"key set twice", "[steps.sync]\nenabled = true\nenabled = false\n",
			3, 1, "steps.sync.enabled", "already defined",
		},
		{
			"syntax error", "[steps.sync\nenabled = true\n",
			1, 12, "", "table",
		},
		{
			emptyCommand, "[steps.depclean]\ncommand = []\n",
			2, 1, "steps.depclean.command", emptyCommand,
		},
		{
			"empty program", "[steps.depclean]\ncommand = [\"\", \"--depclean\"]\n",
			2, 1, "steps.depclean.command", "empty program",
		},
		{
			"empty smart command", "[steps.live-rebuild]\nsmart-command = []\n",
			2, 1, "steps.live-rebuild.smart-command", emptyCommand,
		},
		{
			"empty target", "[steps.update]\ntarget = \"\"\n",
			2, 1, "steps.update.target", "empty target",
		},
		{
			"entry with a space", "[steps.update]\nexclude = [\"app-misc/a app-misc/b\"]\n",
			2, 1, keyExclude, "write each one as its own entry",
		},
		{
			"empty entry", "[steps.update]\nexclude = [\"\"]\n",
			2, 1, keyExclude, "empty entry",
		},
		{
			"lone minus", "[steps.sync]\nalways-sync = [\"-\"]\n",
			2, 1, "steps.sync.always-sync", `"-" removes nothing`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(t.Context(), fstest.MapFS{dropIn("10-broken.toml"): file(tt.content)})
			assert.Equal(t, Config{}, cfg)

			list := problems(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, brokenFile, list[0].File)
			assert.Equal(t, tt.line, list[0].Line, "line")
			assert.Equal(t, tt.column, list[0].Column, "column")
			assert.Equal(t, tt.key, list[0].Key)
			assert.ErrorContains(t, list[0], tt.want)
		})
	}
}

func TestLoadReportsEveryUnknownKeyOfAFile(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-broken.toml"): file("[steps.sync]\nbogus = 1\n\n[steps.update]\nother = 2\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "steps.sync.bogus", list[0].Key)
	assert.Equal(t, "steps.update.other", list[1].Key)
}

func TestLoadReportsTheProblemsOfEveryFile(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		MainFile:                file("[steps.sync]\nbogus = 1\n"),
		dropIn("10-good.toml"):  file("[steps.update]\ntarget = \"@system\"\n"),
		dropIn("20-worse.toml"): file("[steps.update]\ntarget = \"\"\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "/"+MainFile, list[0].File)
	assert.Equal(t, "/etc/emergup/config.d/20-worse.toml", list[1].File)
}

func TestLoadAcceptsDottedKeysAndInlineTables(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[steps]\nupdate.target = \"@system\"\ndepclean = { enabled = false }\n" +
			"\"live-rebuild\" = { mode = \"always\", interval = \"6h\" }\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, "@system", cfg.Steps.Update.Target)
	assert.False(t, cfg.Steps.Depclean.Enabled)
	assert.Equal(t, LiveRebuildAlways, cfg.Steps.LiveRebuild.Mode)
}

// TestCheckedKeysExist guards the value checks against a key renamed in the
// schema: a check on a key that no longer exists would never run.
func TestCheckedKeysExist(t *testing.T) {
	t.Parallel()

	schema := schemaKeys(reflect.TypeFor[Config]())
	for _, rule := range valueRules() {
		assert.Contains(t, schema, rule.key)
	}
}

func TestTextKeysAreTheNamedValuesAndDurations(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t, []string{
		"elevation.method",
		keyMinInterval,
		keyMode,
		"steps.live-rebuild.interval",
	}, textKeys())
}
