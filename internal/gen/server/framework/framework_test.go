// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package framework

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

func TestHTTPHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, Handler{Signature: "(w http.ResponseWriter, r *http.Request)", Writer: "w", Request: "r", ServeSignature: "(w http.ResponseWriter, r *http.Request)"}, HTTPHandler(s))
	assert.Equal(t, `import "net/http"`, s.Imports.Decl())
}

func TestContextHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, Handler{
		Signature:             "(c echo.Context) error",
		Writer:                "c.Response()",
		Request:               "c.Request()",
		Epilogue:              "return nil",
		ServeSignature:        "(w http.ResponseWriter, r *http.Request)",
		Context:               "c",
		ContextServeSignature: "(c echo.Context, w http.ResponseWriter, r *http.Request)",
	}, ContextHandler(s, "echo.Context"))
	assert.Equal(t, `import "net/http"`, s.Imports.Decl())
}

func TestParams(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"owner", "id"}, Params("/owners/{owner}/pets/{id}.json"))
	assert.Nil(t, Params("/pets"))
}

func TestShape(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "GET /owners/{}/pets/{}", Shape("GET /owners/{owner}/pets/{id}"))
	assert.Equal(t, "/files/{}", Shape("/files/{path...}"))
}

func TestConflictsByShape(t *testing.T) {
	t.Parallel()

	get := Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	del := Route{Operation: "DeletePet", Method: "DELETE", Path: "/pets/{petId}", Pattern: "/pets/:petId"}
	again := Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	renamed := Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{animalId}", Pattern: "/pets/:animalId"}

	kept, dropped := ConflictsByShape([]Route{get, del, again, renamed})

	assert.Equal(t, []Route{get, del}, kept)
	assert.Equal(t, []Conflict{
		{Route: again, Reason: "repeats the route of GetPet"},
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
	}, dropped)
}

func TestConflictsByKey(t *testing.T) {
	t.Parallel()

	get := Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	slash := Route{Operation: "GetPetSlash", Method: "GET", Path: "/pets/{id}/", Pattern: "/pets/:id/"}
	again := Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	renamed := Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{animalId}", Pattern: "/pets/:animalId"}

	kept, dropped := ConflictsByKey([]Route{get, slash, again, renamed}, func(r Route) string {
		return r.Method + " " + strings.TrimSuffix(Shape(r.Path), "/")
	})

	assert.Equal(t, []Route{get}, kept)
	assert.Equal(t, []Conflict{
		{Route: slash, Reason: "matches the same requests as GetPet at /pets/{id}"},
		{Route: again, Reason: "repeats the route of GetPet"},
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
	}, dropped)
}

func TestStaticFirst(t *testing.T) {
	t.Parallel()

	files := Route{Operation: "Files", Path: "/files/*"}
	pet := Route{Operation: "Pet", Path: "/pets/{id}"}
	photo := Route{Operation: "Photo", Path: "/pets/{id}/photo"}
	newPet := Route{Operation: "NewPet", Path: "/pets/new"}
	index := Route{Operation: "Index", Path: "/files/index"}
	root := Route{Operation: "Root", Path: "/"}
	json := Route{Operation: "JSON", Path: "/pets/{id}.json"}
	policy := Route{Operation: "Policy", Path: "/pets/{id}:getPolicy"}
	in := []Route{files, pet, photo, newPet, index, root, json, policy}

	assert.Equal(t, []Route{root, newPet, index, policy, json, pet, photo, files}, StaticFirst(in))
	assert.Equal(t, []Route{files, pet, photo, newPet, index, root, json, policy}, in, "the routes given stay as they are")
}

func TestIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "An identifier stays", in: "petId_2", want: "petId_2"},
		{name: "Other characters become underscores", in: "pet-id.v1", want: "pet_id_v1"},
		{name: "A leading digit gets an underscore", in: "1st", want: "_1st"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Identifier(tc.in))
		})
	}
}

func TestCheckMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		wantErr string
	}{
		{name: "A method with a function on every router", method: "GET"},
		{name: "The last of them", method: "TRACE"},
		{name: "QUERY of OpenAPI 3.2", method: "QUERY", wantErr: "the router does not take the method QUERY"},
		{name: "A method a spec adds", method: "PURGE", wantErr: "the router does not take the method PURGE"},
		{name: "Lower case", method: "get", wantErr: "the router does not take the method get"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := CheckMethod(tc.method)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrMethod)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "A path with parameters and a wildcard", path: "/pets/{id}/*"},
		{name: "No leading slash", path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard not last", path: "/files/*/meta", wantErr: "the router rejects the path: * must be last"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Check(tc.path)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrPattern)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestColonPattern(t *testing.T) {
	t.Parallel()

	escaping := Colon{Literal: Escaping(":"), Name: Same, Wildcard: "*", IsPrefixAllowed: true}
	rejecting := Colon{Literal: Rejecting(":*"), Name: Identifier, Wildcard: "*", IsWildcardNamed: true}
	tests := []struct {
		name    string
		colon   Colon
		path    string
		want    string
		wantErr string
	}{
		{name: "Parameters and a prefix, with a colon escaped", colon: escaping, path: "/pets:search/v{id}/{photo-id}", want: `/pets\:search/v:id/:photo-id`},
		{name: "The wildcard", colon: escaping, path: "/files/*", want: "/files/*"},
		{name: "The root", colon: escaping, path: "/", want: "/"},
		{name: "A name made an identifier and the wildcard named", colon: rejecting, path: "/pets/{pet-id}/*", want: "/pets/:pet_id/*rest"},
		{name: "The wildcard takes a name no parameter has", colon: rejecting, path: "/pets/{rest}/*", want: "/pets/:rest/*rest_"},
		{name: "Two names the router gives one name", colon: rejecting, path: "/pets/{pet-id}/{pet_id}", wantErr: `the router rejects the path: parameters "pet-id" and "pet_id" are both pet_id on the router`},
		{name: "No leading slash", colon: escaping, path: "pets", wantErr: "the router rejects the path: it must begin with /"},
		{name: "A parameter with a suffix", colon: escaping, path: "/pets/{id}.json", wantErr: "the router rejects the path: a parameter must end its segment, unlike {id}.json"},
		{name: "Two parameters in one segment", colon: escaping, path: "/pets/{a}{b}", wantErr: "the router rejects the path: a parameter must end its segment, unlike {a}{b}"},
		{name: "A prefix where none is allowed", colon: rejecting, path: "/pets/v{id}", wantErr: "the router rejects the path: a parameter must fill its segment, unlike v{id}"},
		{name: "A parameter without a name", colon: escaping, path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A parameter named twice", colon: escaping, path: "/pets/{id}/{id}", wantErr: `the router rejects the path: parameter "id" is named twice`},
		{name: "A literal the router rejects", colon: rejecting, path: "/pets:search", wantErr: "the router rejects the path: : is read as the start of a parameter in pets:search"},
		{name: "A prefix the router rejects", colon: rejecting, path: "/a*b/{id}", wantErr: "the router rejects the path: * must be last"},
		{name: "A literal before a parameter the router rejects", colon: Colon{Literal: Rejecting("v"), Name: Same, IsPrefixAllowed: true}, path: "/pets/v{id}", wantErr: "the router rejects the path: v is read as the start of a parameter in v"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.colon.Pattern(tc.path)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrPattern)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBrace(t *testing.T) {
	t.Parallel()

	wildcard := func(name string) string { return "{" + name + ":.*}" }
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{name: "Parameters stay", path: "/pets/{id}/photos/{photo-id}.jpg", want: "/pets/{id}/photos/{photo-id}.jpg"},
		{name: "The wildcard", path: "/files/*", want: "/files/{rest:.*}"},
		{name: "The wildcard takes a name no parameter has", path: "/files/{rest}/*", want: "/files/{rest}/{rest_:.*}"},
		{name: "A colon in a name", path: "/geo/{lat:lng}", want: "/geo/{lat_lng}"},
		{name: "Unclosed brace", path: "/pets/{id", wantErr: "the router rejects the path: a { has no }"},
		{name: "Wildcard with a prefix", path: "/files*", wantErr: "the router rejects the path: * must be a segment of its own"},
		{name: "A parameter without a name", path: "/pets/{}", wantErr: "the router rejects the path: a parameter has no name"},
		{name: "A parameter named twice", path: "/pets/{id}/{id}", wantErr: `the router rejects the path: parameter "id" is named twice`},
		{name: "Two names the router gives one name", path: "/geo/{a:b}/{a_b}", wantErr: `the router rejects the path: parameters "a:b" and "a_b" are both a_b on the router`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Brace(tc.path, Unmarked, wildcard)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrPattern)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnmarked(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "lat_lng_x", Unmarked("lat:lng*x"))
	assert.Equal(t, "pet-id", Unmarked("pet-id"))
}

func TestRestName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "rest", RestName([]string{"id"}))
	assert.Equal(t, "rest__", RestName([]string{"rest", "rest_"}))
}

func TestEscapingColon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "A colon inside is escaped", in: "a:b", want: `a\:b`},
		{name: "A colon that begins the segment", in: ":tid", wantErr: "the router rejects the path: a segment beginning with : is read as a parameter"},
		{name: "A star", in: "a*", wantErr: "the router rejects the path: * is read as a wildcard in a*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EscapingColon(tc.in)

			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrPattern)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
