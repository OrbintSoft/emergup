// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Error is a problem in a configuration file: the file, where in it when that
// is known, the key concerned when there is one, and what is wrong.
type Error struct {
	// File is the path of the file as shown to the user.
	File string
	// Line and Column start at 1; they are 0 when the problem has no place
	// in the file, such as a file that cannot be read.
	Line   int
	Column int
	// Key is the dotted key, such as "steps.sync.min-interval", or empty.
	Key string
	Err error
}

// Error formats the problem as "file:line:column: key: what is wrong",
// leaving out the parts that are not known.
func (e *Error) Error() string {
	var text strings.Builder

	text.WriteString(e.File)

	if e.Line > 0 {
		fmt.Fprintf(&text, ":%d:%d", e.Line, e.Column)
	}

	text.WriteString(": ")

	if e.Key != "" {
		text.WriteString(e.Key)
		text.WriteString(": ")
	}

	text.WriteString(e.Err.Error())

	return text.String()
}

// Unwrap returns what is wrong.
func (e *Error) Unwrap() error {
	return e.Err
}

// fileError returns the Error for a problem with the whole of file.
func fileError(file string, err error) *Error {
	return &Error{File: file, Line: 0, Column: 0, Key: "", Err: err}
}

// How the messages name the kinds of values.
const (
	aBool  = "true or false"
	aList  = "a list"
	aTable = "a table"
)

// message is an error that is only a description of a problem.
type message string

func (m message) Error() string {
	return string(m)
}

// decodeProblems turns an error of the TOML decoder for file into Errors.
// keys, the keys the file sets, give the full key of a problem the decoder
// places inside an inline table, where it knows only the last part.
func decodeProblems(file string, keys map[string]keyPlace, err error) []*Error {
	if missing, isMissing := errors.AsType[*toml.StrictMissingError](err); isMissing {
		problems := make([]*Error, 0, len(missing.Errors))
		for i := range missing.Errors {
			problems = append(problems, fromDecodeError(file, keys, &missing.Errors[i], message("unknown key")))
		}

		return problems
	}

	if decodeErr, isDecode := errors.AsType[*toml.DecodeError](err); isDecode {
		return []*Error{fromDecodeError(file, keys, decodeErr, decoderReason(decodeErr))}
	}

	return []*Error{fileError(file, err)}
}

// fromDecodeError returns the Error for a problem the decoder found.
func fromDecodeError(file string, keys map[string]keyPlace, decodeErr *toml.DecodeError, reason error) *Error {
	line, column := decodeErr.Position()

	key, found := keyAt(keys, line, column)
	if !found {
		key = strings.Join(decodeErr.Key(), ".")
	}

	return &Error{File: file, Line: line, Column: column, Key: key, Err: reason}
}

// decoderReason says what the decoder found wrong, in the words of the
// configuration rather than of the Go types it decodes into.
func decoderReason(decodeErr *toml.DecodeError) error {
	text := strings.TrimPrefix(decodeErr.Error(), "toml: ")

	// "cannot decode TOML <found> into struct field <field> of type <expected>"
	rest, isTypeError := strings.CutPrefix(text, "cannot decode TOML ")
	found, field, hasField := strings.Cut(rest, " into struct field ")
	typeAt := strings.LastIndex(field, " of type ")

	if !isTypeError || !hasField || typeAt < 0 {
		return message(text)
	}

	expected := field[typeAt+len(" of type "):]

	return message(fmt.Sprintf("expected %s, found %s", describeGoType(expected), describeTOMLType(found)))
}

// describeGoType names what a value of the Go type looks like in a file.
func describeGoType(name string) string {
	switch name {
	case "bool":
		return aBool
	case "string":
		return "a string"
	case "[]string", "config.Entries":
		return "a list of strings"
	case "config.Duration":
		return (*Duration)(nil).expected()
	case "config.ElevationMethod":
		return (*ElevationMethod)(nil).expected()
	case "config.LiveRebuildMode":
		return (*LiveRebuildMode)(nil).expected()
	}

	if strings.HasPrefix(name, "config.") {
		return aTable
	}

	return name
}

// describeTOMLType names a TOML type as the decoder writes it.
func describeTOMLType(name string) string {
	switch name {
	case "integer":
		return "an integer"
	case "float":
		return "a number"
	case "bool", "boolean":
		return aBool
	case "array":
		return aList
	case "table", "inline table":
		return aTable
	}

	return "a " + name
}
