// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestRejections(t *testing.T) {
	t.Parallel()

	problem := errorDecl("Problem", true, nil)
	invalid := errorDecl("Invalid", true, nil)
	either := errorDecl("Either", false, nil)
	plain := &gomodel.Decl{Name: "Plain", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	response := func(status string, contents ...gomodel.Content) gomodel.Response {
		return gomodel.Response{Status: status, Contents: contents}
	}
	content := func(mediaType string, d *gomodel.Decl) gomodel.Content {
		return gomodel.Content{MediaType: mediaType, Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: d}}}
	}
	body := []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: plain}}}

	tests := []struct {
		name        string
		bodies      []gomodel.Content
		responses   []gomodel.Response
		want        []rejection
		wantUnbuilt []*gomodel.Decl
	}{
		{name: "No error response"},
		{
			name:      "Default answers every status",
			bodies:    body,
			responses: []gomodel.Response{response("200"), response("default", content("application/problem+json", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/problem+json"}},
		},
		{
			name:      "A range in lower case",
			responses: []gomodel.Response{response("4xx", content("application/json", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "Only 400 when there is no body",
			responses: []gomodel.Response{response("400", content("application/json", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "A type of 400 does not answer a 415",
			bodies:    body,
			responses: []gomodel.Response{response("400", content("application/json", problem))},
			want:      []rejection{{status: 400, decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "Each status its own type",
			bodies:    body,
			responses: []gomodel.Response{response("400", content("application/json", invalid)), response("4XX", content("application/json", problem))},
			want:      []rejection{{status: 400, decl: invalid, mediaType: "application/json"}, {status: 415, decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "A wildcard body is never a 415",
			bodies:    []gomodel.Content{{MediaType: "*/*", Type: gomodel.DeclRef{Decl: plain}}},
			responses: []gomodel.Response{response("400", content("application/json", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "A range of one type can be a 415",
			bodies:    []gomodel.Content{{MediaType: "text/*", Type: gomodel.Builtin{Name: "string"}}},
			responses: []gomodel.Response{response("400", content("application/json", invalid)), response("4XX", content("application/json", problem))},
			want:      []rejection{{status: 400, decl: invalid, mediaType: "application/json"}, {status: 415, decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "Any JSON can be a 415",
			bodies:    []gomodel.Content{{MediaType: "application/*+json", Type: gomodel.DeclRef{Decl: plain}}},
			responses: []gomodel.Response{response("400", content("application/json", invalid)), response("4XX", content("application/json", problem))},
			want:      []rejection{{status: 400, decl: invalid, mediaType: "application/json"}, {status: 415, decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "An error type under a range answers as JSON",
			responses: []gomodel.Response{response("400", content("*/*", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/json"}},
		},
		{
			name:      "The first media type of an error type",
			responses: []gomodel.Response{response("default", content("text/plain", plain), content("application/problem+json", problem))},
			want:      []rejection{{decl: problem, mediaType: "application/problem+json"}},
		},
		{
			name:      "A response without an error type is not passed over",
			responses: []gomodel.Response{response("400", content("application/json", plain)), response("default", content("application/json", problem))},
		},
		{
			name:        "A type without a constructor",
			responses:   []gomodel.Response{response("default", content("application/json", either))},
			wantUnbuilt: []*gomodel.Decl{either},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, unbuilt := rejections(&gomodel.Operation{Name: "Op", Spec: &spec.Operation{}, Bodies: tc.bodies, Responses: tc.responses})

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantUnbuilt, unbuilt)
		})
	}
}

func TestRejectViews(t *testing.T) {
	t.Parallel()

	problem := errorDecl("Problem", true, nil)
	invalid := errorDecl("Invalid", true, nil)
	content := func(d *gomodel.Decl) gomodel.Content {
		return gomodel.Content{MediaType: "application/json", Type: gomodel.DeclRef{Decl: d}}
	}
	body := []gomodel.Content{{MediaType: "application/json", Type: gomodel.Builtin{Name: "string"}}}
	m := &gomodel.Model{Decls: []*gomodel.Decl{problem, invalid}, Operations: []*gomodel.Operation{
		{Name: "ListPets", Spec: &spec.Operation{Method: "GET", Path: "/pets"}, Responses: []gomodel.Response{{Status: "default", Contents: []gomodel.Content{content(problem)}}}},
		{Name: "Ping", Spec: &spec.Operation{Method: "GET", Path: "/ping"}},
		{Name: "PutPet", Spec: &spec.Operation{Method: "PUT", Path: "/pets"}, Bodies: body, Responses: []gomodel.Response{
			{Status: "400", Contents: []gomodel.Content{content(invalid)}},
			{Status: "default", Contents: []gomodel.Content{content(problem)}},
		}},
		{Name: "GetPet", Spec: &spec.Operation{Method: "GET", Path: "/pets/1"}, Responses: []gomodel.Response{{Status: "4XX", Contents: []gomodel.Content{content(problem)}}}},
	}}
	g, _ := New(m, allOptions())

	got := rejectViews(m.Operations, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter))

	assert.Equal(t, []RejectView{
		{IDs: []string{`"ListPets"`, `"GetPet"`}, Cases: []RejectCaseView{{New: "NewProblem", MediaType: `"application/json"`}}},
		{IDs: []string{`"PutPet"`}, Cases: []RejectCaseView{
			{Status: 400, New: "NewInvalid", MediaType: `"application/json"`},
			{Status: 415, New: "NewProblem", MediaType: `"application/json"`},
		}},
	}, got)
}

func TestRejectWarnings(t *testing.T) {
	t.Parallel()

	strict := errorDecl("Strict", true, []string{"title", "status"})
	either := errorDecl("Either", false, nil)
	responses := func(d *gomodel.Decl) []gomodel.Response {
		return []gomodel.Response{{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: d}}}}}
	}
	ops := []*gomodel.Operation{
		{Name: "A", Spec: &spec.Operation{}, Responses: responses(strict)},
		{Name: "B", Spec: &spec.Operation{}, Responses: responses(either)},
		{Name: "C", Spec: &spec.Operation{}, Responses: responses(strict)},
		{Name: "D", Spec: &spec.Operation{}, Responses: responses(either)},
	}

	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: "error-mapping", Pointer: "#/Strict", Message: "Strict answers requests the server turns away with detail set and these required properties empty: title, status"},
		{Severity: diag.Warning, Code: "error-mapping", Pointer: "#/Either", Message: `Either has no constructor, so requests the server turns away are answered with {"error": ...}`},
	}, rejectWarnings(ops))
}

func errorDecl(name string, hasConstructor bool, unset []string) *gomodel.Decl {
	return &gomodel.Decl{
		ID:     "#/" + name,
		Name:   name,
		Part:   gomodel.PartTypes,
		Kind:   gomodel.KindStruct,
		Struct: &gomodel.Struct{},
		Error:  &gomodel.ErrorMessage{Path: "detail", HasConstructor: hasConstructor, Unset: unset},
	}
}
