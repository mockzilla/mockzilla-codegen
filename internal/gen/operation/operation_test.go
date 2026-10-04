// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package operation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestGroupField(t *testing.T) {
	t.Parallel()

	n := naming.New(nil)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "Path", in: spec.InPath, want: "PathParams"},
		{name: "Query", in: spec.InQuery, want: "Query"},
		{name: "Header", in: spec.InHeader, want: "Headers"},
		{name: "Cookie", in: spec.InCookie, want: "Cookies"},
		{name: "A location the spec does not know", in: "body-ref", want: "BodyRef"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, GroupField(tc.in, n))
		})
	}
}

func TestBodyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mediaTypes []string
		want       []string
	}{
		{name: "No body"},
		{name: "One body", mediaTypes: []string{"application/xml"}, want: []string{"Body"}},
		{name: "Several bodies take their tags", mediaTypes: []string{"application/json", "text/plain"}, want: []string{"BodyJSON", "BodyText"}},
		{
			name:       "A tag used twice takes the type, then a number",
			mediaTypes: []string{"application/xml", "text/xml", "application/xml; charset=utf-8", "text/xml; q=1"},
			want:       []string{"BodyXML", "BodyTextXML", "BodyApplicationXML", "BodyTextXML2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var bodies []gomodel.Content
			for _, mt := range tc.mediaTypes {
				bodies = append(bodies, gomodel.Content{MediaType: mt})
			}

			got := BodyFields(bodies, naming.New(nil))

			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBodyType(t *testing.T) {
	t.Parallel()

	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Kind: gomodel.KindStruct}}
	tests := []struct {
		name    string
		content gomodel.Content
		want    gomodel.Type
	}{
		{name: "A struct is held by pointer", content: gomodel.Content{MediaType: "application/json", Type: pet}, want: gomodel.Pointer{Elem: pet}},
		{name: "A slice is held as it is", content: gomodel.Content{MediaType: "application/json", Type: gomodel.Slice{Elem: pet}}, want: gomodel.Slice{Elem: pet}},
		{name: "JSON without a schema is any", content: gomodel.Content{MediaType: "application/problem+json"}, want: gomodel.Builtin{Name: "any"}},
		{name: "Text without a schema is a string", content: gomodel.Content{MediaType: "text/csv"}, want: gomodel.Builtin{Name: "string"}},
		{name: "Anything else is bytes", content: gomodel.Content{MediaType: "image/png"}, want: gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, BodyType(tc.content))
		})
	}
}

func TestFirstBody(t *testing.T) {
	t.Parallel()

	xml, jsonBody := gomodel.Content{MediaType: "text/xml"}, gomodel.Content{MediaType: "application/json"}
	tests := []struct {
		name     string
		contents []gomodel.Content
		want     gomodel.Content
		wantOK   bool
	}{
		{name: "No contents"},
		{name: "JSON wins", contents: []gomodel.Content{xml, jsonBody}, want: jsonBody, wantOK: true},
		{name: "Else the first", contents: []gomodel.Content{xml}, want: xml, wantOK: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := FirstBody(tc.contents)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param *spec.Parameter
		want  string
	}{
		{name: "A style the runtime knows", param: &spec.Parameter{In: spec.InQuery, Style: "pipeDelimited"}, want: "StylePipeDelimited"},
		{name: "No style takes the default of the location", param: &spec.Parameter{In: spec.InCookie}, want: "StyleForm"},
		{name: "An unknown style takes the default of the location", param: &spec.Parameter{In: spec.InPath, Style: "odd"}, want: "StyleSimple"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Style(tc.param))
		})
	}
}

func TestIsJSONParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param *spec.Parameter
		want  bool
	}{
		{name: "A schema", param: &spec.Parameter{Schema: &spec.Schema{}}},
		{name: "JSON content", param: &spec.Parameter{Contents: []*spec.MediaType{{Name: "application/json"}}}, want: true},
		{name: "Other content", param: &spec.Parameter{Contents: []*spec.MediaType{{Name: "text/plain"}}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, IsJSONParam(tc.param))
		})
	}
}

func TestStatusOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   int
	}{
		{name: "Code", status: "404", want: 404},
		{name: "Zero is a code", status: "0"},
		{name: "Range takes its start", status: "4XX", want: 400},
		{name: "Default", status: "default", want: 500},
		{name: "Key that is no status", status: "ok", want: 500},
		{name: "Digit and one more character are no range", status: "4X", want: 500},
		{name: "Empty key", status: "", want: 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, StatusOf(tc.status))
		})
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   *spec.Operation
		want string
	}{
		{name: "Nothing", op: &spec.Operation{}},
		{name: "Summary alone", op: &spec.Operation{Summary: "List pets"}, want: "List pets"},
		{name: "Description alone", op: &spec.Operation{Description: "Returns pets."}, want: "Returns pets."},
		{name: "Both", op: &spec.Operation{Summary: "List pets", Description: "Returns pets."}, want: "List pets\n\nReturns pets."},
		{name: "Both the same", op: &spec.Operation{Summary: "Same", Description: "Same"}, want: "Same"},
		{name: "Deprecated without a doc", op: &spec.Operation{Deprecated: true}, want: deprecatedNote},
		{name: "Deprecated with a doc", op: &spec.Operation{Summary: "Old", Deprecated: true}, want: "Old\n\n" + deprecatedNote},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Doc(tc.op))
		})
	}
}

func TestPartsOf(t *testing.T) {
	t.Parallel()

	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes}}
	params := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Query", Part: gomodel.PartParams}}
	str := gomodel.Builtin{Name: "string"}
	tests := []struct {
		name  string
		types []gomodel.Type
		want  []layout.PartID
	}{
		{name: "No types"},
		{name: "Builtins and nil are in no part", types: []gomodel.Type{str, nil, gomodel.Qualified{Name: "Time"}}},
		{
			name:  "Declarations through pointers, slices and maps, each part once and sorted",
			types: []gomodel.Type{params, gomodel.Pointer{Elem: pet}, gomodel.Slice{Elem: pet}, gomodel.Map{Key: str, Elem: params}},
			want:  []layout.PartID{gomodel.PartParams, gomodel.PartTypes},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, PartsOf(tc.types))
		})
	}
}
