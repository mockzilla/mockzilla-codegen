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

func TestBodyView(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}}
	blob := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Blob", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: bytesType}}
	tests := []struct {
		name    string
		content gomodel.Content
		want    BodyView
	}{
		{name: "JSON", content: gomodel.Content{MediaType: "application/vnd.pet+json", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/vnd.pet+json"`}},
		{name: "JSON without a schema", content: gomodel.Content{MediaType: "application/json"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/json"`}},
		{name: "A form", content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FormBody", Value: "opts.Body"}},
		{name: "Multipart into a struct", content: gomodel.Content{MediaType: "multipart/form-data", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "MultipartBody", Value: "opts.Body"}},
		{name: "Multipart without a schema is bytes", content: gomodel.Content{MediaType: "multipart/form-data"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"multipart/form-data"`}},
		{name: "A file streams", content: gomodel.Content{MediaType: "image/png", Type: fileType}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `"image/png"`}},
		{name: "A string with a schema is a pointer", content: gomodel.Content{MediaType: "text/plain", Type: str}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "*opts.Body", MediaType: `"text/plain"`}},
		{name: "A string without a schema is checked for emptiness", content: gomodel.Content{MediaType: "text/csv"}, want: BodyView{IsSet: `opts.Body != ""`, Encoder: "TextBody", Value: "opts.Body", MediaType: `"text/csv"`}},
		{name: "A defined string is converted", content: gomodel.Content{MediaType: "text/markdown", Type: note}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "string(*opts.Body)", MediaType: `"text/markdown"`}},
		{name: "Bytes without a schema", content: gomodel.Content{MediaType: "image/png"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"image/png"`}},
		{name: "Defined bytes are converted", content: gomodel.Content{MediaType: "image/png", Type: blob}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "[]byte(opts.Body)", MediaType: `"image/png"`}},
		{name: "A wildcard sends a struct as JSON", content: gomodel.Content{MediaType: "*/*", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/json"`}},
		{name: "A wildcard sends text as text", content: gomodel.Content{MediaType: "text/*", Type: str}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "*opts.Body", MediaType: `"text/plain"`}},
		{name: "A wildcard sends bytes as bytes", content: gomodel.Content{MediaType: "*/*"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"application/octet-stream"`}},
		{name: "A wildcard streams a file under its own type", content: gomodel.Content{MediaType: "*/*", Type: fileType}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `""`}},
		{name: "XML into a struct cannot be sent", content: gomodel.Content{MediaType: "application/xml", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Value: "opts.Body", MediaType: `"application/xml"`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &gomodel.Model{}
			g, _ := New(m, allOptions())
			f := fixture{m: m, g: g, cfg: "output: {file: ./gen.go}\n"}

			assert.Equal(t, tc.want, bodyView(tc.content, "Body", f.scope(t, PartOperations)))
		})
	}
}

func TestSuccessBody(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	text, jsonBody := gomodel.Content{MediaType: "text/plain", Type: str}, gomodel.Content{MediaType: "application/json", Type: str}
	tests := []struct {
		name      string
		responses []gomodel.Response
		want      gomodel.Response
		wantBody  gomodel.Content
		wantOK    bool
	}{
		{name: "No responses"},
		{name: "No 2xx with a body", responses: []gomodel.Response{{Status: "204"}, {Status: "404", Contents: []gomodel.Content{jsonBody}}}},
		{
			name:      "The lowest 2xx with a body, whatever the order, with its JSON body",
			responses: []gomodel.Response{{Status: "204"}, {Status: "202", Contents: []gomodel.Content{text}}, {Status: "200", Contents: []gomodel.Content{text, jsonBody}}},
			want:      gomodel.Response{Status: "200", Contents: []gomodel.Content{text, jsonBody}},
			wantBody:  jsonBody,
			wantOK:    true,
		},
		{
			name:      "A range counts as its start",
			responses: []gomodel.Response{{Status: "201", Contents: []gomodel.Content{text}}, {Status: "2XX", Contents: []gomodel.Content{jsonBody}}},
			want:      gomodel.Response{Status: "2XX", Contents: []gomodel.Content{jsonBody}},
			wantBody:  jsonBody,
			wantOK:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, c, ok := successBody(&gomodel.Operation{Responses: tc.responses})

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, r)
			assert.Equal(t, tc.wantBody, c)
		})
	}
}

func TestErrorDecl(t *testing.T) {
	t.Parallel()

	problem := &gomodel.Decl{Name: "Problem", Kind: gomodel.KindStruct, Error: &gomodel.ErrorMessage{Path: "detail"}}
	alias := &gomodel.Decl{Name: "Failure", Kind: gomodel.KindAlias, Target: gomodel.DeclRef{Decl: problem}}
	plain := &gomodel.Decl{Name: "Locked", Kind: gomodel.KindStruct}
	tests := []struct {
		name string
		typ  gomodel.Type
		want *gomodel.Decl
	}{
		{name: "An error type", typ: gomodel.DeclRef{Decl: problem}, want: problem},
		{name: "Behind a pointer and an alias", typ: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: alias}}, want: problem},
		{name: "A struct that is no error", typ: gomodel.DeclRef{Decl: plain}},
		{name: "A builtin", typ: gomodel.Builtin{Name: "string"}},
		{name: "No type", typ: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, errorDecl(tc.typ))
		})
	}
}

func TestMethodExpr(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "http.MethodPatch", methodExpr("PATCH", "http"))
	assert.Equal(t, `"QUERY"`, methodExpr("QUERY", "http"))
}

func TestStreamBody(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	events := gomodel.Content{MediaType: "text/event-stream", Item: str}
	lines := gomodel.Content{MediaType: "application/x-ndjson", Item: str}
	jsonBody := gomodel.Content{MediaType: "application/json", Type: str}
	tests := []struct {
		name           string
		responses      []gomodel.Response
		want           gomodel.Response
		wantBody       gomodel.Content
		wantOK         bool
		wantStreamOnly bool
	}{
		{name: "No responses"},
		{name: "No sequential 2xx", responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{jsonBody}}, {Status: "500", Contents: []gomodel.Content{events}}}},
		{
			name:      "The lowest 2xx with a sequential body, with its first sequential body",
			responses: []gomodel.Response{{Status: "201", Contents: []gomodel.Content{jsonBody, lines, events}}, {Status: "200", Contents: []gomodel.Content{jsonBody}}},
			want:      gomodel.Response{Status: "201", Contents: []gomodel.Content{jsonBody, lines, events}},
			wantBody:  lines,
			wantOK:    true,
		},
		{
			name:           "Sequential bodies alone",
			responses:      []gomodel.Response{{Status: "200", Contents: []gomodel.Content{events}}, {Status: "204"}, {Status: "404", Contents: []gomodel.Content{jsonBody}}},
			want:           gomodel.Response{Status: "200", Contents: []gomodel.Content{events}},
			wantBody:       events,
			wantOK:         true,
			wantStreamOnly: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := &gomodel.Operation{Responses: tc.responses}
			r, c, ok := streamBody(op)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, r)
			assert.Equal(t, tc.wantBody, c)
			assert.Equal(t, tc.wantStreamOnly, isStreamOnly(op))
		})
	}
}

func TestFrameType(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	chunk := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Chunk", Kind: gomodel.KindStruct}}
	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Kind: gomodel.KindDefined, Target: str}}
	tests := []struct {
		name    string
		content gomodel.Content
		want    gomodel.Type
	}{
		{name: "A struct", content: gomodel.Content{Item: chunk}, want: chunk},
		{name: "Anything JSON", content: gomodel.Content{Item: gomodel.Builtin{Name: "any"}}, want: gomodel.Builtin{Name: "any"}},
		{name: "No schema is bytes", content: gomodel.Content{}, want: bytesType},
		{name: "A string is bytes", content: gomodel.Content{Item: gomodel.Pointer{Elem: str}}, want: bytesType},
		{name: "A defined string is bytes", content: gomodel.Content{Item: note}, want: bytesType},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, frameType(tc.content))
		})
	}
}
