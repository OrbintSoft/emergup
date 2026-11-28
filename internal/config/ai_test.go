// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The keys of the AI backend, and Claude Code's program.
const (
	claude       = "claude"
	safeMode     = "--safe-mode"
	keyBackend   = "ai.backend"
	keyAICommand = "ai.command.command"
)

// backendNames is how a problem with the backend says what it can be.
const backendNames = "expected one of none, command, claude-code"

// noCommand is what config check says of a command backend without one.
const noCommand = "the command backend has no command: give the program that answers in [ai.command] command"

func TestTheDefaultHasNoAIBackend(t *testing.T) {
	t.Parallel()

	assert.Equal(t, AI{
		Backend:    BackendNone,
		ClaudeCode: ClaudeCodeBackend{Command: []string{claude, safeMode}},
		Command:    CommandBackend{Command: []string{}},
	}, Default().AI)
}

func TestLoadReadsTheClaudeCodeBackend(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[ai]\nbackend = \"claude-code\"\n\n" +
			"[ai.claude-code]\ncommand = [\"claude\", \"--model\", \"sonnet\"]\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, BackendClaudeCode, cfg.AI.Backend)
	assert.Equal(t, []string{claude, "--model", "sonnet"}, cfg.AI.ClaudeCode.Command)
}

func TestLoadReadsTheCommandBackend(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile:          file("[ai]\nbackend = \"command\"\n"),
		dropIn("50.toml"): file("[ai.command]\ncommand = [\"/usr/local/bin/ask\", \"--short\"]\n"),
	})
	require.NoError(t, err)
	assert.Equal(t, BackendCommand, cfg.AI.Backend)
	assert.Equal(t, []string{"/usr/local/bin/ask", "--short"}, cfg.AI.Command.Command,
		"the backend and its command can come from different files")
}

func TestLoadRejectsACommandBackendWithoutACommand(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{dropIn("10-broken.toml"): file("[ai]\nbackend = \"command\"\n")})

	found := problems(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, brokenFile, found[0].File, "against the file that chooses the backend")
	assert.Equal(t, keyBackend, found[0].Key)
	require.ErrorContains(t, found[0], noCommand)
}

func TestLoadAcceptsACommandWithoutTheCommandBackend(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile:          file("[ai]\nbackend = \"command\"\n\n[ai.command]\ncommand = [\"/usr/local/bin/ask\"]\n"),
		dropIn("90.toml"): file("[ai]\nbackend = \"none\"\n\n[ai.command]\ncommand = []\n"),
	})
	require.NoError(t, err, "a drop-in turns the AI off and empties the command")
	assert.Equal(t, BackendNone, cfg.AI.Backend)
	assert.Empty(t, cfg.AI.Command.Command)
}

func TestLoadRejectsWrongAIValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		key     string
		want    string
	}{
		{"an unknown backend", "[ai]\nbackend = \"claude\"\n", 2, keyBackend, `"claude", ` + backendNames},
		{"an integer for the backend", "[ai]\nbackend = 1\n", 2, keyBackend, backendNames + ", found an integer"},
		{"a list for the backend", "[ai]\nbackend = [\"none\"]\n", 2, keyBackend, backendNames + ", found a list"},
		{"an empty program", "[ai.command]\ncommand = [\"\"]\n", 2, keyAICommand, emptyProgram},
		{"no Claude Code command", "[ai.claude-code]\ncommand = []\n", 2, "ai.claude-code.command", emptyCommand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(t.Context(), fstest.MapFS{dropIn("10-broken.toml"): file(tt.content)})

			found := problems(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, tt.line, found[0].Line)
			assert.Equal(t, tt.key, found[0].Key)
			require.ErrorContains(t, found[0], tt.want)
		})
	}
}
