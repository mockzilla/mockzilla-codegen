// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFitsFloat64(t *testing.T) {
	t.Parallel()

	wide := Builtin{Name: "int64"}
	small := Builtin{Name: "int32"}
	node := &Decl{Name: "Node", Kind: KindStruct, Struct: &Struct{}}
	node.Struct.Fields = []*Field{{Name: "Next", Type: Pointer{Elem: DeclRef{Decl: node}}}, {Name: "Name", Type: stringType}}
	structWith := func(fields ...Type) Type {
		s := &Struct{}
		for _, f := range fields {
			s.Fields = append(s.Fields, &Field{Type: f})
		}
		return DeclRef{Decl: &Decl{Kind: KindStruct, Struct: s}}
	}
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{name: "A string fits", t: stringType, want: true},
		{name: "A Nullable fits when its value does", t: Nullable{Elem: small}, want: true},
		{name: "A 32-bit integer fits", t: small, want: true},
		{name: "A float64 fits", t: Builtin{Name: "float64"}, want: true},
		{name: "Any fits, it decodes into a float64 anyway", t: anyType, want: true},
		{name: "An int does not fit", t: Builtin{Name: "int"}},
		{name: "An int64 does not fit", t: wide},
		{name: "A uint64 does not fit", t: Builtin{Name: "uint64"}},
		{name: "A type x-go-type writes out does not fit", t: Builtin{Name: "decimal.Big"}},
		{name: "A time fits", t: Qualified{Import: importTime, Name: "Time"}, want: true},
		{name: "Raw JSON does not fit", t: rawJSON},
		{name: "A type of another package does not fit", t: Qualified{Import: Import{Path: "example.com/ids"}, Name: "ID"}},
		{name: "A pointer to an int64 does not fit", t: Pointer{Elem: wide}},
		{name: "A list of int64 does not fit", t: Slice{Elem: wide}},
		{name: "Bytes fit", t: Slice{Elem: byteType}, want: true},
		{name: "A map of int64 does not fit", t: Map{Key: stringType, Elem: wide}},
		{name: "A map keyed by int64 fits, JSON keys are strings", t: Map{Key: wide, Elem: stringType}, want: true},
		{name: "A struct of small fields fits", t: structWith(stringType, small), want: true},
		{name: "A struct with an int64 field does not fit", t: structWith(stringType, wide)},
		{name: "A struct that holds itself fits", t: DeclRef{Decl: node}, want: true},
		{
			name: "A struct whose extra properties are int64 does not fit",
			t:    DeclRef{Decl: &Decl{Kind: KindStruct, Struct: &Struct{AdditionalProperties: &Field{Type: wide}}}},
		},
		{
			name: "A union with an int64 variant does not fit",
			t:    DeclRef{Decl: &Decl{Kind: KindUnion, Union: &Union{Variants: []*Variant{{Type: stringType}, {Type: wide}}}}},
		},
		{
			name: "A union of small variants fits",
			t:    DeclRef{Decl: &Decl{Kind: KindUnion, Union: &Union{Variants: []*Variant{{Type: stringType}, {Type: small}}}}},
			want: true,
		},
		{name: "An enum of int64 does not fit", t: DeclRef{Decl: &Decl{Kind: KindEnum, Enum: &Enum{Base: wide}}}},
		{name: "An alias of an int64 does not fit", t: DeclRef{Decl: &Decl{Kind: KindAlias, Target: wide}}},
		{name: "A defined string fits", t: DeclRef{Decl: &Decl{Kind: KindDefined, Target: stringType}}, want: true},
		{name: "No type does not fit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FitsFloat64(tt.t))
		})
	}
}
