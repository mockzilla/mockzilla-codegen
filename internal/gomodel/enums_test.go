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

func strVal(s string) spec.Value           { return spec.Value{Kind: spec.KindString, Str: s} }
func numVal(n string) spec.Value           { return spec.Value{Kind: spec.KindNumber, Num: json.Number(n)} }
func boolVal(b bool) spec.Value            { return spec.Value{Kind: spec.KindBool, Bool: b} }
func nullVal() spec.Value                  { return spec.Value{Kind: spec.KindNull} }
func arrayVal(vs ...spec.Value) spec.Value { return spec.Value{Kind: spec.KindArray, Items: vs} }

func TestEnumsInModel(t *testing.T) {
	t.Parallel()

	noPrefix := testOptions()
	noPrefix.EnumPrefix = false
	tests := []struct {
		name string
		opts Options
	}{
		{name: "enums", opts: testOptions()},
		{name: "enums-no-prefix", opts: noPrefix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGolden(t, "enums", tt.name, tt.opts)
		})
	}
}

func TestEnumKindOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema *spec.Schema
		want   enumKind
	}{
		{name: "String type", schema: &spec.Schema{Types: spec.TypeString}, want: enumString},
		{name: "Integer type", schema: &spec.Schema{Types: spec.TypeInteger}, want: enumInteger},
		{name: "Number type", schema: &spec.Schema{Types: spec.TypeNumber}, want: enumNumber},
		{name: "Boolean type", schema: &spec.Schema{Types: spec.TypeBoolean}, want: enumBool},
		{name: "Null alone", schema: &spec.Schema{Types: spec.TypeNull, Enum: []spec.Value{nullVal()}}, want: enumNone},
		{name: "Object type", schema: &spec.Schema{Types: spec.TypeObject}, want: enumNone},
		{name: "No type, strings", schema: &spec.Schema{Enum: []spec.Value{strVal("a"), nullVal()}}, want: enumString},
		{name: "No type, integers", schema: &spec.Schema{Enum: []spec.Value{numVal("1"), numVal("2")}}, want: enumInteger},
		{name: "No type, integer then number", schema: &spec.Schema{Enum: []spec.Value{numVal("1"), numVal("2.5")}}, want: enumNumber},
		{name: "No type, number then integer", schema: &spec.Schema{Enum: []spec.Value{numVal("2.5"), numVal("1")}}, want: enumNumber},
		{name: "No type, booleans", schema: &spec.Schema{Enum: []spec.Value{boolVal(true), boolVal(false)}}, want: enumBool},
		{name: "No type, string and number", schema: &spec.Schema{Enum: []spec.Value{strVal("a"), numVal("1")}}, want: enumNone},
		{name: "No type, array value", schema: &spec.Schema{Enum: []spec.Value{arrayVal(strVal("a"))}}, want: enumNone},
		{name: "No type, no values", schema: &spec.Schema{}, want: enumNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, enumKindOf(tt.schema))
		})
	}
}

func TestEnumValues(t *testing.T) {
	t.Parallel()

	values := []spec.Value{strVal("a"), numVal("1"), numVal("1.5"), boolVal(true), nullVal(), strVal("a"), arrayVal()}
	tests := []struct {
		name    string
		kind    enumKind
		want    []spec.Value
		misfits []spec.Value
	}{
		{name: "String", kind: enumString, want: []spec.Value{strVal("a"), strVal("1"), strVal("1.5"), strVal("true")}, misfits: []spec.Value{arrayVal()}},
		{name: "Integer", kind: enumInteger, want: []spec.Value{numVal("1")}, misfits: []spec.Value{strVal("a"), numVal("1.5"), boolVal(true), strVal("a"), arrayVal()}},
		{name: "Number", kind: enumNumber, want: []spec.Value{numVal("1"), numVal("1.5")}, misfits: []spec.Value{strVal("a"), boolVal(true), strVal("a"), arrayVal()}},
		{name: "Boolean", kind: enumBool, want: []spec.Value{boolVal(true)}, misfits: []spec.Value{strVal("a"), numVal("1"), numVal("1.5"), strVal("a"), arrayVal()}},
		{name: "None", kind: enumNone, misfits: []spec.Value{strVal("a"), numVal("1"), numVal("1.5"), boolVal(true), strVal("a"), arrayVal()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, enumValues(tt.kind, values))
			assert.Equal(t, tt.misfits, misfits(tt.kind, values))
		})
	}
}

func TestEnumBase(t *testing.T) {
	t.Parallel()

	assert.Equal(t, stringType, enumBase(enumString, "date-time", "int"))
	assert.Equal(t, Builtin{Name: "int64"}, enumBase(enumInteger, "", "int64"))
	assert.Equal(t, Builtin{Name: "float32"}, enumBase(enumNumber, "float", "int"))
	assert.Equal(t, boolType, enumBase(enumBool, "", "int"))
}

func TestIsIntegral(t *testing.T) {
	t.Parallel()

	for n, want := range map[string]bool{"1": true, "-3": true, "1.0": true, "1e3": true, "1.5": false, "x": false} {
		assert.Equal(t, want, isIntegral(json.Number(n)), n)
	}
}
