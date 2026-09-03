// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func twoPackages(t *testing.T) *layout.Layout {
	t.Helper()

	cfg, err := config.Parse([]byte("output:\n  file: ./api/gen.go\n  files: {./models/types.go: [models.types]}\n"), "/work")
	require.NoError(t, err)
	l, err := layout.Plan(cfg, []layout.Part{{ID: gomodel.PartTypes}, {ID: gomodel.PartParams}}, layout.Module{Path: "example.com/work", Dir: "/work"})
	require.NoError(t, err)
	return l
}

func TestScopeExpr(t *testing.T) {
	t.Parallel()

	l := twoPackages(t)
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes}
	query := &gomodel.Decl{Name: "ListQuery", Part: gomodel.PartParams}
	orphan := &gomodel.Decl{Name: "Orphan", Part: "server.router"}
	date := gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "Date"}

	tests := []struct {
		name        string
		typ         gomodel.Type
		want        string
		wantImports string
	}{
		{name: "Builtin", typ: gomodel.Builtin{Name: "int64"}, want: "int64"},
		{name: "Declaration in the same folder", typ: gomodel.DeclRef{Decl: query}, want: "ListQuery"},
		{
			name:        "Declaration in another folder is qualified and imported",
			typ:         gomodel.DeclRef{Decl: pet},
			want:        "models.Pet",
			wantImports: `import "example.com/work/models"`,
		},
		{name: "Declaration of an unknown part stays plain", typ: gomodel.DeclRef{Decl: orphan}, want: "Orphan"},
		{
			name:        "Qualified type imports its package",
			typ:         gomodel.Pointer{Elem: date},
			want:        "*runtime.Date",
			wantImports: `import "github.com/mockzilla/mockzilla-codegen/pkg/runtime"`,
		},
		{
			name:        "Qualified type with an alias",
			typ:         gomodel.Qualified{Import: gomodel.Import{Path: "github.com/google/uuid", Alias: "guid"}, Name: "UUID"},
			want:        "guid.UUID",
			wantImports: `import guid "github.com/google/uuid"`,
		},
		{
			name: "Slice and map",
			typ:  gomodel.Map{Key: gomodel.Builtin{Name: "string"}, Elem: gomodel.Slice{Elem: gomodel.DeclRef{Decl: query}}},
			want: "map[string][]ListQuery",
		},
		{name: "Nil type is any", want: "any"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewScope(l.FileOf(gomodel.PartParams), l)
			assert.Equal(t, tc.want, s.Expr(tc.typ))
			assert.Equal(t, tc.wantImports, s.Imports.Decl())
		})
	}
}

func TestScopeQualified(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expr        string
		imp         gomodel.Import
		want        string
		wantImports string
	}{
		{name: "No package", expr: "func() any", want: "func() any"},
		{name: "Own package", expr: "[]ListQuery", imp: gomodel.Import{Path: "example.com/work/api"}, want: "[]ListQuery"},
		{
			name:        "Other package is qualified and imported",
			expr:        "[]Pet",
			imp:         gomodel.Import{Path: "example.com/work/models", Alias: "model"},
			want:        "[]model.Pet",
			wantImports: `import model "example.com/work/models"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewScope(twoPackages(t).FileOf(gomodel.PartParams), nil)
			assert.Equal(t, tc.want, s.Qualified(tc.expr, tc.imp))
			assert.Equal(t, tc.wantImports, s.Imports.Decl())
		})
	}
}

func TestScopeRuntimeGuard(t *testing.T) {
	t.Parallel()

	l := twoPackages(t)
	s := NewScope(l.FileOf(gomodel.PartParams), l)
	assert.Empty(t, s.RuntimeGuard())

	s.Imports.Add("example.com/runtime", "")
	s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	assert.Equal(t, "runtime2.SupportsGeneratorV1", s.RuntimeGuard())
}
