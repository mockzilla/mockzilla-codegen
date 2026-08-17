// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type problem struct {
	Error *struct {
		Message *string `json:"message,omitempty"`
		Code    int     `json:"code"`
	} `json:"error,omitempty"`
	Details []struct {
		Text string `json:"text"`
	} `json:"details,omitempty"`
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	var full problem
	require.NoError(t, SetErrorMessage(&full, "error.message", "boom"))
	full.Error.Code = 7
	require.NoError(t, SetErrorMessage(&full, "details[].text", "first"))

	tests := []struct {
		name  string
		value any
		path  string
		want  string
	}{
		{name: "Nested property", value: full, path: "error.message", want: "boom"},
		{name: "First item of an array", value: full, path: "details[].text", want: "first"},
		{name: "Number is written as JSON", value: full, path: "error.code", want: "7"},
		{name: "Missing property", value: problem{}, path: "error.message", want: "problem"},
		{name: "Empty array", value: map[string]any{"details": []any{}}, path: "details[].text", want: "problem"},
		{name: "Array where an object is expected", value: map[string]any{"error": []any{1}}, path: "error.message", want: "problem"},
		{name: "Value without JSON", value: func() {}, path: "a", want: "problem"},
		{name: "Null value", value: map[string]any{"a": nil}, path: "a", want: "problem"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ErrorMessage(tc.value, tc.path, "problem"))
		})
	}
}

func TestSetErrorMessageError(t *testing.T) {
	t.Parallel()

	var p problem
	require.Error(t, SetErrorMessage(p, "error.message", "boom"))
}
