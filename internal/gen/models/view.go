// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const deprecatedNote = "Deprecated: the spec marks it deprecated."

// kindNames are the runtime.Kind constants, one per gomodel.JSONKind bit.
var kindNames = []string{"KindNull", "KindBool", "KindInteger", "KindNumber", "KindString", "KindArray", "KindObject"}

// PartView is the data of one part: the patterns its checks use and its declarations in model
// order.
type PartView struct {
	Patterns []PatternView
	Decls    []DeclView
}

// DeclView is one declaration. Target is the aliased or underlying type, or the base of an enum.
type DeclView struct {
	Doc        string
	Name       string
	IsStruct   bool
	IsUnion    bool
	IsEnum     bool
	IsAlias    bool
	IsDefined  bool
	Target     string
	Fields     []FieldView
	Values     []ConstView
	Additional *AdditionalView
	Union      *UnionView
	Validate   *ValidateView
	Error      *ErrorView
	Mask       *MaskView
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

// UnionView is what a union's variant fields and methods need. Runtime and JSON are the names
// the packages are imported under, JSON only when shared fields are decoded. Discriminator and
// Shared are quoted.
type UnionView struct {
	Receiver      string
	Runtime       string
	JSON          string
	IsAnyOf       bool
	Discriminator string
	Shared        []string
	Variants      []VariantView
}

// VariantView is one variant field and what decoding needs to know of it. Kinds are runtime.Kind
// constant names; Values, Required and Known are quoted. HasKnown writes Known even when empty.
type VariantView struct {
	Name      string
	Type      string
	Kinds     []string
	Values    []string
	IsDefault bool
	Required  []string
	Known     []string
	HasKnown  bool
	IsClosed  bool
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
	v := DeclView{Doc: withDeprecated(d.Doc, d.Deprecated, d.DeprecatedReason), Name: d.Name}
	switch {
	case d.Union != nil:
		v.IsUnion = true
		v.Fields, _ = structView(d, s)
		v.Union = unionView(d, s)
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

	if d.Validation != nil {
		v.Validate = validateView(d, s)
	}
	if d.Error != nil {
		v.Error = errorView(d, s)
	}
	if len(d.Masks) > 0 {
		v.Mask = maskView(d, s)
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

	return fields, &AdditionalView{
		Receiver: receiver(d.Name),
		Field:    ap.Name,
		Map:      s.Expr(m),
		Value:    s.Expr(m.Elem),
		Known:    known,
		Runtime:  s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
	}
}

func unionView(d *gomodel.Decl, s *gocode.Scope) *UnionView {
	u := d.Union
	v := &UnionView{
		Receiver: receiver(d.Name),
		Runtime:  s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		IsAnyOf:  u.IsAnyOf,
		Variants: make([]VariantView, len(u.Variants)),
	}
	if len(d.Struct.Fields) > 0 {
		v.JSON = s.Import(gomodel.Import{Path: "encoding/json"})
	}
	for _, f := range d.Struct.Fields {
		v.Shared = append(v.Shared, gocode.Quote(f.JSONName))
	}
	if u.Discriminator != "" {
		v.Discriminator = gocode.Quote(u.Discriminator)
		if !slices.ContainsFunc(d.Struct.Fields, func(f *gomodel.Field) bool { return f.JSONName == u.Discriminator }) {
			v.Shared = append(v.Shared, v.Discriminator)
		}
	}

	for i, vr := range u.Variants {
		v.Variants[i] = VariantView{
			Name:      vr.Name,
			Type:      s.Expr(vr.FieldType),
			Kinds:     kinds(vr.Kinds),
			Values:    quoteAll(vr.Values),
			IsDefault: vr.IsDefault,
			Required:  quoteAll(vr.Required),
			Known:     quoteAll(vr.Known),
			HasKnown:  vr.Known != nil,
			IsClosed:  vr.IsClosed,
		}
	}
	return v
}

// kinds names the runtime constants of k: KindAny, or one per kind. A variant whose kinds are not
// known, such as one that loops back to its union, takes any.
func kinds(k gomodel.JSONKind) []string {
	if k == gomodel.JSONAny || k == 0 {
		return []string{"KindAny"}
	}
	var out []string
	for i, name := range kindNames {
		if k&(1<<i) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func quoteAll(values []string) []string {
	var out []string
	for _, v := range values {
		out = append(out, gocode.Quote(v))
	}
	return out
}

// receiver is the receiver name of the methods of a type: its first letter in lower case.
func receiver(name string) string {
	first, _ := utf8.DecodeRuneInString(name)
	return string(unicode.ToLower(first))
}

func fieldView(f *gomodel.Field, jsonTag string, s *gocode.Scope) FieldView {
	tags := append([]gomodel.Tag{{Key: "json", Value: jsonTag}}, f.Tags...)
	return FieldView{
		Doc:  withDeprecated(f.Doc, f.Deprecated, f.DeprecatedReason),
		Name: f.Name,
		Type: s.Expr(f.Type),
		Tag:  gocode.Tag(tags),
	}
}

// jsonValue is the json tag of a property. omitzero joins omitempty on struct values, which
// omitempty alone never drops; a property named "-" needs the trailing comma.
func jsonValue(f *gomodel.Field) string {
	if f.IsJSONIgnored {
		return "-"
	}
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
		return t.Decl.Kind == gomodel.KindStruct || t.Decl.Kind == gomodel.KindUnion
	}
	return false
}

// withDeprecated adds the deprecation paragraph Go tools look for, with x-deprecated-reason when
// the spec gives one.
func withDeprecated(doc string, isDeprecated bool, reason string) string {
	note := deprecatedNote
	if reason != "" {
		note = "Deprecated: " + reason
	}
	switch {
	case !isDeprecated:
		return doc
	case doc == "":
		return note
	default:
		return doc + "\n\n" + note
	}
}
