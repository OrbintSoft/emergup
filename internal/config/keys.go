// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyPlace is where a file sets a key, and the kind of value it gives it.
type keyPlace struct {
	line   int
	column int
	kind   unstable.Kind
}

// fileKeys maps every dotted key that the TOML document data sets to where it
// sets it, keys inside inline tables included. It stops at a syntax error,
// which decoding reports.
func fileKeys(data []byte) map[string]keyPlace {
	var parser unstable.Parser

	parser.Reset(data)

	keys := make(map[string]keyPlace)

	var table []string

	for parser.NextExpression() {
		expression := parser.Expression()

		if expression.Kind == unstable.KeyValue {
			addKeyValue(&parser, keys, table, expression)

			continue
		}

		// Other expressions, such as comments, set no key.
		if expression.Kind == unstable.Table || expression.Kind == unstable.ArrayTable {
			table = keyParts(expression)
		}
	}

	return keys
}

// addKeyValue adds the key of the key-value node to keys, under the table
// prefix, and the keys of its value when that is an inline table.
func addKeyValue(parser *unstable.Parser, keys map[string]keyPlace, prefix []string, keyValue *unstable.Node) {
	parts := keyParts(keyValue)
	if len(parts) == 0 {
		return
	}

	first := keyValue.Key()
	first.Next()

	start := parser.Shape(first.Node().Raw).Start

	key := append(slices.Clone(prefix), parts...)
	value := keyValue.Value()

	keys[strings.Join(key, ".")] = keyPlace{line: start.Line, column: start.Column, kind: value.Kind}

	if value.Kind == unstable.InlineTable {
		for children := value.Children(); children.Next(); {
			addKeyValue(parser, keys, key, children.Node())
		}
	}
}

// keyParts returns the parts of the key of a table header or key-value node.
func keyParts(node *unstable.Node) []string {
	var parts []string

	for key := node.Key(); key.Next(); {
		parts = append(parts, string(key.Node().Data))
	}

	return parts
}

// keyAt returns the key set at line and column, if keys has one there.
func keyAt(keys map[string]keyPlace, line, column int) (string, bool) {
	for key, place := range keys {
		if place.line == line && place.column == column {
			return key, true
		}
	}

	return "", false
}
