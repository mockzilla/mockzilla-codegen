// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var photoBody = gomodel.DeclRef{Decl: &gomodel.Decl{Name: "PhotoBody", Kind: gomodel.KindAlias, Target: fileType}}

func TestInputBody(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	xml, jsonBody := gomodel.Content{MediaType: "application/xml", Type: pet}, gomodel.Content{MediaType: "application/json", Type: pet}
	photo := gomodel.Content{MediaType: "image/png", Type: gomodel.Pointer{Elem: photoBody}}
	tests := []struct {
		name      string
		bodies    []gomodel.Content
		want      gomodel.Content
		wantField string
		wantOK    bool
	}{
		{name: "No body"},
		{name: "The JSON body among several, with its field", bodies: []gomodel.Content{xml, jsonBody}, want: jsonBody, wantField: "BodyJSON", wantOK: true},
		{name: "The first body without JSON", bodies: []gomodel.Content{{MediaType: "text/plain", Type: str}, xml}, want: gomodel.Content{MediaType: "text/plain", Type: str}, wantField: "BodyText", wantOK: true},
		{name: "A file under an alias, which comes as base64", bodies: []gomodel.Content{photo}, want: photo, wantField: "Body", wantOK: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, field, ok := inputBody(&gomodel.Operation{Bodies: tc.bodies}, naming.New(nil))

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantField, field)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestBodySchema(t *testing.T) {
	t.Parallel()

	strSchema := &spec.Schema{Types: spec.TypeString}
	withSchema := &spec.RequestBody{Contents: []*spec.MediaType{{Name: "application/json", Schema: strSchema}, {Name: "text/plain"}}}
	binary := &spec.Schema{Types: spec.TypeString, Format: "binary"}
	webp := &spec.Schema{Types: spec.TypeString, Format: "binary", ContentMediaType: "image/webp"}
	files := &spec.RequestBody{Contents: []*spec.MediaType{{Name: "image/png", Schema: binary}, {Name: "application/octet-stream", Schema: webp}, {Name: "*/*", Schema: binary}, {Name: "application/json", Schema: binary}}}
	tests := []struct {
		name    string
		body    *spec.RequestBody
		content gomodel.Content
		want    string
	}{
		{name: "The schema of the media type", body: withSchema, content: gomodel.Content{MediaType: "application/json"}, want: `{"type":"string"}`},
		{name: "JSON without a schema takes anything", body: withSchema, content: gomodel.Content{MediaType: "application/vnd.pet+json"}, want: `{}`},
		{name: "Text without a schema is a string", body: withSchema, content: gomodel.Content{MediaType: "text/plain"}, want: `{"type":"string"}`},
		{name: "Anything else is base64 of its media type", content: gomodel.Content{MediaType: "image/png"}, want: `{"type":"string","contentEncoding":"base64","contentMediaType":"image/png"}`},
		{name: "A file is base64 of its media type", body: files, content: gomodel.Content{MediaType: "image/png", Type: photoBody}, want: `{"type":"string","format":"binary","contentEncoding":"base64","contentMediaType":"image/png"}`},
		{name: "The media type the schema names stays", body: files, content: gomodel.Content{MediaType: "application/octet-stream", Type: fileType}, want: `{"type":"string","format":"binary","contentEncoding":"base64","contentMediaType":"image/webp"}`},
		{name: "A file under a wildcard names no media type", body: files, content: gomodel.Content{MediaType: "*/*", Type: fileType}, want: `{"type":"string","format":"binary","contentEncoding":"base64"}`},
		{name: "A file in JSON names no media type", body: files, content: gomodel.Content{MediaType: "application/json", Type: fileType}, want: `{"type":"string","format":"binary","contentEncoding":"base64"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := jsonschema.NewBuilder()

			got := bodySchema(b, &gomodel.Operation{Spec: &spec.Operation{Body: tc.body}}, tc.content)

			assert.Equal(t, tc.want, string(b.Document(got)))
		})
	}
}
