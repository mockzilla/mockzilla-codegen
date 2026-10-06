// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package hertz

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

	assert.Equal(t, "hertz", fw.Name())
	assert.Equal(t, gomodel.Import{Path: "github.com/cloudwego/hertz/pkg/app/server"}, fw.Imports()[0])
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
		{name: "Parameters become colon parameters", path: "/pets/{id}/photos/{photo-id}", want: "/pets/:id/photos/:photo-id"},
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "/pets/v:id"},
		{name: "A wildcard is named", path: "/files/*", want: "/files/*rest"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/:rest/*rest_"},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/:lat_lng"},
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
	other := framework.Route{Operation: "GetPhoto", Method: "GET", Path: "/pets/{petId}/photo", Pattern: "/pets/:petId/photo"}
	renamed := framework.Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{petId}", Pattern: "/pets/:petId"}

	kept, dropped := Framework{}.Conflicts([]framework.Route{get, other, renamed})

	assert.Equal(t, []framework.Route{get, other}, kept)
	assert.Equal(t, []framework.Conflict{{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"}}, dropped)
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

	assert.Equal(t, `r.PathValue("pet-id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Equal(t, `r.PathValue("lat_lng")`, Framework{}.PathParam(s, "lat:lng"))
	assert.Empty(t, s.Imports.Decl())
}
