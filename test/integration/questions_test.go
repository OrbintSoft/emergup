// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// updateQuestion is the question of the update, after its pretend.
const updateQuestion = "==> Update these packages? [Y/s/a/?] "

// updateKeys returns the list of the keys of the update's question, each
// line ending with end.
func updateKeys(end string) string {
	return "    y, Enter  update them" + end +
		"    s         skip the update" + end +
		"    a         abort the run" + end +
		"    ?         list the keys" + end
}

// withUpdate makes an upgrade of emergup-test/upgrade available to runs
// that only update.
func withUpdate(ctr *container) {
	ctr.t.Helper()

	ctr.dropIns("update-only")
	ctr.publishTree("snapshot-updates")
	ctr.syncRepository()
}

// UPD-14: on a terminal a question is answered with one key, without
// Enter; the keys pressed before the question are ignored, Enter takes the
// usual action, ? lists the keys, and Ctrl-C or Ctrl-D aborts the run.
func TestUPD14_OnATerminalAQuestionIsAnsweredWithOneKey(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withUpdate(ctr)

	listed := ctr.emergupKeyboard("run")
	listed.waitFor("==> [1/1] update: update @emergup-test\r\n")
	// Pressed while the update's pretend runs, before its question.
	listed.send("s")
	listed.waitFor(updateQuestion)
	listed.send("?")
	listed.waitFor(updateKeys("\r\n") + updateQuestion)

	got := listed.finish("s")
	assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s", got.stdout)
	assert.Contains(t, got.stdout, updateQuestion+"?\r\n"+updateKeys("\r\n")+updateQuestion+"skip the update\r\n",
		"the key pressed before the question is ignored")
	assert.Equal(t, "1.0", ctr.version("upgrade"), "the update is skipped")

	for _, abort := range []struct{ name, key string }{{name: "Ctrl-C", key: "\x03"}, {name: "Ctrl-D", key: "\x04"}} {
		stopped := ctr.emergupKeyboard("run")
		stopped.waitFor(updateQuestion)

		got := stopped.finish(abort.key)
		assert.Equal(t, 130, got.code, "%s, with the output:\n%s", abort.name, got.stdout)
		assert.Regexp(t, `\n==> update stopped after `+took+`: you aborted the run\r\n`, got.stdout, abort.name)
	}

	entered := ctr.emergupKeyboard("run")
	entered.waitFor(updateQuestion)

	got = entered.finish("\r")
	assert.Equal(t, 0, got.code, "the exit status, with the output:\n%s", got.stdout)
	assert.Contains(t, got.stdout, updateQuestion+"update them\r\n", "Enter")
	assert.Equal(t, "1.1", ctr.version("upgrade"), "the update runs")
}

// UPD-14: when standard input is not a terminal, each answer is a line: a
// letter, or nothing for the usual action.
func TestUPD14_WithoutATerminalEachAnswerIsALine(t *testing.T) {
	t.Parallel()

	ctr := start(t)
	withUpdate(ctr)

	skipped := ctr.emergupRun("x\n?\ns\n")
	assert.Contains(t, skipped, updateQuestion+"\n"+
		"==> Not an answer: press one of the keys in brackets, or ? for the list.\n"+
		updateQuestion+"?\n"+updateKeys("\n")+
		updateQuestion+"skip the update\n")
	assert.Equal(t, "1.0", ctr.version("upgrade"), "the update is skipped")

	assert.Contains(t, ctr.emergupRun("\n"), updateQuestion+"update them\n", "an empty line")
	assert.Equal(t, "1.1", ctr.version("upgrade"), "the update runs")
}
