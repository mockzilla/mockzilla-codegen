// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gin

import (
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
	assert.Equal(t, framework.NetHTTP, fw.Family())
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
		{name: "The root", path: "/", want: "/"},
		{name: "A literal colon", path: "/pets:search", wantErr: "the router rejects the path: : is read as the start of a parameter in pets:search"},
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
		routes      []framework.Route
		wantKept    []string
		wantReasons []string
	}{
		{
			name:     "A literal next to a parameter, in either order, and parameters of different methods",
			routes:   routes("GET /pets/{id}", "GET /pets/new", "POST /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x"),
			wantKept: []string{"GET /pets/{id}", "GET /pets/new", "POST /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x"},
		},
		{
			name:     "A wildcard after a shorter route, and a trailing slash after a parameter",
			routes:   routes("GET /pets", "GET /pets/*", "GET /a/{x}/b", "GET /{y}/b/c", "GET /q/{id}", "GET /q/{id}/"),
			wantKept: []string{"GET /pets", "GET /pets/*", "GET /a/{x}/b", "GET /{y}/b/c", "GET /q/{id}", "GET /q/{id}/"},
		},
		{
			name:        "A repeat",
			routes:      routes("GET /pets/{id}", "GET /pets/{id}"),
			wantKept:    []string{"GET /pets/{id}"},
			wantReasons: []string{"repeats the route of Op1"},
		},
		{
			name:        "Parameters named otherwise at one position",
			routes:      routes("GET /pets/{id}", "GET /pets/{petId}", "GET /pets/{a}/y", "GET /pets/v{id}", "GET /pets/v{x}"),
			wantKept:    []string{"GET /pets/{id}", "GET /pets/v{id}"},
			wantReasons: []string{"names its path parameters otherwise than Op1 at /pets/{id}", "names its path parameters otherwise than Op1 at /pets/{id}", "names its path parameters otherwise than Op4 at /pets/v{id}"},
		},
		{
			name:        "Nothing sits next to a wildcard",
			routes:      routes("GET /files/*", "GET /files/x", "GET /files/", "GET /files/{id}", "GET /pets/{id}", "GET /pets/*", "GET /*", "GET /any"),
			wantKept:    []string{"GET /files/*", "GET /pets/{id}", "GET /any"},
			wantReasons: []string{"cannot sit next to the wildcard of Op1 at /files/*", "cannot sit next to the wildcard of Op1 at /files/*", "cannot sit next to the wildcard of Op1 at /files/*", "cannot sit next to the wildcard of Op5 at /pets/{id}", "cannot sit next to the wildcard of Op1 at /files/*"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kept, dropped := Framework{}.Conflicts(tc.routes)

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
func routes(specs ...string) []framework.Route {
	out := make([]framework.Route, len(specs))
	for i, spec := range specs {
		method, path, _ := strings.Cut(spec, " ")
		pattern, err := Framework{}.RoutePattern(method, path)
		if err != nil {
			panic(err)
		}
		out[i] = framework.Route{Operation: "Op" + string(rune('1'+i)), Method: method, Path: path, Pattern: pattern}
	}
	return out
}
