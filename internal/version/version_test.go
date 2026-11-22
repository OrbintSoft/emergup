// SPDX-License-Identifier: GPL-3.0-or-later

package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func buildInfo(mainVersion string) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}}, true
	}
}

func noBuildInfo() (*debug.BuildInfo, bool) {
	return nil, false
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name          string
		stamped       string
		readBuildInfo func() (*debug.BuildInfo, bool)
		want          string
	}{
		{"stamped value wins", "1.2.3", buildInfo("v9.9.9"), "1.2.3"},
		{"module version", "", buildInfo("v0.1.0"), "v0.1.0"},
		{"pseudo-version from a checkout", "", buildInfo("v0.0.0-20261122101500-0123456789ab+dirty"), "v0.0.0-20261122101500-0123456789ab+dirty"},
		{"development build", "", buildInfo("(devel)"), "(devel)"},
		{"empty module version", "", buildInfo(""), "unknown"},
		{"no build information", "", noBuildInfo, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolve(tt.stamped, tt.readBuildInfo))
		})
	}
}

func TestStringIsNeverEmpty(t *testing.T) {
	assert.NotEmpty(t, String())
}
