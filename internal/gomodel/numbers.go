// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Which types keep every digit of the JSON numbers they hold when that JSON is read through
// float64 first.

package gomodel

import "slices"

// exactBuiltins are the predeclared types whose every value a float64 holds, or that JSON writes as
// no number.
var exactBuiltins = []string{
	"string", "bool", "byte", "any",
	"int8", "int16", "int32", "uint8", "uint16", "uint32", "float32", "float64",
}

// FitsFloat64 reports whether a value of type t keeps every digit when its JSON is read through
// float64 first. An integer wider than 53 bits does not, nor does raw JSON or a type the model
// cannot see into: one of another package, or one x-go-type writes out.
func FitsFloat64(t Type) bool {
	return fitsFloat64(t, map[*Decl]bool{})
}

func fitsFloat64(t Type, seen map[*Decl]bool) bool {
	switch t := t.(type) {
	case Builtin:
		return slices.Contains(exactBuiltins, t.Name)
	case Qualified:
		return slices.Contains(stringTypes, Type(t))
	case Pointer:
		return fitsFloat64(t.Elem, seen)
	case Nullable:
		return fitsFloat64(t.Elem, seen)
	case Slice:
		return fitsFloat64(t.Elem, seen)
	case Map:
		return fitsFloat64(t.Elem, seen)
	case DeclRef:
		return declFitsFloat64(t.Decl, seen)
	}
	return false
}

func declFitsFloat64(d *Decl, seen map[*Decl]bool) bool {
	if seen[d] {
		return true
	}
	seen[d] = true

	switch {
	case d.Struct != nil:
		if a := d.Struct.AdditionalProperties; a != nil && !fitsFloat64(a.Type, seen) {
			return false
		}
		return !slices.ContainsFunc(d.Struct.Fields, func(f *Field) bool { return !fitsFloat64(f.Type, seen) })
	case d.Union != nil:
		return !slices.ContainsFunc(d.Union.Variants, func(v *Variant) bool { return !fitsFloat64(v.Type, seen) })
	case d.Enum != nil:
		return fitsFloat64(d.Enum.Base, seen)
	}
	return fitsFloat64(d.Target, seen)
}
