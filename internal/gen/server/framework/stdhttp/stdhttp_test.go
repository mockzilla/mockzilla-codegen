// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package stdhttp

import (
	"net/http"
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

	assert.Equal(t, "std-http", fw.Name())
	assert.Equal(t, framework.NetHTTP, fw.Family())
	assert.Equal(t, []gomodel.Import{{Path: "net/http"}}, fw.Imports())
	_, err := fw.Templates().Open("router.tmpl")
	require.NoError(t, err)
}

// TestRoutePattern checks the pattern of each path, and that ServeMux takes the patterns written.
func TestRoutePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{name: "Parameters become wildcards", path: "/pets/{id}/photos/{photoId}", want: "GET /pets/{id}/photos/{photoId}"},
		{name: "The root matches itself alone", path: "/", want: "GET /{$}"},
		{name: "A trailing slash matches itself alone", path: "/pets/", want: "GET /pets/{$}"},
		{name: "A trailing * takes the rest of the path", path: "/files/*", want: "GET /files/{rest...}"},
		{name: "A * elsewhere is a literal", path: "/files/*/meta", want: "GET /files/*/meta"},
		{name: "A parameter name that is no identifier", path: "/pets/{pet-id}/{1st}/{a b}", want: "GET /pets/{pet_id}/{_1st}/{a_b}"},
		{name: "An escaped literal", path: "/a%20b/{id}", want: "GET /a%20b/{id}"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "An empty segment", path: "/pets//photos", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "A dot segment", path: "/pets/../photos", wantErr: "the router rejects the path: it is not a clean path"},
		{name: "A parameter with a suffix", path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must fill its segment, unlike {id}.json"},
		{name: "Two parameters in one segment", path: "/pets/{a}{b}", wantErr: "the router rejects the path: a parameter must fill its segment, unlike {a}{b}"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a parameter must fill its segment, unlike {id"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A parameter named twice", path: "/pets/{id}/{id}", wantErr: `the router rejects the path: the wildcard "id" is named twice`},
		{name: "Two parameters with one wildcard name", path: "/pets/{a-b}/{a_b}", wantErr: `the router rejects the path: the wildcard "a_b" is named twice`},
		{name: "A parameter named like the rest", path: "/{rest}/*", wantErr: `the router rejects the path: the wildcard "rest" is named twice`},
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
			assert.NotPanics(t, func() { http.NewServeMux().Handle(got, http.NotFoundHandler()) })
		})
	}
}

