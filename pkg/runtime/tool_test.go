// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolResult(t *testing.T) {
	t.Parallel()

	type pet struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		value   any
		want    string
		wantErr bool
	}{
		{name: "An object is written as it is", value: &pet{Name: "Rex"}, want: `{"name":"Rex"}`},
		{name: "A map is an object", value: map[string]int{"a": 1}, want: `{"a":1}`},
		{name: "A list is wrapped", value: []pet{{Name: "Rex"}}, want: `{"result":[{"name":"Rex"}]}`},
		{name: "A number is wrapped", value: int64(2), want: `{"result":2}`},
		{name: "A string is wrapped", value: "high", want: `{"result":"high"}`},
		{name: "A nil pointer is wrapped null", value: (*pet)(nil), want: `{"result":null}`},
		{name: "Nothing is wrapped null", want: `{"result":null}`},
		{name: "A value JSON cannot hold fails", value: func() {}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(ToolResult{Value: tc.value})

			if tc.wantErr {
				var typeErr *json.UnsupportedTypeError
				require.ErrorAs(t, err, &typeErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
