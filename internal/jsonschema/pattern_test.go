// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilePattern(t *testing.T) {
	t.Parallel()

	escaped := `^[\u0041-\u005A]\u00e9\uD83D\uDE00$`
	tests := []struct {
		name    string
		pattern string
		text    string
		isMatch bool
	}{
		{name: "Every kind of escape matches what it stands for", pattern: escaped, text: "B\U000000E9\U0001F600", isMatch: true},
		{name: "A character outside the range", pattern: escaped, text: "b\U000000E9\U0001F600"},
		{name: "A space is any Unicode space", pattern: `^\s$`, text: "\U000000A0", isMatch: true},
		{name: "A dot is no line end", pattern: `^.$`, text: "\r"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			re, err := compilePattern(tc.pattern)

			require.NoError(t, err)
			assert.Equal(t, tc.isMatch, re.MatchString(tc.text))
		})
	}
}

func TestCompilePatternErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
	}{
		{name: "A lone surrogate", pattern: `\uD83D`},
		{name: "Too few hex digits", pattern: `\u12`},
		{name: "A lookahead", pattern: "(?!a)"},
		{name: "A repeat count above 1000", pattern: "a{1,8192}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := compilePattern(tc.pattern)

			assert.Error(t, err)
		})
	}
}
