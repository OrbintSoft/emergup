// SPDX-License-Identifier: GPL-3.0-or-later

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// errNoResult is the error of a request Claude Code ended without
// answering.
var errNoResult = errors.New("it ended without one")

// ClaudeCode is the backend that runs Claude Code as the invoking user, in
// Dir: Claude Code keeps its sessions for the directory it runs in, so the
// requests and the chats of a conversation run in the same one.
type ClaudeCode struct {
	// Runner runs Claude Code; it never runs it as root.
	Runner runner.Runner
	// Args holds its program, then its first arguments.
	Args []string
	// Dir is the directory it runs in.
	Dir string
	// Printer shows the answers, and keeps them in the log.
	Printer ui.Printer
}

// Name implements Backend: the name of the program.
func (c ClaudeCode) Name() string {
	return path.Base(c.Args[0])
}

// Explain implements Backend: one request without tools, in the session of
// conversation, started by the first one. Claude Code writes events, from
// which each piece of the answer is shown as it arrives.
func (c ClaudeCode) Explain(ctx context.Context, conversation *Conversation, request Request) error {
	session := []string{"--session-id", conversation.ID}
	if conversation.Started {
		session = []string{"--resume", conversation.ID}
	}

	args := slices.Concat(c.Args, []string{
		"-p", "--output-format", "stream-json", "--include-partial-messages", "--verbose", "--tools", "",
	}, session)

	answer := &streamDecoder{
		printer: c.Printer, partial: nil, shown: false, last: 0, inSession: false, ended: false, failure: "",
	}

	result, err := c.Runner.Run(ctx, runner.Command{
		Args: args, Mode: runner.Save, Root: false, Env: nil, Watch: nil, Into: answer,
		Stdin: strings.NewReader(request.Text()), Dir: c.Dir,
	})

	answer.finish()

	if answer.inSession {
		conversation.Started = true
	}

	switch {
	case err != nil:
		return fmt.Errorf("%w from %s: %w", errNoAnswer, c.Name(), err)
	case answer.failure != "":
		return fmt.Errorf("%w from %s: %s", errNoAnswer, c.Name(), answer.failure)
	case result.ExitCode != 0:
		return fmt.Errorf("%w from %s: it exited with status %d", errNoAnswer, c.Name(), result.ExitCode)
	case !answer.ended:
		return fmt.Errorf("%w from %s: %w", errNoAnswer, c.Name(), errNoResult)
	}

	return nil
}

// Chat implements Chatter: Claude Code on the terminal, going on with the
// session of conversation.
func (c ClaudeCode) Chat(ctx context.Context, conversation *Conversation) error {
	result, err := c.Runner.Run(ctx, runner.Command{
		Args: slices.Concat(c.Args, []string{"--resume", conversation.ID}), Mode: runner.Terminal, Root: false,
		Env: nil, Watch: nil, Into: nil, Stdin: nil, Dir: c.Dir,
	})

	switch {
	case err != nil:
		return fmt.Errorf("the chat with %s: %w", c.Name(), err)
	case result.ExitCode != 0:
		return exitedError{command: "the chat with " + c.Name(), code: result.ExitCode}
	}

	return nil
}

// streamDecoder reads the events Claude Code writes, one JSON object on
// each line, and shows the pieces of text of the answer as they arrive.
type streamDecoder struct {
	printer ui.Printer
	// partial is the start of a line not ended yet.
	partial []byte
	// shown is the text shown so far, last its last character.
	shown bool
	last  byte
	// inSession is whether an event named the session: Claude Code has
	// started it.
	inSession bool
	// ended is whether the last event, the result, came; failure is what
	// it says went wrong, if anything.
	ended   bool
	failure string
}

// event is what an event of Claude Code holds that the decoder reads.
type event struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"` //nolint:tagliatelle // Claude Code's name.
	Event     struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	IsError bool   `json:"is_error"` //nolint:tagliatelle // Claude Code's name.
	Result  string `json:"result"`
	Subtype string `json:"subtype"`
}

// Write implements io.Writer: it reads each line that data ends.
func (d *streamDecoder) Write(data []byte) (int, error) {
	d.partial = append(d.partial, data...)

	for {
		end := bytes.IndexByte(d.partial, '\n')
		if end < 0 {
			return len(data), nil
		}

		d.read(d.partial[:end])
		d.partial = d.partial[end+1:]
	}
}

// finish reads the last line, when it has no newline, and ends the text
// shown with one.
func (d *streamDecoder) finish() {
	if len(d.partial) > 0 {
		d.read(d.partial)
		d.partial = nil
	}

	if d.shown && d.last != '\n' {
		d.printer.Text("\n")
	}
}

// read reads the event of line; a line that is no event is left aside.
func (d *streamDecoder) read(line []byte) {
	var got event
	if json.Unmarshal(line, &got) != nil {
		return
	}

	if got.SessionID != "" {
		d.inSession = true
	}

	switch got.Type {
	case "stream_event":
		if got.Event.Type == "content_block_delta" && got.Event.Delta.Type == "text_delta" && got.Event.Delta.Text != "" {
			d.printer.Text(got.Event.Delta.Text)
			d.shown, d.last = true, got.Event.Delta.Text[len(got.Event.Delta.Text)-1]
		}
	case "result":
		d.ended = true

		if got.IsError {
			d.failure = cmpOr(got.Result, got.Subtype)
		}
	}
}

// cmpOr returns the first of values that is not empty.
func cmpOr(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
