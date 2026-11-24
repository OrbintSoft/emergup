// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CLI-1: `emergup version` (or `--version`) prints the emergup version.
func TestCLI1_VersionPrintsTheVersion(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	want := result{stdout: "emergup " + setting(t, versionVariable) + "\n", stderr: "", code: 0}

	for _, arg := range []string{"version", "--version"} {
		assert.Equal(t, want, ctr.emergup(arg), arg)
	}
}

// CLI-2: `emergup help` (or `-h`, `--help`) prints the usage on standard
// output.
func TestCLI2_HelpPrintsTheUsage(t *testing.T) {
	t.Parallel()

	ctr := start(t)

	help := ctr.emergup("help")
	require.Equal(t, 0, help.code, help.stderr)
	assert.True(t, strings.HasPrefix(help.stdout, "usage: emergup <command>\n"), help.stdout)
	assert.Empty(t, help.stderr)

	for _, arg := range []string{"-h", "--help"} {
		assert.Equal(t, help, ctr.emergup(arg), arg)
	}
}

// CLI-2: a missing or unknown command, or an unexpected argument, prints
// the usage on standard error and exits with status 2.
func TestCLI2_ABadCommandLinePrintsTheUsageOnStandardError(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	usage := ctr.emergup("help").stdout

	for _, bad := range []struct {
		args    []string
		message string
	}{
		{args: nil, message: ""},
		{args: []string{"bogus"}, message: `emergup: unknown command "bogus"`},
		{args: []string{"version", "extra"}, message: `emergup version: unexpected argument "extra"`},
		{args: []string{"run", "--bogus"}, message: `emergup run: unknown option "--bogus"`},
		{args: []string{"config"}, message: "emergup config: missing subcommand, expected check or show"},
	} {
		stderr := usage
		if bad.message != "" {
			stderr = bad.message + "\n\n" + usage
		}

		assert.Equal(t, result{stdout: "", stderr: stderr, code: 2}, ctr.emergup(bad.args...), "%q", bad.args)
	}
}

// CLI-3: `emergup run` exits with status 130 when the run was stopped by
// the abort key or the end of the input at a question.
func TestCLI3_AnAbortAtAQuestionStopsTheRunWithStatus130(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	// A snapshot that a sync would bring.
	ctr.publish("stopped")

	for _, stop := range []struct{ name, input string }{
		{name: "the end of the input", input: ""},
		{name: "the abort key", input: "a\n"},
	} {
		got := ctr.emergupInput(stop.input, "run")
		assert.Equal(t, 130, got.code, "%s, with the output:\n%s%s", stop.name, got.stdout, got.stderr)
		assert.Contains(t, got.stdout, "==> Run this step? [Y/s/a/?] ", stop.name)
		assert.Contains(t, got.stdout, "\n==> The run was stopped after ", stop.name)
	}

	assert.False(t, ctr.exists(repository+"/stopped"), "the repository is not synced")
}
