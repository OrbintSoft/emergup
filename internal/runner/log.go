// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// logWriter writes to the log of one command. A command's standard output
// and standard error are copied by two goroutines, so writes are serialised;
// a failed write is dropped, so that the copy to the terminal goes on.
type logWriter struct {
	mu  sync.Mutex
	log io.Writer
}

func newLogWriter(log io.Writer) *logWriter {
	return &logWriter{mu: sync.Mutex{}, log: orDiscard(log)}
}

// Write implements io.Writer and always reports success.
func (l *logWriter) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	_, _ = l.log.Write(data)

	return len(data), nil
}

func (l *logWriter) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(l, format, args...)
}

// commandLine writes args as a shell would need them, so that a command in
// the log can be copied and run again.
func commandLine(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}

	return strings.Join(quoted, " ")
}

// shellQuote leaves an argument the shell would not change as it is, and
// puts any other in single quotes.
func shellQuote(arg string) string {
	if arg != "" && strings.IndexFunc(arg, needsQuoting) < 0 {
		return arg
	}

	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func needsQuoting(char rune) bool {
	switch {
	case 'a' <= char && char <= 'z', 'A' <= char && char <= 'Z', '0' <= char && char <= '9':
		return false
	}

	return !strings.ContainsRune("@%+=:,./-_", char)
}
