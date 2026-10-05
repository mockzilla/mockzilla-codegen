// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

const viewWant = `package api

import (
	"mime/multipart"
	"time"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// A pet.
type Pet struct {
	ID   string    ` + "`json:\"id\" yaml:\"id\"`" + `
	Born time.Time ` + "`json:\"born,omitempty,omitzero\"`" + `
	// Deprecated: the spec marks it deprecated.
	Nick   *string        ` + "`json:\"-,omitempty\"`" + `
	Dash   string         ` + "`json:\"-,\"`" + `
	Extras map[string]int ` + "`json:\"-\"`" + `
}

// Get returns the additional property name and whether it is set.
func (p Pet) Get(name string) (value int, found bool) {
	value, found = p.Extras[name]
	return value, found
}

// Set stores value as the additional property name.
func (p *Pet) Set(name string, value int) {
	if p.Extras == nil {
		p.Extras = make(map[string]int)
	}
	p.Extras[name] = value
}

// MarshalJSON writes the properties, then the additional properties in key order.
func (p Pet) MarshalJSON() ([]byte, error) {
	type plain Pet
	return runtime.MarshalAdditional(plain(p), p.Extras, "id", "born", "-", "-")
}

// UnmarshalJSON reads the properties, and every other key into Extras.
func (p *Pet) UnmarshalJSON(data []byte) error {
	type plain Pet
	return runtime.UnmarshalAdditional(data, (*plain)(p), &p.Extras, "id", "born", "-", "-")
}

// UnmarshalForm reads the properties, and every other name of the form into Extras.
func (p *Pet) UnmarshalForm(form *multipart.Form) error {
	type plain Pet
	return runtime.UnmarshalAdditionalForm(form, (*plain)(p), &p.Extras, "id", "born", "-", "-")
}

type Closed struct{}

type Odd struct {
	Extra any ` + "`json:\"-\"`" + `
}

type Pets []Pet

// Old name.
//
// Deprecated: the spec marks it deprecated.
type Name = string

type Status string

const (
	StatusActive Status = "active"
	StatusGone   Status = "gone"
)

// StatusValues returns the values of Status.
func StatusValues() []Status {
	return []Status{
		StatusActive,
		StatusGone,
	}
}

type Level int

const (
	LevelLow Level = 1.0
)
`

func TestViewRendersEveryKind(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Doc: "A pet.", IsForm: true, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{
			{Name: "ID", JSONName: "id", Type: str, Tags: []gomodel.Tag{{Key: "yaml", Value: "id"}}},
			{Name: "Born", JSONName: "born", Type: gomodel.Qualified{Import: gomodel.Import{Path: "time"}, Name: "Time"}, OmitEmpty: true},
			{Name: "Nick", JSONName: "-", Type: gomodel.Pointer{Elem: str}, OmitEmpty: true, Deprecated: true},
			{Name: "Dash", JSONName: "-", Type: str},
		},
		AdditionalProperties: &gomodel.Field{Name: "Extras", JSONName: "-", Type: gomodel.Map{Key: str, Elem: gomodel.Builtin{Name: "int"}}},
	}}
	decls := []*gomodel.Decl{
		pet,
		{Name: "Closed", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}},
		{Name: "Odd", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
			AdditionalProperties: &gomodel.Field{Name: "Extra", JSONName: "-", Type: gomodel.Builtin{Name: "any"}},
		}},
		{Name: "Pets", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}},
		{Name: "Name", Part: gomodel.PartTypes, Kind: gomodel.KindAlias, Target: str, Doc: "Old name.", Deprecated: true},
		{Name: "Status", Part: gomodel.PartEnums, Kind: gomodel.KindEnum, Enum: &gomodel.Enum{Base: str, Values: []gomodel.EnumValue{
			{Name: "StatusActive", Value: spec.Value{Kind: spec.KindString, Str: "active"}},
			{Name: "StatusGone", Value: spec.Value{Kind: spec.KindString, Str: "gone"}},
		}, ValuesFunc: "StatusValues"}},
		{Name: "Level", Part: gomodel.PartEnums, Kind: gomodel.KindEnum, Enum: &gomodel.Enum{Base: gomodel.Builtin{Name: "int"}, Values: []gomodel.EnumValue{
			{Name: "LevelLow", Value: spec.Value{Kind: spec.KindNumber, Num: json.Number("1.0")}},
		}}},
	}
	g := New(&gomodel.Model{Decls: decls})
	s := scope(t, g)
	e, err := render.New([]render.Set{Templates()}, render.Options{Format: true})
	require.NoError(t, err)

	data := render.FileData{Package: "api"}
	for _, part := range s.File.Parts {
		out, partErr := e.RenderPart(part, g.View(part, s))
		require.NoError(t, partErr)
		data.Parts = append(data.Parts, string(out))
	}
	data.Imports = s.Imports.Decl()
	got, err := e.RenderFile(data)

	require.NoError(t, err)
	assert.Equal(t, viewWant, string(got))
}

