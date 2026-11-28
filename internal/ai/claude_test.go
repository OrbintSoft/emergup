// SPDX-License-Identifier: GPL-3.0-or-later

package ai_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/ai"
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The state directory Claude Code runs in, in the tests, the option of
// Claude Code they give, and a session.
const (
	stateDir = "/home/someone/.local/state/emergup"
	safeMode = "--safe-mode"
	aSession = "a-session"
	program  = "claude"
)

// events returns the stream of a request whose answer arrives in pieces,
// as Claude Code writes it, in session.
func events(session string, pieces ...string) string {
	lines := make([]string, 0, len(pieces)+5)
	lines = append(lines,
		`{"type":"system","subtype":"init","session_id":"`+session+`","cwd":"`+stateDir+`"}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,`+
			`"content_block":{"type":"text","text":""}},"session_id":"`+session+`"}`,
	)

	for _, piece := range pieces {
		lines = append(lines, `{"type":"stream_event","event":{"type":"content_block_delta","index":0,`+
			`"delta":{"type":"text_delta","text":"`+piece+`"}},"session_id":"`+session+`"}`)
	}

	whole := strings.Join(pieces, "")
	lines = append(lines,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,`+
			`"delta":{"type":"thinking_delta","thinking":"not shown"}},"session_id":"`+session+`"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"`+whole+`"}]},"session_id":"`+session+`"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"`+whole+`","session_id":"`+session+`"}`,
	)

	return strings.Join(lines, "\n") + "\n"
}

// claude returns the Claude Code backend of the tests on fake, and where
// it shows its answers.
func claude(fake *runnertest.Fake) (ai.ClaudeCode, *bytes.Buffer) {
	var out bytes.Buffer

	return ai.ClaudeCode{
		Runner:  fake,
		Args:    []string{program, safeMode},
		Dir:     stateDir,
		Printer: ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false},
	}, &out
}

// answered is the answer of a request that gives events.
func answered(events string) runnertest.Answer {
	return runnertest.Answer{Result: runner.Result{Output: []byte(events)}}
}

// uuid matches a random UUID, as a session of Claude Code is named.
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestANewConversationHasARandomUUID(t *testing.T) {
	t.Parallel()

	first, second := ai.NewConversation(), ai.NewConversation()
	assert.Regexp(t, uuid, first.ID)
	assert.NotEqual(t, first.ID, second.ID)
	assert.False(t, first.Started)
}

func TestClaudeCodeExplainsInASessionOfItsOwn(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	conversation := ai.NewConversation()
	fake.Script(
		answered(events(conversation.ID, "The compile phase ", "dies.")),
		answered(events(conversation.ID, "Still the compile phase.")),
	)

	backend, out := claude(&fake)
	assert.Equal(t, program, backend.Name())

	require.NoError(t, backend.Explain(t.Context(), conversation, failure()))
	assert.True(t, conversation.Started)
	assert.Equal(t, "The compile phase dies.\n", out.String(), "the pieces of text, then a newline; nothing else")

	require.NoError(t, backend.Explain(t.Context(), conversation, failure()))
	assert.Equal(t, "The compile phase dies.\nStill the compile phase.\n", out.String())

	request := []string{
		program, safeMode, "-p", "--output-format", "stream-json", "--include-partial-messages",
		"--verbose", "--tools", "",
	}

	commands := fake.Commands()
	require.Len(t, commands, 2)
	assert.Equal(t, append(request, "--session-id", conversation.ID), commands[0].Args, "the first request starts it")
	assert.Equal(t, append(request, "--resume", conversation.ID), commands[1].Args, "the next ones go on with it")

	for _, command := range commands {
		assert.Equal(t, runner.Save, command.Mode, "the events are read, not shown")
		assert.False(t, command.Root, "Claude Code runs as the invoking user")
		assert.Equal(t, stateDir, command.Dir)
	}

	assert.Equal(t, []string{failure().Text(), failure().Text()}, fake.Inputs(), "the request on its standard input")
}

func TestClaudeCodeSaysWhenThereIsNoAnswer(t *testing.T) {
	t.Parallel()

	const errorResult = `{"type":"result","subtype":"success","is_error":true,"result":"Credit balance is too low",` +
		`"session_id":"s"}` + "\n"

	tests := []struct {
		name    string
		answer  runnertest.Answer
		want    string
		started bool
	}{
		{"an error", answered(errorResult), "no answer from claude: Credit balance is too low", true},
		{
			"a failed program",
			runnertest.Answer{Result: runner.Result{ExitCode: 1}},
			"no answer from claude: it exited with status 1", false,
		},
		{"no result", answered(""), "no answer from claude: it ended without one", false},
		{
			"a program that does not start",
			runnertest.Answer{Result: runner.Result{ExitCode: -1}, Err: errBroken},
			"no answer from claude: exec format error", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fake runnertest.Fake

			fake.Script(tt.answer)

			backend, _ := claude(&fake)
			conversation := ai.NewConversation()

			require.EqualError(t, backend.Explain(t.Context(), conversation, failure()), tt.want)
			assert.Equal(t, tt.started, conversation.Started, "started once Claude Code has said its session")
		})
	}
}

func TestClaudeCodeChatsInTheSession(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{}, runnertest.Answer{Result: runner.Result{ExitCode: 3}})

	backend, _ := claude(&fake)
	conversation := &ai.Conversation{ID: aSession, Started: true}

	require.NoError(t, backend.Chat(t.Context(), conversation))
	require.EqualError(t, backend.Chat(t.Context(), conversation), "the chat with claude exited with status 3")

	commands := fake.Commands()
	require.Len(t, commands, 2)
	assert.Equal(t, []string{program, safeMode, "--resume", aSession}, commands[0].Args)
	assert.Equal(t, runner.Terminal, commands[0].Mode, "on the terminal")
	assert.False(t, commands[0].Root)
	assert.Equal(t, stateDir, commands[0].Dir)
	assert.Nil(t, commands[0].Stdin, "nothing on its standard input but the user's")
}

func TestChatSendsTheMaterialFirst(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	conversation := ai.NewConversation()
	fake.Script(answered(events(conversation.ID, "It dies.")))

	backend, _ := claude(&fake)
	menu, out := screen('y')

	require.NoError(t, ai.Chat(t.Context(), menu, backend, conversation, failure(), ai.Extras{}))

	commands := fake.Commands()
	require.Len(t, commands, 2, "the request, then the chat")
	assert.Contains(t, commands[0].Args, "-p")
	assert.Equal(t, []string{program, safeMode, "--resume", conversation.ID}, commands[1].Args)
	assert.Contains(t, out.String(), "==> Send them to claude? [Y/v/n/?] send them\n==> The answer of claude:\n")
}

func TestChatGoesOnFromTheConversation(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	backend, _ := claude(&fake)
	menu, out := screen()
	conversation := &ai.Conversation{ID: aSession, Started: true}

	require.NoError(t, ai.Chat(t.Context(), menu, backend, conversation, failure(), ai.Extras{}))

	require.Len(t, fake.Commands(), 1, "nothing asked, nothing sent again")
	assert.NotContains(t, out.String(), "Send them")
}

func TestNoChatWithoutTheMaterial(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	backend, _ := claude(&fake)
	menu, _ := screen('n')

	require.NoError(t, ai.Chat(t.Context(), menu, backend, ai.NewConversation(), failure(), ai.Extras{}))
	assert.Empty(t, fake.Commands(), "the user sent nothing: no chat")
}
