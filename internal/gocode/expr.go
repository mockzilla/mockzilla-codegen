// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Small Go expressions and statements as text: selectors, calls, signatures, returns and literals.

package gocode

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// KeyValue is one element of a keyed composite literal, its key and value written as Go.
type KeyValue struct {
	Key   string
	Value string
}

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

// Composite writes typ{key: value}, one element per line when it has several.
func Composite(typ string, elems []KeyValue) string {
	if len(elems) == 1 {
		return typ + "{" + elems[0].Key + ": " + elems[0].Value + "}"
	}
	var b strings.Builder
	b.WriteString(typ + "{\n")
	for _, e := range elems {
		b.WriteString(e.Key + ": " + e.Value + ",\n")
	}
	b.WriteString("}")
	return b.String()
}

// AddressOf writes &x.
func AddressOf(x string) string {
	return "&" + x
}

// Param writes one parameter of a func: name typ.
func Param(name, typ string) string {
	return name + " " + typ
}

// Signature writes the parameters of a func and its result, when it has one: (params) result.
func Signature(params []string, result string) string {
	out := "(" + strings.Join(params, ", ") + ")"
	if result != "" {
		out += " " + result
	}
	return out
}

// Define writes names := values, one value per name.
func Define(names []string, values ...string) string {
	return strings.Join(names, ", ") + " := " + strings.Join(values, ", ")
}

// Return writes return, with the values when there are any.
func Return(values ...string) string {
	if len(values) == 0 {
		return "return"
	}
	return "return " + strings.Join(values, ", ")
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

// Or writes the conditions joined by ||.
func Or(conds ...string) string {
	return strings.Join(conds, " || ")
}

// And writes the conditions joined by &&, or true when there are none.
func And(conds ...string) string {
	if len(conds) == 0 {
		return "true"
	}
	return strings.Join(conds, " && ")
}

// IsNil writes x == nil.
func IsNil(x string) string {
	return x + " == nil"
}

// Get writes value, ok := x.Get(); ok, which holds when the Nullable x holds a value.
func Get(x, value, ok string) string {
	return value + ", " + ok + " := " + Call(Selector(x, "Get")) + "; " + ok
}

// NotEmpty writes x != "".
func NotEmpty(x string) string {
	return x + ` != ""`
}

// Zero writes the zero value of t, a type whose underlying type is a string or one that can be
// nil: "" or nil.
func Zero(t gomodel.Type) string {
	if gomodel.Underlying(t) == (gomodel.Builtin{Name: "string"}) {
		return `""`
	}
	return "nil"
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
