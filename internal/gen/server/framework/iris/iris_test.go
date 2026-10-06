// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package iris

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

	assert.Equal(t, "iris", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/kataras/iris/v12"}, {Path: "context"}, {Path: "net/http"}}, fw.Imports())
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
		{name: "Parameters are named as identifiers", path: "/pets/{id}/photos/{photo-id}", want: "/pets/{id}/photos/{photo_id}"},
		{name: "A wildcard becomes a path parameter", path: "/files/*", want: "/files/{rest:path}"},
		{name: "The root", path: "/", want: "/"},
		{name: "A trailing slash is dropped", path: "/pets/", want: "/pets"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/{rest}/{rest_:path}"},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/{lat_lng}"},
		{name: "Two names that are one identifier", path: "/pets/{pet-id}/{pet_id}", wantErr: `the router rejects the path: parameters "pet-id" and "pet_id" are both pet_id on the router`},
		{name: "A literal colon", path: "/pets:search", want: "/pets:search"},
		{name: "A literal segment that begins with a colon", path: "/pets/:search", wantErr: "the router rejects the path: a segment beginning with : is read as a parameter"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
		{name: "Wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: a parameter must fill its segment, unlike files*"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", wantErr: "the router rejects the path: a parameter must fill its segment, unlike v{id}"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must fill its segment, unlike {id}.json"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
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

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "/pets/{petId}"}
	slash := framework.Route{Operation: "GetPetSlash", Method: "GET", Path: "/pets/{id}/", Pattern: "/pets/{id}"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, renamed, slash})

	assert.Equal(t, []framework.Route{get}, kept)
	assert.Equal(t, []framework.Conflict{
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
		{Route: slash, Reason: "matches the same requests as GetPet at /pets/{id}"},
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
