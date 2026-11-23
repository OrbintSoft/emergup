// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationText(t *testing.T) {
	t.Parallel()

	var d Duration
	require.NoError(t, d.UnmarshalText([]byte("1h30m")))
	assert.Equal(t, Duration(90*time.Minute), d)
	assert.Equal(t, "1h30m0s", d.String())

	require.ErrorContains(t, d.UnmarshalText([]byte("1d")), `"1d"`)
}

func TestElevationMethodNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method ElevationMethod
	}{
		{"auto", ElevationAuto},
		{"sudo", ElevationSudo},
		{"doas", ElevationDoas},
		{"run0", ElevationRun0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.name, tt.method.String())

			var method ElevationMethod
			require.NoError(t, method.UnmarshalText([]byte(tt.name)))
			assert.Equal(t, tt.method, method)
		})
	}
}

func TestLiveRebuildModeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode LiveRebuildMode
	}{
		{"auto", LiveRebuildAuto},
		{"smart", LiveRebuildSmart},
		{"interval", LiveRebuildInterval},
		{"always", LiveRebuildAlways},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.name, tt.mode.String())

			var mode LiveRebuildMode
			require.NoError(t, mode.UnmarshalText([]byte(tt.name)))
			assert.Equal(t, tt.mode, mode)
		})
	}
}

func TestUnknownNames(t *testing.T) {
	t.Parallel()

	var method ElevationMethod

	err := method.UnmarshalText([]byte("su"))
	require.ErrorContains(t, err, `"su"`)
	require.ErrorContains(t, err, "auto, sudo, doas, run0")

	var mode LiveRebuildMode

	err = mode.UnmarshalText([]byte("smrt"))
	require.ErrorContains(t, err, `"smrt"`)
	require.ErrorContains(t, err, "auto, smart, interval, always")
}

func TestNameOfAValueOutOfRange(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ElevationMethod(9)", ElevationMethod(9).String())
	assert.Equal(t, "LiveRebuildMode(-1)", LiveRebuildMode(-1).String())
}
