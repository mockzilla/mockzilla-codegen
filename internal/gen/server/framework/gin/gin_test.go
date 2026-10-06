// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gin

import (
	"strconv"
	"strings"
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

	assert.Equal(t, "gin", fw.Name())
	assert.Equal(t, []gomodel.Import{{Path: "github.com/gin-gonic/gin"}, {Path: "net/http"}}, fw.Imports())
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
		{name: "A parameter with a prefix", path: "/pets/v{id}", want: "/pets/v:id"},
		{name: "A wildcard is named", path: "/files/*", want: "/files/*rest"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/:rest/*rest_"},
		{name: "The root", path: "/", want: "/"},
		{name: "A literal colon is escaped", path: "/pets:search", want: `/pets\:search`},
		{name: "A literal colon that begins a segment", path: "/t/:tid", want: `/t/\:tid`},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/:lat_lng"},
		{name: "A path that is not clean", path: "/a//b", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "A literal star", path: "/a*b/{id}", wantErr: "the router rejects the path: * must be last"},
		{name: "A wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: * is read as the start of a parameter in files*"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}.json"},
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

// TestConflicts checks which routes are dropped, by the rules of gin's tree.
func TestConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		in          []string
		wantKept    []string
		wantReasons []string
	}{
		{
			name:     "A literal next to a parameter, in either order, and parameters of different methods",
			in:       []string{"GET /pets/{id}", "GET /pets/new", "POST /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x"},
			wantKept: []string{"GET /pets/{id}", "GET /pets/new", "POST /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x"},
		},
		{
			name:     "A wildcard after a shorter route, and a trailing slash after a parameter",
			in:       []string{"GET /pets", "GET /pets/*", "GET /a/{x}/b", "GET /{y}/b/c", "GET /q/{id}", "GET /q/{id}/"},
			wantKept: []string{"GET /pets", "GET /pets/*", "GET /a/{x}/b", "GET /{y}/b/c", "GET /q/{id}", "GET /q/{id}/"},
		},
		{
			name:     "Parameters named otherwise at one position",
			in:       []string{"GET /pets/{id}", "GET /pets/{a}/y", "GET /pets/v{id}", "GET /pets/v{x}/z"},
			wantKept: []string{"GET /pets/{id}", "GET /pets/{a}/y", "GET /pets/v{id}", "GET /pets/v{x}/z"},
		},
		{
			name:        "A repeat, and a route that matches the same requests",
			in:          []string{"GET /pets/{id}", "GET /pets/{id}", "GET /pets/{petId}"},
			wantKept:    []string{"GET /pets/{id}"},
			wantReasons: []string{"repeats the route of Op1", "matches the same requests as Op1 at /pets/{id}"},
		},
		{
			name:        "Nothing sits next to a wildcard",
			in:          []string{"GET /files/*", "GET /files/x", "GET /files/", "GET /files/{id}", "GET /pets/{id}", "GET /pets/*", "GET /*", "GET /any"},
			wantKept:    []string{"GET /files/*", "GET /pets/{id}", "GET /any"},
			wantReasons: []string{"cannot sit next to the wildcard of Op1 at /files/*", "cannot sit next to the wildcard of Op1 at /files/*", "cannot sit next to the wildcard of Op1 at /files/*", "brings a wildcard next to Op5 at /pets/{id}", "brings a wildcard next to Op1 at /files/*"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kept, dropped := Framework{}.Conflicts(routes(t, tc.in...))

			var gotKept, gotReasons []string
			for _, r := range kept {
				gotKept = append(gotKept, r.Method+" "+r.Path)
			}
			for _, c := range dropped {
				gotReasons = append(gotReasons, c.Reason)
			}
			assert.Equal(t, tc.wantKept, gotKept)
			assert.Equal(t, tc.wantReasons, gotReasons)
		})
	}
}

func TestConflictsNames(t *testing.T) {
	t.Parallel()

	kept, _ := Framework{}.Conflicts(routes(t, "GET /a/{x}", "GET /a/{y}/b/*", "GET /a/{z}/c/{w}", "GET /a/{q}/d:e"))

	var got [][]string
	for _, r := range kept {
		got = append(got, append([]string{r.Pattern}, r.Names...))
	}
	assert.Equal(t, [][]string{
		{"/a/:x", "x"},
		{"/a/:x/b/*rest", "y", "rest"},
		{"/a/:x/c/:w", "z", "w"},
		{`/a/:x/d\:e`, "q"},
	}, got, "a later route takes the name an earlier one has at a position, and keeps its own names for the values")
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
	assert.Empty(t, s.Imports.Decl())
}

// routes makes routes named Op1, Op2 and so on from "METHOD /path" strings, with the pattern
// RoutePattern gives each.
func routes(t *testing.T, specs ...string) []framework.Route {
	t.Helper()

	out := make([]framework.Route, len(specs))
	for i, spec := range specs {
		method, path, _ := strings.Cut(spec, " ")
		got, err := Framework{}.RoutePattern(method, path)
		require.NoError(t, err)
		out[i] = framework.Route{Operation: "Op" + strconv.Itoa(i+1), Method: method, Path: path, Pattern: got}
	}
	return out
}
