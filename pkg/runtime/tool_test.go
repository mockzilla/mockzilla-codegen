// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func TestToolError(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 5000)
	split := strings.Repeat("a", 4095) + "é" + strings.Repeat("b", 10)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "An error of no response is kept", err: errors.New("dial failed"), want: "dial failed"},
		{name: "A response without a body is kept", err: &APIError{Status: 404}, want: "unexpected status 404 Not Found"},
		{name: "A body of white space is kept", err: &APIError{Status: 404, Body: []byte(" \n")}, want: "unexpected status 404 Not Found"},
		{
			name: "The body follows the message",
			err:  &APIError{Status: 404, Body: []byte("{\"detail\":\"no such pet\"}\n")},
			want: "unexpected status 404 Not Found\n{\"detail\":\"no such pet\"}",
		},
		{
			name: "The body follows the message of the error type",
			err:  &APIError{Status: 409, Body: []byte(`{"detail":"taken"}`), Err: errors.New("taken")},
			want: "unexpected status 409 Conflict: taken\n{\"detail\":\"taken\"}",
		},
		{
			name: "A wrapped response error is found",
			err:  fmt.Errorf("get pet: %w", &APIError{Status: 500, Body: []byte("down")}),
			want: "get pet: unexpected status 500 Internal Server Error\ndown",
		},
		{
			name: "A body that is no text gives its size",
			err:  &APIError{Status: 500, Body: []byte{0xff, 0xfe, 0x00}},
			want: "unexpected status 500 Internal Server Error\n(3 bytes, not UTF-8 text)",
		},
		{
			name: "A long body is cut",
			err:  &APIError{Status: 502, Body: []byte(long)},
			want: "unexpected status 502 Bad Gateway\n" + long[:4096] + "\n(904 more bytes left out)",
		},
		{
			name: "A long body is cut where a character starts",
			err:  &APIError{Status: 502, Body: []byte(split)},
			want: "unexpected status 502 Bad Gateway\n" + split[:4095] + "\n(12 more bytes left out)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ToolError(tc.err)

			require.EqualError(t, got, tc.want)
			assert.ErrorIs(t, got, tc.err)
		})
	}
}
