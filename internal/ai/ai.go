// SPDX-License-Identifier: GPL-3.0-or-later

// Package ai sends emergup's requests to an AI backend, which answers them
// as the invoking user. Before a request is sent, the user is shown what it
// holds and asked whether to send it.
package ai

import (
	"context"
	"crypto/rand"
	_ "embed" // The instructions to the AI are files of their own.
	"fmt"
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
	// Explain sends request in conversation, and shows the answer, and
	// keeps it in the log, as it arrives.
	Explain(ctx context.Context, conversation *Conversation, request Request) error
}

// Conversation is the conversation of one question, which its AI actions
// share: a backend that keeps conversations goes on with it from one
// request to the next.
type Conversation struct {
	// ID names it to the backend.
	ID string
	// Started reports whether the backend has started it.
	Started bool
}

// NewConversation returns a conversation not started yet, named by a new
// random UUID.
func NewConversation() *Conversation {
	random := make([]byte, uuidBytes)
	_, _ = rand.Read(random) // It never fails.

	random[6] = random[6]&versionMask | version4
	random[8] = random[8]&variantMask | variantRFC

	return &Conversation{
		ID:      fmt.Sprintf("%x-%x-%x-%x-%x", random[0:4], random[4:6], random[6:8], random[8:10], random[10:]),
		Started: false,
	}
}

// What makes random bytes a random UUID: its 16 bytes, the version 4 in the
// high bits of the seventh, the variant of RFC 9562 in those of the ninth.
const (
	uuidBytes   = 16
	versionMask = 0x0f
	version4    = 0x40
	variantMask = 0x3f
	variantRFC  = 0x80
)

// Chatter is a backend that also opens an interactive chat, going on with
// a conversation.
type Chatter interface {
	Backend

	// Chat opens the chat, and returns when it ends.
	Chat(ctx context.Context, conversation *Conversation) error
}

// Chat opens a chat with backend in conversation. When nothing was sent in
// it yet, it first sends request, once the user agrees, as Ask does; with
// nothing sent, there is no chat.
func Chat(ctx context.Context, menu ui.Menu, backend Chatter, conversation *Conversation, request Request,
	extras Extras,
) error {
	if !conversation.Started {
		if err := Ask(ctx, menu, backend, conversation, request, extras); err != nil || !conversation.Started {
			return err
		}
	}

	return backend.Chat(ctx, conversation) //nolint:wrapcheck // A backend's error says what failed already.
}

// Ask shows on menu what request holds and asks whether to send it to
// backend, with extras to add; once the user says to, it sends it, the
// answer shown as it arrives. A request the user does not send is no
// error; Ctrl-C or the end of the input at the question is ui.ErrAborted.
func Ask(ctx context.Context, menu ui.Menu, backend Backend, conversation *Conversation, request Request,
	extras Extras,
) error {
	request, send, err := Confirm(ctx, menu, backend.Name(), request, extras)
	if err != nil || !send {
		return err
	}

	menu.Printer.Info("The answer of " + backend.Name() + ":")

	return backend.Explain(ctx, conversation, request) //nolint:wrapcheck // A backend's error says what failed already.
}

// withNewline returns text ending with exactly one newline.
func withNewline(text string) string {
	return strings.TrimRight(text, "\n") + "\n"
}
