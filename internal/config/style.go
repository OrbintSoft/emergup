// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Errors for a style the configuration cannot take.
var (
	errUnknownStyleWord = errors.New("unknown style word")
	errTooManyColors    = errors.New("has more than two colours")
	errColorOutOfRange  = errors.New("is out of range 0–255")
	errInvalidColor     = errors.New("invalid colour")
)

// Colors are the styles of the kinds of output emergup writes on a terminal.
type Colors struct {
	Heading Style `toml:"heading"`
	Prompt  Style `toml:"prompt"`
	Success Style `toml:"success"`
	Skip    Style `toml:"skip"`
	Failure Style `toml:"failure"`
	Notice  Style `toml:"notice"`
}

// Style is how one kind of output looks on a terminal. A configuration file
// writes it as words separated by spaces: attributes, and at most two
// colours, the text's and then the background's, such as "bold blue" or
// "black bright-yellow". "" is plain text.
type Style struct {
	attributes attribute
	foreground color
	background color
}

// attribute is a set of text attributes, one bit each.
type attribute uint8

// The attributes, in the order a style is written.
const (
	bold attribute = 1 << iota
	dim
	italic
	underline
	reverse
)

// attributeWord is an attribute with its name and its SGR parameter.
type attributeWord struct {
	attribute attribute
	name      string
	parameter string
}

// attributeWords are the attributes, in the order a style is written.
func attributeWords() []attributeWord {
	return []attributeWord{
		{bold, "bold", "1"},
		{dim, "dim", "2"},
		{italic, "italic", "3"},
		{underline, "underline", "4"},
		{reverse, "reverse", "7"},
	}
}

// color is a colour of the terminal: one of the eight named colours or its
// bright variant, the terminal's own colour, one of the 256 numbered colours
// or an RGB colour.
type color struct {
	kind  colorKind
	value uint32
}

// colorKind says how a color is given.
type colorKind uint8

// The kinds of color; the zero value is no colour at all.
const (
	noColor colorKind = iota
	namedColor
	brightColor
	defaultColor
	numberedColor
	rgbColor
)

// The named colours: what each adds to the SGR parameter of the first one.
const (
	black uint32 = iota
	red
	green
	yellow
	blue
	magenta
	cyan
	white
)

// colorNames are the names of the colours, in the order of their values.
func colorNames() []string {
	return []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
}

// The words of a style besides the names.
const (
	brightPrefix = "bright-"
	defaultWord  = "default"
	rgbPrefix    = "#"
)

// The parameters of the SGR sequence that set a colour, for the text; the
// background's are backgroundOffset higher.
const (
	namedBase        = 30
	brightBase       = 90
	defaultParameter = 39
	extendedBase     = 38
	backgroundOffset = 10
)

// The largest numbered colour, and the length of an RGB colour's digits.
const (
	maxNumberedColor = 255
	rgbDigits        = 6
)

// Where the components of an RGB colour are in its value, 0xrrggbb.
const (
	redShift      = 16
	greenShift    = 8
	componentMask = 0xff
)

// style returns a style with attributes and a text colour, on the
// terminal's background.
func style(attributes attribute, foreground color) Style {
	return Style{attributes: attributes, foreground: foreground, background: none()}
}

// none is the absence of a colour: the text keeps the terminal's colours.
func none() color {
	return color{kind: noColor, value: 0}
}

// named returns the named colour value.
func named(value uint32) color {
	return color{kind: namedColor, value: value}
}

// UnmarshalText reads a style such as "bold blue".
func (s *Style) UnmarshalText(text []byte) error {
	read := style(0, none())
	colors := 0

	for word := range strings.FieldsSeq(string(text)) {
		if attribute, isAttribute := attributeNamed(word); isAttribute {
			read.attributes |= attribute

			continue
		}

		parsed, err := parseColor(word)
		if err != nil {
			return err
		}

		switch colors {
		case 0:
			read.foreground = parsed
		case 1:
			read.background = parsed
		default:
			return fmt.Errorf("style %q %w: the first is the text's, the second the background's",
				text, errTooManyColors)
		}

		colors++
	}

	*s = read

	return nil
}

// attributeNamed returns the attribute called name, if there is one.
func attributeNamed(name string) (attribute, bool) {
	for _, each := range attributeWords() {
		if each.name == name {
			return each.attribute, true
		}
	}

	return 0, false
}

