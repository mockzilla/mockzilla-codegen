// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package kratos

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

	assert.Equal(t, "kratos", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/go-kratos/kratos/v2/transport/http", Alias: "khttp"}}, fw.Imports())
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
		{name: "Parameters stay", path: "/pets/{id}/photos/{photo-id}.jpg", want: "/pets/{id}/photos/{photo-id}.jpg"},
		{name: "A wildcard becomes a pattern", path: "/files/*", want: "/files/{rest:.*}"},
		{name: "The root", path: "/", want: "/"},
		{name: "A trailing slash", path: "/pets/", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "A double slash", path: "/a//b", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/{lat_lng}"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/{rest}/{rest_:.*}"},
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
	newPet := framework.Route{Operation: "NewPet", Method: "GET", Path: "/pets/new", Pattern: "/pets/new"}
	again := framework.Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}
	policy := framework.Route{Operation: "GetPolicy", Method: "GET", Path: "/pets/{id}:getPolicy", Pattern: "/pets/{id}:getPolicy"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, newPet, again, policy})

	assert.Equal(t, []framework.Route{newPet, policy, get}, kept, "literals first, then a parameter next to a literal")
	assert.Equal(t, []framework.Conflict{{Route: again, Reason: "repeats the route of GetPet"}}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{
		Signature:             "(c khttp.Context) error",
		Writer:                "c.Response()",
		Request:               "c.Request()",
		Epilogue:              "return nil",
		ServeSignature:        "(w http.ResponseWriter, r *http.Request)",
		Context:               "c",
		ContextServeSignature: "(c khttp.Context, w http.ResponseWriter, r *http.Request)",
	}, Framework{}.Handler(s))
	assert.Equal(t, "import (\n\t\"net/http\"\n\n\tkhttp \"github.com/go-kratos/kratos/v2/transport/http\"\n)", s.Imports.Decl())
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `c.Vars().Get("pet-id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Equal(t, `c.Vars().Get("lat_lng")`, Framework{}.PathParam(s, "lat:lng"))
	assert.Empty(t, s.Imports.Decl())
}
