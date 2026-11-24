// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// errUnsupportedType is a configuration value WriteTOML cannot write, a sign
// of a schema type it does not know yet.
var errUnsupportedType = errors.New("unsupported type")

// defaultOrigin is the origin written for a value no file sets.
const defaultOrigin = "default"

// table is a TOML table as WriteTOML writes it: its header and its values.
type table struct {
	name  string
	lines []string
}

// subTable is a struct field that is a table of its own, with its key.
type subTable struct {
	name  string
	value reflect.Value
}

// WriteTOML writes cfg to out as a configuration file, every value followed by
// a comment that names the file it comes from, or "default". A list of
// entries from several files has one entry per line, each with its origin.
// What it writes reads back to cfg.
func WriteTOML(out io.Writer, cfg Config, origins Origins) error {
	var tables []table

	err := collectTables(&tables, "", reflect.ValueOf(cfg), origins)
	if err != nil {
		return err
	}

	written := make([]string, 0, len(tables))
	for _, each := range tables {
		written = append(written, "["+each.name+"]\n"+strings.Join(each.lines, "\n")+"\n")
	}

	_, err = io.WriteString(out, strings.Join(written, "\n"))
	if err != nil {
		return fmt.Errorf("writing the configuration: %w", err)
	}

	return nil
}

// collectTables appends the table name, holding the values of the struct
// value, to tables when it has values of its own, then the tables of its
// struct fields, in the order of the fields.
func collectTables(tables *[]table, name string, value reflect.Value, origins Origins) error {
	var (
		lines     []string
		subTables []subTable
	)

	err := collectValues(&lines, &subTables, name, value, origins)
	if err != nil {
		return err
	}

	if len(lines) > 0 {
		*tables = append(*tables, table{name: name, lines: lines})
	}

	for _, sub := range subTables {
		err := collectTables(tables, sub.name, sub.value, origins)
		if err != nil {
			return err
		}
	}

	return nil
}

// collectValues appends a line for each value of the struct value, the key
// of the table name, to lines, and its fields that are tables to subTables;
// an embedded struct adds its fields to the same table.
func collectValues(lines *[]string, subTables *[]subTable, name string, value reflect.Value, origins Origins) error {
	for field := range value.Type().Fields() {
		fieldValue := value.FieldByIndex(field.Index)

		if field.Anonymous {
			err := collectValues(lines, subTables, name, fieldValue, origins)
			if err != nil {
				return err
			}

			continue
		}

		if field.Type.Kind() == reflect.Struct && !isTextValue(field.Type) {
			*subTables = append(*subTables, subTable{name: joinKey(name, tomlName(field)), value: fieldValue})

			continue
		}

		line, err := valueLine(tomlName(field), joinKey(name, tomlName(field)), fieldValue.Interface(), origins)
		if err != nil {
			return err
		}

		*lines = append(*lines, line)
	}

	return nil
}

// valueLine writes `name = value  # origin` for the value of key.
func valueLine(name, key string, value any, origins Origins) (string, error) {
	origin := originText(origins.Of(key))

	switch typed := value.(type) {
	case Entries:
		return entriesLine(name, key, typed, origin, origins), nil
	case encoding.TextMarshaler:
		text, err := typed.MarshalText()
		if err != nil {
			return "", fmt.Errorf("writing %s: %w", key, err)
		}

		return name + " = " + tomlString(string(text)) + "  # " + origin, nil
	case bool:
		return name + " = " + strconv.FormatBool(typed) + "  # " + origin, nil
	case int:
		return name + " = " + strconv.Itoa(typed) + "  # " + origin, nil
	case string:
		return name + " = " + tomlString(typed) + "  # " + origin, nil
	case []string:
		return name + " = " + tomlList(typed) + "  # " + origin, nil
	}

	return "", fmt.Errorf("writing %s: %w %T", key, errUnsupportedType, value)
}

// entriesLine writes the list of entries key. The comment of an empty list
// names the file that emptied it; a list whose entries all come from one
// place is written on one line with that place, any other list one entry per
// line, each with its own origin.
func entriesLine(name, key string, entries Entries, keyOrigin string, origins Origins) string {
	if len(entries) == 0 {
		return name + " = []  # " + keyOrigin
	}

	entryOrigins := make([]string, len(entries))
	for i, entry := range entries {
		entryOrigins[i] = originText(origins.OfEntry(key, entry))
	}

	if allEqual(entryOrigins) {
		return name + " = " + tomlList(entries) + "  # " + entryOrigins[0]
	}

	var text strings.Builder

	text.WriteString(name + " = [\n")

	for i, entry := range entries {
		text.WriteString("  " + tomlString(entry) + ",  # " + entryOrigins[i] + "\n")
	}

	text.WriteString("]")

	return text.String()
}

// tomlList writes a list of strings on one line.
func tomlList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = tomlString(value)
	}

	return "[" + strings.Join(quoted, ", ") + "]"
}

// tomlString writes s as a TOML basic string.
func tomlString(s string) string {
	var text strings.Builder

	text.WriteByte('"')

	for _, char := range s {
		switch char {
		case '"':
			text.WriteString(`\"`)
		case '\\':
			text.WriteString(`\\`)
		case '\n':
			text.WriteString(`\n`)
		case '\t':
			text.WriteString(`\t`)
		case '\r':
			text.WriteString(`\r`)
		default:
			if char < ' ' || char == 0x7f {
				fmt.Fprintf(&text, `\u%04X`, char)
			} else {
				text.WriteRune(char)
			}
		}
	}

	text.WriteByte('"')

	return text.String()
}

// originText is how the origin of a value is written: the file, or default.
func originText(file string) string {
	if file == "" {
		return defaultOrigin
	}

	return file
}

func allEqual(values []string) bool {
	for _, value := range values {
		if value != values[0] {
			return false
		}
	}

	return true
}

// tomlName is the key of field in a configuration file.
func tomlName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("toml"), ",")

	return name
}

// joinKey adds name to the dotted key of its table.
func joinKey(table, name string) string {
	if table == "" {
		return name
	}

	return table + "." + name
}