// parseColor reads one colour of a style.
func parseColor(word string) (color, error) {
	if parsed, isNamed := parseNamedColor(word); isNamed {
		return parsed, nil
	}

	if digits, isRGB := strings.CutPrefix(word, rgbPrefix); isRGB {
		value, err := strconv.ParseUint(digits, 16, 32)
		if err != nil || len(digits) != rgbDigits {
			return none(), fmt.Errorf("%w %q, expected #rrggbb", errInvalidColor, word)
		}

		return color{kind: rgbColor, value: uint32(value)}, nil
	}

	if strings.Trim(word, "0123456789") == "" {
		value, err := strconv.ParseUint(word, 10, 32)
		if err != nil || value > maxNumberedColor {
			return none(), fmt.Errorf("colour %s %w", word, errColorOutOfRange)
		}

		return color{kind: numberedColor, value: uint32(value)}, nil
	}

	return none(), fmt.Errorf("%w %q, expected bold, dim, italic, underline, reverse "+
		"or a colour: a name such as red or bright-red, default, a number from 0 to 255 or #rrggbb",
		errUnknownStyleWord, word)
}

// parseNamedColor reads a colour written as a name: a colour's, a bright
// colour's, or the terminal's own.
func parseNamedColor(word string) (color, bool) {
	if word == defaultWord {
		return color{kind: defaultColor, value: 0}, true
	}

	kind := namedColor

	name, isBright := strings.CutPrefix(word, brightPrefix)
	if isBright {
		kind = brightColor
	}

	index := slices.Index(colorNames(), name)
	if index < 0 {
		return none(), false
	}

	return color{kind: kind, value: uint32(index)}, true //nolint:gosec // an index of the eight names
}

// MarshalText writes the style the way a configuration file does: the
// attributes, then the colours. A style read from a file has a background
// only after a text colour, so the colours keep their places.
func (s Style) MarshalText() ([]byte, error) {
	var words []string

	for _, each := range attributeWords() {
		if s.attributes&each.attribute != 0 {
			words = append(words, each.name)
		}
	}

	for _, each := range []color{s.foreground, s.background} {
		if each.kind != noColor {
			words = append(words, each.name())
		}
	}

	return []byte(strings.Join(words, " ")), nil
}

// name is how a configuration file writes the colour.
func (c color) name() string {
	switch c.kind {
	case namedColor:
		return colorNames()[c.value]
	case brightColor:
		return brightPrefix + colorNames()[c.value]
	case defaultColor:
		return defaultWord
	case numberedColor:
		return strconv.FormatUint(uint64(c.value), 10)
	case rgbColor:
		return fmt.Sprintf("%s%06x", rgbPrefix, c.value)
	case noColor:
	}

	return ""
}

// SGR returns the parameters of the terminal's Select Graphic Rendition
// sequence for the style, such as "1;34", or "" for plain text.
func (s *Style) SGR() string {
	var parameters []string

	for _, each := range attributeWords() {
		if s.attributes&each.attribute != 0 {
			parameters = append(parameters, each.parameter)
		}
	}

	if s.foreground.kind != noColor {
		parameters = append(parameters, s.foreground.parameters(0))
	}

	if s.background.kind != noColor {
		parameters = append(parameters, s.background.parameters(backgroundOffset))
	}

	return strings.Join(parameters, ";")
}

// parameters returns the SGR parameters that set the colour, offset by 10
// for the background.
func (c color) parameters(offset int) string {
	switch c.kind {
	case namedColor:
		return strconv.Itoa(namedBase + offset + int(c.value))
	case brightColor:
		return strconv.Itoa(brightBase + offset + int(c.value))
	case defaultColor:
		return strconv.Itoa(defaultParameter + offset)
	case numberedColor:
		return fmt.Sprintf("%d;5;%d", extendedBase+offset, c.value)
	case rgbColor:
		return fmt.Sprintf("%d;2;%d;%d;%d", extendedBase+offset,
			c.value>>redShift, c.value>>greenShift&componentMask, c.value&componentMask)
	case noColor:
	}

	return ""
}

// expected says what a style looks like in a configuration file.
func (*Style) expected() string {
	return `a style such as "bold blue"`
}
