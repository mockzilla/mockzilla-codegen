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
	assert.Equal(t, framework.Native, fw.Family())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/go-kratos/kratos/v2/transport/http", Alias: "khttp"}}, fw.Imports())
	_, err := fw.Templates().Open("router.tmpl")
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
		{name: "A parameter with a colon", path: "/pets/{id:x}", wantErr: `the router rejects the path: parameter "id:x" holds a colon`},
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

func TestConflicts(t *testing.T) {
	t.Parallel()

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}
	newPet := framework.Route{Operation: "NewPet", Method: "GET", Path: "/pets/new", Pattern: "/pets/new"}
	again := framework.Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/{id}"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, newPet, again})

	assert.Equal(t, []framework.Route{newPet, get}, kept, "literals first")
	assert.Equal(t, []framework.Conflict{{Route: again, Reason: "repeats the route of GetPet"}}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{
		Signature: "(c khttp.Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}, Framework{}.Handler(s))
	assert.Equal(t, "import khttp \"github.com/go-kratos/kratos/v2/transport/http\"", s.Imports.Decl())
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `c.Vars().Get("pet-id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Empty(t, s.Imports.Decl())
}
