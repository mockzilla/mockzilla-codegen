// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
)

func TestMarshalUnion(t *testing.T) {
	t.Parallel()

	shared := struct {
		ID int `json:"id"`
	}{ID: 1}

	tests := []struct {
		name    string
		shared  any
		set     []any
		want    string
		wantErr bool
	}{
		{name: "Nothing set is null", want: `null`},
		{name: "Shared alone", shared: shared, want: `{"id":1}`},
		{name: "One variant", set: []any{&cat{Name: "a"}}, want: `{"name":"a","meow":false}`},
		{name: "Shared and a variant merge", shared: shared, set: []any{&dog{Name: "a"}}, want: `{"id":1,"name":"a","bark":false}`},
		{name: "Several objects merge", set: []any{&cat{Name: "a"}, &dog{Name: "b"}}, want: `{"name":"b","meow":false,"bark":false}`},
		{name: "A part that is no object gives the first variant", shared: shared, set: []any{new("x"), &cat{}}, want: `"x"`},
		{name: "Variant that fails to marshal", set: []any{func() {}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalUnion(tc.shared, tc.set...)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestMarshalVariants(t *testing.T) {
	t.Parallel()

	type withType struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	pets := Union{Discriminator: "type", Variants: []Variant{
		{Name: "Cat", Values: []string{"cat"}},
		{Name: "Dog", Values: []string{"dog"}},
	}}
	kitties := Union{Discriminator: "type", Variants: []Variant{
		{Name: "Cat", Values: []string{"cat", "kitty"}},
		{Name: "Dog", Values: []string{"dog"}, IsDefault: true},
	}}
	anyPets := pets
	anyPets.IsAnyOf = true
	open := Union{Discriminator: "type", Variants: []Variant{
		{Name: "Cat", Values: []string{"cat"}},
		{Name: "Other"},
	}}
	twice := Union{Variants: []Variant{{Name: "Cat"}}, Also: []Union{{Variants: []Variant{{Name: "Dog"}, {Name: "Fox"}}}}}
	anyTwice := Union{Variants: []Variant{{Name: "Cat"}}, Also: []Union{{IsAnyOf: true, Variants: []Variant{{Name: "Dog"}, {Name: "Fox"}}}}}
	withPets := Union{Variants: []Variant{{Name: "Text"}}, Also: []Union{pets}}

	tests := []struct {
		name     string
		shared   any
		u        Union
		variants []any
		want     string
		wantErr  string
	}{
		{name: "Nothing set is null", u: pets, variants: []any{(*withType)(nil), (*withType)(nil)}, want: `null`},
		{name: "A missing value is filled", u: pets, variants: []any{&cat{Name: "a"}, (*dog)(nil)}, want: `{"name":"a","meow":false,"type":"cat"}`},
		{name: "An empty value is filled in place", u: pets, variants: []any{&withType{Name: "a"}, nil}, want: `{"type":"cat","name":"a"}`},
		{name: "A value that picks the variant stays", u: pets, variants: []any{nil, &withType{Type: "dog"}}, want: `{"type":"dog","name":""}`},
		{name: "The shared properties carry the value", shared: withType{Type: "dog"}, u: pets, variants: []any{nil, &dog{}}, want: `{"type":"dog","name":"","bark":false}`},
		{name: "A value of another variant", u: pets, variants: []any{&withType{Type: "dog"}, nil}, wantErr: `type: "dog" picks Dog, not Cat`},
		{name: "An unknown value", u: pets, variants: []any{&withType{Type: "nope"}, nil}, wantErr: `type: "nope" picks no variant`},
		{name: "Two values are not filled", u: kitties, variants: []any{&cat{}, nil}, wantErr: `type: an empty value picks Dog, not Cat`},
		{name: "One of two values stays", u: kitties, variants: []any{&withType{Type: "kitty"}, nil}, want: `{"type":"kitty","name":""}`},
		{name: "An empty value needs a default", u: Union{Discriminator: "type", Variants: kitties.Variants[:1]}, variants: []any{&cat{}}, wantErr: `type: must be set, Cat takes cat or kitty`},
		{name: "The default takes an empty value", u: kitties, variants: []any{nil, &withType{}}, want: `{"type":"dog","name":""}`},
		{name: "A variant without values takes any value", u: open, variants: []any{nil, &withType{Type: "x"}}, want: `{"type":"x","name":""}`},
		{name: "Two variants of a oneOf", u: pets, variants: []any{&cat{}, &dog{}}, wantErr: "at most one variant may be set, found 2"},
		{name: "Two variants of an anyOf and an empty value", u: anyPets, variants: []any{&cat{}, &dog{}}, wantErr: `type: must be set, Cat takes cat`},
		{name: "A value that is no object is written as it is", u: pets, variants: []any{new("x"), nil}, want: `"x"`},
		{name: "A variant that fails to marshal", u: pets, variants: []any{func() {}, nil}, wantErr: "json: unsupported type: func()"},
		{name: "A variant of each union is merged", u: twice, variants: []any{&cat{Name: "a"}, &dog{Bark: true}, nil}, want: `{"name":"","meow":false,"bark":true}`},
		{name: "Two variants of a oneOf in Also", u: twice, variants: []any{&cat{}, &dog{}, &dog{}}, wantErr: "at most one variant may be set, found 2"},
		{name: "Two variants of an anyOf in Also", u: anyTwice, variants: []any{nil, &dog{}, &cat{}}, want: `{"name":"","bark":false,"meow":false}`},
		{name: "A union in Also fills its value", u: withPets, variants: []any{nil, &withType{Name: "a"}, nil}, want: `{"type":"cat","name":"a"}`},
		{name: "A union in Also checks its value", u: withPets, variants: []any{nil, &withType{Type: "dog"}, nil}, wantErr: `type: "dog" picks Dog, not Cat`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalVariants(tc.shared, tc.u, tc.variants...)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestDiscriminatorError(t *testing.T) {
	t.Parallel()

	own := validation.Error{Field: "type", Message: "picks no variant", Rule: validation.RuleDiscriminator}
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "No error", err: nil},
		{name: "The discriminator error", err: own, want: own},
		{name: "The count is checked apart", err: validation.AtMostOne(true, true)},
		{name: "An error of a nested union is its own", err: &json.MarshalerError{Err: own}},
		{name: "Any other error", err: errors.New("broken")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, DiscriminatorError(nil, tc.err))
		})
	}
}

func TestMarshalOneOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		set     []any
		want    string
		wantErr string
	}{
		{name: "One variant", set: []any{&cat{Name: "a"}}, want: `{"name":"a","meow":false}`},
		{name: "Nothing set is null", want: `null`},
		{name: "Two variants", set: []any{&cat{}, &dog{}}, wantErr: "at most one variant may be set, found 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalOneOf(nil, tc.set...)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestMarshalUnionText(t *testing.T) {
	t.Parallel()

	errBroken := errors.New("broken")

	tests := []struct {
		name       string
		data       string
		err        error
		want       string
		wantErr    error
		wantErrMsg string
	}{
		{name: "A string loses its quotes", data: `"a \"b\""`, want: `a "b"`},
		{name: "An integer stays as it is", data: `30`, want: `30`},
		{name: "A number stays as it is", data: `1.5e3`, want: `1.5e3`},
		{name: "A boolean stays as it is", data: `false`, want: `false`},
		{name: "Null has no text", data: `null`, wantErr: ErrParamValue, wantErrMsg: "invalid parameter value: cannot write null as text"},
		{name: "An object has no text", data: `{"a":1}`, wantErr: ErrParamValue},
		{name: "A broken string is an error", data: `"a`},
		{name: "The marshal error is passed on", err: errBroken, wantErr: errBroken},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalUnionText([]byte(tc.data), tc.err)

			if tc.want == "" {
				require.Error(t, err)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				}
				if tc.wantErrMsg != "" {
					require.EqualError(t, err, tc.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestMergeObjects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parts   []string
		want    string
		wantErr error
	}{
		{name: "Keys keep their first place and last value", parts: []string{`{"a":1,"b":2}`, `{"c":3,"a":4}`}, want: `{"a":4,"b":2,"c":3}`},
		{name: "No parts", want: `{}`},
		{name: "Values are kept as written", parts: []string{`{"a": [1, 2]}`}, want: `{"a":[1, 2]}`},
		{name: "Part that is no object", parts: []string{`{}`, `[1]`}, wantErr: ErrNotObject},
		{name: "Empty part", parts: []string{``}, wantErr: ErrNotObject},
		{name: "Broken key", parts: []string{`{1:2}`}},
		{name: "Broken value", parts: []string{`{"a":}`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts := make([][]byte, len(tc.parts))
			for i, p := range tc.parts {
				parts[i] = []byte(p)
			}

			got, err := mergeObjects(parts...)

			if tc.want == "" {
				require.Error(t, err)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestIsNil(t *testing.T) {
	t.Parallel()

	var m map[string]int
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "No value", want: true},
		{name: "A nil pointer", value: (*int)(nil), want: true},
		{name: "A nil slice", value: []string(nil), want: true},
		{name: "A nil map", value: m, want: true},
		{name: "A value", value: 0},
		{name: "An empty slice", value: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isNil(tc.value))
		})
	}
}
