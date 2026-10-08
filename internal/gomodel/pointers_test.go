// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldType(t *testing.T) {
	t.Parallel()

	str := Builtin{Name: "string"}
	tests := []struct {
		name string
		t    Type
		p    presence
		want Type
	}{
		{name: "Required", t: str, p: presence{isRequired: true}, want: str},
		{name: "Optional", t: str, want: Pointer{Elem: str}},
		{name: "Required and nullable", t: str, p: presence{isRequired: true, isNullable: true}, want: Pointer{Elem: str}},
		{name: "Optional and nullable", t: str, p: presence{isNullable: true}, want: Pointer{Elem: str}},
		{name: "Optional without pointer", t: str, p: presence{isPointerSkipped: true}, want: str},
		{name: "Required in a cycle", t: str, p: presence{isRequired: true, isInCycle: true}, want: Pointer{Elem: str}},
		{name: "Optional slice", t: Slice{Elem: str}, want: Slice{Elem: str}},
		{name: "Required nullable map", t: Map{Key: str, Elem: str}, p: presence{isRequired: true, isNullable: true}, want: Map{Key: str, Elem: str}},
		{name: "Optional any", t: anyType, want: anyType},
		{name: "Optional raw JSON", t: rawJSON, want: rawJSON},
		{name: "Wrapped", t: str, p: presence{wrap: wrapNonNil}, want: Nullable{Elem: str}},
		{name: "Wrapped in a cycle", t: str, p: presence{wrap: wrapNonNil, isInCycle: true}, want: Nullable{Elem: str}},
		{name: "Wrapped slice", t: Slice{Elem: str}, p: presence{wrap: wrapNonNil}, want: Slice{Elem: str}},
		{name: "Wrapped slice asked for", t: Slice{Elem: str}, p: presence{wrap: wrapAny}, want: Nullable{Elem: Slice{Elem: str}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, fieldType(tt.t, tt.p))
		})
	}
}

func TestElemType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Builtin{Name: "int"}, elemType(Builtin{Name: "int"}, false, true))
	assert.Equal(t, Pointer{Elem: Builtin{Name: "int"}}, elemType(Builtin{Name: "int"}, true, false))
	assert.Equal(t, Nullable{Elem: Builtin{Name: "int"}}, elemType(Builtin{Name: "int"}, true, true))
	assert.Equal(t, Slice{Elem: stringType}, elemType(Slice{Elem: stringType}, true, true))
}

func TestElem(t *testing.T) {
	t.Parallel()

	assert.Equal(t, stringType, Elem(Pointer{Elem: stringType}))
	assert.Equal(t, stringType, Elem(Nullable{Elem: stringType}))
	assert.Equal(t, stringType, Elem(stringType))
}

func TestNilable(t *testing.T) {
	t.Parallel()

	strct := &Decl{Kind: KindStruct}
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{name: "Slice", t: Slice{Elem: stringType}, want: true},
		{name: "Map", t: Map{Key: stringType, Elem: stringType}, want: true},
		{name: "Pointer", t: Pointer{Elem: stringType}, want: true},
		{name: "Any", t: anyType, want: true},
		{name: "Raw JSON", t: rawJSON, want: true},
		{name: "String", t: stringType},
		{name: "Time", t: Qualified{Import: importTime, Name: "Time"}},
		{name: "Struct", t: DeclRef{Decl: strct}},
		{name: "Enum", t: DeclRef{Decl: &Decl{Kind: KindEnum}}},
		{name: "Defined slice", t: DeclRef{Decl: &Decl{Kind: KindDefined, Target: Slice{Elem: stringType}}}, want: true},
		{name: "Alias of an alias of any", t: DeclRef{Decl: &Decl{Kind: KindAlias, Target: DeclRef{Decl: &Decl{Kind: KindAlias, Target: anyType}}}}, want: true},
		{name: "Alias of a struct", t: DeclRef{Decl: &Decl{Kind: KindAlias, Target: DeclRef{Decl: strct}}}},
		{name: "No type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nilable(tt.t))
		})
	}
}

func TestPointersInModel(t *testing.T) {
	t.Parallel()
	checkGolden(t, "pointers", "pointers", testOptions())
}

func TestNullableInModel(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.Nullable = true
	checkGolden(t, "pointers", "pointers-nullable", opts)

	opts.IsValidated, opts.IsServer, opts.HasResponseHeaders = true, true, true
	checkGolden(t, "nullable", "nullable", opts)
}

func TestNullableExtensionWithoutConfig(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.IsValidated = true
	checkGolden(t, "nullable", "nullable-extension", opts)
}

