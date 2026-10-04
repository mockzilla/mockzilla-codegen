// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonschema

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestSchemaPinsDiscriminatedVariants(t *testing.T) {
	t.Parallel()

	str := &spec.Schema{Types: spec.TypeString}
	animal := func(extra string) *spec.Schema {
		return &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "kind", Schema: str}, {Name: extra, Schema: str}}}
	}
	cat := &spec.Ref{Pointer: "/components/schemas/Cat", Name: "Cat", Target: animal("indoor")}
	dog := &spec.Ref{Pointer: "/components/schemas/Dog", Name: "Dog", Target: animal("breed")}
	bird := &spec.Ref{Pointer: "/components/schemas/Bird", Name: "Bird", Target: &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{
		{Name: "kind", Schema: &spec.Schema{Types: spec.TypeString, Const: new(stringValue("bird"))}},
	}}}
	inline := &spec.Ref{Pointer: "/paths/~1pets/post/x", Target: animal("legs")}
	defs := `"$defs":{"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}},"Dog":{"type":"object","properties":{"kind":{"type":"string"},"breed":{"type":"string"}}}}`
	pinnedCat := `{"$ref":"#/$defs/Cat","properties":{"kind":{"const":"cat"}},"required":["kind"]}`
	pinnedDog := `{"$ref":"#/$defs/Dog","properties":{"kind":{"const":"dog"}},"required":["kind"]}`
	mapping := []spec.Mapping{{Value: "cat", Ref: cat}, {Value: "dog", Ref: dog}}
	tests := []struct {
		name   string
		schema *spec.Schema
		want   string
	}{
		{
			name:   "Each variant takes the value the mapping lists for it",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"oneOf":[` + pinnedCat + `,` + pinnedDog + `],` + defs + `}`,
		},
		{
			name: "A variant with two mapping values takes either",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: []spec.Mapping{
				{Value: "cat", Ref: cat}, {Value: "kitten", Ref: cat}, {Value: "dog", Ref: dog},
			}}},
			want: `{"oneOf":[{"$ref":"#/$defs/Cat","properties":{"kind":{"enum":["cat","kitten"]}},"required":["kind"]},` + pinnedDog + `],` + defs + `}`,
		},
		{
			name:   "Without a mapping a variant takes its component name",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind"}},
			want:   `{"oneOf":[{"$ref":"#/$defs/Cat","properties":{"kind":{"const":"Cat"}},"required":["kind"]},{"$ref":"#/$defs/Dog","properties":{"kind":{"const":"Dog"}},"required":["kind"]}],` + defs + `}`,
		},
		{
			name: "A mapping to a schema that is no variant changes nothing, an unresolved one too",
			schema: &spec.Schema{
				OneOf:         []*spec.Schema{{Ref: cat}, {Ref: &spec.Ref{Name: "Gone"}}},
				Discriminator: &spec.Discriminator{Property: "kind", Mapping: []spec.Mapping{{Value: "dog", Ref: dog}, {Value: "x", Ref: &spec.Ref{Name: "X"}}}},
			},
			want: `{"oneOf":[{"$ref":"#/$defs/Cat","properties":{"kind":{"const":"Cat"}},"required":["kind"]},{"$ref":"#/$defs/Gone","properties":{"kind":{"const":"Gone"}},"required":["kind"]}],` +
				`"$defs":{"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}},"Gone":{}}}`,
		},
		{
			name:   "A variant that holds the property to a value through its $ref is only made to require it",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: bird}}, Discriminator: &spec.Discriminator{Property: "kind"}},
			want:   `{"oneOf":[{"$ref":"#/$defs/Bird","required":["kind"]}],"$defs":{"Bird":{"type":"object","properties":{"kind":{"type":"string","const":"bird"}}}}}`,
		},
		{
			name: "An inline variant with a const or an enum of one value is only made to require it",
			schema: &spec.Schema{Discriminator: &spec.Discriminator{Property: "kind"}, OneOf: []*spec.Schema{
				{Types: spec.TypeObject, Required: []string{"name"}, Properties: []*spec.Property{{Name: "kind", Schema: &spec.Schema{Const: new(stringValue("a"))}}}},
				{AllOf: []*spec.Schema{{Properties: []*spec.Property{{Name: "kind", Schema: &spec.Schema{Enum: []spec.Value{stringValue("b")}}}}}}},
				{Required: []string{"kind"}, Properties: []*spec.Property{{Name: "kind", Schema: &spec.Schema{AllOf: []*spec.Schema{{Const: new(stringValue("c"))}}}}}},
			}},
			want: `{"oneOf":[` +
				`{"type":"object","properties":{"kind":{"const":"a"}},"required":["name","kind"]},` +
				`{"allOf":[{"properties":{"kind":{"enum":["b"]}}}],"required":["kind"]},` +
				`{"properties":{"kind":{"allOf":[{"const":"c"}]}},"required":["kind"]}]}`,
		},
		{
			name: "A variant without a value of its own makes the list anyOf",
			schema: &spec.Schema{Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}, OneOf: []*spec.Schema{
				{Ref: cat},
				{Ref: inline},
				{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "kind", Schema: &spec.Schema{Const: new(nullValue)}}}},
				{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "kind", Schema: &spec.Schema{Enum: []spec.Value{stringValue("x"), stringValue("y")}}}}},
			}},
			want: `{"anyOf":[` + pinnedCat + `,{"$ref":"#/$defs/~1paths~1~01pets~1post~1x"},` +
				`{"type":"object","properties":{"kind":{"const":null}}},` +
				`{"type":"object","properties":{"kind":{"enum":["x","y"]}}}],` +
				`"$defs":{"/paths/~1pets/post/x":{"type":"object","properties":{"kind":{"type":"string"},"legs":{"type":"string"}}},"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}}}}`,
		},
		{
			name:   "The default variant takes any value, so the list is anyOf",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping[:1], Default: dog}},
			want:   `{"anyOf":[` + pinnedCat + `,{"$ref":"#/$defs/Dog"}],` + defs + `}`,
		},
		{
			name:   "A member that takes anything makes the list anyOf",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, nil}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"anyOf":[` + pinnedCat + `,{}],"$defs":{"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}}}}`,
		},
		{
			name: "A null member is no variant",
			schema: &spec.Schema{Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}, OneOf: []*spec.Schema{
				{Ref: cat}, {Ref: dog}, {Types: spec.TypeNull, Nullable: true}, {Const: new(nullValue)}, {Enum: []spec.Value{nullValue}},
			}},
			want: `{"oneOf":[` + pinnedCat + `,` + pinnedDog + `,{"type":"null"},{"const":null},{"enum":[null]}],` + defs + `}`,
		},
		{
			name:   "The list stays oneOf next to an anyOf of the schema",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat}, nil}, AnyOf: []*spec.Schema{str}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"oneOf":[` + pinnedCat + `,{}],"anyOf":[{"type":"string"}],"$defs":{"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}}}}`,
		},
		{
			name:   "A nullable union pins its variants inside anyOf with null",
			schema: &spec.Schema{Nullable: true, OneOf: []*spec.Schema{{Ref: cat}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"anyOf":[{"oneOf":[` + pinnedCat + `,` + pinnedDog + `]},{"type":"null"}],` + defs + `}`,
		},
		{
			name:   "A nullable variant is pinned inside anyOf with null",
			schema: &spec.Schema{OneOf: []*spec.Schema{{Ref: cat, Nullable: true}, {Ref: dog}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"oneOf":[{"anyOf":[` + pinnedCat + `,{"type":"null"}]},` + pinnedDog + `],` + defs + `}`,
		},
		{
			name:   "A discriminator without oneOf changes nothing",
			schema: &spec.Schema{AnyOf: []*spec.Schema{{Ref: cat}}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: mapping}},
			want:   `{"anyOf":[{"$ref":"#/$defs/Cat"}],"$defs":{"Cat":{"type":"object","properties":{"kind":{"type":"string"},"indoor":{"type":"string"}}}}}`,
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

func TestSchemaKeepsTheVariantsItPins(t *testing.T) {
	t.Parallel()

	ref := &spec.Ref{Name: "Cat", Target: &spec.Schema{}}
	required := append(make([]string, 0, 2), "name")
	props := append(make([]*spec.Property, 0, 2), &spec.Property{Name: "name", Schema: &spec.Schema{Types: spec.TypeString}})
	variant := &spec.Schema{Ref: ref, Required: required, Properties: props}
	union := &spec.Schema{OneOf: []*spec.Schema{variant}, Discriminator: &spec.Discriminator{Property: "kind", Mapping: []spec.Mapping{{Value: "cat", Ref: ref}}}}
	b := NewBuilder()

	got := b.Document(b.Schema(union))

	want := `{"oneOf":[{"$ref":"#/$defs/Cat","properties":{"name":{"type":"string"},"kind":{"const":"cat"}},"required":["name","kind"]}],"$defs":{"Cat":{}}}`
	assert.Equal(t, want, string(got))
	assert.Equal(t, []string{"name", ""}, required[:2], "the spec keeps its required list")
	assert.Equal(t, []*spec.Property{props[0], nil}, props[:2], "the spec keeps its properties")
}
