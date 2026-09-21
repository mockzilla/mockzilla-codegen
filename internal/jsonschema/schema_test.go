// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func num(s string) json.Number {
	return json.Number(s)
}

func TestSchema(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	tests := []struct {
		name   string
		schema *spec.Schema
		want   string
	}{
		{name: "Nil takes anything", want: `{}`},
		{name: "One type", schema: str, want: `{"type":"string"}`},
		{name: "Several types in a fixed order", schema: &spec.Schema{Types: spec.TypeArray | spec.TypeInteger | spec.TypeString}, want: `{"type":["string","integer","array"]}`},
		{name: "Nullable adds null", schema: &spec.Schema{Types: spec.TypeString, Nullable: true}, want: `{"type":["string","null"]}`},
		{name: "Nullable with null already there", schema: &spec.Schema{Types: spec.TypeNull, Nullable: true}, want: `{"type":"null"}`},
		{
			name:   "Annotations",
			schema: &spec.Schema{Types: spec.TypeString, Format: "date-time", Title: "When", Description: "A time.", Deprecated: true, ReadOnly: true, WriteOnly: true, ContentEncoding: "base64", ContentMediaType: "image/png"},
			want:   `{"type":"string","format":"date-time","title":"When","description":"A time.","deprecated":true,"readOnly":true,"writeOnly":true,"contentEncoding":"base64","contentMediaType":"image/png"}`,
		},
		{
			name:   "Composition",
			schema: &spec.Schema{AllOf: []*spec.Schema{str}, OneOf: []*spec.Schema{str, nil}, AnyOf: []*spec.Schema{str}, Not: str, If: str, Then: str, Else: str},
			want:   `{"allOf":[{"type":"string"}],"oneOf":[{"type":"string"},{}],"anyOf":[{"type":"string"}],"not":{"type":"string"},"if":{"type":"string"},"then":{"type":"string"},"else":{"type":"string"}}`,
		},
		{
			name: "Object in property order",
			schema: &spec.Schema{Types: spec.TypeObject, Required: []string{"name"}, Properties: []*spec.Property{
				{Name: "name", Schema: str, Required: true},
				{Name: "age", Schema: &spec.Schema{Types: spec.TypeInteger}},
			}},
			want: `{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name"]}`,
		},
		{name: "Additional properties allowed", schema: &spec.Schema{AdditionalProperties: spec.Additional{Mode: spec.AdditionalAllowed}}, want: `{"additionalProperties":true}`},
		{name: "Additional properties denied", schema: &spec.Schema{AdditionalProperties: spec.Additional{Mode: spec.AdditionalDenied}}, want: `{"additionalProperties":false}`},
		{name: "Additional properties with a schema", schema: &spec.Schema{AdditionalProperties: spec.Additional{Mode: spec.AdditionalSchema, Schema: str}}, want: `{"additionalProperties":{"type":"string"}}`},
		{name: "Array", schema: &spec.Schema{Types: spec.TypeArray, Items: str, PrefixItems: []*spec.Schema{str}}, want: `{"type":"array","prefixItems":[{"type":"string"}],"items":{"type":"string"}}`},
		{
			name: "Values",
			schema: &spec.Schema{
				Enum:     []spec.Value{{Kind: spec.KindString, Str: "a"}, {Kind: spec.KindNull}},
				Const:    &spec.Value{Kind: spec.KindNumber, Num: num("1")},
				Default:  &spec.Value{Kind: spec.KindBool, Bool: true},
				Examples: []spec.Value{{Kind: spec.KindArray, Items: []spec.Value{{Kind: spec.KindString, Str: "x"}}}, {Kind: spec.KindObject, Fields: []spec.Field{{Name: "k", Value: spec.Value{Kind: spec.KindString, Str: "v"}}}}},
			},
			want: `{"enum":["a",null],"const":1,"default":true,"examples":[["x"],{"k":"v"}]}`,
		},
		{
			name: "Limits",
			schema: &spec.Schema{Limits: spec.Limits{
				Minimum: &spec.Bound{Value: num("0")}, Maximum: &spec.Bound{Value: num("10.5"), Exclusive: true}, MultipleOf: new(num("2")),
				MinLength: new(int64(1)), MaxLength: new(int64(2)), MinItems: new(int64(3)), MaxItems: new(int64(4)), UniqueItems: true, MinProperties: new(int64(5)), MaxProperties: new(int64(6)),
			}},
			want: `{"minimum":0,"exclusiveMaximum":10.5,"multipleOf":2,"minLength":1,"maxLength":2,"minItems":3,"maxItems":4,"uniqueItems":true,"minProperties":5,"maxProperties":6}`,
		},
		{name: "Exclusive minimum", schema: &spec.Schema{Limits: spec.Limits{Minimum: &spec.Bound{Value: num("1"), Exclusive: true}}}, want: `{"exclusiveMinimum":1}`},
		{name: "Extensions and discriminator are left out", schema: &spec.Schema{Extensions: []spec.Extension{{Name: "x-go-type"}}, Discriminator: &spec.Discriminator{Property: "kind"}}, want: `{}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder()

			got := b.Document(b.Schema(tc.schema))

			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestSchemaRefs(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	pet := &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "name", Schema: str}}}
	petRef := &spec.Ref{Pointer: "/components/schemas/Pet", Name: "Pet", Target: pet}
	node := &spec.Schema{Types: spec.TypeObject}
	nodeRef := &spec.Ref{Pointer: "/components/schemas/Node", Name: "Node", Target: node}
	node.Properties = []*spec.Property{{Name: "next", Schema: &spec.Schema{Ref: nodeRef}}}
	inline := &spec.Schema{Ref: &spec.Ref{Pointer: "/paths/~1pets/get/x", Target: str}}
	tests := []struct {
		name   string
		schema *spec.Schema
		want   string
	}{
		{name: "A component goes to $defs under its name", schema: &spec.Schema{Ref: petRef}, want: `{"$ref":"#/$defs/Pet","$defs":{"Pet":{"type":"object","properties":{"name":{"type":"string"}}}}}`},
		{name: "Keywords next to the $ref stay", schema: &spec.Schema{Ref: petRef, Description: "The pet."}, want: `{"$ref":"#/$defs/Pet","description":"The pet.","$defs":{"Pet":{"type":"object","properties":{"name":{"type":"string"}}}}}`},
		{
			name:   "A component used twice is defined once",
			schema: &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "a", Schema: &spec.Schema{Ref: petRef}}, {Name: "b", Schema: &spec.Schema{Ref: petRef}}}},
			want:   `{"type":"object","properties":{"a":{"$ref":"#/$defs/Pet"},"b":{"$ref":"#/$defs/Pet"}},"$defs":{"Pet":{"type":"object","properties":{"name":{"type":"string"}}}}}`,
		},
		{name: "A cycle ends at the $ref", schema: &spec.Schema{Ref: nodeRef}, want: `{"$ref":"#/$defs/Node","$defs":{"Node":{"type":"object","properties":{"next":{"$ref":"#/$defs/Node"}}}}}`},
		{name: "A ref that is no component is defined under its pointer", schema: inline, want: `{"$ref":"#/$defs/~1paths~1~01pets~1get~1x","$defs":{"/paths/~1pets/get/x":{"type":"string"}}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder()

			got := b.Document(b.Schema(tc.schema))

			assert.Equal(t, tc.want, string(got))
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestDocumentSortsDefinitions(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	b := NewBuilder()
	root := new(Object).Set("type", "object")
	for _, name := range []string{"Zebra", "Ant"} {
		b.Schema(&spec.Schema{Ref: &spec.Ref{Pointer: "/components/schemas/" + name, Name: name, Target: str}})
	}

	got := b.Document(root)

	want := `{"type":"object","$defs":{"Ant":{"type":"string"},"Zebra":{"type":"string"}}}`
	assert.Equal(t, want, string(got), "the definitions come sorted")
}