func TestJSONValue(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	date := gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "Date"}
	pet := &gomodel.Decl{Name: "Pet", Kind: gomodel.KindStruct}
	petAlias := &gomodel.Decl{Name: "Animal", Kind: gomodel.KindAlias, Target: gomodel.DeclRef{Decl: pet}}
	idAlias := &gomodel.Decl{Name: "ID", Kind: gomodel.KindAlias, Target: str}
	status := &gomodel.Decl{Name: "Status", Kind: gomodel.KindEnum}

	tests := []struct {
		name  string
		field gomodel.Field
		want  string
	}{
		{name: "Required", field: gomodel.Field{JSONName: "id", Type: str}, want: "id"},
		{name: "Optional builtin", field: gomodel.Field{JSONName: "id", Type: str, OmitEmpty: true}, want: "id,omitempty"},
		{name: "Optional pointer", field: gomodel.Field{JSONName: "p", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}, OmitEmpty: true}, want: "p,omitempty"},
		{name: "Optional type from another package", field: gomodel.Field{JSONName: "d", Type: date, OmitEmpty: true}, want: "d,omitempty,omitzero"},
		{name: "Optional struct", field: gomodel.Field{JSONName: "p", Type: gomodel.DeclRef{Decl: pet}, OmitEmpty: true}, want: "p,omitempty,omitzero"},
		{name: "Optional alias of a struct", field: gomodel.Field{JSONName: "a", Type: gomodel.DeclRef{Decl: petAlias}, OmitEmpty: true}, want: "a,omitempty,omitzero"},
		{name: "Optional alias of a builtin", field: gomodel.Field{JSONName: "i", Type: gomodel.DeclRef{Decl: idAlias}, OmitEmpty: true}, want: "i,omitempty"},
		{name: "Optional enum", field: gomodel.Field{JSONName: "s", Type: gomodel.DeclRef{Decl: status}, OmitEmpty: true}, want: "s,omitempty"},
		{name: "Required struct", field: gomodel.Field{JSONName: "p", Type: gomodel.DeclRef{Decl: pet}}, want: "p"},
		{name: "Property named dash", field: gomodel.Field{JSONName: "-", Type: str}, want: "-,"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, jsonValue(&tc.field))
		})
	}
}

