// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package echo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

func TestFramework(t *testing.T) {
	t.Parallel()

	fw := Framework{}

	assert.Equal(t, "echo", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/labstack/echo/v4"}}, fw.Imports())
	_, err := fw.Templates().Open("templates/router.tmpl")
	require.NoError(t, err)
}

func TestRoutePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{name: "Parameters become colon parameters", path: "/pets/{id}/photos/{photoId}", want: "/pets/:id/photos/:photoId"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "/pets/v:id"},
		{name: "A parameter name that is no identifier", path: "/pets/{pet-id}", want: "/pets/:pet-id"},
		{name: "A literal colon is escaped", path: "/pets:search/v:{id}", want: `/pets\:search/v\::id`},
		{name: "A colon after a parameter is a suffix", path: "/pets/{id}:adopt", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}:adopt"},
		{name: "The root", path: "/", want: "/"},
		{name: "A trailing slash", path: "/pets/", want: "/pets/"},
		{name: "A wildcard last", path: "/files/*", want: "/files/*"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}.json"},
		{name: "Two parameters in one segment", path: "/pets/{a}{b}", wantErr: "the router rejects the path: a parameter must end its segment, unlike {a}{b}"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A literal segment that begins with a colon", path: "/t/:tid", wantErr: "the router rejects the path: a segment beginning with : is read as a parameter"},
		{name: "A star in a literal", path: "/l/a*", wantErr: "the router rejects the path: * is read as a wildcard in a*"},
		{name: "A parameter named twice", path: "/pets/{id}/{id}", wantErr: `the router rejects the path: parameter "id" is named twice`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Framework{}.RoutePattern("GET", tc.path)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, framework.ErrPattern)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRoutePatternMethod(t *testing.T) {
	t.Parallel()

	_, err := Framework{}.RoutePattern("QUERY", "/pets")

	require.EqualError(t, err, "the router does not take the method QUERY")
	require.ErrorIs(t, err, framework.ErrMethod)
}

func TestConflicts(t *testing.T) {
	t.Parallel()

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	again := framework.Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, again})

	assert.Equal(t, []framework.Route{get}, kept)
	assert.Equal(t, []framework.Conflict{{Route: again, Reason: "repeats the route of GetPet"}}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{
		Signature: "(c echo.Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}, Framework{}.Handler(s))
	assert.Equal(t, "import echo \"github.com/labstack/echo/v4\"", s.Imports.Decl())
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `runtime.UnescapePath(r, c.Param("pet-id"))`, Framework{}.PathParam(s, "pet-id"))
	assert.Equal(t, `import "github.com/mockzilla/mockzilla-codegen/pkg/runtime"`, s.Imports.Decl())
}
