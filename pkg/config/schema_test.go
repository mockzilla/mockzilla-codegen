// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaUpToDate(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "config.schema.json")
	got, err := Schema()
	require.NoError(t, err)

	if os.Getenv("UPDATE") == "1" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), "run make schema")
}

func TestSchemaIsStable(t *testing.T) {
	t.Parallel()

	first, err := Schema()
	require.NoError(t, err)
	second, err := Schema()
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.True(t, json.Valid(first))
}

func TestTypeSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  reflect.Type
		want map[string]any
	}{
		{
			name: "String",
			typ:  reflect.TypeFor[string](),
			want: map[string]any{"type": "string"},
		},
		{
			name: "Pointer to bool",
			typ:  reflect.TypeFor[*bool](),
			want: map[string]any{"type": "boolean"},
		},
		{
			name: "Int64",
			typ:  reflect.TypeFor[int64](),
			want: map[string]any{"type": "integer"},
		},
		{
			name: "Byte size takes an integer or a string with a unit",
			typ:  reflect.TypeFor[ByteSize](),
			want: map[string]any{"type": []string{"integer", "string"}, "minimum": 0, "pattern": byteSizePattern},
		},
		{
			name: "Duration is a string",
			typ:  reflect.TypeFor[Duration](),
			want: map[string]any{"type": "string", "pattern": durationPattern},
		},
		{
			name: "Slice",
			typ:  reflect.TypeFor[[]string](),
			want: map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		{
			name: "Map of lists",
			typ:  reflect.TypeFor[map[string][]string](),
			want: map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		{
			name: "Free-form map",
			typ:  reflect.TypeFor[map[string]any](),
			want: map[string]any{"type": "object", "additionalProperties": map[string]any{}},
		},
		{
			name: "Struct with descriptions and enums",
			typ:  reflect.TypeFor[sample](),
			want: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"kind":  map[string]any{"type": "string", "description": "Which kind.", "enum": []string{"a", "b"}},
					"count": map[string]any{"type": "integer"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, typeSchema(tc.typ))
		})
	}
}
