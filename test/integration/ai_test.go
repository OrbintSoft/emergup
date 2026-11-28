// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AI of the tests, where it saves the request it reads, and what it
// answers.
const (
	testAI     = "/usr/local/bin/emergup-test-ai"
	aiRequest  = "/tmp/emergup-test-ai-request"
	aiAnswer   = "The compile phase of emergup-test/upgrade-1.2 dies: this version does not build.\n"
	aiPrompt   = "==> Run this step again? [r/s/a/e/?] "
	sendPrompt = "==> Send them to emergup-test-ai? [Y/v/p/i/n/?] "
)

// withTheAI installs the AI of the tests and makes it the command backend.
func withTheAI(ctr *container) {
	ctr.t.Helper()

	ctr.install("testdata/ai-script/emergup-test-ai.sh", testAI)
	ctr.root("chmod", "0755", testAI)
	ctr.dropIns("ai-command")
}

// requestSent returns what the AI of the tests was sent, after the line
// that names the user it ran as.
func (c *container) requestSent() string {
	c.t.Helper()

	require.True(c.t, c.exists(aiRequest), "the AI was asked")

	return c.readAsTester(aiRequest)
}

// AI-1, AI-4, AI-8: with the command backend configured, the question after
// a failed step offers to explain it; the AI, run as the invoking user, is
// sent the failure report and the profile of the system, its answer is
// shown and logged, and the same question comes again.
func TestAI8_AFailedStepIsExplained(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withTheAI(ctr)

	got := ctr.emergupInput("y\ne\ny\na\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assert.Contains(t, got.stdout, aiPrompt+"explain the failure\n==> To send to emergup-test-ai:\n")
	assertInOrder(t, got.stdout,
		"\n    emergup's instructions ", "\n    The failure report's summary ",
		"\n    The end of the step's output ", "\n    The system (emerge --version) ",
		sendPrompt+"send them\n==> The answer of emergup-test-ai:\n"+aiAnswer,
		"This answer comes from the AI of the integration tests.\n"+aiPrompt+"end the run\n")

	request := ctr.requestSent()
	assert.True(t, strings.HasPrefix(request, "user=tester\n"), "the AI runs as the invoking user: %q",
		request[:min(len(request), 40)])
	assertInOrder(t, request,
		"A step of the update has just\nfailed",
		"\n## The failure report's summary\n\nThe failure report of an emergup run.\n",
		"  emergup-test/upgrade-1.2::emergup-test, in its compile phase\n",
		"\n## The end of the step's output\n\n",
		"this version does not build",
		"\n## The system (emerge --version)\n\nPortage 3.")
	assert.NotContains(t, request, "## The installed packages", "only when the user adds them")

	assert.Contains(t, ctr.runLog(tester, got.stdout), aiAnswer, "the answer is in the log")
}

// AI-2: before anything is sent, the user can see all of it, and add the
// installed packages and emerge --info; or send nothing.
func TestAI2_TheUserSeesAndChoosesWhatIsSent(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)
	withTheAI(ctr)

	refused := ctr.emergupInput("y\ne\nn\na\n", "run")
	assert.Equal(t, 1, refused.code, "the exit status, with the output:\n%s%s", refused.stdout, refused.stderr)
	assert.Contains(t, refused.stdout, sendPrompt+"send nothing\n"+aiPrompt+"end the run\n")
	assert.False(t, ctr.exists(aiRequest), "nothing is sent")

	got := ctr.emergupInput("y\ne\nv\np\ni\ny\na\n", "run")
	assert.Equal(t, 1, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	assertInOrder(t, got.stdout,
		sendPrompt+"show all of it\n    You are helping someone who updates a Gentoo Linux system with emergup, a\n",
		"\n    ## The failure report's summary\n",
		sendPrompt+"add the installed packages\n",
		"\n    The installed packages ",
		"==> Send them to emergup-test-ai? [Y/v/i/n/?] add emerge --info\n",
		"\n    The configuration of Portage (emerge --info) ",
		"==> Send them to emergup-test-ai? [Y/v/n/?] send them\n")

	request := ctr.requestSent()
	assert.Contains(t, request, "\n## The installed packages\n\n")
	assert.Contains(t, request, "\nemergup-test/upgrade-1.0\n", "the package installed in the image")
	assert.Contains(t, request, "\n## The configuration of Portage (emerge --info)\n\nPortage 3.")
}

// AI-1, AI-6: AI is off without a backend, and when emergup runs as root
// even with one; a command backend without a command is a problem of the
// configuration.
func TestAI6_NoAIWithoutABackendOrAsRoot(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withBrokenUpdate(ctr)

	off := ctr.emergupInput("y\na\n", "run")
	assert.Equal(t, 1, off.code, "the exit status, with the output:\n%s%s", off.stdout, off.stderr)
	assert.Contains(t, off.stdout, againPrompt+"end the run\n", "AI is off by default")

	withTheAI(ctr)

	asRoot := ctr.emergupAs(root, "y\na\n", "run")
	assert.Equal(t, 1, asRoot.code, "the exit status, with the output:\n%s%s", asRoot.stdout, asRoot.stderr)
	assert.Contains(t, asRoot.stdout, "and AI assistance is off.\n")
	assert.Contains(t, asRoot.stdout, againPrompt+"end the run\n", "AI is off as root")
	assert.False(t, ctr.exists(aiRequest))

	ctr.dropIns("ai-no-command")

	check := ctr.emergup("config", "check")
	assert.Equal(t, 1, check.code)
	assert.Contains(t, check.stderr, "/etc/emergup/config.d/40-ai.toml: ai.backend: the command backend has no "+
		"command: give the program that answers in [ai.command] command\n")
}
