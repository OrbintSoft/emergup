// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Where the Claude Code of the tests records its calls and its requests,
// and the question after a failure with it.
const (
	claudeCalls    = "/tmp/emergup-test-claude"
	claudeRequests = "/tmp/emergup-test-claude-request-"
	claudePrompt   = "==> Run this step again? [r/s/a/e/c/?] "
)

// claudeCall matches a call of the Claude Code of the tests: the user, the
// directory and the arguments, the session last.
var claudeCall = regexp.MustCompile(`^user=(\S+) dir=(\S+) args=(.*) (--session-id|--resume) (\S+)$`)

// call is one call of the Claude Code of the tests.
type call struct {
	user, dir, args, option, session string
}

// withClaudeCode installs the Claude Code of the tests and makes it the
// backend, with its default command.
func withClaudeCode(ctr *container) {
	ctr.t.Helper()

	ctr.install("testdata/claude-script/claude.sh", "/usr/local/bin/claude")
	ctr.root("chmod", "0755", "/usr/local/bin/claude")
	ctr.dropIns("ai-claude-code")
}

// claudeCallsOf returns the calls of the Claude Code of the tests, in order.
func (c *container) claudeCallsOf() []call {
	c.t.Helper()

	if !c.exists(claudeCalls) {
		return nil
	}

	var calls []call

	for line := range strings.Lines(c.readAsTester(claudeCalls)) {
		found := claudeCall.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
		require.NotNil(c.t, found, "a call of Claude Code: %q", line)

		calls = append(calls, call{user: found[1], dir: found[2], args: found[3], option: found[4], session: found[5]})
	}

	return calls
}

// AI-1, AI-8, AI-9, AI-10: with Claude Code, e explains the failure in a
// session of its own, the answer shown as it arrives; a second e and then
// c go on with that session, the second in a chat; each runs as the
// invoking user, in emergup's state directory, with the default command.
func TestAI10_ClaudeCodeExplainsAndChatsInOneSession(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withClaudeCode(ctr)

	got := ctr.emergupInput("y\ne\ny\ne\ny\nc\na\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assertInOrder(t, got.stdout,
		claudePrompt+"explain the failure\n==> To send to claude:\n",
		"==> Send them to claude? [Y/v/p/i/n/?] send them\n"+
			"==> The answer of claude:\nThe compile phase of emergup-test/upgrade-1.2 dies.\n"+
			claudePrompt+"chat about the failure\n",
		"The chat of the integration tests, in session ",
		claudePrompt+"end the run\n")
	assert.NotContains(t, got.stdout, "not shown twice", "the answer is shown once, from its pieces")

	calls := ctr.claudeCallsOf()
	require.Len(t, calls, 3)
	assert.True(t, strings.HasPrefix(calls[0].args, "--safe-mode -p --output-format stream-json "), calls[0].args)
	assert.Equal(t, "--session-id", calls[0].option, "the first request starts the session")
	assert.Equal(t, calls[0].args, calls[1].args)
	assert.Equal(t, "--resume", calls[1].option, "the second goes on with it")
	assert.Equal(t, "--safe-mode", calls[2].args, "the chat is Claude Code on the terminal")
	assert.Equal(t, "--resume", calls[2].option)

	for _, each := range calls[1:] {
		assert.Equal(t, calls[0].session, each.session, "all in the session of the first explanation")
	}

	for _, each := range calls {
		assert.Equal(t, tester, each.user)
		assert.Equal(t, "/home/tester/.local/state/emergup", each.dir)
	}

	request := ctr.readAsTester(claudeRequests + calls[0].session)
	assert.Contains(t, request, "\n## The failure report's summary\n\nThe failure report of an emergup run.\n")
	assert.Contains(t, request, "\n## The system (emerge --version)\n\nPortage 3.")

	assert.Contains(t, ctr.runLog(tester, got.stdout), "The compile phase of emergup-test/upgrade-1.2 dies.\n",
		"the answer is in the log")
}

// AI-9, AI-10: a chat with nothing sent yet first sends the material, and
// the question after another failure starts a new session.
func TestAI9_EachFailureHasASessionOfItsOwn(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withClaudeCode(ctr)

	got := ctr.emergupInput("y\nc\ny\nr\ny\ne\ny\na\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)
	assert.Contains(t, got.stdout, claudePrompt+"chat about the failure\n==> To send to claude:\n")

	calls := ctr.claudeCallsOf()
	require.Len(t, calls, 3, "the request and the chat of the first failure, the request of the second")
	assert.Equal(t, "--session-id", calls[0].option)
	assert.Equal(t, "--resume", calls[1].option)
	assert.Equal(t, calls[0].session, calls[1].session)
	assert.Equal(t, "--session-id", calls[2].option)
	assert.NotEqual(t, calls[0].session, calls[2].session)
}
