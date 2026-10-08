// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestGroupView(t *testing.T) {
	t.Parallel()

	field := &gomodel.Field{Name: "IDs", Type: gomodel.Builtin{Name: "string"}}
	jsonContent := []*spec.MediaType{{Name: "application/json"}}
	tests := []struct {
		name  string
		param *spec.Parameter
		want  GroupView
	}{
		{
			name:  "A query parameter keeps reserved characters when it allows them",
			param: &spec.Parameter{Name: "ids", In: spec.InQuery, Style: "form", Explode: true, AllowReserved: true, Schema: &spec.Schema{}},
			want: GroupView{Field: "Query", Params: []ParamView{
				{Encoder: "QueryParam", Value: "opts.Query.IDs", Name: `"ids"`, Style: "StyleForm", IsExplode: true, IsReserved: true},
			}},
		},
		{
			name:  "A query parameter with content is written by its media type",
			param: &spec.Parameter{Name: "ids", In: spec.InQuery, AllowReserved: true, Contents: jsonContent},
			want: GroupView{Field: "Query", Params: []ParamView{
				{Encoder: "QueryParam", Value: "opts.Query.IDs", Name: `"ids"`, Style: "StyleForm", IsJSON: true},
			}},
		},
		{
			name:  "A form cookie keeps them too",
			param: &spec.Parameter{Name: "ids", In: spec.InCookie, Style: "form", AllowReserved: true, Schema: &spec.Schema{}},
			want: GroupView{Field: "Cookies", Params: []ParamView{
				{Encoder: "CookieParam", Value: "opts.Cookies.IDs", Name: `"ids"`, Style: "StyleForm", IsReserved: true},
			}},
		},
		{
			name:  "A cookie-style cookie is never escaped",
			param: &spec.Parameter{Name: "ids", In: spec.InCookie, Style: "cookie", AllowReserved: true, Schema: &spec.Schema{}},
			want: GroupView{Field: "Cookies", Params: []ParamView{
				{Encoder: "CookieParam", Value: "opts.Cookies.IDs", Name: `"ids"`, Style: "StyleCookie"},
			}},
		},
		{
			name:  "A path parameter escapes them",
			param: &spec.Parameter{Name: "ids", In: spec.InPath, Style: "simple", AllowReserved: true, Schema: &spec.Schema{}},
			want: GroupView{Field: "PathParams", Params: []ParamView{
				{Encoder: "PathParam", Value: "opts.PathParams.IDs", Name: `"ids"`, Style: "StyleSimple", IsRequired: true},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, _ := New(&gomodel.Model{}, allOptions())
			group := gomodel.ParamGroup{In: tc.param.In, Decl: &gomodel.Decl{Struct: &gomodel.Struct{Fields: []*gomodel.Field{field}}}, Params: []*spec.Parameter{tc.param}}

			assert.Equal(t, tc.want, groupView(g, group))
		})
	}
}

