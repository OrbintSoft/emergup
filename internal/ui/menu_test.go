// SPDX-License-Identifier: GPL-3.0-or-later

package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The question the tests ask, and how it is written.
const (
	question = "Update @world?"
	prompt   = "==> Update @world? [Y/s/a/?] "
)

// errBroken is the error of a keyboard that cannot be read.
var errBroken = errors.New("input/output error")

// The actions of the question.
//
//nolint:gochecknoglobals // The actions every test asks about, never changed.
var (
	proceed = ui.Action{Key: 'y', Name: "continue"}
	skip    = ui.Action{Key: 's', Name: "skip this step"}
	abort   = ui.Action{Key: 'a', Name: "abort the run"}
)

// answer is what a keys returns for one ReadKey.
type answer struct {
	key rune
	err error
}

// pressed returns the answers of the keys pressed one after the other.
func pressed(keys ...rune) []answer {
	answers := make([]answer, 0, len(keys))
	for _, key := range keys {
		answers = append(answers, answer{key: key, err: nil})
	}

	return answers
}

// keys is a Keyboard that returns answers in order. Reading past the last
// one fails the test.
type keys struct {
	t       *testing.T
	answers []answer
}

func (k *keys) ReadKey(context.Context) (rune, error) {
	if len(k.answers) == 0 {
		k.t.Error("a key was read after the last answer")

		return 0, io.EOF
	}

	next := k.answers[0]
	k.answers = k.answers[1:]

	return next.key, next.err
}

// ask asks the question with a menu that reads answers, and returns what
// it wrote, uncoloured, and the action chosen.
func ask(ctx context.Context, t *testing.T, yes bool, answers []answer) (string, ui.Action, error) {
	t.Helper()

	var out bytes.Buffer

	menu := ui.Menu{
		Printer:  ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false},
		Keyboard: &keys{t: t, answers: answers},
		Yes:      yes,
	}
	action, err := menu.Ask(ctx, question, proceed, skip, abort)

	return out.String(), action, err
}

func TestMenuAnswers(t *testing.T) {
	t.Parallel()

	const (
		list = "    y, Enter  continue\n" +
			"    s         skip this step\n" +
			"    a         abort the run\n" +
			"    ?         list the keys\n"
		notAnAnswer = "==> Not an answer: press one of the keys in brackets, or ? for the list.\n"
	)

	tests := []struct {
		name    string
		answers []answer
		want    ui.Action
		written string
	}{
		{"Enter takes the usual action", pressed(ui.KeyEnter), proceed, prompt + "continue\n"},
		{"the usual action's key", pressed('y'), proceed, prompt + "continue\n"},
		{"another action's key", pressed('s'), skip, prompt + "skip this step\n"},
		{"a capital letter", pressed('S'), skip, prompt + "skip this step\n"},
		{"an action that aborts", pressed('a'), abort, prompt + "abort the run\n"},
		{"the list", pressed('?', 'a'), abort, prompt + "?\n" + list + prompt + "abort the run\n"},
		{"a key that is not an answer", pressed('x', 's'), skip, prompt + "\n" + notAnAnswer + prompt + "skip this step\n"},
		{"Escape", pressed('\x1b', 'y'), proceed, prompt + "\n" + notAnAnswer + prompt + "continue\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			written, action, err := ask(t.Context(), t, false, tt.answers)
			require.NoError(t, err)
			assert.Equal(t, tt.want, action)
			assert.Equal(t, tt.written, written)
		})
	}
}

func TestMenuAbortsOnCtrlCAndTheEndOfTheInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []answer
	}{
		{"Ctrl-C", pressed(ui.KeyInterrupt)},
		{"the end of the input", []answer{{key: 0, err: io.EOF}}},
		{"Ctrl-C after a key that is not an answer", pressed('x', ui.KeyInterrupt)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			written, _, err := ask(t.Context(), t, false, tt.answers)
			require.ErrorIs(t, err, ui.ErrAborted)
			assert.True(t, strings.HasSuffix(written, prompt+"\n"), "the line of the question is ended: %q", written)
		})
	}
}

func TestMenuReturnsTheContextsError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	written, _, err := ask(ctx, t, false, []answer{{key: 0, err: ctx.Err()}})
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, ui.ErrAborted)
	assert.Equal(t, prompt+"\n", written)
}

func TestMenuReportsAKeyboardError(t *testing.T) {
	t.Parallel()

	written, _, err := ask(t.Context(), t, false, []answer{{key: 0, err: errBroken}})
	require.ErrorIs(t, err, errBroken)
	require.ErrorContains(t, err, "reading the answer: input/output error")
	require.NotErrorIs(t, err, ui.ErrAborted)
	assert.Equal(t, prompt+"\n", written)
}

func TestMenuWithYesTakesTheUsualActionWithoutReading(t *testing.T) {
	t.Parallel()

	written, action, err := ask(t.Context(), t, true, nil)
	require.NoError(t, err)
	assert.Equal(t, proceed, action)
	assert.Equal(t, prompt+"continue (--yes)\n", written)
}

func TestMenuWithOneAction(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	menu := ui.Menu{
		Printer:  ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false},
		Keyboard: &keys{t: t, answers: pressed(ui.KeyEnter)},
		Yes:      false,
	}
	action, err := menu.Ask(t.Context(), "Go on?", proceed)
	require.NoError(t, err)
	assert.Equal(t, proceed, action)
	assert.Equal(t, "==> Go on? [Y/?] continue\n", out.String())
}

func TestMenuColoursTheQuestionAndTheNotice(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	menu := ui.Menu{
		Printer:  ui.Printer{Out: &out, Colors: config.Default().Colors, Color: true},
		Keyboard: &keys{t: t, answers: pressed('x', 's')},
		Yes:      false,
	}
	_, err := menu.Ask(t.Context(), question, proceed, skip, abort)
	require.NoError(t, err)

	coloured := "\x1b[1;36m==> Update @world? [Y/s/a/?]\x1b[0m "
	assert.Equal(t, coloured+"\n"+
		"\x1b[35m==> Not an answer: press one of the keys in brackets, or ? for the list.\x1b[0m\n"+
		coloured+"skip this step\n", out.String())
}
