// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrAborted is returned by Ask when the question is answered with Ctrl-C
// or the input ends: neither takes the usual action.
var ErrAborted = errors.New("aborted")

// The key that lists the others, and what the list says of it.
const (
	listKey  = '?'
	listName = "list the keys"
)

// notAnAnswer is the notice written for a key that chooses no action.
const notAnAnswer = "Not an answer: press one of the keys in brackets, or ? for the list."

// Action is one of the answers to a question.
type Action struct {
	// Key is the lower-case letter that chooses it.
	Key rune
	// Name says what it does: it is written after the question once the
	// action is chosen, and in the list of the keys.
	Name string
}

// Menu asks questions on Printer and reads their answers from Keyboard.
type Menu struct {
	Printer  Printer
	Keyboard Keyboard
	// Yes takes the usual action of every question, without reading a key.
	Yes bool
}

// Ask writes question followed by the keys of actions and returns the
// action chosen. The first action is the usual one: Enter takes it too, and
// its key is shown in capitals. Keys are read until one chooses an action:
// ? lists them, any other key says it is not an answer. actions must not be
// empty, and their keys must differ from one another and from ?.
func (m Menu) Ask(ctx context.Context, question string, actions ...Action) (Action, error) {
	text := question + " " + keyHint(actions)

	if m.Yes {
		m.Printer.prompt(text)
		m.answer(actions[0].Name + " (--yes)")

		return actions[0], nil
	}

	for {
		m.Printer.prompt(text)

		key, err := m.Keyboard.ReadKey(ctx)
		if err != nil || key == KeyInterrupt {
			m.answer("")

			return Action{Key: 0, Name: ""}, readError(ctx, err)
		}

		if action, found := choose(actions, key); found {
			m.answer(action.Name)

			return action, nil
		}

		if key == listKey {
			m.answer(string(listKey))
			writeList(m.Printer.Out, actions)

			continue
		}

		m.answer("")
		m.Printer.Notice(notAnAnswer)
	}
}

// answer ends the line of the question with text.
func (m Menu) answer(text string) {
	_, _ = fmt.Fprintln(m.Printer.Out, text)
}

// readError returns what Ask returns when no key could be read: err is nil
// for Ctrl-C.
func readError(ctx context.Context, err error) error {
	switch {
	case err == nil, errors.Is(err, io.EOF):
		return ErrAborted
	case ctx.Err() != nil:
		return err
	}

	return fmt.Errorf("reading the answer: %w", err)
}

// keyHint returns the keys of actions as the question shows them: "[Y/s/?]".
func keyHint(actions []Action) string {
	keys := make([]string, 0, len(actions)+1)
	keys = append(keys, string(unicode.ToUpper(actions[0].Key)))

	for _, action := range actions[1:] {
		keys = append(keys, string(action.Key))
	}

	keys = append(keys, string(listKey))

	return "[" + strings.Join(keys, "/") + "]"
}

// choose returns the action of actions that key chooses.
func choose(actions []Action, key rune) (Action, bool) {
	if key == KeyEnter {
		return actions[0], true
	}

	key = unicode.ToLower(key)
	for _, action := range actions {
		if action.Key == key {
			return action, true
		}
	}

	return Action{Key: 0, Name: ""}, false
}

// writeList writes to out the keys of actions and what each one does.
func writeList(out io.Writer, actions []Action) {
	keys := make([]string, 0, len(actions)+1)
	names := make([]string, 0, len(actions)+1)

	keys = append(keys, string(actions[0].Key)+", Enter")
	names = append(names, actions[0].Name)

	for _, action := range actions[1:] {
		keys = append(keys, string(action.Key))
		names = append(names, action.Name)
	}

	keys = append(keys, string(listKey))
	names = append(names, listName)

	width := utf8.RuneCountInString(keys[0])
	for index, key := range keys {
		_, _ = fmt.Fprintf(out, "    %-*s  %s\n", width, key, names[index])
	}
}
