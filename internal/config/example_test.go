// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

	for key, typ := range schemaKeys(reflect.TypeFor[Config]()) {
		if typ.Kind() != reflect.Struct || isTextValue(typ) {
			assert.Contains(t, shown, key, "the example main file does not show %s", key)
		}
	}
}
