// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		field      string
		isRequired bool
		want       string
	}{
		{name: "Required", field: "id", isRequired: true, want: "`json:\"id\"`"},
		{name: "Optional is left out when empty", field: "X-Trace", want: "`json:\"X-Trace,omitempty\"`"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, jsonTag(tc.field, tc.isRequired))
		})
	}
}
