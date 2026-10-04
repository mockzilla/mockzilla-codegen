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

var nullValue = spec.Value{Kind: spec.KindNull}

func stringValue(s string) spec.Value {
	return spec.Value{Kind: spec.KindString, Str: s}
}

func numberValue(n string) spec.Value {
	return spec.Value{Kind: spec.KindNumber, Num: num(n)}
}

func boolValue(b bool) spec.Value {
	return spec.Value{Kind: spec.KindBool, Bool: b}
}

func arrayValue(items ...spec.Value) spec.Value {
	return spec.Value{Kind: spec.KindArray, Items: items}
}

func objectValue(fields ...spec.Field) spec.Value {
	return spec.Value{Kind: spec.KindObject, Fields: fields}
}

func typed(t spec.TypeSet) *spec.Schema {
	return &spec.Schema{Types: t}
}

func TestMisfit(t *testing.T) {
	t.Parallel()

	str, integer := typed(spec.TypeString), typed(spec.TypeInteger)
	cycle := &spec.Schema{}
	cycle.AllOf = []*spec.Schema{{Ref: &spec.Ref{Name: "Cycle", Target: cycle}}}
	props := func(add spec.Additional, ps ...*spec.Property) *spec.Schema {
		return &spec.Schema{Properties: ps, AdditionalProperties: add}
	}
	tests := []struct {
		name   string
		value  spec.Value
		schema *spec.Schema
		want   string
	}{
		{name: "No schema takes anything", value: stringValue("x")},
		{name: "A number too large for a float64", value: objectValue(spec.Field{Name: "a", Value: arrayValue(numberValue("1e999"))}), want: "it holds a number too large for a float64"},
		{name: "A small number is read as zero", value: numberValue("1e-999"), schema: integer},

		{name: "Type", value: stringValue("20"), schema: integer, want: "it is a string, the schema wants integer"},
		{name: "An integer is a number", value: numberValue("20"), schema: typed(spec.TypeNumber)},
		{name: "A number without a fraction is an integer", value: numberValue("2.0"), schema: integer},
		{name: "A number with a fraction is no integer", value: numberValue("2.5"), schema: integer, want: "it is a number, the schema wants integer"},
		{name: "Null", value: nullValue, schema: str, want: "it is null, the schema wants string"},
		{name: "Null of a nullable schema", value: nullValue, schema: &spec.Schema{Types: spec.TypeString, Nullable: true}},
		{name: "Null of a nullable schema without types", value: nullValue, schema: &spec.Schema{Nullable: true, Enum: []spec.Value{stringValue("a")}}},
		{name: "Null of a nullable $ref", value: nullValue, schema: &spec.Schema{Nullable: true, Ref: &spec.Ref{Name: "S", Target: str}}},
		{name: "Another value of a nullable schema without types", value: stringValue("b"), schema: &spec.Schema{Nullable: true, Enum: []spec.Value{stringValue("a")}}, want: "it is none of the enum values"},
		{name: "Several types", value: boolValue(true), schema: typed(spec.TypeObject | spec.TypeArray), want: "it is a boolean, the schema wants object or array"},
		{name: "An object", value: objectValue(), schema: typed(spec.TypeArray), want: "it is an object, the schema wants array"},
		{name: "An array", value: arrayValue(), schema: typed(spec.TypeObject), want: "it is an array, the schema wants object"},

		{name: "Enum", value: stringValue("c"), schema: &spec.Schema{Enum: []spec.Value{stringValue("a"), stringValue("b")}}, want: "it is none of the enum values"},
		{name: "Enum compares numbers by value", value: numberValue("1.0"), schema: &spec.Schema{Enum: []spec.Value{numberValue("1")}}},
		{name: "Const", value: boolValue(false), schema: &spec.Schema{Const: new(boolValue(true))}, want: "it is not the const value"},

		{name: "Multiple of", value: numberValue("3"), schema: &spec.Schema{Limits: spec.Limits{MultipleOf: new(num("2"))}}, want: "it is no multiple of 2"},
		{name: "A multiple", value: numberValue("0.75"), schema: &spec.Schema{Limits: spec.Limits{MultipleOf: new(num("0.25"))}}},
		{name: "Minimum", value: numberValue("0"), schema: &spec.Schema{Limits: spec.Limits{Minimum: &spec.Bound{Value: num("1")}}}, want: "it is below the minimum 1"},
		{name: "On the minimum", value: numberValue("1"), schema: &spec.Schema{Limits: spec.Limits{Minimum: &spec.Bound{Value: num("1")}}}},
		{name: "Exclusive minimum", value: numberValue("1"), schema: &spec.Schema{Limits: spec.Limits{Minimum: &spec.Bound{Value: num("1"), Exclusive: true}}}, want: "it is not above the exclusive minimum 1"},
		{name: "Maximum", value: numberValue("11"), schema: &spec.Schema{Limits: spec.Limits{Maximum: &spec.Bound{Value: num("10")}}}, want: "it is above the maximum 10"},
		{name: "Exclusive maximum", value: numberValue("10"), schema: &spec.Schema{Limits: spec.Limits{Maximum: &spec.Bound{Value: num("10"), Exclusive: true}}}, want: "it is not below the exclusive maximum 10"},
		{name: "Below an exclusive maximum", value: numberValue("9.5"), schema: &spec.Schema{Limits: spec.Limits{Maximum: &spec.Bound{Value: num("10"), Exclusive: true}}}},
		{name: "Bounds apply to numbers only", value: stringValue("0"), schema: &spec.Schema{Limits: spec.Limits{Minimum: &spec.Bound{Value: num("1")}}}},

		{name: "Min length counts code points", value: stringValue("é"), schema: &spec.Schema{Limits: spec.Limits{MinLength: new(int64(2))}}, want: "it is shorter than 2 characters"},
		{name: "Max length counts code points", value: stringValue("éé"), schema: &spec.Schema{Limits: spec.Limits{MaxLength: new(int64(2))}}},
		{name: "Max length", value: stringValue("abc"), schema: &spec.Schema{Limits: spec.Limits{MaxLength: new(int64(2))}}, want: "it is longer than 2 characters"},
		{name: "Length before pattern", value: stringValue("abc"), schema: &spec.Schema{Pattern: "^x$", Limits: spec.Limits{MaxLength: new(int64(2))}}, want: "it is longer than 2 characters"},
		{name: "Pattern", value: stringValue("red"), schema: &spec.Schema{Pattern: "^[0-9a-f]{6}$"}, want: `it does not match the pattern "^[0-9a-f]{6}$"`},
		{name: "A pattern matches anywhere", value: stringValue("a-1"), schema: &spec.Schema{Pattern: `\d`}},
		{name: "A pattern with a \\u escape", value: stringValue("a b"), schema: &spec.Schema{Pattern: `^[\u0021-\u007E]+$`}, want: `it does not match the pattern "^[\\u0021-\\u007E]+$"`},
		{name: "A pattern that is not RE2 checks nothing", value: stringValue("root"), schema: &spec.Schema{Pattern: "^(?!root$).+$"}},
		{name: "A pattern applies to strings only", value: numberValue("1"), schema: &spec.Schema{Pattern: "^x$"}},

		{name: "A $ref", value: stringValue("x"), schema: &spec.Schema{Ref: &spec.Ref{Name: "N", Target: integer}}, want: "it is a string, the schema wants integer"},
		{name: "Keywords next to a $ref", value: stringValue("x"), schema: &spec.Schema{Ref: &spec.Ref{Name: "S", Target: str}, Enum: []spec.Value{stringValue("y")}}, want: "it is none of the enum values"},
		{name: "A $ref cycle ends", value: stringValue("x"), schema: cycle},

		{name: "All of", value: numberValue("5"), schema: &spec.Schema{AllOf: []*spec.Schema{integer, str}}, want: "it is an integer, the schema wants string"},
		{name: "Any of none", value: boolValue(true), schema: &spec.Schema{AnyOf: []*spec.Schema{integer, str}}, want: "it fits none of anyOf"},
		{name: "Any of one", value: stringValue("x"), schema: &spec.Schema{AnyOf: []*spec.Schema{integer, str}}},
		{name: "One of none", value: boolValue(true), schema: &spec.Schema{OneOf: []*spec.Schema{integer, str}}, want: "it fits none of oneOf"},
		{name: "One of two", value: numberValue("1"), schema: &spec.Schema{OneOf: []*spec.Schema{integer, typed(spec.TypeNumber)}}, want: "it fits more than one of oneOf"},
		{name: "One of one", value: numberValue("1"), schema: &spec.Schema{OneOf: []*spec.Schema{integer, str}}},
		{name: "Not", value: stringValue("x"), schema: &spec.Schema{Not: str}, want: "it fits the schema of not"},
		{name: "Not another", value: numberValue("1"), schema: &spec.Schema{Not: str}},
		{name: "If then", value: stringValue("x"), schema: &spec.Schema{If: str, Then: &spec.Schema{Limits: spec.Limits{MinLength: new(int64(2))}}}, want: "it is shorter than 2 characters"},
		{name: "If else", value: numberValue("1"), schema: &spec.Schema{If: str, Else: str}, want: "it is an integer, the schema wants string"},
		{name: "If without else", value: numberValue("1"), schema: &spec.Schema{If: str, Then: integer}},

		{name: "Prefix items", value: arrayValue(numberValue("1")), schema: &spec.Schema{PrefixItems: []*spec.Schema{str}, Items: integer}, want: "/0 is an integer, the schema wants string"},
		{name: "Items after the prefix", value: arrayValue(stringValue("a"), stringValue("b")), schema: &spec.Schema{PrefixItems: []*spec.Schema{str}, Items: integer}, want: "/1 is a string, the schema wants integer"},
		{name: "Min items", value: arrayValue(), schema: &spec.Schema{Limits: spec.Limits{MinItems: new(int64(1))}}, want: "it has fewer than 1 items"},
		{name: "Max items", value: arrayValue(nullValue, nullValue), schema: &spec.Schema{Limits: spec.Limits{MaxItems: new(int64(1))}}, want: "it has more than 1 items"},
		{name: "Unique items", value: arrayValue(numberValue("1"), stringValue("1"), numberValue("1.0")), schema: &spec.Schema{Limits: spec.Limits{UniqueItems: true}}, want: "it has item 2 twice"},
		{name: "Items that differ", value: arrayValue(arrayValue(boolValue(true)), arrayValue(boolValue(false)), nullValue), schema: &spec.Schema{Limits: spec.Limits{UniqueItems: true}}},

		{name: "A property", value: objectValue(spec.Field{Name: "a/b", Value: stringValue("x")}), schema: props(spec.Additional{}, &spec.Property{Name: "a/b", Schema: integer}), want: "/a~1b is a string, the schema wants integer"},
		{name: "A nested property", value: objectValue(spec.Field{Name: "a", Value: arrayValue(stringValue("x"))}), schema: props(spec.Additional{}, &spec.Property{Name: "a", Schema: &spec.Schema{Items: integer}}), want: "/a/0 is a string, the schema wants integer"},
		{name: "Another property", value: objectValue(spec.Field{Name: "b", Value: stringValue("x")}), schema: props(spec.Additional{}, &spec.Property{Name: "a", Schema: integer})},
		{name: "No other property", value: objectValue(spec.Field{Name: "b", Value: stringValue("x")}), schema: props(spec.Additional{Mode: spec.AdditionalDenied}), want: `it has the property "b", which the schema does not allow`},
		{name: "Other properties of a schema", value: objectValue(spec.Field{Name: "b", Value: stringValue("x")}), schema: props(spec.Additional{Mode: spec.AdditionalSchema, Schema: integer}), want: "/b is a string, the schema wants integer"},
		{name: "Other properties allowed", value: objectValue(spec.Field{Name: "b", Value: stringValue("x")}), schema: props(spec.Additional{Mode: spec.AdditionalAllowed, Schema: integer})},
		{name: "Min properties", value: objectValue(), schema: &spec.Schema{Limits: spec.Limits{MinProperties: new(int64(1))}}, want: "it has fewer than 1 properties"},
		{name: "Max properties", value: objectValue(spec.Field{Name: "a", Value: nullValue}, spec.Field{Name: "b", Value: nullValue}), schema: &spec.Schema{Limits: spec.Limits{MaxProperties: new(int64(1))}}, want: "it has more than 1 properties"},
		{name: "Required", value: objectValue(spec.Field{Name: "a", Value: nullValue}), schema: &spec.Schema{Required: []string{"a", "b"}}, want: `it lacks the required property "b"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, misfit(tc.value, tc.schema))
		})
	}
}

func TestIsEqual(t *testing.T) {
	t.Parallel()

	ab := objectValue(spec.Field{Name: "a", Value: numberValue("1")}, spec.Field{Name: "b", Value: nullValue})
	tests := []struct {
		name string
		a, b spec.Value
		want bool
	}{
		{name: "Kinds differ", a: stringValue("1"), b: numberValue("1")},
		{name: "Strings", a: stringValue("a"), b: stringValue("a"), want: true},
		{name: "Numbers by value", a: numberValue("100"), b: numberValue("1e2"), want: true},
		{name: "Booleans", a: boolValue(true), b: boolValue(false)},
		{name: "Nulls", a: nullValue, b: nullValue, want: true},
		{name: "Arrays", a: arrayValue(stringValue("a")), b: arrayValue(stringValue("a"), stringValue("b"))},
		{name: "Objects in any order", a: ab, b: objectValue(spec.Field{Name: "b", Value: nullValue}, spec.Field{Name: "a", Value: numberValue("1.0")}), want: true},
		{name: "Objects of other sizes", a: ab, b: objectValue(spec.Field{Name: "a", Value: numberValue("1")})},
		{name: "Objects with other names", a: ab, b: objectValue(spec.Field{Name: "a", Value: numberValue("1")}, spec.Field{Name: "c", Value: nullValue})},
		{name: "Objects with other values", a: ab, b: objectValue(spec.Field{Name: "a", Value: numberValue("2")}, spec.Field{Name: "b", Value: nullValue})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isEqual(tc.a, tc.b))
		})
	}
}