func TestHeldAndValidates(t *testing.T) {
	t.Parallel()

	str := Builtin{Name: "string"}
	pet := &Decl{Name: "Pet", Kind: KindStruct, Struct: &Struct{}, Validation: &Validation{}}
	plain := &Decl{Name: "Plain", Kind: KindStruct, Struct: &Struct{}}

	assert.Equal(t, Pointer{Elem: str}, Held(str))
	assert.Equal(t, Slice{Elem: str}, Held(Slice{Elem: str}))
	assert.True(t, Validates(DeclRef{Decl: pet}))
	assert.True(t, Validates(Pointer{Elem: DeclRef{Decl: pet}}))
	assert.True(t, Validates(emailType))
	assert.False(t, Validates(DeclRef{Decl: plain}))
	assert.False(t, Validates(str))

	alias := &Decl{Name: "Animal", Kind: KindAlias, Target: DeclRef{Decl: pet}}
	assert.Same(t, pet, StructDecl(Pointer{Elem: DeclRef{Decl: alias}}))
	assert.Nil(t, StructDecl(str))
	assert.Equal(t, DeclRef{Decl: pet}, Underlying(DeclRef{Decl: alias}))
	note := &Decl{Name: "Note", Kind: KindDefined, Target: DeclRef{Decl: &Decl{Name: "Text", Kind: KindAlias, Target: str}}}
	assert.Equal(t, str, Underlying(DeclRef{Decl: note}))
}

func TestZeroLiteral(t *testing.T) {
	t.Parallel()

	level := &Decl{Name: "Level", Kind: KindEnum, Enum: &Enum{Base: Builtin{Name: "int32"}}}
	tests := []struct {
		name string
		typ  Type
		want string
	}{
		{name: "String", typ: stringType, want: `""`},
		{name: "Email", typ: emailType, want: `""`},
		{name: "Number", typ: Builtin{Name: "float64"}, want: "0"},
		{name: "Bool", typ: boolType, want: "false"},
		{name: "Enum through an alias", typ: DeclRef{Decl: &Decl{Kind: KindAlias, Target: DeclRef{Decl: level}}}, want: "0"},
		{name: "Struct", typ: DeclRef{Decl: &Decl{Kind: KindStruct}}},
		{name: "Time", typ: Qualified{Import: importTime, Name: "Time"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ZeroLiteral(tc.typ))
		})
	}
}

func TestFormDecl(t *testing.T) {
	t.Parallel()

	pet := &Decl{Name: "Pet", Kind: KindStruct}
	alias := &Decl{Name: "Animal", Kind: KindAlias, Target: DeclRef{Decl: pet}}
	choice := &Decl{Name: "Choice", Kind: KindUnion, IsForm: true}
	scalars := &Decl{Name: "Scalars", Kind: KindUnion}
	tests := []struct {
		name string
		typ  Type
		want *Decl
	}{
		{name: "A struct behind a pointer and an alias", typ: Pointer{Elem: DeclRef{Decl: alias}}, want: pet},
		{name: "A union that reads forms", typ: Pointer{Elem: DeclRef{Decl: choice}}, want: choice},
		{name: "A union that does not", typ: DeclRef{Decl: scalars}},
		{name: "A builtin", typ: Builtin{Name: "string"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, FormDecl(tc.typ))
		})
	}
}

func TestErrorDecl(t *testing.T) {
	t.Parallel()

	problem := &Decl{Name: "Problem", Kind: KindStruct, Error: &ErrorMessage{Path: "detail"}}
	alias := &Decl{Name: "Failure", Kind: KindAlias, Target: DeclRef{Decl: problem}}
	fault := &Decl{Name: "Fault", Kind: KindUnion, Error: &ErrorMessage{Path: "detail"}}
	plain := &Decl{Name: "Locked", Kind: KindStruct}
	tests := []struct {
		name string
		typ  Type
		want *Decl
	}{
		{name: "An error type", typ: DeclRef{Decl: problem}, want: problem},
		{name: "Behind a pointer and an alias", typ: Pointer{Elem: DeclRef{Decl: alias}}, want: problem},
		{name: "A union", typ: Pointer{Elem: DeclRef{Decl: fault}}, want: fault},
		{name: "Behind a Nullable", typ: Nullable{Elem: DeclRef{Decl: problem}}, want: problem},
		{name: "A struct that is no error", typ: DeclRef{Decl: plain}},
		{name: "A builtin", typ: Builtin{Name: "string"}},
		{name: "No type", typ: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ErrorDecl(tc.typ))
		})
	}
}
