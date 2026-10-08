// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var (
	textSchema  = &spec.Schema{Types: spec.TypeString}
	textList    = &spec.Schema{Types: spec.TypeArray, Items: textSchema}
	pointSchema = &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: "x", Schema: &spec.Schema{Types: spec.TypeInteger}}}}
)

func styleCollector() *collector {
	diags := &diag.Collector{}
	n := naming.New(nil)
	ext := newExtReader(n, diags)
	flat := newFlattener(ext, diags)
	r := readers{flat: flat, unions: newUnionReader(n, flat), ext: ext, formats: map[string]Type{"decimal": Builtin{Name: "string"}}}
	return newCollector(&spec.Document{}, r, diags)
}

func objectOf(name string, s *spec.Schema) *spec.Schema {
	return &spec.Schema{Types: spec.TypeObject, Properties: []*spec.Property{{Name: name, Schema: s}}}
}

func TestParamIssue(t *testing.T) {
	t.Parallel()

	node := &spec.Schema{Types: spec.TypeObject}
	node.Properties = []*spec.Property{{Name: "name", Schema: textSchema}, {Name: "child", Schema: node}}
	tests := []struct {
		name  string
		param spec.Parameter
		want  string
	}{
		{name: "A parameter with content has no style", param: spec.Parameter{In: spec.InQuery, Style: "odd", Contents: []*spec.MediaType{{Name: "application/json"}}}},
		{name: "A list of values", param: spec.Parameter{In: spec.InQuery, Style: "form", Schema: textList}},
		{
			name:  "A union with an object variant",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Schema: &spec.Schema{OneOf: []*spec.Schema{textSchema, pointSchema}}},
			want:  "is or holds a union of more than scalars",
		},
		{
			name:  "A style the location does not allow comes before the shape",
			param: spec.Parameter{In: spec.InQuery, Style: "label", Schema: &spec.Schema{Types: spec.TypeArray, Items: textList}},
			want:  "has style label, which OpenAPI allows only in path parameters",
		},
		{
			name:  "A list of lists",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Explode: true, Schema: &spec.Schema{Types: spec.TypeArray, Items: textList}},
			want:  "is a list of lists, which OpenAPI leaves to the implementation",
		},
		{
			name:  "A list of objects",
			param: spec.Parameter{In: spec.InHeader, Style: "simple", Schema: &spec.Schema{Types: spec.TypeArray, Items: pointSchema}},
			want:  "is a list of objects, which OpenAPI leaves to the implementation",
		},
		{name: "An exploded form object repeats the key of a list", param: spec.Parameter{In: spec.InQuery, Style: "form", Explode: true, Schema: objectOf("tags", textList)}},
		{name: "An exploded cookie object repeats the key of a list", param: spec.Parameter{In: spec.InCookie, Style: "cookie", Explode: true, Schema: objectOf("tags", textList)}},
		{
			name:  "A form object that is not exploded has no key to repeat",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Schema: objectOf("tags", textList)},
			want:  `holds a list in property "tags", which OpenAPI leaves to the implementation`,
		},
		{
			name:  "A simple object has no key to repeat",
			param: spec.Parameter{In: spec.InPath, Style: "simple", Explode: true, Schema: objectOf("tags", textList)},
			want:  `holds a list in property "tags", which OpenAPI leaves to the implementation`,
		},
		{
			name:  "An exploded form object holds no object",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Explode: true, Schema: objectOf("size", pointSchema)},
			want:  `holds an object in property "size", which OpenAPI leaves to the implementation`,
		},
		{
			name:  "An exploded form object holds no list of lists",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Explode: true, Schema: objectOf("grid", &spec.Schema{Types: spec.TypeArray, Items: textList})},
			want:  `holds a list of lists in property "grid", which OpenAPI leaves to the implementation`,
		},
		{
			name: "A map whose values are lists",
			param: spec.Parameter{In: spec.InQuery, Style: "form", Schema: &spec.Schema{
				Types: spec.TypeObject, AdditionalProperties: spec.Additional{Mode: spec.AdditionalSchema, Schema: textList},
			}},
			want: "holds a list in its map values, which OpenAPI leaves to the implementation",
		},
		{name: "A deep object nests objects and lists", param: spec.Parameter{In: spec.InQuery, Style: "deepObject", Schema: objectOf("size", objectOf("tags", textList))}},
		{name: "A deep object that refers to itself", param: spec.Parameter{In: spec.InQuery, Style: "deepObject", Schema: node}},
		{
			name:  "A deep object holds no list of objects at any depth",
			param: spec.Parameter{In: spec.InQuery, Style: "deepObject", Schema: objectOf("size", objectOf("points", &spec.Schema{Types: spec.TypeArray, Items: pointSchema}))},
			want:  `holds a list of objects in property "points", which OpenAPI leaves to the implementation`,
		},
		{name: "A schema with no type is not looked into", param: spec.Parameter{In: spec.InQuery, Style: "deepObject", Schema: &spec.Schema{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, styleCollector().paramIssue(&tc.param))
		})
	}
}

func TestParamShapeOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema *spec.Schema
		want   paramShape
	}{
		{name: "No schema", want: paramUnknown},
		{name: "A string", schema: textSchema, want: paramValue},
		{name: "An enum", schema: &spec.Schema{Types: spec.TypeString, Enum: []spec.Value{strVal("a")}}, want: paramValue},
		{name: "A format mapped to a type of its own", schema: &spec.Schema{Types: spec.TypeString, Format: "decimal"}, want: paramUnknown},
		{name: "A union of scalars", schema: &spec.Schema{OneOf: []*spec.Schema{textSchema, {Types: spec.TypeInteger}}}, want: paramValue},
		{name: "A union with an object variant", schema: &spec.Schema{OneOf: []*spec.Schema{textSchema, pointSchema}}, want: paramUnknown},
		{name: "A list", schema: textList, want: paramList},
		{name: "A struct", schema: pointSchema, want: paramObject},
		{name: "A map", schema: &spec.Schema{Types: spec.TypeObject}, want: paramObject},
		{name: "No type", schema: &spec.Schema{}, want: paramUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _ := styleCollector().paramShapeOf(tc.schema)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStyleIssue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param spec.Parameter
		shape paramShape
		want  string
	}{
		{name: "Matrix in a path", param: spec.Parameter{In: spec.InPath, Style: "matrix", Explode: true}, shape: paramObject},
		{name: "Label in a path", param: spec.Parameter{In: spec.InPath, Style: "label"}, shape: paramList},
		{name: "Simple in a path", param: spec.Parameter{In: spec.InPath, Style: "simple"}, shape: paramValue},
		{name: "Simple in a header, exploded", param: spec.Parameter{In: spec.InHeader, Style: "simple", Explode: true}, shape: paramObject},
		{name: "Form in a query, exploded", param: spec.Parameter{In: spec.InQuery, Style: "form", Explode: true}, shape: paramObject},
		{name: "Space delimited list", param: spec.Parameter{In: spec.InQuery, Style: "spaceDelimited"}, shape: paramList},
		{name: "Pipe delimited object", param: spec.Parameter{In: spec.InQuery, Style: "pipeDelimited"}, shape: paramObject},
		{name: "Deep object, its explode of no effect", param: spec.Parameter{In: spec.InQuery, Style: "deepObject"}, shape: paramObject},
		{name: "Deep object of a type not looked into", param: spec.Parameter{In: spec.InQuery, Style: "deepObject"}, shape: paramUnknown},
		{name: "Form cookie value, exploded", param: spec.Parameter{In: spec.InCookie, Style: "form", Explode: true}, shape: paramValue},
		{name: "Form cookie list, not exploded", param: spec.Parameter{In: spec.InCookie, Style: "form"}, shape: paramList},
		{name: "Cookie style list, exploded", param: spec.Parameter{In: spec.InCookie, Style: "cookie", Explode: true}, shape: paramList},
		{
			name:  "A style OpenAPI does not name",
			param: spec.Parameter{In: spec.InQuery, Style: "odd"},
			want:  `has style "odd", which OpenAPI does not define`,
		},
		{
			name:  "Form in a path",
			param: spec.Parameter{In: spec.InPath, Style: "form"},
			want:  "has style form, which OpenAPI allows only in query and cookie parameters",
		},
		{
			name:  "Anything but simple in a header",
			param: spec.Parameter{In: spec.InHeader, Style: "matrix"},
			want:  "has style matrix, which OpenAPI allows only in path parameters",
		},
		{
			name:  "Simple in a query",
			param: spec.Parameter{In: spec.InQuery, Style: "simple"},
			want:  "has style simple, which OpenAPI allows only in path and header parameters",
		},
		{
			name:  "Cookie style in a query",
			param: spec.Parameter{In: spec.InQuery, Style: "cookie"},
			want:  "has style cookie, which OpenAPI allows only in cookie parameters",
		},
		{
			name:  "Space delimited value",
			param: spec.Parameter{In: spec.InQuery, Style: "spaceDelimited"},
			shape: paramValue,
			want:  "has style spaceDelimited, which OpenAPI defines for a list or an object only",
		},
		{
			name:  "Pipe delimited, exploded",
			param: spec.Parameter{In: spec.InQuery, Style: "pipeDelimited", Explode: true},
			shape: paramList,
			want:  "has style pipeDelimited with explode true, which OpenAPI does not define",
		},
		{
			name:  "Deep object of a list",
			param: spec.Parameter{In: spec.InQuery, Style: "deepObject"},
			shape: paramList,
			want:  "has style deepObject, which OpenAPI defines for an object only",
		},
		{
			name:  "Deep object of a value",
			param: spec.Parameter{In: spec.InQuery, Style: "deepObject", Explode: true},
			shape: paramValue,
			want:  "has style deepObject, which OpenAPI defines for an object only",
		},
		{
			name:  "Form cookie object, exploded",
			param: spec.Parameter{In: spec.InCookie, Style: "form", Explode: true},
			shape: paramObject,
			want:  "has style form with explode true, which OpenAPI says writes the wrong delimiter for several cookie values (use style cookie)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, styleIssue(&tc.param, tc.shape))
		})
	}
}

func TestParamStylesGolden(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.IsServer, opts.HasResponseHeaders = true, true
	opts.OperationSuffixes = []string{"ServiceRequestOptions", "ResponseData"}
	checkGolden(t, "styles", "styles", opts)
}
