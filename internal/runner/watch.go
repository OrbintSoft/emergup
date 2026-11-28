// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bytes"
	"io"
	"sync"
)

// maxWatchedLine is the longest line a watch gets: a longer one, such as a
// progress bar redrawn with carriage returns, is cut there.
const maxWatchedLine = 64 * 1024

// watcher hands a command's Watch what the log gets, line by line. Each
// stream has its own lines, written whole under a lock that both share, so
// that a line of standard error never cuts one of standard output. A
// failed write is dropped, as for the log.
type watcher struct {
	mu      sync.Mutex
	out     io.Writer
	streams []*watchedStream
}

func newWatcher(out io.Writer) *watcher {
	return &watcher{mu: sync.Mutex{}, out: out, streams: nil}
}

// stream returns the writer of one stream of the command; nothing reaches
// it without a watch.
func (w *watcher) stream() io.Writer {
	if w.out == nil {
		return io.Discard
	}

	stream := &watchedStream{watcher: w, pending: nil}
	w.streams = append(w.streams, stream)

	return stream
}

// flush hands the watch the last line of each stream, when it has no
// newline, with one.
func (w *watcher) flush() {
	for _, stream := range w.streams {
		if len(stream.pending) > 0 {
			w.write(append(stream.pending, '\n'))
			stream.pending = nil
		}
	}
}

func (w *watcher) write(lines []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = w.out.Write(lines)
}

// watchedStream keeps the part of a line its stream has not ended yet.
type watchedStream struct {
	watcher *watcher
	pending []byte
}

// Write implements io.Writer and always reports success.
func (s *watchedStream) Write(data []byte) (int, error) {
	s.pending = append(s.pending, data...)

	if end := bytes.LastIndexByte(s.pending, '\n'); end >= 0 {
		s.watcher.write(s.pending[:end+1])
		s.pending = append([]byte(nil), s.pending[end+1:]...)
	}

	for len(s.pending) > maxWatchedLine {
		s.watcher.write(append(s.pending[:maxWatchedLine:maxWatchedLine], '\n'))
		s.pending = append([]byte(nil), s.pending[maxWatchedLine:]...)
	}

	return len(data), nil
}
