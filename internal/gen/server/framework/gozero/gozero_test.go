// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gozero

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

	assert.Equal(t, "go-zero", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, gomodel.Import{Path: "github.com/zeromicro/go-zero/rest/httpx"}, fw.Imports()[0])
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
		{name: "Parameters become colon parameters", path: "/pets/{id}/photos/{photo-id}", want: "/pets/:id/photos/:photo-id"},
		{name: "The trailing slash is dropped", path: "/pets/", want: "/pets"},
		{name: "The root", path: "/", want: "/"},
		{name: "A literal colon inside a segment", path: "/pets:search", want: "/pets:search"},
		{name: "A wildcard", path: "/files/*", wantErr: "the router rejects the path: the router has no wildcard"},
		{name: "A double slash", path: "/a//b", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "A segment beginning with a colon", path: "/:pets", wantErr: "the router rejects the path: a segment beginning with : is read as a parameter"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", wantErr: "the router rejects the path: a parameter must fill its segment, unlike v{id}"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}.json"},
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
	slash := framework.Route{Operation: "GetPetSlash", Method: "GET", Path: "/pets/{id}/", Pattern: "/pets/:id"}
	other := framework.Route{Operation: "GetPhoto", Method: "GET", Path: "/pets/{petId}/photo", Pattern: "/pets/:petId/photo"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, slash, other})

	assert.Equal(t, []framework.Route{get, other}, kept)
	assert.Equal(t, []framework.Conflict{{Route: slash, Reason: "matches the same requests as GetPet at /pets/{id}"}}, dropped)
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

	assert.Equal(t, `pathvar.Vars(r)["pet-id"]`, Framework{}.PathParam(s, "pet-id"))
	assert.Equal(t, `import "github.com/zeromicro/go-zero/rest/pathvar"`, s.Imports.Decl())
}
