// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportSetAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		adds [][2]string
		want []string
	}{
		{
			name: "Name comes from the last path element",
			adds: [][2]string{{"encoding/json", ""}, {"github.com/google/uuid", ""}},
			want: []string{"json", "uuid"},
		},
		{
			name: "Major version, go- prefix and dot suffix are not part of the name",
			adds: [][2]string{{"github.com/go-chi/chi/v5", ""}, {"gopkg.in/yaml.v3", ""}, {"example.com/go-redis", ""}},
			want: []string{"chi", "yaml", "redis"},
		},
		{
			name: "v1 and a lone version element are names",
			adds: [][2]string{{"example.com/v1", ""}, {"v2", ""}},
			want: []string{"v1", "v2"},
		},
		{
			name: "Names that are no identifier fall back",
			adds: [][2]string{{"example.com/type", ""}, {"example.com/9lives", ""}, {"example.com/---", ""}},
			want: []string{"pkg", "pkg2", "pkg3"},
		},
		{
			name: "Preferred name wins over the guess",
			adds: [][2]string{{"example.com/models-v2", "models"}},
			want: []string{"models"},
		},
		{
			name: "Clashing names get numbers in the order added",
			adds: [][2]string{{"example.com/a/models", ""}, {"example.com/b/models", ""}, {"example.com/c/models", ""}},
			want: []string{"models", "models2", "models3"},
		},
		{
			name: "Adding a path again gives its first name",
			adds: [][2]string{{"example.com/a/models", ""}, {"example.com/b/models", ""}, {"example.com/a/models", "other"}},
			want: []string{"models", "models2", "models"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewImportSet()
			var got []string
			for _, add := range tc.adds {
				got = append(got, s.Add(add[0], add[1]))
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestImportSetDecl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		adds [][2]string
		want string
	}{
		{
			name: "Nothing imported",
			want: "",
		},
		{
			name: "One import on one line",
			adds: [][2]string{{"time", ""}},
			want: `import "time"`,
		},
		{
			name: "Standard library first, each group sorted, names only where needed",
			adds: [][2]string{
				{"github.com/mockzilla/codegen/pkg/runtime", ""},
				{"time", ""},
				{"github.com/go-chi/chi/v5", ""},
				{"encoding/json", ""},
			},
			want: "import (\n\t\"encoding/json\"\n\t\"time\"\n\n\tchi \"github.com/go-chi/chi/v5\"\n\t\"github.com/mockzilla/codegen/pkg/runtime\"\n)",
		},
		{
			name: "Only other imports",
			adds: [][2]string{{"example.com/a/models", ""}, {"example.com/b/models", ""}},
			want: "import (\n\t\"example.com/a/models\"\n\tmodels2 \"example.com/b/models\"\n)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewImportSet()
			for _, add := range tc.adds {
				s.Add(add[0], add[1])
			}
			assert.Equal(t, tc.want, s.Decl())
		})
	}
}

func TestImportSetHas(t *testing.T) {
	t.Parallel()

	s := NewImportSet()
	s.Add("time", "")

	assert.True(t, s.Has("time"))
	assert.False(t, s.Has("fmt"))
}
