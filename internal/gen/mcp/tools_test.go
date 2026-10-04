// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestIsRounded(t *testing.T) {
	t.Parallel()

	str := &gomodel.Field{Type: gomodel.Builtin{Name: "string"}}
	wide := &gomodel.Field{Type: gomodel.Builtin{Name: "int64"}}
	tests := []struct {
		name string
		tool *tool
		want bool
	}{
		{name: "No input is not rounded", tool: &tool{}},
		{name: "A string parameter is not rounded", tool: &tool{params: []param{{field: str}}}},
		{name: "An int64 parameter is rounded", tool: &tool{params: []param{{field: str}, {field: wide}}}, want: true},
		{name: "A text body is not rounded", tool: &tool{body: &body{content: gomodel.Content{MediaType: "text/plain"}}}},
		{name: "An int64 body is rounded", tool: &tool{body: &body{content: gomodel.Content{MediaType: "application/json", Type: wide.Type}}}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isRounded(tc.tool))
		})
	}
}

func TestTextResult(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}}
	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	tests := []struct {
		name        string
		typ         gomodel.Type
		want        string
		wantPointer bool
	}{
		{name: "A string is the text", typ: str, want: "out"},
		{name: "A pointer to a string is dereferenced", typ: gomodel.Pointer{Elem: str}, want: "*out", wantPointer: true},
		{name: "A defined string is converted", typ: note, want: "string(out)"},
		{name: "A pointer to a defined string is both", typ: gomodel.Pointer{Elem: note}, want: "string(*out)", wantPointer: true},
		{name: "A struct is no text", typ: gomodel.Pointer{Elem: pet}},
		{name: "Bytes are no text", typ: gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &gomodel.Model{}
			g, _ := New(m, testOptions())
			got, isPointer := textResult(tc.typ, fixture{m: m, g: g, cfg: "output: {file: ./gen.go}\n"}.scope(t, PartTools))

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantPointer, isPointer)
		})
	}
}

func TestFileResult(t *testing.T) {
	t.Parallel()

	photo := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Photo", Part: gomodel.PartTypes, Kind: gomodel.KindAlias, Target: fileType}}
	tests := []struct {
		name        string
		mediaType   string
		typ         gomodel.Type
		want        string
		wantPointer bool
	}{
		{name: "A file is dereferenced", mediaType: "*/*", typ: gomodel.Pointer{Elem: fileType}, want: "*out", wantPointer: true},
		{name: "An alias of a file is dereferenced", mediaType: "application/pdf", typ: gomodel.Pointer{Elem: photo}, want: "*out", wantPointer: true},
		{name: "Image bytes take their media type", mediaType: "image/png", typ: bytesType, want: `runtime.NewFile(out, "", "image/png")`},
		{name: "Audio bytes take their media type", mediaType: "audio/wav", typ: bytesType, want: `runtime.NewFile(out, "", "audio/wav")`},
		{name: "Bytes under a wildcard are no file", mediaType: "image/*", typ: bytesType},
		{name: "Other bytes are no file", mediaType: "application/octet-stream", typ: bytesType},
		{name: "A string is no file", mediaType: "image/svg+xml", typ: gomodel.Builtin{Name: "string"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &gomodel.Model{}
			g, _ := New(m, testOptions())
			got, isPointer := fileResult(tc.mediaType, tc.typ, fixture{m: m, g: g, cfg: "output: {file: ./gen.go}\n"}.scope(t, PartTools))

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantPointer, isPointer)
		})
	}
}
