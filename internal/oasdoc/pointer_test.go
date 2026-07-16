// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		token   string
		escaped string
	}{
		{name: "Plain token is unchanged", token: "Pet", escaped: "Pet"},
		{name: "Slash", token: "/pets/{id}", escaped: "~1pets~1{id}"},
		{name: "Tilde goes first so ~1 in the input survives", token: "a~1/b", escaped: "a~01~1b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.escaped, Escape(tt.token))
			assert.Equal(t, tt.token, Unescape(tt.escaped))
		})
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ptr     string
		want    []string
		wantErr bool
	}{
		{name: "Empty pointer is the root", ptr: ""},
		{name: "Bare fragment is the root", ptr: "#"},
		{name: "Tokens are unescaped", ptr: "/paths/~1pets~1{id}/get", want: []string{"paths", "/pets/{id}", "get"}},
		{name: "Fragment tokens are percent-decoded", ptr: "#/paths/~1pets~1%7Bid%7D", want: []string{"paths", "/pets/{id}"}},
		{name: "Plain pointers are not percent-decoded", ptr: "/a%20b", want: []string{"a%20b"}},
		{name: "Empty token", ptr: "/", want: []string{""}},
		{name: "Missing leading slash", ptr: "paths", wantErr: true},
		{name: "Bad percent escape", ptr: "#/a%zz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Split(tt.ptr)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrPointer)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
