// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func TestPetJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Pet
	}{
		{name: "Cat by its required property", data: `{"name":"Tom","meow":true}`, want: Pet{Cat: &Cat{Name: new("Tom"), Meow: true}}},
		{name: "Dog by its required property", data: `{"bark":false}`, want: Pet{Dog: &Dog{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Pet
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestPetNoMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       string
		wantErr    error
		wantErrMsg string
	}{
		{name: "A kind no variant takes", data: `"Tom"`, wantErr: runtime.ErrNoVariant, wantErrMsg: "no union variant matches for a JSON string"},
		{
			name:       "No required property",
			data:       `{"name":"Tom"}`,
			wantErr:    runtime.ErrNoVariant,
			wantErrMsg: "no union variant matches for a JSON object: Cat needs meow, Dog needs bark",
		},
		{
			name:       "The required properties of both",
			data:       `{"meow":true,"bark":true}`,
			wantErr:    runtime.ErrAmbiguous,
			wantErrMsg: "more than one union variant matches: Cat and Dog",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Pet
			err := json.Unmarshal([]byte(tc.data), &got)

			require.ErrorIs(t, err, tc.wantErr)
			require.EqualError(t, err, tc.wantErrMsg)
		})
	}
}

func TestPetMarshalTwo(t *testing.T) {
	t.Parallel()

	_, err := json.Marshal(Pet{Cat: &Cat{}, Dog: &Dog{}})

	require.ErrorContains(t, err, "at most one variant may be set, found 2")
}

func TestPetValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pet     Pet
		wantErr string
	}{
		{name: "One variant", pet: Pet{Cat: &Cat{}}},
		{name: "No variant", wantErr: "exactly one variant must be set, found 0"},
		{name: "Two variants", pet: Pet{Cat: &Cat{}, Dog: &Dog{}}, wantErr: "exactly one variant must be set, found 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.pet.Validate()

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestValueJSON(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		data string
		want Value
	}{
		{name: "Date-time string", data: `"2026-09-30T12:00:00Z"`, want: Value{Time: &at}},
		{name: "Other string", data: `"hello"`, want: Value{String: new("hello")}},
		{name: "Integer", data: `42`, want: Value{Int64: new(int64(42))}},
		{name: "Fraction", data: `4.5`, want: Value{Float64: new(4.5)}},
		{name: "Boolean", data: `true`, want: Value{Bool: new(true)}},
		{name: "Array", data: `["a","b"]`, want: Value{Strings: []string{"a", "b"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Value
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestShapeNested(t *testing.T) {
	t.Parallel()

	var got Shape
	require.NoError(t, json.Unmarshal([]byte(`7`), &got))
	assert.Equal(t, Shape{Option2: &ShapeOption2{Int: new(7)}}, got)

	require.NoError(t, json.Unmarshal([]byte(`{"bark":true}`), &got))
	assert.Equal(t, Shape{Pet: &Pet{Dog: &Dog{Bark: true}}}, got)
}

func TestOwnerJSON(t *testing.T) {
	t.Parallel()

	const data = `{"pet":{"meow":true},"pets":[{"bark":true},{"meow":false}],"tags":{"a":"x","b":2},"best":null}`

	var got Owner
	require.NoError(t, json.Unmarshal([]byte(data), &got))

	assert.Equal(t, Owner{
		Pet:  Pet{Cat: &Cat{Meow: true}},
		Pets: []Pet{{Dog: &Dog{Bark: true}}, {Cat: &Cat{}}},
		Tags: map[string]OwnerTagsValue{"a": {String: new("x")}, "b": {Int: new(2)}},
	}, got)

	out, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"pet":{"meow":true},"pets":[{"bark":true},{"meow":false}],"tags":{"a":"x","b":2}}`, string(out))
}
