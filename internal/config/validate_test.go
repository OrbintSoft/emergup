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
	keyKeepEnv     = "elevation.keep-env"
	keyMinInterval = "steps.sync.min-interval"
	keyMode        = "steps.live-rebuild.mode"
	keyHeading     = "colors.heading"
	keyKeep        = "runs.keep"
	unknownKey     = "unknown key"
	emptyCommand   = "empty command"
	emptyProgram   = "empty program"
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
			"integer for a style", "[colors]\nheading = 4\n",
			2, 1, keyHeading, `expected a style such as "bold blue", found an integer`,
		},
		{
			"invalid style", "[colors]\nheading = \"blod blue\"\n",
			2, 11, keyHeading, `unknown style word "blod"`,
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
			emptyProgram, "[steps.depclean]\ncommand = [\"\", \"--depclean\"]\n",
			2, 1, "steps.depclean.command", emptyProgram,
		},
		{
			"empty news command", "[steps.news]\ncommand = []\n",
			2, 1, "steps.news.command", emptyCommand,
		},
		{
			"empty smart command", "[steps.live-rebuild]\nsmart-command = []\n",
			2, 1, "steps.live-rebuild.smart-command", emptyCommand,
		},
		{
			"negative number of runs to keep", "[runs]\nkeep = -1\n",
			2, 1, keyKeep, "must be 0 or more; 0 keeps every run",
		},
		{
			"string for an integer", "[runs]\nkeep = \"ten\"\n",
			2, 8, keyKeep, "expected an integer, found a string",
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
		{
			"two variables as one", "[elevation]\nkeep-env = [\"EDITOR,VISUAL\"]\n",
			2, 1, keyKeepEnv, `entry "EDITOR,VISUAL" is not a variable name: use letters, digits and underscores, ` +
				"not starting with a digit",
		},
		{
			"variable starting with a digit", "[elevation]\nkeep-env = [\"1X\"]\n",
			2, 1, keyKeepEnv, `entry "1X" is not a variable name`,
		},
		{
			"removal of no variable", "[elevation]\nkeep-env = [\"-EDI-TOR\"]\n",
			2, 1, keyKeepEnv, `entry "-EDI-TOR" is not a variable name`,
		},
		{
			"variable with a space", "[elevation]\nkeep-env = [\"A B\"]\n",
			2, 1, keyKeepEnv, "write each one as its own entry",
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

func TestLoadReportsEveryProblemOfAFile(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-broken.toml"): file("[steps.sync]\nmin-interval = \"soon\"\nbogus = 1\ncommand = 5\n\n" +
			"[steps.live-rebuild]\nmode = \"smrt\"\n\n[steps.update]\nexclude = [\"a b\"]\n\n[runs]\nkeep = -1\n"),
	})

	got := make([]string, 0, len(problems(t, err)))
	for _, problem := range problems(t, err) {
		got = append(got, problem.Error())
	}

	assert.Equal(t, []string{
		brokenFile + `:2:16: steps.sync.min-interval: invalid duration "soon", expected for example "24h" or "90m"`,
		brokenFile + ":3:1: steps.sync.bogus: unknown key",
		brokenFile + ":4:11: steps.sync.command: expected a list of strings, found an integer",
		brokenFile + `:7:8: steps.live-rebuild.mode: unknown live-rebuild mode "smrt", ` +
			"expected one of auto, smart, interval, always",
		brokenFile + `:10:1: steps.update.exclude: entry "a b" contains a space: write each one as its own entry`,
		brokenFile + ":13:1: runs.keep: must be 0 or more; 0 keeps every run",
	}, got)
}

func TestLoadGoesOnAfterAValueInAnInlineTable(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-broken.toml"): file("[steps]\ndepclean = { command = 5 }\n\n[runs]\nkeep = -1\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 2)
	require.ErrorContains(t, list[0], "expected a list of strings, found an integer")
	assert.Equal(t, keyKeep, list[1].Key)
}

func TestLoadGoesOnPastQuotedKeys(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-broken.toml"): file("[steps.sync]\n\"min-interval\" = \"soon\"\n'bogus' = 1\n\n[ \"runs\" ]\nkeep = -1\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, keyMinInterval, list[0].Key)
	assert.Equal(t, "steps.sync.bogus", list[1].Key)
	assert.Equal(t, keyKeep, list[2].Key)
}

func TestLoadStopsAtATableDefinedTwice(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		dropIn("10-broken.toml"): file("[runs]\nkeep = 1\n\n[runs]\nkeep = 2\n\n[steps.sync]\nbogus = 1\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 4, list[0].Line)
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

func TestTextKeysAreTheNamedValuesDurationsAndStyles(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t, []string{
		"elevation.method",
		keyMinInterval,
		keyMode,
		"steps.live-rebuild.interval",
		keyHeading,
		"colors.prompt",
		"colors.success",
		"colors.skip",
		"colors.failure",
		"colors.notice",
		"hooks.*.on-failure",
		"ai.backend",
	}, textKeys())
}
