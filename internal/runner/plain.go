// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"io"
	"strings"
)

// The bytes that start and end escape sequences.
const (
	escape = 0x1b
	bell   = 0x07
)

// sequenceState is where a plainWriter is in the text it is given.
type sequenceState int

// The states of a plainWriter.
const (
	// inText: the bytes are text.
	inText sequenceState = iota
	// afterEscape: an ESC starts a sequence.
	afterEscape
	// inControl: a control sequence (ESC [), such as a colour, until its
	// final byte.
	inControl
	// inEscapeSequence: an escape sequence with intermediate bytes (ESC ( B),
	// until its final byte.
	inEscapeSequence
	// inString: a string sequence (ESC ]), such as the terminal's title,
	// until BEL or ESC \.
	inString
	// inStringEscape: an ESC inside a string sequence.
	inStringEscape
)

// plainWriter copies to out the text it is given without the terminal's
// escape sequences, as ECMA-48 defines them: colours, cursor movements,
// titles. A sequence may be split across writes. A string sequence ends at
// the end of a line, so that a broken one cannot leave out the rest of the
// output.
type plainWriter struct {
	out   io.Writer
	state sequenceState
	text  []byte
}

// newPlainWriter returns a writer that copies to out what it is given,
// without the terminal's escape sequences.
func newPlainWriter(out io.Writer) io.Writer {
	return &plainWriter{out: out, state: inText, text: nil}
}

// Write implements io.Writer. It reports the whole of data as written, so
// that the copy to the terminal beside it goes on whatever out does.
func (p *plainWriter) Write(data []byte) (int, error) {
	p.text = p.text[:0]

	for _, char := range data {
		if p.isText(char) {
			p.text = append(p.text, char)
		}
	}

	if len(p.text) > 0 {
		_, _ = p.out.Write(p.text)
	}

	return len(data), nil
}

// isText moves past char and reports whether it is text.
func (p *plainWriter) isText(char byte) bool {
	switch p.state {
	case inText:
		if char == escape {
			p.state = afterEscape

			return false
		}

		return true
	case afterEscape:
		return p.startSequence(char)
	case inControl:
		return p.continueSequence(char, isParameterOrIntermediate, isControlFinal)
	case inEscapeSequence:
		return p.continueSequence(char, isIntermediate, isEscapeFinal)
	case inString:
		return p.continueString(char)
	case inStringEscape:
		if char == '\\' {
			p.state = inText

			return false
		}

		return p.startSequence(char)
	}

	return true
}

// startSequence moves past the byte after an ESC.
func (p *plainWriter) startSequence(char byte) bool {
	switch {
	case char == '[':
		p.state = inControl
	case isStringStart(char):
		p.state = inString
	case char == escape:
	case isIntermediate(char):
		p.state = inEscapeSequence
	case isEscapeFinal(char):
		p.state = inText
	default:
		return p.breakOff(char)
	}

	return false
}

// continueSequence moves past a byte of a sequence that goes on with the
// bytes middle accepts and ends with one final accepts.
func (p *plainWriter) continueSequence(char byte, middle, final func(byte) bool) bool {
	switch {
	case middle(char):
	case final(char):
		p.state = inText
	default:
		return p.breakOff(char)
	}

	return false
}

// continueString moves past a byte of a string sequence.
func (p *plainWriter) continueString(char byte) bool {
	switch char {
	case bell:
		p.state = inText
	case escape:
		p.state = inStringEscape
	case '\n':
		p.state = inText

		return true
	}

	return false
}

// breakOff ends a sequence that char does not belong to: char is read again
// as text, or as the start of another sequence.
func (p *plainWriter) breakOff(char byte) bool {
	p.state = inText

	return p.isText(char)
}

// isStringStart reports whether ESC and char start a string sequence: an
// operating system command, a device control string, or a privacy, start or
// application program command.
func isStringStart(char byte) bool {
	return strings.IndexByte("]PX^_", char) >= 0
}

func isIntermediate(char byte) bool {
	return 0x20 <= char && char <= 0x2f
}

func isParameterOrIntermediate(char byte) bool {
	return 0x20 <= char && char <= 0x3f
}

func isControlFinal(char byte) bool {
	return 0x40 <= char && char <= 0x7e
}

func isEscapeFinal(char byte) bool {
	return 0x30 <= char && char <= 0x7e
}

// StripEscapes returns text without the terminal's escape sequences, as the
// log has it.
func StripEscapes(text string) string {
	var plain strings.Builder

	_, _ = newPlainWriter(&plain).Write([]byte(text))

	return plain.String()
}
