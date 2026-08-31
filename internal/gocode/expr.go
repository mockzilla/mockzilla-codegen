// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// durationUnits are the units a duration is written in, largest first.
var durationUnits = []struct {
	unit time.Duration
	name string
}{
	{unit: time.Hour, name: "Hour"},
	{unit: time.Minute, name: "Minute"},
	{unit: time.Second, name: "Second"},
	{unit: time.Millisecond, name: "Millisecond"},
	{unit: time.Microsecond, name: "Microsecond"},
}

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

// AddressOf writes &x.
func AddressOf(x string) string {
	return "&" + x
}

// Duration writes d in the largest unit that holds it whole, such as 30 * time.Second, where
// timePkg is the name the time package is imported under.
func Duration(d time.Duration, timePkg string) string {
	for _, u := range durationUnits {
		if d%u.unit == 0 {
			return strconv.FormatInt(int64(d/u.unit), 10) + " * " + Selector(timePkg, u.name)
		}
	}
	return strconv.FormatInt(int64(d), 10) + " * " + Selector(timePkg, "Nanosecond")
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
