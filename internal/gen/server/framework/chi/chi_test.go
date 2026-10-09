// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package chi

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

	assert.Equal(t, "chi", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/go-chi/chi/v5"}}, fw.Imports())
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
		{name: "Parameters stay as OpenAPI writes them", path: "/pets/{id}/photos/{photoId}", want: "/pets/{id}/photos/{photoId}"},
		{name: "A wildcard last", path: "/files/*", want: "/files/*"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
		{name: "Parameter named twice", path: "/pets/{id}/{id}", wantErr: `the router rejects the path: parameter "id" is named twice`},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/{lat_lng}"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
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

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}
	del := framework.Route{Operation: "DeletePet", Method: "DELETE", Path: "/pets/{petId}", Pattern: "/pets/{petId}"}
	again := framework.Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{animalId}", Pattern: "/pets/{animalId}"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, del, again, renamed})

	assert.Equal(t, []framework.Route{get, del}, kept)
	assert.Equal(t, []framework.Conflict{
		{Route: again, Reason: "repeats the route of GetPet"},
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
	}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{Signature: "(w http.ResponseWriter, r *http.Request)", Writer: "w", Request: "r", ServeSignature: "(w http.ResponseWriter, r *http.Request)"}, Framework{}.Handler(s))
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `runtime.UnescapePath(r, chi.URLParam(r, "petId"))`, Framework{}.PathParam(s, "petId"))
	assert.Equal(t, `runtime.UnescapePath(r, chi.URLParam(r, "lat_lng"))`, Framework{}.PathParam(s, "lat:lng"))
	assert.Equal(t, "import (\n\tchi \"github.com/go-chi/chi/v5\"\n\t\"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"\n)", s.Imports.Decl())
}
