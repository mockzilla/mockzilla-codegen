// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package anyof

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Person
	}{
		{name: "Both variants match", data: `{"name":"Ann","age":30}`, want: Person{Named: &Named{Name: "Ann"}, Aged: &Aged{Age: 30}}},
		{name: "One variant matches", data: `{"age":30}`, want: Person{Aged: &Aged{Age: 30}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Person
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestPersonValidate(t *testing.T) {
	t.Parallel()

	require.EqualError(t, Person{}.Validate(), "at least one variant must be set")
}

func TestStampJSON(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	var got Stamp
	require.NoError(t, json.Unmarshal([]byte(`"2026-09-30T00:00:00Z"`), &got))
	assert.Equal(t, Stamp{Time: &at, String: new("2026-09-30T00:00:00Z")}, got)

	out, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `"2026-09-30T00:00:00Z"`, string(out))

	out, err = json.Marshal(Stamp{})
	require.NoError(t, err)
	assert.JSONEq(t, `null`, string(out))
}
