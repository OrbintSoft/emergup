// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// The AI backend of the tests, a synthetic line of emerge --version, and
// what the failing custom step build shows.
const (
	askAI         = "/usr/local/bin/ask-ai"
	withAI        = "\n[ai]\nbackend = \"command\"\n\n[ai.command]\ncommand = [\"" + askAI + "\"]\n"
	emergeVersion = "Portage 3.0.83 (python 3.13.5-final-0, default/linux/amd64/23.0, gcc-14, glibc-2.41-r4, x86_64)"
	buildShows    = "build: the sources do not compile\n"
	explainLine   = "==> Run this step again? [r/s/a/e/?] explain the failure\n"
	noAIQuestion  = "==> Run this step again? [r/s/a/?] "
)

// failingBuild is a session whose only step, the custom step build, fails
// as buildShows shows, with the configuration extra after it, answering
// keys.
func failingBuild(t *testing.T, extra string, keys ...rune) *session {
	t.Helper()

	sess := newSession(t, keys...)
	sess.root = fstest.MapFS{
		configFile:                         {Data: []byte(onlyFailingBuild + extra)},
		"var/db/pkg/app-misc/foo-1.2/SLOT": {Data: []byte("0\n")},
	}
	sess.runner.codes = map[string]int{buildProgram: 1}
	sess.runner.shown = map[string]string{buildProgram: buildShows}
	sess.runner.outputs["emerge --version"] = emergeVersion + "\n"
	sess.runner.outputs["emerge --info"] = "Portage 3.0.83\nACCEPT_KEYWORDS=\"amd64\"\n"

	return sess
}

// askedAI returns the AI command sess ran and what it read, and fails the
// test unless it ran one exactly.
func askedAI(t *testing.T, sess *session) (runner.Command, string) {
	t.Helper()

	var (
		asked  []runner.Command
		inputs []string
	)

	for index, command := range sess.runner.Commands() {
		if command.Args[0] == askAI {
			asked = append(asked, command)
			inputs = append(inputs, sess.runner.recorded.Inputs()[index])
		}
	}

	require.Len(t, asked, 1, "the AI is asked once")

	return asked[0], inputs[0]
}

func TestAFailedStepIsExplainedByTheAI(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withAI, 'y', 'e', 'y', 'a')

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code)

	assert.Contains(t, stdout, explainLine+"==> To send to ask-ai:\n    emergup's instructions ")
	assert.Contains(t, stdout, "\n    The failure report's summary ")
	assert.Contains(t, stdout, "\n    The end of the step's output    1 line\n")
	assert.Contains(t, stdout, "\n    The system (emerge --version)   1 line\n"+
		"==> Send them to ask-ai? [Y/v/p/i/n/?] send them\n"+
		"==> The answer of ask-ai:\n"+
		"==> Run this step again? [r/s/a/e/?] end the run\n", "the same question after the answer")

	command, input := askedAI(t, sess)
	assert.False(t, command.Root, "the AI runs as the invoking user")
	assert.Equal(t, runner.Stream, command.Mode)
	assert.Contains(t, input, "A step of the update has just\nfailed")
	assert.Contains(t, input, "\n## The failure report's summary\n\nThe failure report of an emergup run.\n")
	assert.Contains(t, input, "Step:     build (")
	assert.Contains(t, input, "\n## The end of the step's output\n\n"+buildShows)
	assert.Contains(t, input, "\n## The system (emerge --version)\n\n"+emergeVersion+"\n")
	assert.NotContains(t, input, "The installed packages", "only when the user adds them")
}

func TestTheUserAddsWhatTheAIGets(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withAI, 'y', 'e', 'p', 'i', 'y', 'a')

	code, _, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code)

	_, input := askedAI(t, sess)
	assert.Contains(t, input, "\n## The installed packages\n\napp-misc/foo-1.2\n")
	assert.Contains(t, input, "\n## The configuration of Portage (emerge --info)\n\nPortage 3.0.83\n")
}

func TestAnAIThatGivesNoAnswerIsSaid(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withAI, 'y', 'e', 'y', 'a')
	sess.runner.codes[askAI] = 3

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code, "the run fails once, for the step")
	assert.Contains(t, stdout, "==> The answer of ask-ai:\n"+
		"==> no answer from ask-ai: it exited with status 3\n"+
		"==> Run this step again? [r/s/a/e/?] end the run\n")
}

