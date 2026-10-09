// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestAdapterViewChecksBodies(t *testing.T) {
	t.Parallel()

	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{Name: "Name", JSONName: "name", Type: gomodel.Builtin{Name: "string"}, Required: true, Value: &gomodel.BodyValue{}},
	}}}
	op := &gomodel.Operation{
		Name:   "CreatePet",
		Spec:   &spec.Operation{Method: "POST", Path: "/pets"},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}, Body: &gomodel.BodyValue{Object: pet}}},
	}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet}, Operations: []*gomodel.Operation{op}}
	g, _ := New(m, allOptions())

	v := adapterView(g, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter))

	assert.Equal(t, "validation", v.Validation)
}

func TestBodyView(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	blob := &gomodel.Decl{Name: "Blob", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}}
	image := &gomodel.Decl{Name: "Image", Part: gomodel.PartTypes, Kind: gomodel.KindAlias, Target: fileType}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet, blob, image}}
	g, _ := New(m, allOptions())
	view := func(v BodyView) BodyView {
		v.Runtime, v.OperationID, v.IsRequired, v.Field, v.Target = "runtime", `"Op"`, true, "Body", "&opts.Body"
		if v.Case == "" {
			v.Case = v.MediaType
		}
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
			want:    view(BodyView{Kind: "form", MediaType: `"application/x-www-form-urlencoded"`, IsForm: true, Encoding: "nil"}),
		},
		{
			name:    "Multipart fills a struct",
			content: gomodel.Content{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "multipart", MediaType: `"multipart/form-data"`, IsMultipart: true, Type: "Pet", Encoding: "nil"}),
		},
		{
			name:    "A form with an encoding is read with it",
			content: gomodel.Content{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: pet}, Encoding: map[string]gomodel.Encoding{"a": {ContentType: "application/json"}, "b": {Style: "form", IsExplode: true}}},
			want: view(BodyView{
				Kind: "multipart", MediaType: `"multipart/form-data"`, IsMultipart: true, Type: "Pet", Encoding: "encoding",
				EncodingLiteral: "runtime.Encoding{\n\"a\": {ContentType: \"application/json\"},\n\"b\": {\nStyle: runtime.StyleForm,\nIsExplode: true,\n},\n}",
			}),
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
			name:    "Text into a number is its text",
			content: gomodel.Content{MediaType: "text/plain", Type: gomodel.Builtin{Name: "int"}},
			want:    view(BodyView{Kind: "value", MediaType: `"text/plain"`, IsValue: true}),
		},
		{
			name:    "Text into any is its text",
			content: gomodel.Content{MediaType: "text/*", Type: gomodel.Builtin{Name: "any"}},
			want:    view(BodyView{Kind: "value", MediaType: `"text/*"`, Case: `runtime.MediaRange(contentType) == "text/*"`, IsValue: true}),
		},
		{
			name:    "The text range into a struct is taken in as it is",
			content: gomodel.Content{MediaType: "text/*", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "none", MediaType: `"text/*"`, Case: `runtime.MediaRange(contentType) == "text/*"`}),
		},
		{
			name:    "The multipart range fills a struct",
			content: gomodel.Content{MediaType: "multipart/*", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "multipart", MediaType: `"multipart/*"`, Case: `runtime.MediaRange(contentType) == "multipart/*"`, IsMultipart: true, Type: "Pet", Encoding: "nil"}),
		},
		{
			name:    "Bytes into a defined byte slice",
			content: gomodel.Content{MediaType: "image/png", Type: gomodel.DeclRef{Decl: blob}},
			want:    view(BodyView{Kind: "bytes", MediaType: `"image/png"`, IsBytes: true, Assign: "Blob(data)"}),
		},
		{
			name:    "A file streams",
			content: gomodel.Content{MediaType: "image/png", Type: gomodel.DeclRef{Decl: image}},
			want:    view(BodyView{Kind: "file", MediaType: `"image/png"`, IsFile: true, Assign: "runtime.Ptr(Image(file))"}),
		},
		{
			name:    "Anything else into a struct is taken in as it is",
			content: gomodel.Content{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "none", MediaType: `"application/xml"`}),
		},
		{
			name:    "Any JSON into a struct",
			content: gomodel.Content{MediaType: "application/*+json", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "json", MediaType: `"application/*+json"`, Case: "runtime.IsJSON(contentType)", IsJSON: true}),
		},
		{
			name:    "A wildcard into a struct is JSON",
			content: gomodel.Content{MediaType: "*/*", Type: gomodel.DeclRef{Decl: pet}},
			want:    view(BodyView{Kind: "json", MediaType: `"*/*"`, IsJSON: true}),
		},
		{
			name:    "A wildcard into a string is text",
			content: gomodel.Content{MediaType: "text/*", Type: str},
			want:    view(BodyView{Kind: "text", MediaType: `"text/*"`, Case: `runtime.MediaRange(contentType) == "text/*"`, IsText: true, Assign: "runtime.Ptr(text)"}),
		},
		{
			name:    "A wildcard into a file streams",
			content: gomodel.Content{MediaType: "*/*", Type: gomodel.DeclRef{Decl: image}},
			want:    view(BodyView{Kind: "file", MediaType: `"*/*"`, IsFile: true, Assign: "runtime.Ptr(Image(file))"}),
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
			at := bodyAt{id: `"Op"`, isRequired: true, scope: fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter), table: newPresenceTable(nil, true, "runtime")}

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
			{MediaType: "application/*+json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "*/*+json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "text/*"},
			{MediaType: "Text/*"},
			{MediaType: "image/*"},
			{MediaType: ""},
		},
	}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet}, Operations: []*gomodel.Operation{op}}
	g, _ := New(m, allOptions())

	v := handlerView(g, op, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter), newPresenceTable(nil, true, "runtime"))

	fields := func(bodies []BodyView) []string {
		out := make([]string, len(bodies))
		for i, b := range bodies {
			out[i] = b.Case + " " + b.Field
		}
		return out
	}
	assert.True(t, v.HasBody)
	assert.Empty(t, v.Tag)
	assert.Equal(t, `contentType == ""`, v.Empty)
	assert.Equal(t, []string{`contentType == "application/json" BodyJSON`, `contentType == "text/json" BodyTextJSON`}, fields(v.Bodies))
	assert.Equal(t, []string{
		"runtime.IsJSON(contentType) BodyApplicationJSON2",
		`runtime.MediaRange(contentType) == "text/*" BodyText`,
		`runtime.MediaRange(contentType) == "image/*" BodyImage`,
	}, fields(v.Patterns))
	assert.Equal(t, []string{`"*/*" BodyAny`}, fields(v.Wildcards))
}

func TestHandlerViewSwitchesOnTheMediaTypeWithoutPatterns(t *testing.T) {
	t.Parallel()

	op := &gomodel.Operation{
		Name:   "PutNote",
		Spec:   &spec.Operation{Method: "PUT", Path: "/note"},
		Bodies: []gomodel.Content{{MediaType: "text/plain"}, {MediaType: "*/*"}},
	}
	m := &gomodel.Model{Operations: []*gomodel.Operation{op}}
	g, _ := New(m, allOptions())

	v := handlerView(g, op, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartAdapter), newPresenceTable(nil, true, "runtime"))

	assert.Equal(t, "contentType", v.Tag)
	assert.Equal(t, `""`, v.Empty)
	require.Len(t, v.Bodies, 1)
	assert.Equal(t, `"text/plain"`, v.Bodies[0].Case)
}
