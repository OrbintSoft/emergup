// SPDX-License-Identifier: GPL-3.0-or-later

// Package ai sends emergup's requests to an AI backend, which answers them
// as the invoking user. Before a request is sent, the user is shown what it
// holds and asked whether to send it.
package ai

import (
	"context"
	_ "embed" // The instructions to the AI are files of their own.
	"strings"

	"github.com/OrbintSoft/emergup/internal/ui"
)

// explainFailure is what emergup asks of the AI about a failed step.
//
//go:embed explain-failure.txt
var explainFailure string

// Part is one part of a request: what it is, and its text.
type Part struct {
	Title string
	Text  string
}

// Request is what emergup asks a backend: its own instructions, then the
// parts the answer is about.
type Request struct {
	Instructions string
	Parts        []Part
}

// Text returns the request as a backend reads it: the instructions, then
// each part under its title.
func (r Request) Text() string {
	var text strings.Builder

	text.WriteString(withNewline(r.Instructions))

	for _, part := range r.Parts {
		text.WriteString("\n## " + part.Title + "\n\n")
		text.WriteString(withNewline(part.Text))
	}

	return text.String()
}

// ExplainFailure returns the request to explain a failed step from parts,
// such as its failure report and the profile of the system.
func ExplainFailure(parts ...Part) Request {
	return Request{Instructions: explainFailure, Parts: parts}
}

// Backend answers requests.
type Backend interface {
	// Name names it to the user, as the question before a request does.
	Name() string
	// Explain sends request, and shows the answer, and keeps it in the
	// log, as it arrives.
	Explain(ctx context.Context, request Request) error
}

// Ask shows on menu what request holds and asks whether to send it to
// backend, with extras to add; once the user says to, it sends it, the
// answer shown as it arrives. A request the user does not send is no
// error; Ctrl-C or the end of the input at the question is ui.ErrAborted.
func Ask(ctx context.Context, menu ui.Menu, backend Backend, request Request, extras Extras) error {
	request, send, err := Confirm(ctx, menu, backend.Name(), request, extras)
	if err != nil || !send {
		return err
	}

	menu.Printer.Info("The answer of " + backend.Name() + ":")

	return backend.Explain(ctx, request) //nolint:wrapcheck // A backend's error says what failed already.
}

// withNewline returns text ending with exactly one newline.
func withNewline(text string) string {
	return strings.TrimRight(text, "\n") + "\n"
}
