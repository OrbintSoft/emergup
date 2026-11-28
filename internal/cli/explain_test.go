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
