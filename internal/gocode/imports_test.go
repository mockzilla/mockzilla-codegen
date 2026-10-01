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
			adds: [][2]string{{"example.com/type", ""}, {"example.com/9lives", ""}, {"example.com/---", ""}, {"example.com/_", ""}},
			want: []string{"pkg", "pkg2", "pkg3", "pkg4"},
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
		{
			name: "Imports under _ and . take no name and no number",
			adds: [][2]string{{"embed", "_"}, {"net/http/pprof", "_"}, {"example.com/a", "."}, {"example.com/b", "."}},
			want: []string{"_", "_", ".", "."},
		},
		{
			name: "Path under _ or . still gets a name when code asks for one",
			adds: [][2]string{{"embed", "_"}, {"example.com/a", "."}, {"embed", ""}, {"example.com/a", ""}, {"embed", "_"}},
			want: []string{"_", ".", "embed", "a", "_"},
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
				{"github.com/mockzilla/mockzilla-codegen/pkg/runtime", ""},
				{"time", ""},
				{"github.com/go-chi/chi/v5", ""},
				{"encoding/json", ""},
			},
			want: "import (\n\t\"encoding/json\"\n\t\"time\"\n\n\tchi \"github.com/go-chi/chi/v5\"\n\t\"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"\n)",
		},
		{
			name: "Only other imports",
			adds: [][2]string{{"example.com/a/models", ""}, {"example.com/b/models", ""}},
			want: "import (\n\t\"example.com/a/models\"\n\tmodels2 \"example.com/b/models\"\n)",
		},
		{
			name: "Imports under _ and . are written as given",
			adds: [][2]string{{"net/http/pprof", "_"}, {"embed", "_"}, {"example.com/b", "."}, {"example.com/a", "."}},
			want: "import (\n\t_ \"embed\"\n\t_ \"net/http/pprof\"\n\n\t. \"example.com/a\"\n\t. \"example.com/b\"\n)",
		},
		{
			name: "Name stands in for _ and next to .",
			adds: [][2]string{{"embed", "_"}, {"embed", ""}, {"example.com/a", ""}, {"example.com/a", "."}, {"time", ""}, {"time", "_"}},
			want: "import (\n\t\"embed\"\n\t\"time\"\n\n\t. \"example.com/a\"\n\t\"example.com/a\"\n)",
		},
		{
			name: ". stands in for _ in either order",
			adds: [][2]string{{"example.com/a", "_"}, {"example.com/a", "."}, {"example.com/b", "."}, {"example.com/b", "_"}},
			want: "import (\n\t. \"example.com/a\"\n\t. \"example.com/b\"\n)",
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
	s.Add("embed", "_")

	assert.True(t, s.Has("time"))
	assert.True(t, s.Has("embed"))
	assert.False(t, s.Has("fmt"))
}

func TestImportSetPaths(t *testing.T) {
	t.Parallel()

	s := NewImportSet()
	assert.Empty(t, s.Paths())

	s.Add("time", "")
	s.Add("embed", "_")
	s.Add("example.com/a", ".")
	s.Add("example.com/a", "")
	assert.Equal(t, []string{"embed", "example.com/a", "time"}, s.Paths())
}

func TestImportSetTrim(t *testing.T) {
	t.Parallel()

	adds := [][2]string{{"context", ""}, {"errors", ""}, {"example.com/a/models", ""}, {"example.com/b/models", ""}, {"embed", "_"}, {"example.com/dsl", "."}}
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "Names before a dot stay, in types and in calls",
			src:  "\nvar ErrNone = errors.New(\"none\")\n\nfunc Find(ctx context.Context) (*models2.Pet, error) {\n\treturn nil, ErrNone\n}\n",
			want: "import (\n\t\"context\"\n\t_ \"embed\"\n\t\"errors\"\n\n\tmodels2 \"example.com/b/models\"\n\t. \"example.com/dsl\"\n)",
		},
		{
			name: "Code that names no package keeps the imports without a name",
			src:  "\ntype Pets struct{}\n",
			want: "import (\n\t_ \"embed\"\n\n\t. \"example.com/dsl\"\n)",
		},
		{
			name: "Name in a comment, in a string or after a dot is no use",
			src:  "\n// Pets wraps context.Context.\ntype Pets struct{ errors string }\n\nfunc (p Pets) Text() string { return \"models.Pet\" + p.errors }\n",
			want: "import (\n\t_ \"embed\"\n\n\t. \"example.com/dsl\"\n)",
		},
		{
			name: "Parameter or local with the name of a package is no use",
			src: "\ntype list struct{ items []string }\n\nfunc (l list) Len() int { return len(l.items) }\n\n" +
				"func Total(context list) int {\n\terrors := list{}\n\treturn context.Len() + errors.Len()\n}\n",
			want: "import (\n\t_ \"embed\"\n\n\t. \"example.com/dsl\"\n)",
		},
		{
			name: "Package named in one func and hidden by a parameter in another stays",
			src:  "\nfunc Find(ctx context.Context) error { return ctx.Err() }\n\nfunc Count(context []string) int { return len(context) }\n",
			want: "import (\n\t\"context\"\n\t_ \"embed\"\n\n\t. \"example.com/dsl\"\n)",
		},
		{
			name: "Code that does not parse loses nothing",
			src:  "\ntype Pets struct{\n",
			want: "import (\n\t\"context\"\n\t_ \"embed\"\n\t\"errors\"\n\n\t\"example.com/a/models\"\n\tmodels2 \"example.com/b/models\"\n\t. \"example.com/dsl\"\n)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewImportSet()
			for _, add := range adds {
				s.Add(add[0], add[1])
			}
			s.Trim([]byte(tc.src))
			assert.Equal(t, tc.want, s.Decl())
		})
	}
}

func TestImportSetTrimFreesTheName(t *testing.T) {
	t.Parallel()

	s := NewImportSet()
	s.Add("example.com/a/models", "")
	s.Trim([]byte("\ntype Pets struct{}\n"))

	assert.Equal(t, "models", s.Add("example.com/b/models", ""))
	assert.Equal(t, `import "example.com/b/models"`, s.Decl())
}