// TestConflicts checks which routes are dropped, and that ServeMux takes the kept ones together
// and panics on each dropped one next to them.
func TestConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		routes      []framework.Route
		wantKept    []string
		wantReasons []string
	}{
		{
			name:     "Routes that differ in method or segment count",
			routes:   routes("GET /pets", "POST /pets", "GET /pets/{id}", "DELETE /pets/{petId}", "HEAD /pets/{id}", "GET /pets/{id}/{$}"),
			wantKept: []string{"GET /pets", "POST /pets", "GET /pets/{id}", "DELETE /pets/{petId}", "HEAD /pets/{id}", "GET /pets/{id}/{$}"},
		},
		{
			name:     "A literal is more specific than a wildcard, and both than the rest",
			routes:   routes("GET /pets/{id}", "GET /pets/mine", "GET /pets/{rest...}", "GET /{rest...}", "GET /{$}"),
			wantKept: []string{"GET /pets/{id}", "GET /pets/mine", "GET /pets/{rest...}", "GET /{rest...}", "GET /{$}"},
		},
		{
			name:        "A repeat",
			routes:      routes("GET /pets/{id}", "GET /pets/{id}"),
			wantKept:    []string{"GET /pets/{id}"},
			wantReasons: []string{"matches the same requests as Op1 at /pets/{id}"},
		},
		{
			name:        "Parameters named otherwise",
			routes:      routes("GET /pets/{id}", "GET /pets/{petId}", "GET /files/{path...}", "GET /files/{rest...}"),
			wantKept:    []string{"GET /pets/{id}", "GET /files/{path...}"},
			wantReasons: []string{"matches the same requests as Op1 at /pets/{id}", "matches the same requests as Op3 at /files/{path...}"},
		},
		{
			name:        "A HEAD route repeats a GET one",
			routes:      routes("HEAD /pets", "GET /pets", "GET /pets"),
			wantKept:    []string{"HEAD /pets", "GET /pets"},
			wantReasons: []string{"matches the same requests as Op2 at /pets"},
		},
		{
			name:        "The ambiguous pair of the net/http docs",
			routes:      routes("GET /a/{x}", "GET /{y}/b"),
			wantKept:    []string{"GET /a/{x}"},
			wantReasons: []string{"overlaps with Op1 at /a/{x}, and neither is more specific"},
		},
		{
			name:        "Ambiguous in the middle",
			routes:      routes("GET /a/{x}/c", "GET /{y}/b/{z}", "GET /{y}/b/c"),
			wantKept:    []string{"GET /a/{x}/c"},
			wantReasons: []string{"overlaps with Op1 at /a/{x}/c, and neither is more specific", "overlaps with Op1 at /a/{x}/c, and neither is more specific"},
		},
		{
			name:        "Ambiguous with the rest of the path",
			routes:      routes("GET /a/{x...}", "GET /{y}/b/c", "GET /{y}/b/{z...}"),
			wantKept:    []string{"GET /a/{x...}"},
			wantReasons: []string{"overlaps with Op1 at /a/{x...}, and neither is more specific", "overlaps with Op1 at /a/{x...}, and neither is more specific"},
		},
		{
			name:        "A GET route matches HEAD requests too",
			routes:      routes("GET /a/{x}", "HEAD /{y}/b"),
			wantKept:    []string{"GET /a/{x}"},
			wantReasons: []string{"overlaps with Op1 at /a/{x}, and neither is more specific"},
		},
		{
			name:        "The earliest conflicting route is named, whether it takes the rest or not",
			routes:      routes("GET /a/{x...}", "GET /b/{x}", "GET /{y}/c"),
			wantKept:    []string{"GET /a/{x...}", "GET /b/{x}"},
			wantReasons: []string{"overlaps with Op1 at /a/{x...}, and neither is more specific"},
		},
		{
			name:     "Nothing",
			wantKept: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kept, dropped := Framework{}.Conflicts(tc.routes)

			var keptPatterns, reasons []string
			for _, r := range kept {
				keptPatterns = append(keptPatterns, r.Pattern)
			}
			for _, c := range dropped {
				reasons = append(reasons, c.Reason)
			}
			assert.Equal(t, tc.wantKept, keptPatterns)
			assert.Equal(t, tc.wantReasons, reasons)

			mux := http.NewServeMux()
			for _, r := range kept {
				assert.NotPanics(t, func() { mux.Handle(r.Pattern, http.NotFoundHandler()) }, r.Pattern)
			}
			for _, c := range dropped {
				assert.Panics(t, func() { mux.Handle(c.Route.Pattern, http.NotFoundHandler()) }, c.Route.Pattern)
			}
		})
	}
}

func TestPathParam(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, `r.PathValue("petId")`, Framework{}.PathParam(s, "petId"))
	assert.Equal(t, `r.PathValue("pet_id")`, Framework{}.PathParam(s, "pet-id"))
	assert.Empty(t, s.Imports.Decl())
}

// routes are the routes of the patterns, named Op1, Op2 and so on, with the pattern's path as
// their path.
func routes(patterns ...string) []framework.Route {
	out := make([]framework.Route, len(patterns))
	for i, p := range patterns {
		method, path, _ := strings.Cut(p, " ")
		out[i] = framework.Route{Operation: "Op" + strconv.Itoa(i+1), Method: method, Path: path, Pattern: p}
	}
	return out
}
