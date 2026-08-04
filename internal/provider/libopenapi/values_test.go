// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want spec.Value
	}{
		{name: "Null", src: "v: null", want: spec.Value{Kind: spec.KindNull}},
		{name: "Bool", src: "v: True", want: spec.Value{Kind: spec.KindBool, Bool: true}},
		{name: "Integer", src: "v: 42", want: spec.Value{Kind: spec.KindNumber, Num: "42"}},
		{name: "Hex integer", src: "v: 0x1F", want: spec.Value{Kind: spec.KindNumber, Num: "31"}},
		{name: "Float keeps its form", src: "v: 1.50", want: spec.Value{Kind: spec.KindNumber, Num: "1.50"}},
		{name: "Infinity stays a string", src: "v: .inf", want: spec.Value{Kind: spec.KindString, Str: ".inf"}},
		{name: "Quoted number is a string", src: "v: '42'", want: spec.Value{Kind: spec.KindString, Str: "42"}},
		{name: "Timestamp is a string", src: "v: 2024-01-02", want: spec.Value{Kind: spec.KindString, Str: "2024-01-02"}},
		{name: "Alias is followed", src: "a: &x hi\nv: *x", want: spec.Value{Kind: spec.KindString, Str: "hi"}},
		{
			name: "Nested object and array keep order",
			src:  "v: {z: [1, a], a: {}}",
			want: spec.Value{Kind: spec.KindObject, Fields: []spec.Field{
				{Name: "z", Value: spec.Value{Kind: spec.KindArray, Items: []spec.Value{
					{Kind: spec.KindNumber, Num: "1"},
					{Kind: spec.KindString, Str: "a"},
				}}},
				{Name: "a", Value: spec.Value{Kind: spec.KindObject, Fields: []spec.Field{}}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var doc yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.src), &doc))
			root := doc.Content[0]
			assert.Equal(t, tt.want, value(root.Content[len(root.Content)-1]))
		})
	}
}

func TestOptionalValue(t *testing.T) {
	t.Parallel()

	assert.Nil(t, optionalValue(nil))
	assert.Equal(t, &spec.Value{Kind: spec.KindString, Str: "a"}, optionalValue(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "a"}))
}

func TestIntNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   json.Number
		wantOK bool
	}{
		{name: "Plain", in: "-12", want: "-12", wantOK: true},
		{name: "Sign", in: "+12", want: "12", wantOK: true},
		{name: "Underscores", in: "1_000", want: "1000", wantOK: true},
		{name: "Octal", in: "0o17", want: "15", wantOK: true},
		{name: "Hex beyond int64", in: "0xFFFFFFFFFFFFFFFF", want: "18446744073709551615", wantOK: true},
		{name: "Not a number", in: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := intNumber(tt.in)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFloatNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   json.Number
		wantOK bool
	}{
		{name: "Plain", in: "1e3", want: "1e3", wantOK: true},
		{name: "Sign", in: "+1.5", want: "1.5", wantOK: true},
		{name: "Trailing dot", in: "1.", want: "1", wantOK: true},
		{name: "Underscores", in: "1_000.5", want: "1000.5", wantOK: true},
		{name: "Infinity", in: "-.inf"},
		{name: "Not a number", in: ".nan"},
		{name: "Garbage", in: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := floatNumber(tt.in)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
