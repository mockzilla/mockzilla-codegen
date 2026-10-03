// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// A name split into words, with symbols spelled out and accented letters folded to ASCII.

package naming

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var symbolWords = map[rune]string{
	' ':  "Space",
	'!':  "Not",
	'"':  "Quote",
	'#':  "Hash",
	'$':  "Dollar",
	'%':  "Percent",
	'&':  "And",
	'\'': "Apostrophe",
	'(':  "LeftParen",
	')':  "RightParen",
	'*':  "Asterisk",
	'+':  "Plus",
	',':  "Comma",
	'-':  "Minus",
	'.':  "Dot",
	'/':  "Slash",
	':':  "Colon",
	';':  "Semicolon",
	'<':  "LessThan",
	'=':  "Equal",
	'>':  "GreaterThan",
	'?':  "Question",
	'@':  "At",
	'[':  "LeftBracket",
	'\\': "Backslash",
	']':  "RightBracket",
	'^':  "Caret",
	'_':  "Underscore",
	'`':  "Backtick",
	'{':  "LeftBrace",
	'|':  "Or",
	'}':  "RightBrace",
	'~':  "Tilde",
}

var latinFolds = []struct{ lower, upper, to string }{
	{"àáâãäåāăą", "ÀÁÂÃÄÅĀĂĄ", "a"},
	{"æ", "Æ", "ae"},
	{"çćĉċč", "ÇĆĈĊČ", "c"},
	{"ďđð", "ĎĐÐ", "d"},
	{"èéêëēĕėęě", "ÈÉÊËĒĔĖĘĚ", "e"},
	{"ĝğġģ", "ĜĞĠĢ", "g"},
	{"ĥħ", "ĤĦ", "h"},
	{"ìíîïĩīĭįı", "ÌÍÎÏĨĪĬĮİ", "i"},
	{"ĵ", "Ĵ", "j"},
	{"ķ", "Ķ", "k"},
	{"ĺļľŀł", "ĹĻĽĿŁ", "l"},
	{"ñńņň", "ÑŃŅŇ", "n"},
	{"òóôõöøōŏő", "ÒÓÔÕÖØŌŎŐ", "o"},
	{"œ", "Œ", "oe"},
	{"ŕŗř", "ŔŖŘ", "r"},
	{"śŝşš", "ŚŜŞŠ", "s"},
	{"ß", "ẞ", "ss"},
	{"ţťŧ", "ŢŤŦ", "t"},
	{"þ", "Þ", "th"},
	{"ùúûüũūŭůűų", "ÙÚÛÜŨŪŬŮŰŲ", "u"},
	{"ŵ", "Ŵ", "w"},
	{"ýÿŷ", "ÝŸŶ", "y"},
	{"źżž", "ŹŻŽ", "z"},
}

var predeclared = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"true": true, "false": true, "iota": true, "nil": true,
	"append": true, "cap": true, "clear": true, "close": true, "complex": true, "copy": true,
	"delete": true, "imag": true, "len": true, "make": true, "max": true, "min": true, "new": true,
	"panic": true, "print": true, "println": true, "real": true, "recover": true,
}

// rawWords never returns an empty slice: symbols are spelled out, else the first rune in hex.
func rawWords(part string, initialisms map[string]string) []string {
	if part == "" {
		return []string{"Empty"}
	}

	if ws := split(normalize(part), initialisms); len(ws) > 0 {
		return ws
	}

	var b strings.Builder
	for _, r := range part {
		b.WriteString(symbolWords[r])
	}
	if b.Len() == 0 {
		r, _ := utf8.DecodeRuneInString(part)
		b.WriteString("X" + strings.ToUpper(strconv.FormatInt(int64(r), 16)))
	}
	return split([]rune(b.String()), initialisms)
}

func normalize(s string) []rune {
	rs := []rune(s)
	out := make([]rune, 0, len(rs))

	for i, r := range rs {
		switch {
		case isLetter(r) || isDigit(r):
			out = append(out, r)
		case r == '+':
			out = append(out, []rune(" Plus ")...)
		case r == '@':
			out = append(out, []rune(" At ")...)
		case r == '-' && i == 0:
			out = append(out, []rune("Minus ")...)
		case r == '.' && i > 0 && i+1 < len(rs) && isDigit(rs[i-1]) && isDigit(rs[i+1]):
			out = append(out, []rune("Dot")...)
		default:
			out = append(out, []rune(fold(r))...)
		}
	}
	return out
}

func fold(r rune) string {
	for _, f := range latinFolds {
		switch {
		case strings.ContainsRune(f.lower, r):
			return f.to
		case strings.ContainsRune(f.upper, r):
			return strings.ToUpper(f.to[:1]) + f.to[1:]
		}
	}
	return " "
}

// An upper-case run keeps a plural "s" when it is an initialism: "userIDs" gives "user", "IDs".
func split(rs []rune, initialisms map[string]string) []string {
	var out []string
	var cur []rune

	for i, r := range rs {
		if r == ' ' {
			out, cur = flush(out, cur)
			continue
		}

		n := len(cur)
		switch {
		case n == 0:
		case isLower(cur[n-1]) && isUpper(r):
			out, cur = flush(out, cur)
		case isDigit(cur[n-1]) && isUpper(r):
			out, cur = flush(out, cur)
		case isDigit(cur[n-1]) && isLower(r) && slices.ContainsFunc(cur, isLetter):
			out, cur = flush(out, cur)
		case n >= 2 && isUpper(cur[n-1]) && isUpper(cur[n-2]) && isLower(r):
			if isPluralInitialism(cur, rs[i:], initialisms) {
				break
			}
			last := cur[n-1]
			out, cur = flush(out, cur[:n-1])
			cur = append(cur, last)
		}
		cur = append(cur, r)
	}
	out, _ = flush(out, cur)
	return out
}

func flush(out []string, cur []rune) ([]string, []rune) {
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out, cur[:0]
}

func isPluralInitialism(run, rest []rune, initialisms map[string]string) bool {
	if rest[0] != 's' || (len(rest) > 1 && isLower(rest[1])) {
		return false
	}
	_, ok := initialisms[strings.ToLower(string(run))]
	return ok
}

func isLetter(r rune) bool { return isUpper(r) || isLower(r) }
func isUpper(r rune) bool  { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool  { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
