// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonschema

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
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
		{name: "Nullable without types takes anything", schema: &spec.Schema{Nullable: true, Description: "Anything."}, want: `{"description":"Anything."}`},
		{name: "Nullable with an enum that has null", schema: &spec.Schema{Types: spec.TypeString, Nullable: true, Enum: []spec.Value{{Kind: spec.KindString, Str: "a"}, {Kind: spec.KindNull}}}, want: `{"type":["string","null"],"enum":["a",null]}`},
		{
			name: "Nullable with an enum without null is anyOf with null, the docs and samples outside",
			schema: &spec.Schema{
				Types: spec.TypeString, Nullable: true, Format: "color", Title: "Color", Description: "A color.", Deprecated: true, ReadOnly: true, WriteOnly: true,
				Enum: []spec.Value{{Kind: spec.KindString, Str: "red"}}, Default: &spec.Value{Kind: spec.KindNull}, Examples: []spec.Value{{Kind: spec.KindString, Str: "red"}},
				Limits: spec.Limits{MinLength: new(int64(1))},
			},
			want: `{"anyOf":[{"type":"string","format":"color","enum":["red"],"minLength":1},{"type":"null"}],"title":"Color","description":"A color.","deprecated":true,"readOnly":true,"writeOnly":true,"default":null,"examples":["red"]}`,
		},
		{name: "Nullable with a const", schema: &spec.Schema{Nullable: true, Const: &spec.Value{Kind: spec.KindString, Str: "on"}}, want: `{"anyOf":[{"const":"on"},{"type":"null"}]}`},
		{name: "Nullable with a null const", schema: &spec.Schema{Nullable: true, Const: &spec.Value{Kind: spec.KindNull}}, want: `{"const":null}`},
		{name: "Nullable composition", schema: &spec.Schema{Nullable: true, OneOf: []*spec.Schema{str}}, want: `{"anyOf":[{"oneOf":[{"type":"string"}]},{"type":"null"}]}`},
		{
			name:   "Annotations",
			schema: &spec.Schema{Types: spec.TypeString, Format: "date-time", Title: "When", Description: "A time.", Deprecated: true, ReadOnly: true, WriteOnly: true, ContentEncoding: "base64", ContentMediaType: "image/png"},
			want:   `{"type":"string","format":"date-time","title":"When","description":"A time.","deprecated":true,"readOnly":true,"writeOnly":true,"contentEncoding":"base64","contentMediaType":"image/png"}`,
		},
		{name: "Binary is base64", schema: &spec.Schema{Types: spec.TypeString, Format: "binary"}, want: `{"type":"string","format":"binary","contentEncoding":"base64"}`},
		{name: "Byte in any case is base64", schema: &spec.Schema{Types: spec.TypeString, Format: "Byte"}, want: `{"type":"string","format":"Byte","contentEncoding":"base64"}`},
		{name: "The encoding the schema names stays", schema: &spec.Schema{Types: spec.TypeString, Format: "binary", ContentEncoding: "base32"}, want: `{"type":"string","format":"binary","contentEncoding":"base32"}`},
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
				Const:    &spec.Value{Kind: spec.KindString, Str: "a"},
				Default:  &spec.Value{Kind: spec.KindString, Str: "a"},
				Examples: []spec.Value{{Kind: spec.KindArray, Items: []spec.Value{{Kind: spec.KindString, Str: "x"}}}, {Kind: spec.KindObject, Fields: []spec.Field{{Name: "k", Value: spec.Value{Kind: spec.KindString, Str: "v"}}}}, {Kind: spec.KindNumber, Num: num("1")}, {Kind: spec.KindBool, Bool: true}},
			},
			want: `{"enum":["a",null],"const":"a","default":"a","examples":[["x"],{"k":"v"},1,true]}`,
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
		{name: "Pattern after the lengths", schema: &spec.Schema{Types: spec.TypeString, Pattern: `^\d+$`, Limits: spec.Limits{MaxLength: new(int64(4))}}, want: `{"type":"string","maxLength":4,"pattern":"^\\d+$"}`},
		{name: "Pattern with \\u escapes", schema: &spec.Schema{Pattern: `^[\u0020-\u007E\u00e9]+\u2026$`}, want: `{"pattern":"^[ -\\x7E\\xE9]+…$"}`},
		{name: "Extensions and discriminator are left out", schema: &spec.Schema{Extensions: []spec.Extension{{Name: "x-go-type"}}, Discriminator: &spec.Discriminator{Property: "kind"}}, want: `{}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder()

			got := b.Document(b.Schema(tc.schema))

			assert.Equal(t, tc.want, string(got))
			assert.Empty(t, b.Diagnostics())
		})
	}
}

func TestSchemaLeavesOutADefaultThatDoesNotFit(t *testing.T) {
	t.Parallel()

	limit := &spec.Schema{
		Types:   spec.TypeInteger,
		Default: &spec.Value{Kind: spec.KindString, Str: "20"},
		Origin:  spec.Origin{Pointer: "/components/schemas/Limit", File: "api.yaml", Line: 7, Col: 5},
	}
	ref := &spec.Ref{Pointer: "/components/schemas/Limit", Name: "Limit", Target: limit}
	long := &spec.Schema{Types: spec.TypeString, Default: &spec.Value{Kind: spec.KindArray, Items: []spec.Value{
		{Kind: spec.KindString, Str: "a value long enough"}, {Kind: spec.KindString, Str: "not to be quoted"},
	}}}
	root := &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{
		{Name: "a", Schema: &spec.Schema{Ref: ref}},
		{Name: "b", Schema: &spec.Schema{Ref: ref}},
		{Name: "c", Schema: long},
	}}
	b := NewBuilder()

	got := b.Document(b.Schema(root))

	want := `{"type":"object","properties":{"a":{"$ref":"#/$defs/Limit"},"b":{"$ref":"#/$defs/Limit"},"c":{"type":"string"}},"$defs":{"Limit":{"type":"integer"}}}`
	assert.Equal(t, want, string(got), "neither default is written")
	assert.Equal(t, []diag.Diagnostic{
		{
			Severity: diag.Warning,
			Code:     diag.CodeDefaultIgnored,
			Pointer:  "/components/schemas/Limit",
			Origin:   diag.Origin{File: "api.yaml", Line: 7, Col: 5},
			Message:  `the default "20" does not fit its schema, so the tool input leaves it out: it is a string, the schema wants integer`,
		},
		{
			Severity: diag.Warning,
			Code:     diag.CodeDefaultIgnored,
			Message:  "the default does not fit its schema, so the tool input leaves it out: it is an array, the schema wants string",
		},
	}, b.Diagnostics(), "a component used twice is warned about once")
}

func TestSchemaLeavesOutAPatternThatIsNotRE2(t *testing.T) {
	t.Parallel()

	owner := &spec.Schema{
		Types:   spec.TypeString,
		Pattern: "^(?!root$).+$",
		Origin:  spec.Origin{Pointer: "/components/schemas/Owner", File: "api.yaml", Line: 9, Col: 7},
	}
	b := NewBuilder()

	got := b.Document(b.Schema(owner))

	assert.JSONEq(t, `{"type":"string"}`, string(got))
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodePatternUnsupported,
		Pointer:  "/components/schemas/Owner",
		Origin:   diag.Origin{File: "api.yaml", Line: 9, Col: 7},
		Message:  "pattern \"^(?!root$).+$\" is not RE2 (error parsing regexp: invalid or unsupported Perl syntax: `(?!`), so the tool input leaves it out",
	}}, b.Diagnostics())
}

func TestSchemaLeavesOutACountAboveInt32(t *testing.T) {
	t.Parallel()

	most, above := int64(math.MaxInt32), int64(math.MaxInt32)+1
	name := &spec.Schema{
		Types:  spec.TypeString,
		Limits: spec.Limits{MinLength: &most, MaxLength: &above},
		Origin: spec.Origin{Pointer: "/components/schemas/Name", File: "api.yaml", Line: 4, Col: 7},
	}
	b := NewBuilder()

	got := b.Document(b.Schema(name))

	assert.JSONEq(t, `{"type":"string","minLength":2147483647}`, string(got))
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodeLimitUnsupported,
		Pointer:  "/components/schemas/Name",
		Origin:   diag.Origin{File: "api.yaml", Line: 4, Col: 7},
		Message:  "maxLength 2147483648 is above 2147483647, the most the MCP SDK takes, so the tool input leaves it out",
	}}, b.Diagnostics())
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
		{
			name:   "A nullable $ref is anyOf with null",
			schema: &spec.Schema{Ref: petRef, Nullable: true, Description: "The pet, if any."},
			want:   `{"anyOf":[{"$ref":"#/$defs/Pet"},{"type":"null"}],"description":"The pet, if any.","$defs":{"Pet":{"type":"object","properties":{"name":{"type":"string"}}}}}`,
		},
		{
			name:   "A nullable allOf of a $ref is anyOf with null",
			schema: &spec.Schema{Nullable: true, AllOf: []*spec.Schema{{Ref: petRef}}},
			want:   `{"anyOf":[{"allOf":[{"$ref":"#/$defs/Pet"}]},{"type":"null"}],"$defs":{"Pet":{"type":"object","properties":{"name":{"type":"string"}}}}}`,
		},
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

func TestSchemaLeavesOutReadOnly(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	integer := &spec.Schema{Types: spec.TypeInteger}
	id := &spec.Schema{Types: spec.TypeInteger, ReadOnly: true}
	idRef := &spec.Ref{Pointer: "/components/schemas/ID", Name: "ID", Target: id}
	stored := &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "id", Schema: id}, {Name: "name", Schema: str}}}
	storedRef := &spec.Ref{Pointer: "/components/schemas/Stored", Name: "Stored", Target: stored}
	loop := &spec.Schema{Types: spec.TypeObject, Required: []string{"id"}, Properties: []*spec.Property{{Name: "id", Schema: id}}}
	loopRef := &spec.Ref{Pointer: "/components/schemas/Loop", Name: "Loop", Target: loop}
	loop.AllOf = []*spec.Schema{{Ref: loopRef}}
	tests := []struct {
		name   string
		schema *spec.Schema
		want   string
	}{
		{
			name:   "A readOnly property is left out with its required entry",
			schema: &spec.Schema{Types: spec.TypeObject, Required: []string{"id", "name"}, Properties: []*spec.Property{{Name: "id", Schema: id}, {Name: "name", Schema: str}}},
			want:   `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		},
		{
			name:   "Only readOnly properties leave an object without properties",
			schema: &spec.Schema{Types: spec.TypeObject, Required: []string{"id"}, Properties: []*spec.Property{{Name: "id", Schema: id}}},
			want:   `{"type":"object"}`,
		},
		{
			name:   "A property readOnly through its $ref",
			schema: &spec.Schema{Types: spec.TypeObject, Required: []string{"id", "name"}, Properties: []*spec.Property{{Name: "id", Schema: &spec.Schema{Ref: idRef}}, {Name: "name", Schema: str}}},
			want:   `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		},
		{
			name:   "A property readOnly through an allOf member",
			schema: &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "id", Schema: &spec.Schema{Types: spec.TypeInteger, AllOf: []*spec.Schema{{ReadOnly: true}, str}}}}},
			want:   `{"type":"object"}`,
		},
		{
			name:   "A required entry next to a $ref leaves out what its target holds readOnly",
			schema: &spec.Schema{Ref: storedRef, Required: []string{"id", "name"}},
			want:   `{"$ref":"#/$defs/Stored","required":["name"],"$defs":{"Stored":{"type":"object","properties":{"name":{"type":"string"}}}}}`,
		},
		{
			name: "An allOf member leaves out what another member holds readOnly",
			schema: &spec.Schema{AllOf: []*spec.Schema{
				{Ref: storedRef},
				{Types: spec.TypeObject, Required: []string{"id", "name", "age"}, Properties: []*spec.Property{{Name: "id", Schema: integer}, {Name: "age", Schema: integer}}},
			}},
			want: `{"allOf":[{"$ref":"#/$defs/Stored"},{"type":"object","properties":{"age":{"type":"integer"}},"required":["name","age"]}],"$defs":{"Stored":{"type":"object","properties":{"name":{"type":"string"}}}}}`,
		},
		{
			name: "A nested object keeps a property its parent holds readOnly",
			schema: &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{
				{Name: "id", Schema: id},
				{Name: "owner", Schema: &spec.Schema{Types: spec.TypeObject, Required: []string{"id"}, Properties: []*spec.Property{{Name: "id", Schema: integer}}}},
			}},
			want: `{"type":"object","properties":{"owner":{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}}}`,
		},
		{
			name:   "An allOf that refers to itself ends",
			schema: &spec.Schema{Ref: loopRef},
			want:   `{"$ref":"#/$defs/Loop","$defs":{"Loop":{"type":"object","allOf":[{"$ref":"#/$defs/Loop"}]}}}`,
		},
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

func TestRejectsNull(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	tests := []struct {
		name   string
		schema *spec.Schema
		want   bool
	}{
		{name: "Types and limits do not", schema: &spec.Schema{Types: spec.TypeString, Limits: spec.Limits{MinLength: new(int64(1))}, Properties: []*spec.Property{{Name: "a", Schema: str}}}},
		{name: "A $ref", schema: &spec.Schema{Ref: &spec.Ref{Name: "S", Target: str}}, want: true},
		{name: "All of", schema: &spec.Schema{AllOf: []*spec.Schema{str}}, want: true},
		{name: "One of", schema: &spec.Schema{OneOf: []*spec.Schema{str}}, want: true},
		{name: "Any of", schema: &spec.Schema{AnyOf: []*spec.Schema{str}}, want: true},
		{name: "Not", schema: &spec.Schema{Not: str}, want: true},
		{name: "If", schema: &spec.Schema{If: str}, want: true},
		{name: "An enum without null", schema: &spec.Schema{Enum: []spec.Value{stringValue("on")}}, want: true},
		{name: "An enum with null", schema: &spec.Schema{Enum: []spec.Value{stringValue("on"), nullValue}}},
		{name: "A const", schema: &spec.Schema{Const: new(stringValue("on"))}, want: true},
		{name: "A null const", schema: &spec.Schema{Const: new(nullValue)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, rejectsNull(tc.schema))
		})
	}
}
