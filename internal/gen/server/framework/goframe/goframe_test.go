// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package goframe

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

	assert.Equal(t, "goframe", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/gogf/gf/v2/net/ghttp"}, {Path: "github.com/gogf/gf/v2/util/guid"}, {Path: "net/http"}}, fw.Imports())
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
		{name: "Parameters stay as fields named as identifiers", path: "/pets/{id}/photos/{photo-id}.jpg", want: "GET:/pets/{id}/photos/{photo_id}.jpg"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "GET:/pets/v{id}"},
		{name: "A wildcard is named", path: "/files/*", want: "GET:/files/*rest"},
		{name: "The trailing slash is dropped", path: "/pets/", want: "GET:/pets"},
		{name: "The root", path: "/", want: "GET:/"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "A wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: * must be a segment of its own"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A parameter named twice", path: "/pets/{id}/{id}", wantErr: `the router rejects the path: parameter "id" is named twice`},
		{name: "A literal colon", path: "/pets:search", wantErr: "the router rejects the path: a colon or an at sign is read as the start of a parameter or a domain"},
		{name: "A literal at sign", path: "/users/{id}@home", wantErr: "the router rejects the path: a colon or an at sign is read as the start of a parameter or a domain"},
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

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "GET:/pets/{id}"}
	post := framework.Route{Operation: "PostPet", Method: "POST", Path: "/pets/{id}", Pattern: "POST:/pets/{id}"}
	slash := framework.Route{Operation: "GetPetSlash", Method: "GET", Path: "/pets/{id}/", Pattern: "GET:/pets/{id}"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "GET:/pets/{petId}"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, post, slash, renamed})

	assert.Equal(t, []framework.Route{get, post}, kept)
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
