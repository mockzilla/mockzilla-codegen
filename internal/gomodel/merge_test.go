// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestAllOfInModel(t *testing.T) {
	t.Parallel()
	checkGolden(t, "allof", "allof", testOptions())
}

func TestTypeSetText(t *testing.T) {
	t.Parallel()

	assert.Empty(t, typeSetText(0))
	assert.Equal(t, "string", typeSetText(spec.TypeString))
	assert.Equal(t, "number or integer or null", typeSetText(spec.TypeNumber|spec.TypeInteger|spec.TypeNull))
}

func TestTighter(t *testing.T) {
	t.Parallel()

	bound := func(v string, isExclusive bool) *spec.Bound {
		return &spec.Bound{Value: json.Number(v), Exclusive: isExclusive}
	}
	tests := []struct {
		name string
		a, b *spec.Bound
		sign int
		want *spec.Bound
	}{
		{name: "One unset", b: bound("1", false), sign: 1, want: bound("1", false)},
		{name: "The larger minimum", a: bound("1.5", false), b: bound("2e0", false), sign: 1, want: bound("2e0", false)},
		{name: "The smaller maximum", a: bound("1.5", false), b: bound("2", false), sign: -1, want: bound("1.5", false)},
		{name: "The exclusive one on a tie", a: bound("2", false), b: bound("2.0", true), sign: 1, want: bound("2.0", true)},
		{name: "The first that is no number", a: bound("x", false), b: bound("2", false), sign: 1, want: bound("x", false)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tighter(tc.a, tc.b, tc.sign))
		})
	}
}

func TestSameValue(t *testing.T) {
	t.Parallel()

	num := func(n string) spec.Value { return spec.Value{Kind: spec.KindNumber, Num: json.Number(n)} }
	str := func(s string) spec.Value { return spec.Value{Kind: spec.KindString, Str: s} }
	obj := func(fields ...spec.Field) spec.Value { return spec.Value{Kind: spec.KindObject, Fields: fields} }
	list := func(items ...spec.Value) spec.Value { return spec.Value{Kind: spec.KindArray, Items: items} }
	tests := []struct {
		name string
		a, b spec.Value
		want bool
	}{
		{name: "Numbers by value", a: num("2"), b: num("2.0"), want: true},
		{name: "Other numbers", a: num("2"), b: num("3")},
		{name: "Number and its text", a: num("2"), b: str("2")},
		{name: "Strings", a: str("a"), b: str("a"), want: true},
		{name: "Nulls", want: true},
		{name: "Lists in order", a: list(num("1"), str("a")), b: list(num("1.0"), str("a")), want: true},
		{name: "Lists out of order", a: list(num("1"), str("a")), b: list(str("a"), num("1"))},
		{name: "Objects in any order", a: obj(spec.Field{Name: "a", Value: num("1")}, spec.Field{Name: "b", Value: str("x")}), b: obj(spec.Field{Name: "b", Value: str("x")}, spec.Field{Name: "a", Value: num("1")}), want: true},
		{name: "Object with another value", a: obj(spec.Field{Name: "a", Value: num("1")}), b: obj(spec.Field{Name: "a", Value: num("2")})},
		{name: "Object with more fields", a: obj(spec.Field{Name: "a", Value: num("1")}), b: obj(spec.Field{Name: "a", Value: num("1")}, spec.Field{Name: "b", Value: num("1")})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, sameValue(tc.a, tc.b))
		})
	}
}
