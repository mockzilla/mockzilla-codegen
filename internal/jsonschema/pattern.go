// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The pattern keyword, written so that Go's regexp, which the MCP SDK checks it with, and an
// ECMA-262 engine read it the same.

package jsonschema

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// compilePattern compiles an ECMA-262 pattern with Go's regexp after portable rewrote its \u
// escapes. The source of the result is the pattern the document writes.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(portable(pattern))
}

// portable rewrites each \uXXXX escape, which Go's regexp does not take, into a form both engines
// read: \xHH up to U+00FF, the character itself above it, a pair of surrogates as the one
// character they make. A lone surrogate stays, so the pattern does not compile.
func portable(pattern string) string {
	var b strings.Builder
	for rest := pattern; rest != ""; {
		r, n := unicodeEscape(rest)
		switch {
		case n > 0 && r <= 0xFF:
			fmt.Fprintf(&b, `\x%02X`, r)
		case n > 0:
			b.WriteRune(r)
		case len(rest) > 1 && rest[0] == '\\':
			// Any other escape is copied whole, so the u of \\u is no escape.
			n = 2
			b.WriteString(rest[:n])
		default:
			n = 1
			b.WriteByte(rest[0])
		}
		rest = rest[n:]
	}
	return b.String()
}

// unicodeEscape reads the \uXXXX escape s starts with, two of them for a pair of surrogates, and
// returns the character and the bytes read, or 0 and 0.
func unicodeEscape(s string) (rune, int) {
	r := hexEscape(s)
	if r < 0 {
		return 0, 0
	}

	if !utf16.IsSurrogate(r) {
		return r, 6
	}

	if pair := utf16.DecodeRune(r, hexEscape(s[6:])); pair != unicode.ReplacementChar {
		return pair, 12
	}
	return 0, 0
}

// hexEscape is the code unit of the \uXXXX escape s starts with, or -1.
func hexEscape(s string) rune {
	if len(s) < 6 || !strings.HasPrefix(s, `\u`) {
		return -1
	}

	n, err := strconv.ParseUint(s[2:6], 16, 16)
	if err != nil {
		return -1
	}
	return rune(n)
}
