// SPDX-License-Identifier: GPL-3.0-or-later

package ui_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The question without a usual action the tests ask, and how it is
// written.
const (
	failedQuestion = "The step failed. What now?"
	failedPrompt   = "==> The step failed. What now? [r/s/a/?] "
)

// The actions of the question without a usual action.
//
//nolint:gochecknoglobals // The actions every test asks about, never changed.
var (
	again = ui.Action{Key: 'r', Name: "run it again"}
	goOn  = ui.Action{Key: 's', Name: "go on with the next step"}
	end   = ui.Action{Key: 'a', Name: "end the run"}
)

// choose asks the question without a usual action with a menu that reads
// answers, and returns what it wrote, uncoloured, and the action chosen.
func choose(ctx context.Context, t *testing.T, yes bool, answers []answer) (string, ui.Action, error) {
	t.Helper()

	var out bytes.Buffer

	menu := ui.Menu{
		Printer:  ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false},
		Keyboard: &keys{t: t, answers: answers},
		Yes:      yes,
	}
	action, err := menu.Choose(ctx, failedQuestion, again, goOn, end)

	return out.String(), action, err
}

func TestChooseAnswers(t *testing.T) {
	t.Parallel()

	const (
		list = "    r  run it again\n" +
			"    s  go on with the next step\n" +
			"    a  end the run\n" +
			"    ?  list the keys\n"
		notAnAnswer = "==> Not an answer: press one of the keys in brackets, or ? for the list.\n"
	)

	tests := []struct {
		name    string
		answers []answer
		want    ui.Action
		written string
	}{
		{"the first action's key", pressed('r'), again, failedPrompt + "run it again\n"},
		{"another action's key", pressed('s'), goOn, failedPrompt + "go on with the next step\n"},
		{"a capital letter", pressed('A'), end, failedPrompt + "end the run\n"},
		{
			"Enter is not an answer", pressed(ui.KeyEnter, 's'), goOn,
			failedPrompt + "\n" + notAnAnswer + failedPrompt + "go on with the next step\n",
		},
		{
			"the list has no usual action", pressed('?', 'a'), end,
			failedPrompt + "?\n" + list + failedPrompt + "end the run\n",
		},
		{
			"a key that is not an answer", pressed('x', 'r'), again,
			failedPrompt + "\n" + notAnAnswer + failedPrompt + "run it again\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			written, action, err := choose(t.Context(), t, false, tt.answers)
			require.NoError(t, err)
			assert.Equal(t, tt.want, action)
			assert.Equal(t, tt.written, written)
		})
	}
}

func TestChooseAbortsOnCtrlCAndTheEndOfTheInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []answer
	}{
		{"Ctrl-C", pressed(ui.KeyInterrupt)},
		{"the end of the input", []answer{{key: 0, err: io.EOF}}},
		{"the end of the input after Enter", []answer{{key: ui.KeyEnter, err: nil}, {key: 0, err: io.EOF}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := choose(t.Context(), t, false, tt.answers)
			require.ErrorIs(t, err, ui.ErrAborted)
		})
	}
}

func TestChooseWithYesHasNoAnswer(t *testing.T) {
	t.Parallel()

	written, action, err := choose(t.Context(), t, true, nil)
	require.ErrorIs(t, err, ui.ErrNoUsualAction)
	require.NotErrorIs(t, err, ui.ErrAborted)
	assert.Equal(t, ui.Action{Key: 0, Name: ""}, action)
	assert.Empty(t, written, "nothing is asked")
}

func TestAskStillHasItsUsualAction(t *testing.T) {
	t.Parallel()

	written, action, err := ask(t.Context(), t, false, pressed('?', ui.KeyEnter))
	require.NoError(t, err)
	assert.Equal(t, proceed, action)
	assert.Equal(t, prompt+"?\n"+
		"    y, Enter  continue\n"+
		"    s         skip this step\n"+
		"    a         abort the run\n"+
		"    ?         list the keys\n"+
		prompt+"continue\n", written)
}
