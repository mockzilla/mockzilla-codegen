// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gorillamux

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

	assert.Equal(t, "gorilla-mux", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/gorilla/mux"}, {Path: "net/http"}}, fw.Imports())
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
		{name: "Two parameters together", path: "/pets/{a}{b}", want: "/pets/{a}{b}"},
		{name: "A wildcard becomes a pattern", path: "/files/*", want: "/files/{rest:.*}"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/{rest}/{rest_:.*}"},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/{lat_lng}"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
		{name: "Wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: * must be a segment of its own"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "Two names the router gives one name", path: "/geo/{a:b}/{a_b}", wantErr: `the router rejects the path: parameters "a:b" and "a_b" are both a_b on the router`},
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
	files := framework.Route{Operation: "GetFile", Method: "GET", Path: "/files/*", Pattern: "/files/{rest:.*}"}
	index := framework.Route{Operation: "GetIndex", Method: "GET", Path: "/files/index", Pattern: "/files/index"}
	newPet := framework.Route{Operation: "NewPet", Method: "GET", Path: "/pets/new", Pattern: "/pets/new"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "/pets/{petId}"}
	policy := framework.Route{Operation: "GetPolicy", Method: "GET", Path: "/pets/{id}:getPolicy", Pattern: "/pets/{id}:getPolicy"}
	json := framework.Route{Operation: "GetJSON", Method: "GET", Path: "/pets/{id}.json", Pattern: "/pets/{id}.json"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, files, index, newPet, renamed, json, policy})

	assert.Equal(t, []framework.Route{index, newPet, policy, json, get, files}, kept, "literals first, a parameter next to the longer literal before one next to a shorter, the wildcard last")
	assert.Equal(t, []framework.Conflict{{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"}}, dropped)
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

	assert.Equal(t, `mux.Vars(r)["pet-id"]`, Framework{}.PathParam(s, "pet-id"))
	assert.Equal(t, `mux.Vars(r)["lat_lng"]`, Framework{}.PathParam(s, "lat:lng"))
	assert.Equal(t, `import "github.com/gorilla/mux"`, s.Imports.Decl())
}
