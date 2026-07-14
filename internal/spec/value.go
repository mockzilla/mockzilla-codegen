// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import "encoding/json"

type ValueKind int

const (
	KindNull ValueKind = iota
	KindString
	KindNumber
	KindBool
	KindArray
	KindObject
)

// Value is a JSON value; only the field for its Kind is set, and numbers keep their written form.
type Value struct {
	Kind   ValueKind
	Str    string
	Num    json.Number
	Bool   bool
	Items  []Value
	Fields []Field
}

type Field struct {
	Name  string
	Value Value
}
