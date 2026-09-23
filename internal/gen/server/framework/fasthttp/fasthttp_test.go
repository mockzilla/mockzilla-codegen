// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package fasthttp

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

	assert.Equal(t, "fasthttp", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, gomodel.Import{Path: "github.com/fasthttp/router"}, fw.Imports()[0])
	for _, name := range []string{"router.tmpl", "scaffold-main.tmpl"} {
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
		{name: "Parameters stay", path: "/pets/{id}/photos/{photo-id}.jpg", want: "/pets/{id}/photos/{photo-id}.jpg"},
		{name: "Two parameters apart", path: "/pets/{a}-{b}", want: "/pets/{a}-{b}"},
		{name: "A wildcard becomes the catch-all", path: "/files/*", want: "/files/{rest:*}"},
		{name: "The root wildcard", path: "/*", want: "/{rest:*}"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
		{name: "Wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: * must be a segment of its own"},
		{name: "Two parameters together", path: "/pets/{a}{b}", wantErr: "the router rejects the path: two parameters must have a character between them"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A parameter with a colon", path: "/pets/{id:x}", wantErr: `the router rejects the path: parameter "id:x" holds a colon`},
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

// TestConflicts checks which routes are dropped, by the rules of the router's tree.
func TestConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		routes      []framework.Route
		wantKept    []string
		wantReasons []string
	}{
		{
			name:     "Literals, parameters and catch-alls that the router holds together",
			routes:   routes("GET /pets/{id}", "GET /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x", "GET /pets/{a}/y", "GET /pets/{a}", "GET /files", "GET /files/*", "GET /files/x", "GET /a/{x}/", "GET /a/{x}/{y}"),
			wantKept: []string{"GET /pets/{id}", "GET /pets/new", "POST /pets/{petId}", "GET /pets/v{id}", "GET /pets/{id}/x", "GET /pets/{a}/y", "GET /files", "GET /files/*", "GET /files/x", "GET /a/{x}/", "GET /a/{x}/{y}"},
			wantReasons: []string{
				"clashes with the parameter of Op1 at /pets/{id}",
			},
		},
		{
			name:        "A repeat and the trailing slash",
			routes:      routes("GET /pets/{id}", "GET /pets/{id}", "GET /pets/{id}/", "GET /x/", "GET /x"),
			wantKept:    []string{"GET /pets/{id}", "GET /x/"},
			wantReasons: []string{"repeats the route of Op1", "matches the same requests as Op1 at /pets/{id}", "matches the same requests as Op4 at /x/"},
		},
		{
			name:        "The parent of a catch-all",
			routes:      routes("GET /files/*", "GET /files", "GET /files/"),
			wantKept:    []string{"GET /files/*", "GET /files/"},
			wantReasons: []string{"matches the same requests as Op1 at /files/*"},
		},
		{
			name:        "Parameters after one prefix in one segment",
			routes:      routes("GET /pets/{id}.json", "GET /pets/{id}.xml", "GET /pets/v{id}", "GET /pets/v{x}", "GET /pets/w{x}"),
			wantKept:    []string{"GET /pets/{id}.json", "GET /pets/v{id}", "GET /pets/w{x}"},
			wantReasons: []string{"clashes with the parameter of Op1 at /pets/{id}.json", "clashes with the parameter of Op3 at /pets/v{id}"},
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

func TestClashes(t *testing.T) {
	t.Parallel()

	assert.True(t, clashes("/pets/{id}.json", "/pets/{id}.xml"))
	assert.False(t, clashes("/pets/{id}", "/pets/{id}"), "the same pattern differs in no segment")
	assert.False(t, clashes("/pets/{id}/x", "/pets/{a}/y"), "two segments differ")
	assert.False(t, clashes("/pets/{id}", "/pets/{id}/x"), "the segment counts differ")
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
