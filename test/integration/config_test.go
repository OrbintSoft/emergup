// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CFG-1: configuration is read from /etc/emergup/config.toml and then from
// /etc/emergup/config.d/*.toml in lexical order.
func TestCFG1_TheDropInsAreReadInLexicalOrderAfterTheMainFile(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("lexical-order")

	assert.Equal(t, result{
		stdout: "Read /etc/emergup/config.toml\n" +
			"Read /etc/emergup/config.d/10-first.toml\n" +
			"Read /etc/emergup/config.d/9-second.toml\n" +
			"The configuration is valid.\n",
		stderr: "",
		code:   0,
	}, ctr.emergup("config", "check"))

	show := ctr.emergup("config", "show")
	require.Equal(t, 0, show.code, show.stderr)
	assert.Contains(t, show.stdout, "\n"+`min-interval = "2h"  # /etc/emergup/config.d/9-second.toml`+"\n")
}

// CFG-2: emergup works with no configuration file at all, using generic
// defaults.
func TestCFG2_WithoutConfigurationTheDefaultsApply(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.root("rm", "-r", "/etc/emergup")

	assert.Equal(t, result{
		stdout: "No configuration file: the generic defaults apply.\nThe configuration is valid.\n",
		stderr: "",
		code:   0,
	}, ctr.emergup("config", "check"))

	show := ctr.emergup("config", "show")
	require.Equal(t, 0, show.code, show.stderr)

	for line := range strings.Lines(show.stdout) {
		line = strings.TrimSuffix(line, "\n")
		if line != "" && !strings.HasPrefix(line, "[") {
			assert.True(t, strings.HasSuffix(line, "  # default"), line)
		}
	}

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, "==> [2/6] update: update @world\n")
}

// CFG-3: `emergup config check` reports each problem with its file and
// line, exiting with status 1 when there is one; unknown keys are errors.
// A run with such a configuration cannot start.
func TestCFG3_CheckReportsEachProblemWithItsFileAndLine(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("problems")

	want := result{
		stdout: "",
		stderr: "/etc/emergup/config.d/10-unknown.toml:7:1: steps.update.bogus: unknown key\n" +
			"/etc/emergup/config.d/10-unknown.toml:9:2: steps.nope: unknown key\n" +
			"/etc/emergup/config.d/20-value.toml:6:1: runs.keep: must be 0 or more; 0 keeps every run\n" +
			"/etc/emergup/config.d/30-syntax.toml:5:12: expected ']' to close table name\n" +
			`/etc/emergup/config.d/40-one-file.toml:7:16: steps.sync.min-interval: invalid duration "soon", ` +
			`expected for example "24h" or "90m"` + "\n" +
			"/etc/emergup/config.d/40-one-file.toml:8:1: steps.sync.bogus: unknown key\n" +
			"/etc/emergup/config.d/40-one-file.toml:9:11: steps.sync.command: expected a list of strings, " +
			"found an integer\n" +
			`/etc/emergup/config.d/40-one-file.toml:12:8: steps.live-rebuild.mode: unknown live-rebuild mode "smrt", ` +
			"expected one of auto, smart, interval, always\n" +
			"8 problems in the configuration.\n",
		code: 1,
	}

	assert.Equal(t, want, ctr.emergup("config", "check"), "config check")
	assert.Equal(t, want, ctr.emergup("run", "--pretend"), "run --pretend")
}

// CFG-4: `emergup config show` prints the effective configuration as TOML,
// each value with the file it came from or `default`.
func TestCFG4_ShowPrintsEachValueWithWhereItComesFrom(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("origins")

	show := ctr.emergup("config", "show")
	require.Equal(t, 0, show.code, show.stderr)

	for _, line := range []string{
		`method = "auto"  # default`,
		`target = "@emergup-test"  # /etc/emergup/config.toml`,
		`min-interval = "12h"  # /etc/emergup/config.d/50-sync.toml`,
	} {
		assert.Contains(t, show.stdout, "\n"+line+"\n")
	}

	// What show prints is a configuration file that gives the same values.
	shown := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(shown, []byte(show.stdout), 0o600))
	ctr.root("rm", "-r", configDirectory)
	ctr.install(shown, configFile)

	again := ctr.emergup("config", "show")
	require.Equal(t, 0, again.code, again.stderr)
	assert.Equal(t, values(show.stdout), values(again.stdout))
}

// values returns what `emergup config show` printed without the origins.
func values(show string) string {
	var lines strings.Builder

	for line := range strings.Lines(show) {
		value, _, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "  # ")
		lines.WriteString(value + "\n")
	}

	return lines.String()
}

// CFG-5: lists of entries add up across the configuration files: a later
// file adds entries, `-entry` removes one added earlier and `-*` clears the
// list.
func TestCFG5_ListsAddUpAcrossTheFiles(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	ctr.dropIns("lists")

	show := ctr.emergup("config", "show")
	require.Equal(t, 0, show.code, show.stderr)
	assert.Contains(t, show.stdout, "\n"+`always-sync = ["gentoo"]  # /etc/emergup/config.d/20-second.toml`+"\n")
	assert.Contains(t, show.stdout, "\nexclude = [\n"+
		`  "app-misc/two",  # /etc/emergup/config.d/10-first.toml`+"\n"+
		`  "app-misc/three",  # /etc/emergup/config.d/20-second.toml`+"\n"+
		"]\n")

	pretend := ctr.emergup("run", "--pretend")
	assert.Equal(t, 0, pretend.code, "the exit status, with the output:\n%s%s", pretend.stdout, pretend.stderr)
	assert.Contains(t, pretend.stdout, " --exclude app-misc/two --exclude app-misc/three @emergup-test\n")
}

// CFG-6: the colour of each kind of output can be changed in the
// configuration, or turned off for that kind alone.
func TestCFG6_TheHeadingsTakeTheirConfiguredStyle(t *testing.T) {
	t.Parallel()

	const heading = "==> [1/6] sync: sync the repositories"

	for _, style := range []struct{ name, dropIns, want string }{
		{name: "default", dropIns: "", want: "\x1b[1;34m" + heading + "\x1b[0m\r\n"},
		{name: "styled", dropIns: "heading-styled", want: "\x1b[3;38;5;202;48;2;16;32;48m" + heading + "\x1b[0m\r\n"},
		{name: "plain", dropIns: "heading-plain", want: heading + "\r\n"},
	} {
		t.Run(style.name, func(t *testing.T) {
			t.Parallel()

			ctr := start(t)
			if style.dropIns != "" {
				ctr.dropIns(style.dropIns)
			}

			got := ctr.emergupTerminal("run", "--pretend")
			assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s", got.stdout)
			assert.True(t, strings.HasPrefix(got.stdout, style.want), "%q", got.stdout)
		})
	}
}
