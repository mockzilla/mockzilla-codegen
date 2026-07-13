// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByteSizeUnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		want    ByteSize
		wantErr string
	}{
		{name: "Plain integer is bytes", text: "1024", want: 1024},
		{name: "Zero is allowed", text: "0", want: 0},
		{name: "Bytes unit", text: "512B", want: 512},
		{name: "Kilobytes are 1024 bytes", text: "512KB", want: 512 << 10},
		{name: "Megabytes", text: "32MB", want: 32 << 20},
		{name: "Gigabytes", text: "2GB", want: 2 << 30},
		{
			name:    "Lower case unit is rejected",
			text:    "32mb",
			wantErr: `invalid byte size "32mb", want an integer or a size such as 512KB or 32MB`,
		},
		{
			name:    "Unit without a number is rejected",
			text:    "MB",
			wantErr: `invalid byte size "MB", want an integer or a size such as 512KB or 32MB`,
		},
		{
			name:    "Negative size is rejected",
			text:    "-1",
			wantErr: `invalid byte size "-1", want an integer or a size such as 512KB or 32MB`,
		},
		{
			name:    "Space before the unit is rejected",
			text:    "32 MB",
			wantErr: `invalid byte size "32 MB", want an integer or a size such as 512KB or 32MB`,
		},
		{
			name:    "Size past int64 is rejected",
			text:    "9999999999GB",
			wantErr: `invalid byte size "9999999999GB", want an integer or a size such as 512KB or 32MB`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got ByteSize
			err := got.UnmarshalText([]byte(tc.text))

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrByteSize)
				assert.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
