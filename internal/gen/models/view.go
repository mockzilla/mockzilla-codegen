// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const deprecatedNote = "Deprecated: the spec marks it deprecated."

// PartView is the data of one part: its declarations in model order.
type PartView struct {
	Decls []DeclView
}

// DeclView is one declaration. Target is the aliased or underlying type, or the base of an enum.
type DeclView struct {
	Doc        string
	Name       string
	IsStruct   bool
	IsEnum     bool
	IsAlias    bool
	IsDefined  bool
	Target     string
	Fields     []FieldView
	Values     []ConstView
	Additional *AdditionalView
}

// FieldView is one struct field; Tag is the whole tag literal.
type FieldView struct {
	Doc  string
	Name string
	Type string
	Tag  string
}

// ConstView is one enum constant; Value is its literal.
type ConstView struct {
	Name  string
	Value string
}

// AdditionalView is what the methods of a struct with additional properties need. Field is the
// field that holds them and Map its type, Value the type of its values, Known the quoted JSON names
// of the other fields and Runtime the name the runtime package is imported under.
type AdditionalView struct {
	Receiver string
	Field    string
	Map      string
	Value    string
	Known    []string
	Runtime  string
}

func declView(d *gomodel.Decl, s *gocode.Scope) DeclView {
	v := DeclView{Doc: withDeprecated(d.Doc, d.Deprecated), Name: d.Name}
	switch {
	case d.Struct != nil:
		v.IsStruct = true
		v.Fields, v.Additional = structView(d, s)
	case d.Enum != nil:
		v.IsEnum = true
		v.Target = s.Expr(d.Enum.Base)
		for _, ev := range d.Enum.Values {
			v.Values = append(v.Values, ConstView{Name: ev.Name, Value: gocode.Literal(ev.Value)})
		}
	case d.Kind == gomodel.KindAlias:
		v.IsAlias = true
		v.Target = s.Expr(d.Target)
	default:
		v.IsDefined = true
		v.Target = s.Expr(d.Target)
	}
	return v
}

func structView(d *gomodel.Decl, s *gocode.Scope) ([]FieldView, *AdditionalView) {
	fields := make([]FieldView, 0, len(d.Struct.Fields)+1)
	known := make([]string, 0, len(d.Struct.Fields))
	for _, f := range d.Struct.Fields {
		fields = append(fields, fieldView(f, jsonValue(f), s))
		known = append(known, gocode.Quote(f.JSONName))
	}

	ap := d.Struct.AdditionalProperties
	if ap == nil {
		return fields, nil
	}
	fields = append(fields, fieldView(ap, "-", s))
	m, ok := ap.Type.(gomodel.Map)
	if !ok {
		return fields, nil
	}

	first, _ := utf8.DecodeRuneInString(d.Name)
	return fields, &AdditionalView{
		Receiver: string(unicode.ToLower(first)),
		Field:    ap.Name,
		Map:      s.Expr(m),
		Value:    s.Expr(m.Elem),
		Known:    known,
		Runtime:  s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
	}
}

func fieldView(f *gomodel.Field, jsonTag string, s *gocode.Scope) FieldView {
	tags := append([]gomodel.Tag{{Key: "json", Value: jsonTag}}, f.Tags...)
	return FieldView{
		Doc:  withDeprecated(f.Doc, f.Deprecated),
		Name: f.Name,
		Type: s.Expr(f.Type),
		Tag:  gocode.Tag(tags),
	}
}

// jsonValue is the json tag of a property. omitzero joins omitempty on struct values, which
// omitempty alone never drops; a property named "-" needs the trailing comma.
func jsonValue(f *gomodel.Field) string {
	parts := []string{f.JSONName}
	if f.OmitEmpty {
		parts = append(parts, "omitempty")
		if isStructValue(f.Type) {
			parts = append(parts, "omitzero")
		}
	}

	value := strings.Join(parts, ",")
	if value == "-" {
		return "-,"
	}
	return value
}

// isStructValue reports whether t may be a struct held by value. Types from other packages count,
// since their kind is not known here.
func isStructValue(t gomodel.Type) bool {
	switch t := t.(type) {
	case gomodel.Qualified:
		return true
	case gomodel.DeclRef:
		if t.Decl.Kind == gomodel.KindAlias {
			return isStructValue(t.Decl.Target)
		}
		return t.Decl.Kind == gomodel.KindStruct
	}
	return false
}

func withDeprecated(doc string, isDeprecated bool) string {
	switch {
	case !isDeprecated:
		return doc
	case doc == "":
		return deprecatedNote
	default:
		return doc + "\n\n" + deprecatedNote
	}
}