func TestBodyView(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}}
	blob := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Blob", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: bytesType}}
	image := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Image", Part: gomodel.PartTypes, Kind: gomodel.KindAlias, Target: fileType}}
	tests := []struct {
		name    string
		content gomodel.Content
		want    BodyView
	}{
		{name: "JSON", content: gomodel.Content{MediaType: "application/vnd.pet+json", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/vnd.pet+json"`}},
		{name: "JSON without a schema", content: gomodel.Content{MediaType: "application/json"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/json"`}},
		{name: "JSON with parameters goes under them", content: gomodel.Content{MediaType: "application/json; charset=utf-8", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/json; charset=utf-8"`}},
		{name: "A form", content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FormBody", Value: "opts.Body", Encoding: "nil"}},
		{name: "A form in another case", content: gomodel.Content{MediaType: "Application/X-WWW-Form-Urlencoded; charset=utf-8", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FormBody", Value: "opts.Body", Encoding: "nil"}},
		{name: "Multipart into a struct", content: gomodel.Content{MediaType: "multipart/form-data", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "MultipartBody", Value: "opts.Body", Encoding: "nil"}},
		{name: "Multipart with an encoding", content: gomodel.Content{MediaType: "multipart/form-data", Type: pet, Encoding: map[string]string{"tag": "application/json"}}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "MultipartBody", Value: "opts.Body", Encoding: `runtime.Encoding{"tag": "application/json"}`}},
		{name: "Multipart without a schema is bytes", content: gomodel.Content{MediaType: "multipart/form-data"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"multipart/form-data"`}},
		{name: "A file streams", content: gomodel.Content{MediaType: "image/png", Type: fileType}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `"image/png"`}},
		{name: "A file under an alias streams", content: gomodel.Content{MediaType: "image/png", Type: image}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `"image/png"`}},
		{name: "A string with a schema is a pointer", content: gomodel.Content{MediaType: "text/plain", Type: str}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "*opts.Body", MediaType: `"text/plain"`}},
		{name: "A string without a schema is checked for emptiness", content: gomodel.Content{MediaType: "text/csv"}, want: BodyView{IsSet: `opts.Body != ""`, Encoder: "TextBody", Value: "opts.Body", MediaType: `"text/csv"`}},
		{name: "A defined string is converted", content: gomodel.Content{MediaType: "text/markdown", Type: note}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "string(*opts.Body)", MediaType: `"text/markdown"`}},
		{name: "Bytes without a schema", content: gomodel.Content{MediaType: "image/png"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"image/png"`}},
		{name: "Defined bytes are converted", content: gomodel.Content{MediaType: "image/png", Type: blob}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "[]byte(opts.Body)", MediaType: `"image/png"`}},
		{name: "A wildcard sends a struct as JSON", content: gomodel.Content{MediaType: "*/*", Type: pet}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body", MediaType: `"application/json"`}},
		{name: "A wildcard sends text as text", content: gomodel.Content{MediaType: "text/*", Type: str}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "TextBody", Value: "*opts.Body", MediaType: `"text/plain"`}},
		{name: "A wildcard sends bytes as bytes", content: gomodel.Content{MediaType: "*/*"}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "BytesBody", Value: "opts.Body", MediaType: `"application/octet-stream"`}},
		{name: "A wildcard streams a file under its own type", content: gomodel.Content{MediaType: "*/*", Type: fileType}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `""`}},
		{name: "A wildcard streams a file under an alias", content: gomodel.Content{MediaType: "*/*", Type: image}, want: BodyView{IsSet: "opts.Body != nil", Encoder: "FileBody", Value: "*opts.Body", MediaType: `""`}},
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

func TestIsSendable(t *testing.T) {
	t.Parallel()

	jsonBody := BodyView{IsSet: "opts.Body != nil", Encoder: "JSONBody", Value: "opts.Body"}
	xml := BodyView{IsSet: "opts.BodyXML != nil", Value: "opts.BodyXML", MediaType: `"application/xml"`}
	tests := []struct {
		name       string
		bodies     []BodyView
		isRequired bool
		want       bool
	}{
		{name: "No body", want: true},
		{name: "A required body without media types", isRequired: true, want: true},
		{name: "An optional body the client cannot send", bodies: []BodyView{xml}, want: true},
		{name: "A required body with one media type the client can send", bodies: []BodyView{xml, jsonBody}, isRequired: true, want: true},
		{name: "A required body the client cannot send", bodies: []BodyView{xml, xml}, isRequired: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isSendable(tc.bodies, tc.isRequired))
		})
	}
}

func TestSuccessBody(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	text, jsonBody := gomodel.Content{MediaType: "text/plain", Type: str}, gomodel.Content{MediaType: "application/json", Type: str}
	xml := gomodel.Content{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Kind: gomodel.KindStruct}}}
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
			name:      "A body the client cannot decode is passed over",
			responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{xml}}, {Status: "201", Contents: []gomodel.Content{xml, text}}},
			want:      gomodel.Response{Status: "201", Contents: []gomodel.Content{xml, text}},
			wantBody:  text,
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

			r, c, ok := SuccessBody(&gomodel.Operation{Responses: tc.responses})

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, r)
			assert.Equal(t, tc.wantBody, c)
		})
	}
}

func TestAccept(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	text, jsonBody := gomodel.Content{MediaType: "text/plain", Type: str}, gomodel.Content{MediaType: "application/json", Type: str}
	events := gomodel.Content{MediaType: "text/event-stream", Type: str}
	problem := gomodel.Content{MediaType: "application/problem+json", Type: str}
	tests := []struct {
		name      string
		responses []gomodel.Response
		want      string
	}{
		{name: "No bodies", responses: []gomodel.Response{{Status: "204"}}},
		{
			name:      "The body the method returns first, then the order of the spec without repeats",
			responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{text, jsonBody}}, {Status: "404", Contents: []gomodel.Content{problem, text}}},
			want:      "application/json, text/plain, application/problem+json",
		},
		{
			name:      "Sequential media types are the stream's",
			responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{events, jsonBody}}},
			want:      "application/json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, accept(&gomodel.Operation{Responses: tc.responses}))
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
			assert.Equal(t, tc.wantStreamOnly, IsStreamOnly(op))
		})
	}
}
