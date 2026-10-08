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

func TestRequiredCountsGolden(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.IsValidated = true
	checkGolden(t, "required", "required", opts)
}

func TestRequiredCount(t *testing.T) {
	t.Parallel()

	str := Builtin{Name: "string"}
	name := &Field{Name: "Name", JSONName: "name", Type: str, Required: true}
	email := &Field{Name: "Email", JSONName: "email", Type: Pointer{Elem: str}}
	phone := &Field{Name: "Phone", JSONName: "phone", Type: Nullable{Elem: str}}
	tags := &Field{Name: "Tags", JSONName: "tags", Type: Slice{Elem: str}}
	nick := &Field{Name: "Nick", JSONName: "nick", Type: str}
	d := &Decl{Name: "Contact", Struct: &Struct{Fields: []*Field{name, email, phone, tags, nick}}}
	lists := func(names ...[]string) []*spec.Schema {
		out := make([]*spec.Schema, len(names))
		for i, l := range names {
			out[i] = &spec.Schema{Required: l}
		}
		return out
	}

	tests := []struct {
		name    string
		schema  *spec.Schema
		want    Count
		wantWhy string
	}{
		{
			name:   "oneOf counts each member's fields",
			schema: &spec.Schema{OneOf: lists([]string{"email"}, []string{"phone", "tags"})},
			want:   Count{Func: "ExactlyOneOf", Fields: [][]*Field{{email}, {phone, tags}}, Names: "email or (phone and tags)"},
		},
		{
			name:   "anyOf with a required field that is always set",
			schema: &spec.Schema{AnyOf: lists([]string{"name"}, []string{"email"})},
			want:   Count{Func: "AtLeastOneOf", Fields: [][]*Field{nil, {email}}, Names: "name or email"},
		},
		{
			name:    "A property the struct lacks",
			schema:  &spec.Schema{OneOf: lists([]string{"email"}, []string{"fax"})},
			wantWhy: `it has no property "fax"`,
		},
		{
			name:   "An optional field without a pointer counts by its zero value",
			schema: &spec.Schema{OneOf: lists([]string{"email"}, []string{"nick"})},
			want:   Count{Func: "ExactlyOneOf", Fields: [][]*Field{{email}, {nick}}, Names: "email or nick"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, why := requiredCount(d, tc.schema)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantWhy, why)
		})
	}
}
