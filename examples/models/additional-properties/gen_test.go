// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package additional

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsJSON(t *testing.T) {
	t.Parallel()

	const data = `{"host":"a","at":"2026-09-30T00:00:00Z","mem":2,"cpu":0.5}`

	var m Metrics
	require.NoError(t, json.Unmarshal([]byte(data), &m))

	at := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, Metrics{Host: "a", At: &at, AdditionalProperties: map[string]float64{"cpu": 0.5, "mem": 2}}, m)

	out, err := json.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, `{"host":"a","at":"2026-09-30T00:00:00Z","cpu":0.5,"mem":2}`, string(out))
}

func TestMetricsGetSet(t *testing.T) {
	t.Parallel()

	var m Metrics
	_, found := m.Get("cpu")
	assert.False(t, found)

	m.Set("cpu", 0.5)
	m.Set("host", 1)
	value, found := m.Get("cpu")
	assert.True(t, found)
	assert.InDelta(t, 0.5, value, 0)

	out, err := json.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, `{"host":"","cpu":0.5}`, string(out))
}

func TestPropertyNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   interface{ Validate() error }
		wantErr string
	}{
		{name: "Keys that match", value: Headers{"content-type": "a", "x-trace": "b"}},
		{name: "Key that does not match the pattern", value: Headers{"Bad Key": "a"}, wantErr: "Bad Key: must match ^[a-z][a-z-]*$"},
		{name: "Key that is too long", value: Headers{"a-very-long-header-name": "a"}, wantErr: "a-very-long-header-name: must be at most 20 characters long"},
		{name: "Key from the enum", value: Units{"cm": 3}},
		{name: "Key outside the enum", value: Units{"lb": 3}, wantErr: "lb: must be one of cm, kg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.value.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
