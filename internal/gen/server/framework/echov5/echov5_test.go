// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package echov5

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

	assert.Equal(t, "echo-v5", fw.Name())
	assert.Equal(t, framework.Native, fw.Family())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/labstack/echo/v5"}}, fw.Imports())
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
		{name: "Parameters become colon parameters", path: "/pets/{id}/photos/{photoId}", want: "/pets/:id/photos/:photoId"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "/pets/v:id"},
		{name: "A literal colon is escaped", path: "/pets:search/v:{id}", want: `/pets\:search/v\::id`},
		{name: "A wildcard last", path: "/files/*", want: "/files/*"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
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

func TestConflicts(t *testing.T) {
	t.Parallel()

	get := framework.Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "/pets/:petId"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, renamed})

	assert.Equal(t, []framework.Route{get}, kept)
	assert.Equal(t, []framework.Conflict{{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"}}, dropped)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, framework.Handler{
		Signature: "(c *echo.Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}, Framework{}.Handler(s))
	assert.Equal(t, "import echo \"github.com/labstack/echo/v5\"", s.Imports.Decl())
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `c.Param("pet-id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Empty(t, s.Imports.Decl())
}
