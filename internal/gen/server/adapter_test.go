// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestBodyView(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	blob := &gomodel.Decl{Name: "Blob", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet, blob}}
	g, _ := New(m, allOptions())
	view := func(v BodyView) BodyView {
		v.Runtime, v.OperationID, v.IsRequired, v.Field, v.Target, v.Return = "runtime", `"Op"`, true, "Body", "&opts.Body", "return"
		return v
	}

	tests := []struct {
		name    string
		content gomodel.Content
		want    BodyView
	}{
		{
			name:    "JSON decodes into the field",
			content: gomodel.Content{MediaType: "application/vnd.api+json", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "json", MediaType: `"application/vnd.api+json"`, IsJSON: true}),
		},
		{
			name:    "Parameters and case of the media type are dropped",
			content: gomodel.Content{MediaType: "Application/JSON; charset=utf-8", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "json", MediaType: `"application/json"`, IsJSON: true}),
		},
		{
			name:    "A form decodes into the field",
			content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: gomodel.Map{Key: str, Elem: str}},
			want:    view(BodyView{Kind: "form", MediaType: `"application/x-www-form-urlencoded"`, IsForm: true}),
		},
		{
			name:    "Multipart fills a struct",
			content: gomodel.Content{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "multipart", MediaType: `"multipart/form-data"`, IsMultipart: true, Type: "Pet"}),
		},
		{
			name:    "Multipart without a struct is taken in as it is",
			content: gomodel.Content{MediaType: "multipart/form-data", Type: gomodel.Map{Key: str, Elem: str}},
			want:    view(BodyView{Kind: "none", MediaType: `"multipart/form-data"`}),
		},
		{
			name:    "Text into a string",
			content: gomodel.Content{MediaType: "text/csv", Type: str},
			want:    view(BodyView{Kind: "text", MediaType: `"text/csv"`, IsText: true, Assign: "runtime.Ptr(text)"}),
		},
		{
			name:    "Any other media type into a string is text",
			content: gomodel.Content{MediaType: "application/xml", Type: str},
			want:    view(BodyView{Kind: "text", MediaType: `"application/xml"`, IsText: true, Assign: "runtime.Ptr(text)"}),
		},
		{
			name:    "Text into anything else is taken in as it is",
			content: gomodel.Content{MediaType: "text/csv", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "none", MediaType: `"text/csv"`}),
		},
		{
			name:    "Bytes into a defined byte slice",
			content: gomodel.Content{MediaType: "image/png", Type: gomodel.DeclRef{Decl: blob}},
			want:    view(BodyView{Kind: "bytes", MediaType: `"image/png"`, IsBytes: true, Assign: "Blob(data)"}),
		},
		{
			name:    "Anything else into a struct is taken in as it is",
			content: gomodel.Content{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "none", MediaType: `"application/xml"`}),
		},
		{
			name:    "A wildcard into a struct is JSON",
			content: gomodel.Content{MediaType: "*/*", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "json", MediaType: `"*/*"`, IsJSON: true}),
		},
		{
			name:    "A wildcard into a string is text",
			content: gomodel.Content{MediaType: "text/*", Type: str},
			want:    view(BodyView{Kind: "text", MediaType: `"text/*"`, IsText: true, Assign: "runtime.Ptr(text)"}),
		},
		{
			name:    "A wildcard without a schema is bytes",
			content: gomodel.Content{MediaType: "*/*"},
			want:    view(BodyView{Kind: "bytes", MediaType: `"*/*"`, IsBytes: true, Assign: "data"}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// bodyView imports into the scope, so each subtest has its own.
			at := bodyAt{id: `"Op"`, isRequired: true, ret: "return", scope: fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter)}

			assert.Equal(t, tc.want, bodyView(tc.content, "Body", at))
		})
	}
}

func TestHandlerViewBodies(t *testing.T) {
	t.Parallel()

	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	op := &gomodel.Operation{
		Name: "PutPet",
		Spec: &spec.Operation{Method: "PUT", Path: "/pet"},
		Bodies: []gomodel.Content{
			{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "application/json; charset=utf-8", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "text/json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "*/*", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "application/*"},
			{MediaType: ""},
		},
	}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet}, Operations: []*gomodel.Operation{op}}
	g, _ := New(m, allOptions())

	v := handlerView(g, op, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter))

	fields := func(bodies []BodyView) []string {
		out := make([]string, len(bodies))
		for i, b := range bodies {
			out[i] = b.MediaType + " " + b.Field
		}
		return out
	}
	assert.True(t, v.HasBody)
	assert.Equal(t, []string{`"application/json" BodyJSON`, `"text/json" BodyTextJSON`}, fields(v.Bodies))
	assert.Equal(t, []string{`"*/*" BodyAny`}, fields(v.Wildcards))
}

func TestErrorVar(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "problem", errorVar("Problem", 0))
	assert.Equal(t, "data2", errorVar("Data", 1))
}
