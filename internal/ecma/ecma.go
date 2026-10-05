// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package ecma writes ECMA-262 patterns in Go's regexp syntax, keeping their ECMA-262 meaning.
package ecma

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

var (
	// spaces is what \s matches: the WhiteSpace and LineTerminator characters of ECMA-262.
	spaces = []span{
		{'\t', '\r'},
		{' ', ' '},
		{0xA0, 0xA0},
		{0x1680, 0x1680},
		{0x2000, 0x200A},
		{0x2028, 0x2029},
		{0x202F, 0x202F},
		{0x205F, 0x205F},
		{0x3000, 0x3000},
		{0xFEFF, 0xFEFF},
	}
	nonSpaces = complement(spaces)
	// lineEnds is what . does not match.
	lineEnds = []span{{'\n', '\n'}, {'\r', '\r'}, {0x2028, 0x2029}}
	controls = map[rune]string{'\t': `\t`, '\n': `\n`, '\v': `\v`, '\f': `\f`, '\r': `\r`}
)

// span is the characters from lo to hi.
type span struct {
	lo, hi rune
}

// translator writes characters above U+00FF as \x{XXXX}, or as themselves when isPortable.
type translator struct {
	isPortable bool
}

// RE2 writes pattern for Go's regexp.
func RE2(pattern string) string {
	return translator{}.translate(pattern)
}

// Portable writes pattern so that Go's regexp and an ECMA-262 engine read it the same.
func Portable(pattern string) string {
	return translator{isPortable: true}.translate(pattern)
}

func (t translator) translate(pattern string) string {
	var b strings.Builder
	for rest := pattern; rest != ""; {
		text, n := t.item(rest)
		b.WriteString(text)
		rest = rest[n:]
	}
	return b.String()
}

// item translates the item s starts with, outside a class, and returns its length.
func (t translator) item(s string) (string, int) {
	switch {
	case s[0] == '.':
		return set(true, t.spans(lineEnds)), 1
	case s[0] == '[':
		return t.class(s)
	case s[0] != '\\' || len(s) == 1:
		return s[:1], 1
	case s[1] == 's':
		return set(false, t.spans(spaces)), 2
	case s[1] == 'S':
		return set(true, t.spans(spaces)), 2
	}
	return t.escape(s)
}

// class translates the class s starts with; one without its ] is copied whole.
func (t translator) class(s string) (string, int) {
	isNegated := strings.HasPrefix(s, "[^")
	i := 1
	if isNegated {
		i = 2
	}

	var items []string
	for i < len(s) && s[i] != ']' {
		text, n := t.classItem(s[i:])
		items = append(items, text)
		i += n
	}
	if i == len(s) {
		return s, i
	}
	return t.joinClass(isNegated, items), i + 1
}

// classItem translates one item of a class, leaving \s and \S to joinClass.
func (t translator) classItem(s string) (string, int) {
	switch {
	case s[0] == '[':
		// Go would read [: as the start of a POSIX class.
		return `\[`, 1
	case s[0] != '\\' || len(s) == 1:
		return s[:1], 1
	case s[1] == 's' || s[1] == 'S':
		return s[:2], 2
	case s[1] == 'b':
		return t.char('\b'), 2
	}
	return t.escape(s)
}

// joinClass writes a class of items, in which \s and \S still have their ECMA-262 meaning.
func (t translator) joinClass(isNegated bool, items []string) string {
	hasSpace, hasNonSpace := slices.Contains(items, `\s`), slices.Contains(items, `\S`)
	hasOther := slices.ContainsFunc(items, func(s string) bool { return s != `\s` && s != `\S` })

	switch {
	case len(items) == 0:
		// [] matches nothing and [^] everything.
		return set(!isNegated, `\s\S`)
	case hasSpace && hasNonSpace:
		// Every character, which Go's [\s\S] is too.
		return set(isNegated, `\s\S`)
	case hasNonSpace && !hasOther:
		// The space set turned around reads better than the list of all other characters.
		return set(!isNegated, t.spans(spaces))
	}

	var b strings.Builder
	for _, s := range items {
		switch s {
		case `\s`:
			b.WriteString(t.spans(spaces))
		case `\S`:
			b.WriteString(t.spans(nonSpaces))
		default:
			b.WriteString(s)
		}
	}
	return set(isNegated, b.String())
}

// escape writes the character of a \u or \c escape, and any other escape as it is.
func (t translator) escape(s string) (string, int) {
	if r, n := charEscape(s); n > 0 {
		return t.char(r), n
	}
	return s[:2], 2
}

// char writes r so that it stands for r alone, in a class or outside one.
func (t translator) char(r rune) string {
	if name, ok := controls[r]; ok {
		return name
	}

	switch {
	case r == ' ':
		return " "
	case r <= 0xFF:
		return fmt.Sprintf(`\x%02X`, r)
	case t.isPortable:
		return string(r)
	}
	return fmt.Sprintf(`\x{%04X}`, r)
}

// spans writes each span of list as one character, two, or a range.
func (t translator) spans(list []span) string {
	var b strings.Builder
	for _, s := range list {
		b.WriteString(t.char(s.lo))
		if s.hi > s.lo+1 {
			b.WriteByte('-')
		}
		if s.hi > s.lo {
			b.WriteString(t.char(s.hi))
		}
	}
	return b.String()
}

// set is a class of body, negated or not.
func set(isNegated bool, body string) string {
	if isNegated {
		return "[^" + body + "]"
	}
	return "[" + body + "]"
}

// complement is every character list leaves out. list is sorted.
func complement(list []span) []span {
	var out []span
	next := rune(0)
	for _, s := range list {
		if s.lo > next {
			out = append(out, span{lo: next, hi: s.lo - 1})
		}
		next = s.hi + 1
	}
	return append(out, span{lo: next, hi: unicode.MaxRune})
}

// charEscape reads \uXXXX, a surrogate pair of them, or \c and a letter; 0 and 0 for none.
func charEscape(s string) (rune, int) {
	if len(s) > 2 && strings.HasPrefix(s, `\c`) && isLetter(s[2]) {
		return rune(s[2] % 32), 3
	}

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

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
