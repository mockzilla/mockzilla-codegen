// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package fiber

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

	assert.Equal(t, "fiber", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, []gomodel.Import{
		{Path: "github.com/gofiber/fiber/v3"},
		{Path: "github.com/valyala/fasthttp/fasthttpadaptor"},
		{Path: "net/http"},
		{Path: "strings"},
	}, fw.Imports())
	for _, name := range []string{"templates/router.tmpl", "templates/scaffold-main.tmpl"} {
		_, err := fw.Templates().Open(name)
		require.NoError(t, err)
	}
}

func TestRoutePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{name: "Parameters become colon parameters named as identifiers", path: "/pets/{id}/photos/{photo-id}", want: "/pets/:id/photos/:photo_id"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "/pets/v:id"},
		{name: "Special characters are escaped", path: "/pets:search/a+b/c?/{id}", want: `/pets\:search/a\+b/c\?/:id`},
		{name: "A wildcard last", path: "/files/*", want: "/files/*"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}.json"},
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
	slash := framework.Route{Operation: "GetPetSlash", Method: "GET", Path: "/pets/{id}/", Pattern: "/pets/:id/"}
	files := framework.Route{Operation: "GetFile", Method: "GET", Path: "/files/*", Pattern: "/files/*"}
	index := framework.Route{Operation: "GetIndex", Method: "GET", Path: "/files/index", Pattern: "/files/index"}
	newPet := framework.Route{Operation: "NewPet", Method: "GET", Path: "/pets/new", Pattern: "/pets/new"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "/pets/:petId"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, slash, files, index, newPet, renamed})

	assert.Equal(t, []framework.Route{index, newPet, get, files}, kept, "literals first, the wildcard last")
	assert.Equal(t, []framework.Conflict{
		{Route: slash, Reason: "matches the same requests as GetPet at /pets/{id}"},
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
	}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{Signature: "(w http.ResponseWriter, r *http.Request)", Return: "return"}, Framework{}.Handler(s))
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `r.PathValue("pet_id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Empty(t, s.Imports.Decl())
}
