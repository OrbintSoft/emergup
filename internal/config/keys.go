// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bytes"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyPlace is where a file sets a key, and the kind of value it gives it.
type keyPlace struct {
	line   int
	column int
	offset int
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

	start := keyStart(parser, keyValue)

	key := append(slices.Clone(prefix), parts...)
	value := keyValue.Value()

	keys[strings.Join(key, ".")] = keyPlace{line: start.Line, column: start.Column, offset: start.Offset, kind: value.Kind}

	if value.Kind == unstable.InlineTable {
		for children := value.Children(); children.Next(); {
			addKeyValue(parser, keys, key, children.Node())
		}
	}
}

// keyStart returns where the key of a table header or key-value node starts:
// at its first part, a quoted part's quote included.
func keyStart(parser *unstable.Parser, node *unstable.Node) unstable.Position {
	first := node.Key()
	first.Next()

	return parser.Shape(first.Node().Raw).Start
}

// region is the part of a TOML document that a key-value outside any inline
// table spans, in bytes: from its key up to the next key-value or table
// header, the blank lines and comments after it included.
type region struct {
	start int
	end   int
}

// keyValueRegions returns the regions of the key-values of the TOML document
// data, in order, and whether data is TOML: false at a syntax error.
func keyValueRegions(data []byte) ([]region, bool) {
	var parser unstable.Parser

	parser.Reset(data)

	var regions []region

	open := -1

	for parser.NextExpression() {
		expression := parser.Expression()

		isKeyValue := expression.Kind == unstable.KeyValue
		isHeader := expression.Kind == unstable.Table || expression.Kind == unstable.ArrayTable

		// Other expressions, such as comments, end no region.
		if !isKeyValue && !isHeader {
			continue
		}

		start := keyStart(&parser, expression).Offset
		if isHeader {
			start = headerStart(data, start)
		}

		if open >= 0 {
			regions = append(regions, region{start: open, end: start})
		}

		open = -1
		if isKeyValue {
			open = start
		}
	}

	if parser.Error() != nil {
		return nil, false
	}

	if open >= 0 {
		regions = append(regions, region{start: open, end: len(data)})
	}

	return regions, true
}

// headerStart returns the offset of the "[" or "[[" that opens the table
// header whose key starts at the offset key; only spaces and tabs may come
// between them.
func headerStart(data []byte, key int) int {
	start := key
	for start > 0 && (data[start-1] == ' ' || data[start-1] == '\t') {
		start--
	}

	for start > 0 && data[start-1] == '[' {
		start--
	}

	return start
}

// offsetAt returns the offset in data of line and column, both counted from
// 1 and in bytes, as the TOML decoder places its errors.
func offsetAt(data []byte, line, column int) int {
	offset := 0
	for range line - 1 {
		next := bytes.IndexByte(data[offset:], '\n')
		if next < 0 {
			return len(data)
		}

		offset += next + 1
	}

	return min(offset+column-1, len(data))
}

// blank replaces the bytes of data in r with spaces, all but the line
// breaks, so that every other byte keeps its line and column.
func blank(data []byte, r region) {
	for i := r.start; i < r.end; i++ {
		if data[i] != '\n' {
			data[i] = ' '
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
