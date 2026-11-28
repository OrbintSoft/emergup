// SPDX-License-Identifier: GPL-3.0-or-later

package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// TestTheDecoderReadsEventsThatArriveInPieces keeps the answer whole when
// a pipe gives the events a few bytes at a time, and the last line without
// its newline.
func TestTheDecoderReadsEventsThatArriveInPieces(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	decoder := &streamDecoder{printer: ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false}}
	stream := "a warning that is no event\n" +
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"one\n"}}}` +
		"\n" +
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"two"}}}` +
		"\n" +
		`{"type":"result","is_error":true,"subtype":"error_max_turns","session_id":"s"}`

	for index := range len(stream) {
		_, _ = decoder.Write([]byte{stream[index]})
	}

	decoder.finish()

	assert.Equal(t, "one\ntwo\n", out.String())
	assert.True(t, decoder.ended)
	assert.True(t, decoder.inSession)
	assert.Equal(t, "error_max_turns", decoder.failure, "the kind of error, when it has no text")
}
