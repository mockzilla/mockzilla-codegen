// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"strings"
	"unicode"
)

// Selector writes x.name.
func Selector(x, name string) string {
	return x + "." + name
}

// Deref writes *x.
func Deref(x string) string {
	return "*" + x
}

// Call writes fn(args...).
func Call(fn string, args ...string) string {
	return fn + "(" + strings.Join(args, ", ") + ")"
}

// NotNil writes x != nil.
func NotNil(x string) string {
	return x + " != nil"
}

// Index writes x[key].
func Index(x, key string) string {
	return x + "[" + key + "]"
}

// RawString returns s as a raw string literal, or a quoted one when s holds a backquote or a
// character a raw literal cannot show.
func RawString(s string) string {
	isPlain := !strings.ContainsFunc(s, func(r rune) bool {
		return r == '`' || r == '\r' || r != '\n' && r != '\t' && !unicode.IsPrint(r)
	})
	if isPlain {
		return "`" + s + "`"
	}
	return Quote(s)
}