func TestThereIsNoAIWithoutABackendAsRootOrUnderYes(t *testing.T) {
	t.Parallel()

	off := failingBuild(t, "", 'y', 'a')
	code, stdout, _ := off.run(t, commandRun)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, noAIQuestion+"end the run\n", "AI is off by default")

	asRoot := failingBuild(t, withAI, 'y', 'a')
	asRoot.system.euid = 0
	code, stdout, _ = asRoot.run(t, commandRun)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, noAIQuestion+"end the run\n", "AI is off as root")

	unattended := failingBuild(t, withAI)
	unattended.keys.silent = true
	code, stdout, _ = unattended.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.NotContains(t, stdout, "Run this step again")

	for _, sess := range []*session{off, asRoot, unattended} {
		for _, command := range sess.runner.Commands() {
			assert.NotEqual(t, askAI, command.Args[0])
			assert.NotEqual(t, strings.Fields("emerge --version"), command.Args, "nothing gathered for the AI")
		}
	}
}

// The Claude Code backend of the tests, and the state directory it runs in.
const (
	withClaude = "\n[ai]\nbackend = \"claude-code\"\n"
	claudeCode = "claude"
	stateDir   = "/state/emergup"
)

// claudeAnswers makes Claude Code answer every request of sess with text,
// as its events give it.
func claudeAnswers(sess *session, text string) {
	sess.runner.outputs["claude --safe-mode"] = `{"type":"system","subtype":"init","session_id":"s"}` + "\n" +
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"` +
		text + `"}},"session_id":"s"}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"` + text + `","session_id":"s"}` + "\n"
}

// claudeRuns returns the commands sess ran Claude Code with.
func claudeRuns(sess *session) []runner.Command {
	var runs []runner.Command

	for _, command := range sess.runner.Commands() {
		if command.Args[0] == claudeCode {
			runs = append(runs, command)
		}
	}

	return runs
}

// sessionOf returns the session the Claude Code command line args names,
// with --session-id or --resume, and which.
func sessionOf(t *testing.T, args []string) (string, string) {
	t.Helper()

	require.GreaterOrEqual(t, len(args), 2)

	option, session := args[len(args)-2], args[len(args)-1]
	require.Contains(t, []string{"--session-id", "--resume"}, option, "the session ends the command line")

	return option, session
}

func TestClaudeCodeExplainsAndChatsInOneSession(t *testing.T) {
	t.Parallel()

	const claudePrompt = "==> Run this step again? [r/s/a/e/c/?] "

	sess := failingBuild(t, withClaude, 'y', 'e', 'y', 'c', 'a')
	claudeAnswers(sess, "The sources do not compile.")

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, claudePrompt+"explain the failure\n==> To send to claude:\n")
	assert.Contains(t, stdout, "==> The answer of claude:\nThe sources do not compile.\n"+
		claudePrompt+"chat about the failure\n"+claudePrompt+"end the run\n", "no screen before the chat")

	runs := claudeRuns(sess)
	require.Len(t, runs, 2)

	assert.Equal(t, []string{claudeCode, "--safe-mode", "-p"}, runs[0].Args[:3], "the configured command, then -p")
	assert.Equal(t, runner.Save, runs[0].Mode)

	started, session := sessionOf(t, runs[0].Args)
	assert.Equal(t, "--session-id", started)
	assert.Equal(t, []string{claudeCode, "--safe-mode", "--resume", session}, runs[1].Args, "the chat goes on with it")
	assert.Equal(t, runner.Terminal, runs[1].Mode)

	for _, run := range runs {
		assert.Equal(t, stateDir, run.Dir, "in emergup's state directory")
		assert.False(t, run.Root)
	}

	assert.NotContains(t, strings.Join(runs[0].Args, " "), "Step:", "the material is never on the command line")
}

func TestAChatFirstSendsTheMaterial(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withClaude, 'y', 'c', 'y', 'a')
	claudeAnswers(sess, "The sources do not compile.")

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "chat about the failure\n==> To send to claude:\n")

	runs := claudeRuns(sess)
	require.Len(t, runs, 2, "the request, then the chat")
	assert.Contains(t, runs[0].Args, "-p")
	assert.Equal(t, runner.Terminal, runs[1].Mode)
}

func TestEachFailureHasASessionOfItsOwn(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withClaude, 'y', 'e', 'y', 'r', 'e', 'y', 'a')
	claudeAnswers(sess, "Again.")

	code, _, _ := sess.run(t, commandRun)
	assert.Equal(t, 1, code)

	runs := claudeRuns(sess)
	require.Len(t, runs, 2)

	firstOption, first := sessionOf(t, runs[0].Args)
	secondOption, second := sessionOf(t, runs[1].Args)
	assert.Equal(t, "--session-id", firstOption)
	assert.Equal(t, "--session-id", secondOption, "the question after the second failure starts a new one")
	assert.NotEqual(t, first, second)
}

func TestTheCommandBackendHasNoChat(t *testing.T) {
	t.Parallel()

	sess := failingBuild(t, withAI, 'y', 'a')

	_, stdout, _ := sess.run(t, commandRun)
	assert.Contains(t, stdout, "==> Run this step again? [r/s/a/e/?] end the run\n")
}
