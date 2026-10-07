// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestEnvelopeFields(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	headers := &gomodel.Decl{Name: "Headers", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	op := &gomodel.Operation{Responses: []gomodel.Response{
		{Status: "200", Contents: []gomodel.Content{
			{MediaType: "application/json", Type: pet},
			{MediaType: "application/xml", Type: pet},
			{MediaType: "text/xml", Type: str},
			{MediaType: "application/xml; charset=utf-8", Type: str},
			{MediaType: "text/xml; q=1", Type: str},
			{MediaType: "text/xml; q=2", Type: str},
			{MediaType: "text/plain"},
			{MediaType: "text/stream", Type: str},
			{MediaType: "text/event-stream", Item: pet},
		}, Headers: headers},
		{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json"}}},
	}}
	m := &gomodel.Model{}
	g, _ := New(m, allOptions())
	f := fixture{m: m, g: g, cfg: "output: {file: ./gen.go}\n"}

	fields := envelopeFields(g, op)

	type row struct {
		FieldView
		status    string
		mediaType string
		isStream  bool
		isHeaders bool
	}
	s := f.scope(t, PartResponses)
	got := make([]row, len(fields))
	for i, field := range fields {
		got[i] = row{FieldView: field.view(s), status: field.status, mediaType: field.mediaType, isStream: field.isStream, isHeaders: field.isHeaders}
	}
	assert.Equal(t, []row{
		{FieldView: FieldView{Name: "JSON200", Type: "*Pet", Doc: "JSON200 is the body of a 200 response as application/json."}, status: "200", mediaType: "application/json"},
		{FieldView: FieldView{Name: "XML200", Type: "*string", Doc: "XML200 is the body of a 200 response as text/xml."}, status: "200", mediaType: "text/xml"},
		{FieldView: FieldView{Name: "ApplicationXML200", Type: "*string", Doc: "ApplicationXML200 is the body of a 200 response as application/xml; charset=utf-8."}, status: "200", mediaType: "application/xml; charset=utf-8"},
		{FieldView: FieldView{Name: "TextXML200", Type: "*string", Doc: "TextXML200 is the body of a 200 response as text/xml; q=1."}, status: "200", mediaType: "text/xml; q=1"},
		{FieldView: FieldView{Name: "TextXML2002", Type: "*string", Doc: "TextXML2002 is the body of a 200 response as text/xml; q=2."}, status: "200", mediaType: "text/xml; q=2"},
		{FieldView: FieldView{Name: "Text200", Type: "*string", Doc: "Text200 is the body of a 200 response as text/plain."}, status: "200", mediaType: "text/plain"},
		{FieldView: FieldView{Name: "TextStream200", Type: "*string", Doc: "TextStream200 is the body of a 200 response as text/stream."}, status: "200", mediaType: "text/stream"},
		{FieldView: FieldView{Name: "EventStream200", Type: "*string", Doc: "EventStream200 is the body of a 200 response as text/event-stream."}, status: "200", mediaType: "text/event-stream"},
		{FieldView: FieldView{Name: "JSONDefault", Type: "any", Doc: "JSONDefault is the body of a default response as application/json."}, status: "default", mediaType: "application/json"},
		{FieldView: FieldView{Name: "Stream200", Type: "*httpclient.Stream[Pet]", Doc: "Stream200 is the stream of a 200 response as text/event-stream."}, status: "200", mediaType: "text/event-stream", isStream: true},
		{FieldView: FieldView{Name: "Headers200", Type: "*Headers", Doc: "Headers200 holds the headers the spec declares for a 200 response."}, status: "200", isHeaders: true},
	}, got)
}

func TestIsDecodable(t *testing.T) {
	t.Parallel()

	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	image := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Image", Kind: gomodel.KindAlias, Target: fileType}}
	tests := []struct {
		name    string
		content gomodel.Content
		want    bool
	}{
		{name: "JSON into a struct", content: gomodel.Content{MediaType: "application/json", Type: pet}, want: true},
		{name: "JSON with parameters", content: gomodel.Content{MediaType: "Application/JSON; charset=utf-8", Type: pet}, want: true},
		{name: "A form into a struct", content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: pet}, want: true},
		{name: "A wildcard into a struct", content: gomodel.Content{MediaType: "*/*", Type: pet}, want: true},
		{name: "Text into a string", content: gomodel.Content{MediaType: "text/html", Type: gomodel.Builtin{Name: "string"}}, want: true},
		{name: "Anything into bytes", content: gomodel.Content{MediaType: "image/png"}, want: true},
		{name: "Anything into a file", content: gomodel.Content{MediaType: "application/pdf", Type: image}, want: true},
		{name: "XML into a struct", content: gomodel.Content{MediaType: "application/xml", Type: pet}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isDecodable(tc.content))
		})
	}
}
