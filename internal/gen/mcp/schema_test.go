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

func TestInputBody(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	xml, jsonBody := gomodel.Content{MediaType: "application/xml", Type: pet}, gomodel.Content{MediaType: "application/json", Type: pet}
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
		{name: "A file cannot come in JSON", bodies: []gomodel.Content{{MediaType: "image/png", Type: fileType}}},
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
	tests := []struct {
		name    string
		body    *spec.RequestBody
		content gomodel.Content
		want    string
	}{
		{name: "The schema of the media type", body: withSchema, content: gomodel.Content{MediaType: "application/json"}, want: `{"type":"string"}`},
		{name: "JSON without a schema takes anything", body: withSchema, content: gomodel.Content{MediaType: "application/vnd.pet+json"}, want: `{}`},
		{name: "Text without a schema is a string", body: withSchema, content: gomodel.Content{MediaType: "text/plain"}, want: `{"type":"string"}`},
		{name: "Anything else is base64", content: gomodel.Content{MediaType: "image/png"}, want: `{"type":"string","contentEncoding":"base64"}`},
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
