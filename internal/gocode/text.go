// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

const commentWidth = 100

// Comment turns text into // lines at most 100 columns wide. Long lines wrap at spaces; the
// line breaks and indentation of text stay, runs of blank lines become one. Characters Go source
// cannot hold are dropped. Blank text gives "".
func Comment(text string) string {
	var out []string
	isBlank := false
	for line := range strings.Lines(sourceSafe(text)) {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		if line == "" {
			isBlank = len(out) > 0
			continue
		}
		if isBlank {
			out = append(out, "//")
			isBlank = false
		}
		out = append(out, wrap(line)...)
	}
	return strings.Join(out, "\n")
}

// Quote returns s as a Go string literal.
func Quote(s string) string {
	return strconv.Quote(s)
}

// Tag returns a struct tag literal, raw unless a value holds a backquote. No tags give "".
func Tag(tags []gomodel.Tag) string {
	if len(tags) == 0 {
		return ""
	}

	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = t.Key + ":" + strconv.Quote(t.Value)
	}
	text := strings.Join(parts, " ")
	if strings.Contains(text, "`") {
		return strconv.Quote(text)
	}
	return "`" + text + "`"
}

// Literal returns v as a Go constant. Numbers keep the form the spec wrote them in.
func Literal(v spec.Value) string {
	switch v.Kind {
	case spec.KindString:
		return strconv.Quote(v.Str)
	case spec.KindNumber:
		return v.Num.String()
	case spec.KindBool:
		return strconv.FormatBool(v.Bool)
	default:
		return "nil"
	}
}

// wrap splits one line into comment lines, keeping its indentation on each. A word longer than
// the width, such as a URL, gets a line of its own.
func wrap(line string) []string {
	rest := strings.TrimLeftFunc(line, unicode.IsSpace)
	prefix := "// " + line[:len(line)-len(rest)]

	var out []string
	cur := prefix
	for _, word := range strings.Fields(rest) {
		switch {
		case cur == prefix:
			cur += word
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(word) > commentWidth:
			out = append(out, cur)
			cur = prefix + word
		default:
			cur += " " + word
		}
	}
	return append(out, cur)
}

// sourceSafe drops what Go source cannot hold: invalid UTF-8, NUL and other control characters
// but tab and newline, and byte order marks.
func sourceSafe(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return r
		}
		if unicode.IsControl(r) || r == '\uFEFF' || r == utf8.RuneError {
			return -1
		}
		return r
	}, strings.ReplaceAll(text, "\r\n", "\n"))
}
