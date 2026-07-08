// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationUnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		want    Duration
		wantErr string
	}{
		{name: "Seconds", text: "30s", want: Duration(30 * time.Second)},
		{name: "Mixed units", text: "1m30s", want: Duration(90 * time.Second)},
		{name: "Fraction", text: "1.5h", want: Duration(90 * time.Minute)},
		{name: "Zero", text: "0", want: 0},
		{name: "Missing unit is rejected", text: "30", wantErr: `invalid duration "30", want a value such as 30s or 1m30s`},
		{name: "Unknown unit is rejected", text: "2d", wantErr: `invalid duration "2d", want a value such as 30s or 1m30s`},
		{name: "Negative duration is rejected", text: "-1s", wantErr: `invalid duration "-1s", want a value such as 30s or 1m30s`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Duration
			err := got.UnmarshalText([]byte(tc.text))

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrDuration)
				assert.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
