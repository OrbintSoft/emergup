// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/cli"
	"github.com/OrbintSoft/emergup/internal/version"
)

const usageLine = "usage: emergup <command>"

func run(t *testing.T, invokedAs string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Main(t.Context(), &out, &errOut, invokedAs, args)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := run(t, "/usr/bin/emergup", arg)
			assert.Equal(t, 0, code)
			assert.Equal(t, "emergup "+version.String()+"\n", stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := run(t, "/usr/bin/emergup", arg)
			assert.Equal(t, 0, code)
			assert.Contains(t, stdout, usageLine)
			assert.Contains(t, stdout, "version")
			assert.Empty(t, stderr)
		})
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{"no command", nil, ""},
		{"unknown command", []string{"frobnicate"}, `emergup: unknown command "frobnicate"`},
		{"unknown option", []string{"--frobnicate"}, `emergup: unknown option "--frobnicate"`},
		{"argument after version", []string{"version", "extra"}, `emergup version: unexpected argument "extra"`},
		{"argument after help", []string{"help", "extra"}, `emergup help: unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(t, "/usr/bin/emergup", tt.args...)
			assert.Equal(t, 2, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, tt.message)
			assert.Contains(t, stderr, usageLine)
		})
	}
}

func TestUsageNamesTheInvokedProgram(t *testing.T) {
	code, stdout, _ := run(t, "/opt/test/emergup-dev", "help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "usage: emergup-dev <command>")
}
