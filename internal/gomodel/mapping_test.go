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

func TestPrimitive(t *testing.T) {
	t.Parallel()

	three, half, yes, list := numVal("3"), numVal("0.5"), boolVal(true), arrayVal()
	tests := []struct {
		name   string
		schema spec.Schema
		want   Type
	}{
		{name: "Integer without a format", schema: spec.Schema{Types: spec.TypeInteger}, want: Builtin{Name: "int64"}},
		{name: "Integer int8", schema: spec.Schema{Types: spec.TypeInteger, Format: "int8"}, want: Builtin{Name: "int8"}},
		{name: "Integer uint64", schema: spec.Schema{Types: spec.TypeInteger, Format: "uint64"}, want: Builtin{Name: "uint64"}},
		{name: "Integer with an unknown format", schema: spec.Schema{Types: spec.TypeInteger, Format: "big"}, want: Builtin{Name: "int64"}},
		{name: "Nullable integer", schema: spec.Schema{Types: spec.TypeInteger | spec.TypeNull, Format: "INT32"}, want: Builtin{Name: "int32"}},
		{name: "Number without a format", schema: spec.Schema{Types: spec.TypeNumber}, want: Builtin{Name: "float64"}},
		{name: "Number double", schema: spec.Schema{Types: spec.TypeNumber, Format: "double"}, want: Builtin{Name: "float64"}},
		{name: "Number float", schema: spec.Schema{Types: spec.TypeNumber, Format: "float"}, want: Builtin{Name: "float32"}},
		{name: "Number int32", schema: spec.Schema{Types: spec.TypeNumber, Format: "int32"}, want: Builtin{Name: "int32"}},
		{name: "Number with an integer format", schema: spec.Schema{Types: spec.TypeNumber, Format: "integer"}, want: Builtin{Name: "int64"}},
		{name: "Number with an unknown format", schema: spec.Schema{Types: spec.TypeNumber, Format: "decimal"}, want: Builtin{Name: "float64"}},
		{name: "Boolean", schema: spec.Schema{Types: spec.TypeBoolean, Format: "flag"}, want: boolType},
		{name: "String", schema: spec.Schema{Types: spec.TypeString}, want: stringType},
		{name: "String date", schema: spec.Schema{Types: spec.TypeString, Format: "date"}, want: Qualified{Import: importRuntime, Name: "Date"}},
		{name: "String date-time", schema: spec.Schema{Types: spec.TypeString, Format: "date-time"}, want: Qualified{Import: importTime, Name: "Time"}},
		{name: "String email", schema: spec.Schema{Types: spec.TypeString, Format: "email"}, want: Qualified{Import: importRuntime, Name: "Email"}},
		{name: "String uuid", schema: spec.Schema{Types: spec.TypeString, Format: "uuid"}, want: stringType},
		{name: "String byte", schema: spec.Schema{Types: spec.TypeString, Format: "byte"}, want: Slice{Elem: Builtin{Name: "byte"}}},
		{name: "String binary", schema: spec.Schema{Types: spec.TypeString, Format: "binary"}, want: Qualified{Import: importRuntime, Name: "File"}},
		{name: "String json", schema: spec.Schema{Types: spec.TypeString, Format: "json"}, want: rawJSON},
		{name: "No type, binary format", schema: spec.Schema{Format: "binary"}, want: Qualified{Import: importRuntime, Name: "File"}},
		{name: "No type, integer format", schema: spec.Schema{Format: "int16"}, want: Builtin{Name: "int16"}},
		{name: "No type, number format", schema: spec.Schema{Format: "float"}, want: Builtin{Name: "float32"}},
		{name: "No type, string const", schema: spec.Schema{Const: new(strVal("a"))}, want: stringType},
		{name: "No type, integer const", schema: spec.Schema{Const: &three}, want: Builtin{Name: "int64"}},
		{name: "No type, number const", schema: spec.Schema{Const: &half}, want: Builtin{Name: "float64"}},
		{name: "No type, boolean const", schema: spec.Schema{Const: &yes}, want: boolType},
		{name: "No type, array const", schema: spec.Schema{Const: &list}, want: anyType},
		{name: "No type, unknown format", schema: spec.Schema{Format: "color"}, want: anyType},
		{name: "No type at all", want: anyType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, primitive(&tt.schema, "int64"))
		})
	}
}
