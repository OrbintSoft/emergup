// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlainWriterLeavesOutEscapeSequences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{"text", "hello\n", "hello\n"},
		{"colours", "\x1b[32;01m*\x1b[39;49;00m done\n", "* done\n"},
		{"cursor movement", "\x1b[2Kline\x1b[1A", "line"},
		{"title ended by BEL", "\x1b]2;emerge: (3 of 7)\x07next\n", "next\n"},
		{"title ended by ST", "\x1b]0;title\x1b\\next", "next"},
		{"character set", "\x1b(Bplain", "plain"},
		{"two-byte sequences", "\x1b7saved\x1b8", "saved"},
		{"an unterminated string ends at the end of the line", "\x1b]2;title\nnext\n", "\nnext\n"},
		{"a broken sequence gives back its text", "\x1b[3\nx", "\nx"},
		{"an escape that starts again", "\x1b\x1b[1mbold", "bold"},
		{"an escape at the end", "end\x1b", "end"},
		{"other control characters stay", "a\tb\rc\x07\n", "a\tb\rc\x07\n"},
		{"UTF-8", "café ✓\n", "café ✓\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var whole bytes.Buffer

			written, err := newPlainWriter(&whole).Write([]byte(tt.text))
			require.NoError(t, err)
			assert.Equal(t, len(tt.text), written, "the whole text counts as written")
			assert.Equal(t, tt.want, whole.String())

			var split bytes.Buffer

			plain := newPlainWriter(&split)
			for i := range len(tt.text) {
				_, err := plain.Write([]byte{tt.text[i]})
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, split.String(), "a sequence split across writes")
		})
	}
}

func TestStripEscapes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Total: 0 packages\n", StripEscapes("\x1b[32mTotal: \x1b[1m0\x1b[0m packages\n"))
	assert.Equal(t, "plain\n", StripEscapes("plain\n"))
	assert.Equal(t, "after\n", StripEscapes("\x1b]0;title\aafter\n"))
	assert.Empty(t, StripEscapes(""))
}
