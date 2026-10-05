// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package ecma

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
)

const (
	re2Spaces = `\t-\r \xA0\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}`
	re2Others = `\x00-\x08\x0E-\x1F\x21-\x9F\xA1-\x{167F}\x{1681}-\x{1FFF}\x{200B}-\x{2027}\x{202A}-\x{202E}` +
		`\x{2030}-\x{205E}\x{2060}-\x{2FFF}\x{3001}-\x{FEFE}\x{FF00}-\x{10FFFF}`
)

func TestRE2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "Nothing to translate", pattern: `^[a-z]+\d*(?:\.\w+)?$`, want: `^[a-z]+\d*(?:\.\w+)?$`},
		{name: "A space is any space or line end", pattern: `^\s+$`, want: `^[` + re2Spaces + `]+$`},
		{name: "Not a space", pattern: `^\S$`, want: `^[^` + re2Spaces + `]$`},
		{name: "A dot is anything but a line end", pattern: `^.+$`, want: `^[^\n\r\x{2028}\x{2029}]+$`},
		{name: "A dot in a class is a dot", pattern: `[.]`, want: `[.]`},
		{name: "A space in a class", pattern: `[\w\s-]`, want: `[\w` + re2Spaces + `-]`},
		{name: "A space in a negated class", pattern: `[^\s,]`, want: `[^` + re2Spaces + `,]`},
		{name: "A class of only not a space", pattern: `[\S]+`, want: `[^` + re2Spaces + `]+`},
		{name: "A negated class of only not a space", pattern: `[^\S\S]`, want: `[` + re2Spaces + `]`},
		{name: "Not a space next to another item", pattern: `[a\S]`, want: `[a` + re2Others + `]`},
		{name: "Space and not a space are every character", pattern: `^[\w\W\s\S]*$`, want: `^[\s\S]*$`},
		{name: "Negated space and not a space are none", pattern: `[^\S\s]`, want: `[^\s\S]`},
		{name: "An empty negated class is every character", pattern: `a[^]`, want: `a[\s\S]`},
		{name: "An empty class is none", pattern: `a[]`, want: `a[^\s\S]`},
		{name: "A ] right after [ closes the class", pattern: `[]a]`, want: `[^\s\S]a]`},
		{name: "A control character", pattern: `\cJ[\cm\cA]`, want: `\n[\r\x01]`},
		{name: "No letter after \\c", pattern: `\c1\c`, want: `\c1\c`},
		{name: "A backspace in a class", pattern: `\b[\b]`, want: `\b[\x08]`},
		{name: "A [ in a class is a character", pattern: `[[:alnum:]]`, want: `[\[:alnum:]]`},
		{name: "Escapes of metacharacters stay", pattern: `\.\[\]\\[\]\\-]`, want: `\.\[\]\\[\]\\-]`},
		{name: "Up to U+00FF in hex", pattern: `\u000D?\u000A[\u0020-\u00ff]`, want: `\r?\n[ -\xFF]`},
		{name: "A metacharacter stays a character", pattern: `a\u002Eb[\u002D\u005D]`, want: `a\x2Eb[\x2D\x5D]`},
		{name: "Above U+00FF in braces", pattern: `[\u00A0-\uD7FF]\u2026`, want: `[\xA0-\x{D7FF}]\x{2026}`},
		{name: "A pair of surrogates is one character", pattern: `\uD83D\uDE00`, want: `\x{1F600}`},
		{name: "A lone surrogate stays", pattern: `\uD83Dx\uDE00`, want: `\uD83Dx\uDE00`},
		{name: "A high surrogate before another escape stays", pattern: `\uD83D\u0041`, want: `\uD83D\x41`},
		{name: "Five hex digits are four and a character", pattern: `\u10000`, want: `\x{1000}0`},
		{name: "An escaped backslash before u", pattern: `\\u0041`, want: `\\u0041`},
		{name: "A u after an escaped backslash pair", pattern: `\\\u0041`, want: `\\\x41`},
		{name: "Too few hex digits", pattern: `\u12`, want: `\u12`},
		{name: "No hex digits", pattern: `\uzzzz`, want: `\uzzzz`},
		{name: "A class without its end", pattern: `a[\s.`, want: `a[\s.`},
		{name: "A backslash at the end", pattern: `a\`, want: `a\`},
		{name: "A lookahead stays", pattern: `(?=\d)`, want: `(?=\d)`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, RE2(tc.pattern))
		})
	}
}

func TestPortable(t *testing.T) {
	t.Parallel()

	spaceSet := "\\t-\\r \\xA0\U00001680\U00002000-\U0000200A\U00002028\U00002029\U0000202F\U0000205F\U00003000\U0000FEFF"
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "A space", pattern: `\s[\s]`, want: "[" + spaceSet + "][" + spaceSet + "]"},
		{name: "A dot", pattern: `.`, want: "[^\\n\\r\U00002028\U00002029]"},
		{name: "Up to U+00FF in hex", pattern: `\u0021-\u00ff`, want: `\x21-\xFF`},
		{name: "Above U+00FF the character", pattern: `[\u00A0-\uD7FF]\u2026`, want: "[\\xA0-\U0000D7FF]\U00002026"},
		{name: "A pair of surrogates is one character", pattern: `\uD83D\uDE00`, want: "\U0001F600"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Portable(tc.pattern))
		})
	}
}

func TestMeaning(t *testing.T) {
	t.Parallel()

	isSpace := func(r rune) bool {
		return unicode.Is(unicode.Zs, r) || strings.ContainsRune("\t\n\v\f\r\U00002028\U00002029\U0000FEFF", r)
	}
	tests := []struct {
		name    string
		pattern string
		isMatch func(r rune) bool
	}{
		{name: "A space", pattern: `\s`, isMatch: isSpace},
		{name: "Not a space", pattern: `\S`, isMatch: func(r rune) bool { return !isSpace(r) }},
		{name: "Not a space or a", pattern: `[a\S]`, isMatch: func(r rune) bool { return r == 'a' || !isSpace(r) }},
		{name: "A space but no tab", pattern: `[^\t\S]`, isMatch: func(r rune) bool { return r != '\t' && isSpace(r) }},
		{name: "A dot", pattern: `.`, isMatch: func(r rune) bool { return !strings.ContainsRune("\n\r\U00002028\U00002029", r) }},
	}
	chars := []rune{0x1F600, unicode.MaxRune}
	for r := range rune(0x10000) {
		if !utf16.IsSurrogate(r) {
			chars = append(chars, r)
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var wrong []string
			for _, translated := range []string{RE2(tc.pattern), Portable(tc.pattern)} {
				re := regexp.MustCompile("^" + translated + "$")
				for _, r := range chars {
					if re.MatchString(string(r)) != tc.isMatch(r) {
						wrong = append(wrong, strconv.QuoteRune(r))
					}
				}
			}
			assert.Empty(t, wrong)
		})
	}
}
