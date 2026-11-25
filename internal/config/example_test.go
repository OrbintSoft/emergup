// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// examples is the example configuration of the repository, laid out as on
// the root of a system.
func examples() fs.FS {
	return os.DirFS(filepath.Join("..", "..", "examples"))
}

func TestExampleConfigurationIsValid(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), examples())
	require.NoError(t, err)
	assert.Equal(t,
		Entries{"x11-drivers/nvidia-drivers", "sys-kernel/gentoo-sources", "sys-kernel/linux-headers"},
		cfg.Steps.Update.Exclude)
	assert.Equal(t, Entries{"orbintsoft"}, cfg.Steps.Sync.AlwaysSync)
}

// commentedSetting is a setting the example main file shows commented out:
// "#key = value", where a comment in prose starts with "# ".
var commentedSetting = regexp.MustCompile(`(?m)^#([a-z][a-z0-9-]* = )`)

// TestExampleMainFileDocumentsEveryDefault keeps the example main file in
// step with the code: with its settings uncommented it must give exactly the
// generic defaults, and it must show every value of the schema.
func TestExampleMainFileDocumentsEveryDefault(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(examples(), MainFile)
	require.NoError(t, err)

	uncommented := commentedSetting.ReplaceAll(data, []byte("$1"))

	cfg, err := Load(t.Context(), fstest.MapFS{MainFile: &fstest.MapFile{Data: uncommented}})
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)

	shown := fileKeys(uncommented)

	// The hooks have no defaults: the file shows an example instead.
	for key, typ := range schemaKeys(reflect.TypeFor[Config]()) {
		isHookKey := key == hooksTable || strings.HasPrefix(key, hooksTable+".")
		if !isHookKey && (typ.Kind() != reflect.Struct || isTextValue(typ)) {
			assert.Contains(t, shown, key, "the example main file does not show %s", key)
		}
	}
}

// exampleHook matches the hook the example main file shows: the lines of
// a commented "# [hooks.<name>]" table, each starting with "# ".
var exampleHook = regexp.MustCompile(`(?m)^# (\[hooks\.[a-z0-9-]+\]\n)((?:# .+\n)+)`)

// TestExampleMainFileShowsAValidHook keeps the example hook of the main file
// valid: uncommented, it is a hook that runs.
func TestExampleMainFileShowsAValidHook(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(examples(), MainFile)
	require.NoError(t, err)

	match := exampleHook.FindSubmatch(data)
	require.NotNil(t, match, "the example main file shows no hook")

	hook := string(match[1]) + strings.ReplaceAll(string(match[2]), "# ", "")

	cfg, err := Load(t.Context(), fstest.MapFS{MainFile: &fstest.MapFile{Data: []byte(hook)}})
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 1)

	for _, each := range cfg.Hooks {
		assert.True(t, each.Enabled)
		assert.Equal(t, HookPlace("before-update"), each.At)
		assert.NotEmpty(t, each.Command)
	}
}
