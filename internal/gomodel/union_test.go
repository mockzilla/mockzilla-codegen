// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestUnionsInModel(t *testing.T) {
	t.Parallel()

	checkGolden(t, "unions", "unions", testOptions())
}

func TestJSONKinds(t *testing.T) {
	t.Parallel()

	str := Builtin{Name: "string"}
	pet := &Decl{Name: "Pet", Kind: KindStruct, Struct: &Struct{}}
	status := &Decl{Name: "Status", Kind: KindEnum, Enum: &Enum{Base: str}}
	id := &Decl{Name: "ID", Kind: KindAlias, Target: Builtin{Name: "int64"}}
	loop := &Decl{Name: "Loop", Kind: KindUnion, Struct: &Struct{}}
	loop.Union = &Union{Variants: []*Variant{{Type: DeclRef{Decl: loop}}, {Type: Builtin{Name: "bool"}}}}

	tests := []struct {
		name string
		typ  Type
		want JSONKind
	}{
		{name: "String", typ: str, want: JSONString},
		{name: "Boolean", typ: Builtin{Name: "bool"}, want: JSONBool},
		{name: "Integer", typ: Builtin{Name: "uint8"}, want: JSONInteger},
		{name: "Float takes integers too", typ: Builtin{Name: "float32"}, want: JSONInteger | JSONNumber},
		{name: "Any", typ: anyType, want: JSONAny},
		{name: "Raw JSON", typ: rawJSON, want: JSONAny},
		{name: "Time is a string", typ: Qualified{Import: importTime, Name: "Time"}, want: JSONString},
		{name: "Pointer", typ: Pointer{Elem: str}, want: JSONString},
		{name: "Nullable", typ: Nullable{Elem: str}, want: JSONString},
		{name: "Bytes are a string", typ: Slice{Elem: byteType}, want: JSONString},
		{name: "Slice", typ: Slice{Elem: str}, want: JSONArray},
		{name: "Map", typ: Map{Key: str, Elem: str}, want: JSONObject},
		{name: "Struct", typ: DeclRef{Decl: pet}, want: JSONObject},
		{name: "Enum", typ: DeclRef{Decl: status}, want: JSONString},
		{name: "Alias", typ: DeclRef{Decl: id}, want: JSONInteger},
		{name: "Union that holds itself", typ: DeclRef{Decl: loop}, want: JSONBool},
		{name: "No type", want: JSONAny},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, jsonKinds(tc.typ, map[*Decl]bool{}))
		})
	}
}

func TestTypeName(t *testing.T) {
	t.Parallel()

	str := Builtin{Name: "string"}
	pet := &Decl{Name: "Pet"}

	tests := []struct {
		name string
		typ  Type
		want string
	}{
		{name: "Builtin", typ: Builtin{Name: "int64"}, want: "Int64"},
		{name: "Other package", typ: Qualified{Import: importTime, Name: "Time"}, want: "Time"},
		{name: "Declaration", typ: DeclRef{Decl: pet}, want: "Pet"},
		{name: "Pointer", typ: Pointer{Elem: str}, want: "String"},
		{name: "Nullable", typ: Nullable{Elem: str}, want: "String"},
		{name: "Bytes", typ: Slice{Elem: byteType}, want: "Bytes"},
		{name: "Slice", typ: Slice{Elem: DeclRef{Decl: pet}}, want: "Pets"},
		{name: "Slice of nullable items", typ: Slice{Elem: Pointer{Elem: str}}, want: "Strings"},
		{name: "Slice of slices", typ: Slice{Elem: Slice{Elem: str}}, want: "StringsList"},
		{name: "Map", typ: Map{Key: str, Elem: Builtin{Name: "bool"}}, want: "BoolMap"},
		{name: "No type", want: "Value"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, typeName(tc.typ, naming.New(nil)))
		})
	}
}

func TestStructOf(t *testing.T) {
	t.Parallel()

	st := &Struct{}
	pet := &Decl{Kind: KindStruct, Struct: st}
	alias := &Decl{Kind: KindAlias, Target: DeclRef{Decl: pet}}
	pets := &Decl{Kind: KindDefined, Target: Slice{Elem: DeclRef{Decl: pet}}}

	assert.Same(t, st, structOf(DeclRef{Decl: alias}))
	assert.Nil(t, structOf(DeclRef{Decl: pets}))
	assert.Nil(t, structOf(Builtin{Name: "string"}))
}

func TestTarget(t *testing.T) {
	t.Parallel()

	end := &spec.Schema{Types: spec.TypeString}
	a := &spec.Schema{}
	b := &spec.Schema{Ref: &spec.Ref{Target: a}}
	a.Ref = &spec.Ref{Target: b}
	dangling := &spec.Schema{Ref: &spec.Ref{Pointer: "#/components/schemas/Gone"}}

	assert.Same(t, end, target(&spec.Schema{Ref: &spec.Ref{Target: end}}))
	assert.Same(t, a, target(a))
	assert.Same(t, dangling, target(dangling))
}