// TestViewRendersUnions compares with testdata/unions.golden. UPDATE=1 writes it instead.
func TestViewRendersUnions(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	at := gomodel.Qualified{Import: gomodel.Import{Path: "time"}, Name: "Time"}
	cat := &gomodel.Decl{Name: "Cat", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, IsForm: true, Struct: &gomodel.Struct{}, Union: &gomodel.Union{
		Variants: []*gomodel.Variant{
			{Name: "Cat", FieldType: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: cat}}, Kinds: gomodel.JSONObject, Required: []string{"meow"}, Known: []string{"meow", "name"}},
			{Name: "Float", FieldType: gomodel.Pointer{Elem: gomodel.Builtin{Name: "float64"}}, Kinds: gomodel.JSONInteger | gomodel.JSONNumber},
			{Name: "Nested", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONObject, Shapes: []gomodel.Shape{
				{Required: []string{"a"}, Known: []string{"a", "b"}},
				{Known: []string{}, IsClosed: true},
				{},
			}},
		},
	}}
	stamp := &gomodel.Decl{Name: "Stamp", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: &gomodel.Union{
		IsAnyOf:    true,
		IsNullable: true,
		Variants: []*gomodel.Variant{
			{Name: "Time", FieldType: gomodel.Pointer{Elem: at}, Kinds: gomodel.JSONString},
			{Name: "Any", FieldType: gomodel.Builtin{Name: "any"}, Kinds: gomodel.JSONAny},
			{Name: "Loop", FieldType: gomodel.Pointer{Elem: str}},
		},
	}}
	id := &gomodel.Decl{Name: "ID", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: &gomodel.Union{
		IsText: true,
		Variants: []*gomodel.Variant{
			{Name: "Int", FieldType: gomodel.Pointer{Elem: gomodel.Builtin{Name: "int"}}, Kinds: gomodel.JSONInteger},
			{Name: "String", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString},
		},
	}}
	contact := &gomodel.Decl{Name: "Contact", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Doc: "A contact.", IsForm: true, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "ID", JSONName: "id", Type: str}},
	}, Union: &gomodel.Union{
		IsNullable:    true,
		Discriminator: "kind",
		Variants: []*gomodel.Variant{
			{Name: "Email", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONObject, Values: []string{"email", "mail"}, Known: []string{}, IsClosed: true},
			{Name: "Other", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONObject, IsDefault: true},
		},
	}}
	person := &gomodel.Decl{Name: "Person", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Kind", JSONName: "kind", Type: str}},
	}, Union: &gomodel.Union{
		IsAnyOf:       true,
		Discriminator: "kind",
		Variants:      []*gomodel.Variant{{Name: "Name", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString}},
	}}
	g := New(&gomodel.Model{Decls: []*gomodel.Decl{cat, pet, stamp, id, contact, person}})
	checkRender(t, g, "unions")
}

func TestViewRendersGetters(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	sortEnum := &gomodel.Decl{Name: "Sort", Part: gomodel.PartEnums, Kind: gomodel.KindEnum, Enum: &gomodel.Enum{Base: str, Values: []gomodel.EnumValue{
		{Name: "SortName", Value: spec.Value{Kind: spec.KindString, Str: "name"}},
	}}}
	long := spec.Value{Kind: spec.KindString, Str: "a default too long to quote in a doc line"}
	query := &gomodel.Decl{Name: "ListQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{
			{Name: "Limit", JSONName: "limit", Type: gomodel.Pointer{Elem: gomodel.Builtin{Name: "int"}}, OmitEmpty: true, Getter: &gomodel.Getter{
				Name: "GetLimit", Default: spec.Value{Kind: spec.KindNumber, Num: json.Number("20")},
			}},
			{Name: "Sort", JSONName: "sort", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: sortEnum}}, OmitEmpty: true, Getter: &gomodel.Getter{
				Name: "GetSort", Default: spec.Value{Kind: spec.KindString, Str: "name"},
			}},
			{Name: "Tags", JSONName: "tags", Type: gomodel.Slice{Elem: str}, OmitEmpty: true, Getter: &gomodel.Getter{
				Name: "GetTags", Default: spec.Value{Kind: spec.KindArray, Items: []spec.Value{{Kind: spec.KindString, Str: "new"}}},
			}},
			{Name: "Note", JSONName: "note", Type: gomodel.Pointer{Elem: str}, OmitEmpty: true, Getter: &gomodel.Getter{Name: "GetNote", Default: long}},
			{Name: "Page", JSONName: "page", Type: gomodel.Builtin{Name: "int"}},
		},
	}}
	g := New(&gomodel.Model{Decls: []*gomodel.Decl{sortEnum, query}})
	checkRender(t, g, "getters")
}

// checkRender renders every part of g into one file and compares it with testdata/<name>.golden.
// UPDATE=1 writes the file instead.
func checkRender(t *testing.T, g *Generator, name string) {
	t.Helper()

	s := scope(t, g)
	e, err := render.New([]render.Set{Templates()}, render.Options{Format: true})
	require.NoError(t, err)

	data := render.FileData{Package: "api"}
	for _, part := range s.File.Parts {
		out, partErr := e.RenderPart(part, g.View(part, s))
		require.NoError(t, partErr)
		data.Parts = append(data.Parts, string(out))
	}
	data.Imports = s.Imports.Decl()
	got, err := e.RenderFile(data)
	require.NoError(t, err)

	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}
