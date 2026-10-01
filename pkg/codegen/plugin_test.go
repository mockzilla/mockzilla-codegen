// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeRefExpr(t *testing.T) {
	t.Parallel()

	pet := TypeRef{Name: "[]Pet", Package: "models", ImportPath: "example.com/work/models"}
	tests := []struct {
		name string
		typ  TypeRef
		from string
		want string
	}{
		{name: "Type that needs no import", typ: TypeRef{Name: "func() any"}, from: "example.com/work/api", want: "func() any"},
		{name: "Type of the package written", typ: pet, from: "example.com/work/models", want: "[]Pet"},
		{name: "Type of another package", typ: pet, from: "example.com/work/api", want: "[]models.Pet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.typ.Expr(tc.from))
		})
	}
}

func TestTypeRefCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     TypeRef
		wantMsg string
	}{
		{name: "Type that needs no import, whatever it is", typ: TypeRef{Name: "func(Pet) Option[Pet]"}},
		{name: "Package without an import path, whatever the type is", typ: TypeRef{Name: "Page[Pet]", Package: "api"}},
		{name: "Map of slices of pointers", typ: TypeRef{Name: "map[string][]*Pet", Package: "models", ImportPath: "example.com/work/models"}},
		{
			name:    "Generic type with an import path",
			typ:     TypeRef{Name: "Option[Pet]", Package: "opt", ImportPath: "example.com/opt"},
			wantMsg: `type "Option[Pet]" of example.com/opt is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{
			name:    "Qualified name with an import path",
			typ:     TypeRef{Name: "models.Pet", ImportPath: "example.com/work/models"},
			wantMsg: `type "models.Pet" of example.com/work/models is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{name: "Import path without a package", typ: TypeRef{Name: "*Span", ImportPath: "example.com/trace"}},
		{
			name:    "Package that is no identifier",
			typ:     TypeRef{Name: "*Span", Package: "open-trace", ImportPath: "example.com/trace"},
			wantMsg: `type "*Span" of example.com/trace: "open-trace" is no package name`,
		},
		{
			name:    "Package that no type can be written with",
			typ:     TypeRef{Name: "*Span", Package: "_", ImportPath: "example.com/trace"},
			wantMsg: `type "*Span" of example.com/trace: "_" is no package name`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.typ.check()

			if tc.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errTypeRef)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}

func TestImportCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		imp     Import
		wantMsg string
	}{
		{name: "Path alone", imp: Import{Path: "net/http"}},
		{name: "Alias that is an identifier", imp: Import{Path: "net/http", Alias: "nethttp"}},
		{name: "Import for its side effects", imp: Import{Path: "embed", Alias: "_"}},
		{name: "Import of the names themselves", imp: Import{Path: "example.com/dsl", Alias: "."}},
		{name: "No path", imp: Import{Alias: "http"}, wantMsg: "import without a path"},
		{
			name:    "Alias with a dash",
			imp:     Import{Path: "net/http", Alias: "net-http"},
			wantMsg: `import of net/http: the alias "net-http" is not _, . or an identifier`,
		},
		{
			name:    "Alias that is a keyword",
			imp:     Import{Path: "example.com/type", Alias: "type"},
			wantMsg: `import of example.com/type: the alias "type" is not _, . or an identifier`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.imp.check()

			if tc.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errImport)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}

func TestScaffoldKindString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "service", ScaffoldService.String())
	assert.Equal(t, "middleware", ScaffoldMiddleware.String())
	assert.Equal(t, "main", ScaffoldMain.String())
	assert.Equal(t, "unknown", ScaffoldKind(9).String())
}
