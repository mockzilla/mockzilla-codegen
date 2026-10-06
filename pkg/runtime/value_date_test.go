// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDateJSON(t *testing.T) {
	t.Parallel()

	type holder struct {
		Day  Date  `json:"day"`
		Next *Date `json:"next"`
	}

	in := holder{Day: NewDate(2026, time.September, 29)}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	assert.JSONEq(t, `{"day":"2026-09-29","next":null}`, string(b))

	var out holder
	require.NoError(t, json.Unmarshal(b, &out))
	assert.Equal(t, in, out)
	assert.Equal(t, "2026-09-29", out.Day.String())
}

func TestDateUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    Date
		wantErr string
	}{
		{name: "Date", in: `"2026-01-02"`, want: NewDate(2026, time.January, 2)},
		{name: "Null keeps the value", in: `null`, want: NewDate(2000, time.January, 1)},
		{name: "Not a string", in: `20260102`, wantErr: "invalid date: 20260102"},
		{name: "Date and time", in: `"2026-01-02T10:00:00Z"`, wantErr: `invalid date: "2026-01-02T10:00:00Z"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewDate(2000, time.January, 1)
			err := d.UnmarshalJSON([]byte(tt.in))
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrInvalidDate)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, d)
		})
	}
}

func TestDateText(t *testing.T) {
	t.Parallel()

	b, err := NewDate(1999, time.December, 31).MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "1999-12-31", string(b))

	var d Date
	require.NoError(t, d.UnmarshalText(b))
	assert.Equal(t, NewDate(1999, time.December, 31), d)
	require.ErrorIs(t, d.UnmarshalText([]byte("31.12.1999")), ErrInvalidDate)
}
