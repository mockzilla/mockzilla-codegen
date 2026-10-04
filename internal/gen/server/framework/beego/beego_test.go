// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package beego

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

	assert.Equal(t, "beego", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, []gomodel.Import{
		{Path: "github.com/beego/beego/v2/server/web"},
		{Path: "github.com/beego/beego/v2/server/web/context", Alias: "bcontext"},
		{Path: "io"},
		{Path: "net/http"},
		{Path: "strings"},
	}, fw.Imports())
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
		{name: "Parameters become colon parameters named as identifiers", path: "/pets/{id}/photos/{photo-id}", want: "/pets/:id/photos/:photo_id"},
		{name: "A wildcard last", path: "/files/*", want: "/files/*"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", wantErr: "the router rejects the path: a parameter must fill its segment, unlike v{id}"},
		{name: "A literal colon", path: "/pets:search", wantErr: "the router rejects the path: : is read as the start of a parameter in pets:search"},
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
	again := framework.Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, again})

	assert.Equal(t, []framework.Route{get}, kept)
	assert.Equal(t, []framework.Conflict{{Route: again, Reason: "repeats the route of GetPet"}}, dropped)
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
