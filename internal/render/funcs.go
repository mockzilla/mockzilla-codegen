// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import (
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/mockzilla/codegen/internal/gocode"
)

// funcs is the func map of every template. toGoComment and escapeGoString keep the names template
// authors know from other generators.
func funcs() template.FuncMap {
	return template.FuncMap{
		"comment":        gocode.Comment,
		"quote":          gocode.Quote,
		"tag":            gocode.Tag,
		"lower":          strings.ToLower,
		"ucFirst":        ucFirst,
		"toGoComment":    gocode.Comment,
		"escapeGoString": escapeGoString,
	}
}

func ucFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// escapeGoString returns s escaped for use inside a Go string literal.
func escapeGoString(s string) string {
	quoted := gocode.Quote(s)
	return quoted[1 : len(quoted)-1]
}
