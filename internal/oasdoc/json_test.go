// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnescapeSlashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "Slash escape in a value and a key", src: `{"a\/b": "c\/d"}`, want: `{"a/b": "c/d"}`},
		{name: "Other escapes are kept", src: `{"a": "\\\/ \" A \/"}`, want: `{"a": "\\/ \" A /"}`},
		{name: "No slash escape", src: `{"a": "b/c"}`, want: `{"a": "b/c"}`},
		{name: "YAML is left alone", src: "a: \"b\\/c\"\n", want: "a: \"b\\/c\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, string(UnescapeSlashes([]byte(tt.src))))
		})
	}
}
