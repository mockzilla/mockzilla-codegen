// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package diag is how every stage reports a problem in a spec: a severity, a code, a message,
// and where in the spec it is.
package diag

import (
	"cmp"
	"slices"
)

type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	default:
		return "error"
	}
}

// Origin is a position in a source file; Line and Col are 1-based, zero means unknown.
type Origin struct {
	File string
	Line int
	Col  int
}

type Diagnostic struct {
	Severity Severity
	Code     string
	Pointer  string
	Origin   Origin
	Message  string
}

type Collector struct {
	items []Diagnostic
}

func (c *Collector) Append(ds ...Diagnostic) {
	c.items = append(c.items, ds...)
}

// List returns the diagnostics sorted by file, line, column, code, pointer and message.
func (c *Collector) List() []Diagnostic {
	out := slices.Clone(c.items)
	slices.SortStableFunc(out, compare)
	return out
}

func compare(a, b Diagnostic) int {
	return cmp.Or(
		cmp.Compare(a.Origin.File, b.Origin.File),
		cmp.Compare(a.Origin.Line, b.Origin.Line),
		cmp.Compare(a.Origin.Col, b.Origin.Col),
		cmp.Compare(a.Code, b.Code),
		cmp.Compare(a.Pointer, b.Pointer),
		cmp.Compare(a.Message, b.Message),
	)
}
