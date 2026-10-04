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

func TestPortable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "No escape", pattern: `^[a-z]+\d*$`, want: `^[a-z]+\d*$`},
		{name: "Up to U+00FF in hex", pattern: `\u000D?\u000A[\u0020-\u00ff]`, want: `\x0D?\x0A[\x20-\xFF]`},
		{name: "A metacharacter stays escaped", pattern: `a\u002Eb[\u002D\u005D]`, want: `a\x2Eb[\x2D\x5D]`},
		{name: "Above U+00FF the character", pattern: `[\u00A0-\uD7FF]\u2026`, want: "[\\xA0-\U0000D7FF]\U00002026"},
		{name: "A pair of surrogates is one character", pattern: `\uD83D\uDE00`, want: "\U0001F600"},
		{name: "A lone surrogate stays", pattern: `\uD83Dx\uDE00`, want: `\uD83Dx\uDE00`},
		{name: "A high surrogate before another escape stays", pattern: `\uD83D\u0041`, want: `\uD83D\x41`},
		{name: "Five hex digits are four and a character", pattern: `\u10000`, want: "\U00001000" + "0"},
		{name: "An escaped backslash before u", pattern: `\\u0041`, want: `\\u0041`},
		{name: "A u after an escaped backslash pair", pattern: `\\\u0041`, want: `\\\x41`},
		{name: "Too few hex digits", pattern: `\u12`, want: `\u12`},
		{name: "No hex digits", pattern: `\uzzzz`, want: `\uzzzz`},
		{name: "A backslash at the end", pattern: `a\`, want: `a\`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, portable(tc.pattern))
		})
	}
}

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
