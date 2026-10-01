// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
)

// CheckFuncs returns what is wrong with the first entry of m, by name, that a template cannot
// take: a name that is no identifier, a value that is no func, or a func that returns anything
// but one value, or one value and an error.
func CheckFuncs(m template.FuncMap) error {
	for _, name := range slices.Sorted(maps.Keys(m)) {
		if err := checkFunc(name, m[name]); err != nil {
			return err
		}
	}
	return nil
}

// checkFunc asks text/template, which tells an entry it cannot take by a panic alone.
func checkFunc(name string, fn any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrFunc, r)
		}
	}()
	_ = template.New("").Funcs(template.FuncMap{name: fn})
	return nil
}

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
