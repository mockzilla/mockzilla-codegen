// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	prop := []*spec.Property{{Name: "a", Schema: &spec.Schema{}}}
	tests := []struct {
		name   string
		schema spec.Schema
		want   shape
	}{
		{name: "Nothing", want: shapeAny},
		{name: "Null alone", schema: spec.Schema{Types: spec.TypeNull}, want: shapeAny},
		{name: "oneOf", schema: spec.Schema{OneOf: []*spec.Schema{{}, {Types: spec.TypeString}}}, want: shapeUnion},
		{name: "anyOf", schema: spec.Schema{AnyOf: []*spec.Schema{{}, {Types: spec.TypeString}}}, want: shapeUnion},
		{name: "oneOf of one member is no union", schema: spec.Schema{OneOf: []*spec.Schema{{Types: spec.TypeString}}}, want: shapeAny},
		{name: "oneOf of a member and null is no union", schema: spec.Schema{OneOf: []*spec.Schema{{}, {Types: spec.TypeNull}}}, want: shapeAny},
		{name: "if with then and else", schema: spec.Schema{Then: &spec.Schema{}, Else: &spec.Schema{}}, want: shapeUnion},
		{name: "Type list", schema: spec.Schema{Types: spec.TypeString | spec.TypeInteger}, want: shapeUnion},
		{name: "Nullable string is no union", schema: spec.Schema{Types: spec.TypeString | spec.TypeNull}, want: shapePrimitive},
		{name: "Enum", schema: spec.Schema{Types: spec.TypeString, Enum: []spec.Value{strVal("a")}}, want: shapeEnum},
		{name: "Enum of null only", schema: spec.Schema{Types: spec.TypeString, Enum: []spec.Value{nullVal()}}, want: shapePrimitive},
		{name: "Object with properties", schema: spec.Schema{Types: spec.TypeObject, Properties: prop}, want: shapeStruct},
		{name: "Properties without a type", schema: spec.Schema{Properties: prop}, want: shapeStruct},
		{name: "Closed object", schema: spec.Schema{Types: spec.TypeObject, AdditionalProperties: spec.Additional{Mode: spec.AdditionalDenied}}, want: shapeStruct},
		{name: "Object", schema: spec.Schema{Types: spec.TypeObject}, want: shapeMap},
		{name: "Values without a type", schema: spec.Schema{AdditionalProperties: spec.Additional{Mode: spec.AdditionalAllowed}}, want: shapeMap},
		{name: "Array", schema: spec.Schema{Types: spec.TypeArray}, want: shapeArray},
		{name: "Items without a type", schema: spec.Schema{Items: &spec.Schema{}}, want: shapeArray},
		{name: "Format without a type", schema: spec.Schema{Format: "date"}, want: shapePrimitive},
		{name: "Const without a type", schema: spec.Schema{Const: new(strVal("a"))}, want: shapePrimitive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classify(&tt.schema))
		})
	}
}

func TestRefOf(t *testing.T) {
	t.Parallel()

	ref := &spec.Ref{Pointer: "/components/schemas/Pet", Name: "Pet"}
	tests := []struct {
		name   string
		schema spec.Schema
		want   *spec.Ref
	}{
		{name: "Plain ref", schema: spec.Schema{Ref: ref}, want: ref},
		{name: "Ref with docs and flags", schema: spec.Schema{Ref: ref, Description: "d", Nullable: true, Types: spec.TypeObject}, want: ref},
		{name: "Ref with properties", schema: spec.Schema{Ref: ref, Properties: []*spec.Property{{Name: "a"}}}},
		{name: "Ref with allOf", schema: spec.Schema{Ref: ref, AllOf: []*spec.Schema{{}}}},
		{name: "allOf of one ref", schema: spec.Schema{AllOf: []*spec.Schema{{Ref: ref}}, Description: "d"}, want: ref},
		{name: "allOf of nested allOf", schema: spec.Schema{AllOf: []*spec.Schema{{AllOf: []*spec.Schema{{Ref: ref}}}}}, want: ref},
		{name: "allOf of an inline schema", schema: spec.Schema{AllOf: []*spec.Schema{{Types: spec.TypeString}}}},
		{name: "allOf of two refs", schema: spec.Schema{AllOf: []*spec.Schema{{Ref: ref}, {Ref: ref}}}},
		{name: "allOf of a ref and docs", schema: spec.Schema{AllOf: []*spec.Schema{{Description: "d"}, {Ref: ref}, {Nullable: true}}}, want: ref},
		{name: "allOf of a ref and a default", schema: spec.Schema{AllOf: []*spec.Schema{{Ref: ref}, {Default: new(strVal("a"))}}}, want: ref},
		{name: "allOf of a ref and required", schema: spec.Schema{AllOf: []*spec.Schema{{Ref: ref}, {Required: []string{"a"}}}}},
		{name: "Ref with required", schema: spec.Schema{Ref: ref, Required: []string{"a"}}},
		{name: "allOf of docs only", schema: spec.Schema{AllOf: []*spec.Schema{{Description: "d"}}}},
		{name: "No ref", schema: spec.Schema{Types: spec.TypeString}},
		{name: "oneOf of a ref and null", schema: spec.Schema{OneOf: []*spec.Schema{{Ref: ref}, {Types: spec.TypeNull}}}, want: ref},
		{name: "anyOf of a ref and a null enum", schema: spec.Schema{AnyOf: []*spec.Schema{{Enum: []spec.Value{nullVal()}}, {Ref: ref}}}, want: ref},
		{name: "oneOf of a ref and a null const", schema: spec.Schema{OneOf: []*spec.Schema{{Ref: ref}, {Const: new(nullVal())}}}, want: ref},
		{name: "Ref next to a oneOf of one member", schema: spec.Schema{Ref: ref, OneOf: []*spec.Schema{{Ref: ref}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, refOf(&tt.schema))
		})
	}
}

func TestIsDocOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema spec.Schema
		want   bool
	}{
		{name: "Description and flags", schema: spec.Schema{Description: "d", ReadOnly: true, Nullable: true}, want: true},
		{name: "Limits", schema: spec.Schema{Limits: spec.Limits{MaxLength: new(int64(5))}}, want: true},
		{name: "Default", schema: spec.Schema{Default: new(strVal("a"))}, want: true},
		{name: "Required", schema: spec.Schema{Required: []string{"a"}}},
		{name: "Type", schema: spec.Schema{Types: spec.TypeString}},
		{name: "Format", schema: spec.Schema{Format: "date"}},
		{name: "Const", schema: spec.Schema{Const: new(strVal("a"))}},
		{name: "Ref", schema: spec.Schema{Ref: &spec.Ref{}}},
		{name: "allOf", schema: spec.Schema{AllOf: []*spec.Schema{{}}}},
		{name: "Items", schema: spec.Schema{Items: &spec.Schema{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isDocOnly(&tt.schema))
		})
	}
}

func TestDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema spec.Schema
		want   string
	}{
		{name: "Own", schema: spec.Schema{Description: "own", AllOf: []*spec.Schema{{Description: "member"}}}, want: "own"},
		{name: "From a doc member", schema: spec.Schema{AllOf: []*spec.Schema{{Ref: &spec.Ref{}}, {Description: "member"}}}, want: "member"},
		{name: "Not from a shaped member", schema: spec.Schema{AllOf: []*spec.Schema{{Types: spec.TypeString, Description: "member"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, description(&tt.schema))
		})
	}
}
