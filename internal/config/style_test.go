// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boldBlue is a style several cases share.
const boldBlue = "bold blue"

// parseStyle reads text as a configuration file would.
func parseStyle(t *testing.T, text string) Style {
	t.Helper()

	var style Style
	require.NoError(t, style.UnmarshalText([]byte(text)))

	return style
}

func TestStyleText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
		sgr  string
	}{
		{"plain", "", "", ""},
		{"only spaces", "   ", "", ""},
		{"attribute and colour", boldBlue, boldBlue, "1;34"},
		{"attributes come first", "green bold", "bold green", "1;32"},
		{"every attribute", "reverse underline italic dim bold", "bold dim italic underline reverse", "1;2;3;4;7"},
		{"an attribute given twice", "bold blue bold", boldBlue, "1;34"},
		{"extra spaces", "  bold\tgreen  ", "bold green", "1;32"},
		{"every name", "black", "black", "30"},
		{"the last name", "white", "white", "37"},
		{"text and background", "red white", "red white", "31;47"},
		{"bright colours", "bright-black bright-white", "bright-black bright-white", "90;107"},
		{"default colour", "default red", "default red", "39;41"},
		{"numbered colours", "208 0", "208 0", "38;5;208;48;5;0"},
		{"the last number", "255", "255", "38;5;255"},
		{"rgb colours", "#FF8800 #000000", "#ff8800 #000000", "38;2;255;136;0;48;2;0;0;0"},
		{"magenta and cyan", "magenta cyan", "magenta cyan", "35;46"},
		{"yellow", "underline yellow", "underline yellow", "4;33"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			style := parseStyle(t, tt.text)

			text, err := style.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(text))
			assert.Equal(t, tt.sgr, style.SGR())
			assert.Equal(t, style, parseStyle(t, string(text)), "the written style reads back the same")
		})
	}
}

func TestStyleRejectsWhatIsNotAStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{"unknown word", "blod blue", `unknown style word "blod"`},
		{"upper case", "Bold", `unknown style word "Bold"`},
		{"three colours", "red green blue", `style "red green blue" has more than two colours`},
		{"number out of range", "256", "colour 256 is out of range 0–255"},
		{"negative number", "-1", `unknown style word "-1"`},
		{"short rgb", "#f80", `invalid colour "#f80", expected #rrggbb`},
		{"not hexadecimal", "#gg0000", `invalid colour "#gg0000", expected #rrggbb`},
		{"bright alone", "bright-", `unknown style word "bright-"`},
		{"bright default", "bright-default", `unknown style word "bright-default"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			style := parseStyle(t, "bold red")
			require.ErrorContains(t, style.UnmarshalText([]byte(tt.text)), tt.want)
			assert.Equal(t, parseStyle(t, "bold red"), style, "a style that is not read is left as it was")
		})
	}
}

func TestStyleSaysWhatItExpects(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `a style such as "bold blue"`, (*Style).expected(nil))
}
