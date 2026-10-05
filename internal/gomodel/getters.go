// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Getters: the method that returns an optional field, or its default when the field is nil.

package gomodel

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// Getter is the method that returns a field, or Default when the field is nil.
type Getter struct {
	Name    string
	Default spec.Value
}

// Read-only tables: the number types a default can be a constant of, with their size.
var (
	intBits = map[string]uint{
		"int": 64, "int8": 8, "int16": 16, "int32": 32, "int64": 64,
		"uint": 64, "uint8": 8, "uint16": 16, "uint32": 32, "uint64": 64,
	}
	floatBits = map[string]int{"float32": 32, "float64": 64}
)

// planGetters gives every optional field with a default a getter, when Go writes the default as a
// constant or a list of them.
func planGetters(decls []*Decl, c *diag.Collector) {
	for _, d := range decls {
		if d.Struct == nil {
			continue
		}
		for _, f := range d.Struct.Fields {
			if f.def != nil {
				planGetter(d, f, c)
			}
		}
	}
}

func planGetter(d *Decl, f *Field, c *diag.Collector) {
	name := "Get" + f.Name
	t := Elem(f.Type)
	var why string
	switch {
	case t == f.Type && !nilable(t):
		why = "x-go-type-skip-optional-pointer makes it a plain value, which is never nil"
	case !hasLiteral(t, *f.def):
		why = "Go has no constant of type " + typeText(t) + " for its default"
	case isFieldName(d, name):
		c.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeNameClash,
			Pointer:  f.schema.Origin.Pointer,
			Origin:   f.Origin,
			Message:  fmt.Sprintf("%s.%s gets no getter: %s has a field named %s", d.Name, f.Name, d.Name, name),
		})
		return
	default:
		f.Getter = &Getter{Name: name, Default: *f.def}
		return
	}

	c.Append(diag.Diagnostic{
		Severity: diag.Info,
		Code:     diag.CodeGetterSkipped,
		Pointer:  f.schema.Origin.Pointer,
		Origin:   f.Origin,
		Message:  fmt.Sprintf("%s.%s gets no getter: %s", d.Name, f.Name, why),
	})
}

// isFieldName reports whether d has a field or variant named name.
func isFieldName(d *Decl, name string) bool {
	return slices.ContainsFunc(d.Struct.Fields, func(f *Field) bool { return f.Name == name }) ||
		d.Union != nil && slices.ContainsFunc(d.Union.Variants, func(v *Variant) bool { return v.Name == name })
}

// hasLiteral reports whether Go writes v as a value of type t: a constant, or a list of them.
func hasLiteral(t Type, v spec.Value) bool {
	switch t := t.(type) {
	case Builtin:
		return isConstant(t.Name, v)
	case Qualified:
		return t == emailType && v.Kind == spec.KindString
	case Slice:
		return v.Kind == spec.KindArray && t.Elem != byteType &&
			!slices.ContainsFunc(v.Items, func(item spec.Value) bool { return !hasLiteral(t.Elem, item) })
	case DeclRef:
		switch t.Decl.Kind {
		case KindEnum:
			_, ok := t.Decl.Enum.Const(v)
			return ok
		case KindAlias, KindDefined:
			return hasLiteral(t.Decl.Target, v)
		case KindStruct, KindUnion:
		}
	}
	return false
}

// isConstant reports whether v is a constant of the predeclared type named name, in its range.
func isConstant(name string, v spec.Value) bool {
	switch {
	case name == "string":
		return v.Kind == spec.KindString
	case name == "bool":
		return v.Kind == spec.KindBool
	case v.Kind != spec.KindNumber:
		return false
	}
	if bits, ok := floatBits[name]; ok {
		_, err := strconv.ParseFloat(v.Num.String(), bits)
		return err == nil
	}

	bits, ok := intBits[name]
	r, isNumber := new(big.Rat).SetString(v.Num.String())
	if !ok || !isNumber || !r.IsInt() {
		return false
	}
	n := r.Num()
	if strings.HasPrefix(name, "u") {
		return n.Sign() >= 0 && n.BitLen() <= int(bits)
	}
	return n.BitLen() < int(bits) || n.Cmp(new(big.Int).Lsh(big.NewInt(-1), bits-1)) == 0
}
