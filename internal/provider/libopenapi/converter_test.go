// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// dumper prints Ref targets by pointer only, which keeps cycles finite.
type dumper struct {
	w           *strings.Builder
	withOrigins bool
}

func (d dumper) fields(v reflect.Value, depth int) {
	v = reflect.Indirect(v)
	for i := range v.NumField() {
		f, name := v.Field(i), v.Type().Field(i).Name
		if f.IsZero() || name == "Target" || (name == "Origin" && !d.withOrigins) {
			continue
		}

		pad := strings.Repeat("  ", depth)
		if line, ok := inline(f); ok {
			fmt.Fprintf(d.w, "%s%s: %s\n", pad, name, line)
			continue
		}
		fmt.Fprintf(d.w, "%s%s:\n", pad, name)
		d.block(f, depth+1)
	}
}

func (d dumper) block(v reflect.Value, depth int) {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Slice {
		d.fields(v, depth)
		return
	}

	pad := strings.Repeat("  ", depth)
	for i := range v.Len() {
		item := v.Index(i)
		if line, ok := inline(item); ok {
			fmt.Fprintf(d.w, "%s- %s\n", pad, line)
			continue
		}
		fmt.Fprintf(d.w, "%s-\n", pad)
		d.fields(item, depth+1)
	}
}

func TestParseGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "Info, servers, security and webhooks", fixture: "document"},
		{name: "Method order, merged path parameters, callbacks and security overrides", fixture: "operations"},
		{name: "Parameter styles, defaults and refs", fixture: "parameters"},
		{name: "Request bodies, encodings and refs", fixture: "bodies"},
		{name: "Response order, headers and refs", fixture: "responses"},
		{name: "Schema keywords, discriminators, deep refs and build errors", fixture: "schemas"},
		{name: "Recursive schemas", fixture: "cycles"},
		{name: "3.2 query, additional operations, item schemas and default mapping", fixture: "v32"},
		{name: "3.0 values of the wrong kind, unknown locations and unused path parameters", fixture: "invalid"},
		{name: "3.1 boolean schemas, 3.0 exclusive flags and parameters of webhooks", fixture: "invalid31"},
		{name: "Parameters, headers and properties named \"\"", fixture: "names"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, diags := parseFixture(t, tt.fixture+".yaml")
			golden(t, tt.fixture+".golden", dump(doc, true)+dumpDiagnostics(diags))
		})
	}
}

func TestParsePairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "3.0 nullable matches a 3.1 null type", fixture: "nullable"},
		{name: "3.0 boolean exclusive bounds match 3.1 numeric ones", fixture: "bounds"},
		{name: "3.0 example matches 3.1 examples", fixture: "examples"},
		{name: "Keywords next to a $ref are kept in both versions", fixture: "siblings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc30, diags30 := parseFixture(t, tt.fixture+".30.yaml")
			doc31, diags31 := parseFixture(t, tt.fixture+".31.yaml")
			assert.Equal(t, spec.V30, doc30.Version)
			assert.Equal(t, spec.V31, doc31.Version)
			assert.Empty(t, diags30)
			assert.Empty(t, diags31)

			doc30.Version, doc31.Version = 0, 0
			assert.Equal(t, dump(doc30, false), dump(doc31, false))
		})
	}
}

func TestParseCycles(t *testing.T) {
	t.Parallel()

	doc, _ := parseFixture(t, "cycles.yaml")
	schemas := map[string]*spec.Schema{}
	for _, s := range doc.Components.Schemas {
		schemas[s.Name] = s.Value
	}

	tests := []struct {
		name   string
		target *spec.Schema
		want   *spec.Schema
	}{
		{name: "Self reference", target: schemas["Node"].Properties[0].Schema.Ref.Target, want: schemas["Node"]},
		{name: "Mutual reference one way", target: schemas["A"].Properties[0].Schema.Ref.Target, want: schemas["B"]},
		{name: "Mutual reference back", target: schemas["B"].Properties[0].Schema.Ref.Target, want: schemas["A"]},
		{name: "Through array items", target: schemas["Tree"].Properties[0].Schema.Items.Ref.Target, want: schemas["Tree"]},
		{name: "Through a map", target: schemas["Dict"].AdditionalProperties.Schema.Ref.Target, want: schemas["Dict"]},
		{name: "Through allOf", target: schemas["Derived"].Properties[0].Schema.Ref.Target.AllOf[0].Ref.Target, want: schemas["Derived"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, tt.target)
		})
	}
}

func TestConverterPosition(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Line: 7, Column: 3}
	positions := map[string]diag.Origin{
		"":                        {File: "orig.yaml", Line: 1, Col: 1},
		"/components/schemas":     {File: "orig.yaml", Line: 20, Col: 3},
		"/components/schemas/Pet": {File: "pet.yaml", Line: 2, Col: 1},
	}
	tests := []struct {
		name      string
		positions map[string]diag.Origin
		ptr       string
		node      *yaml.Node
		want      diag.Origin
	}{
		{name: "Exact pointer", positions: positions, ptr: "/components/schemas/Pet", want: diag.Origin{File: "pet.yaml", Line: 2, Col: 1}},
		{name: "Nearest ancestor when the pointer is gone", positions: positions, ptr: "/components/schemas/Gone/properties/a", node: node, want: diag.Origin{File: "orig.yaml", Line: 20, Col: 3}},
		{name: "Root when nothing closer is known", positions: positions, ptr: "/paths/~1a", want: diag.Origin{File: "orig.yaml", Line: 1, Col: 1}},
		{name: "Node when no positions are given", ptr: "/components/schemas/Pet", node: node, want: diag.Origin{File: "spec.yaml", Line: 7, Col: 3}},
		{name: "Node when the positions miss every ancestor", positions: map[string]diag.Origin{"/x": {}}, ptr: "/a", node: node, want: diag.Origin{File: "spec.yaml", Line: 7, Col: 3}},
		{name: "Node for a pointer into another file", positions: map[string]diag.Origin{"/x": {}}, ptr: "other.yaml#", node: node, want: diag.Origin{File: "spec.yaml", Line: 7, Col: 3}},
		{name: "File only when there is no node", ptr: "/a", want: diag.Origin{File: "spec.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newConverter(spec.V31, provider.ParseOptions{File: "spec.yaml", Positions: tt.positions})
			assert.Equal(t, tt.want, c.position(tt.ptr, tt.node))
		})
	}
}

func TestConverterDocumentWithoutParts(t *testing.T) {
	t.Parallel()

	c := newConverter(spec.V31, provider.ParseOptions{})
	assert.Equal(t, &spec.Document{Version: spec.V31}, c.document(&v3.Document{}))
}

func TestBuildIssues(t *testing.T) {
	t.Parallel()

	circular := &index.ResolvingError{ErrorRef: errors.New("loop"), CircularReference: &index.CircularReferenceResult{}}
	err := errors.Join(errors.New("odd thing"), errors.Join(circular, errors.New("other thing")))
	cycles := []*index.CircularReferenceResult{
		{Journey: []*index.Reference{{Name: "A"}, {Name: "B"}}},
	}

	c := newConverter(spec.V31, provider.ParseOptions{File: "spec.yaml"})
	c.buildIssues(err, cycles)
	file := diag.Origin{File: "spec.yaml"}
	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: file, Message: "odd thing"},
		{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: file, Message: "other thing"},
		{Severity: diag.Info, Code: diag.CodeCircularRef, Origin: file, Message: "circular reference: A -> B"},
	}, c.diags.List())
}

func TestRefsShareTargets(t *testing.T) {
	t.Parallel()

	doc, _ := parseFixture(t, "schemas.yaml")
	schemas := map[string]*spec.Schema{}
	for _, s := range doc.Components.Schemas {
		schemas[s.Name] = s.Value
	}
	pathSchema := doc.Operations[0].Responses[0].Contents[0].Schema

	tests := []struct {
		name   string
		target *spec.Schema
		want   *spec.Schema
	}{
		{name: "Ref to a component", target: pathSchema.Items.Ref.Target, want: schemas["Pet"]},
		{name: "Ref next to sibling keywords", target: schemas["Described"].Ref.Target, want: schemas["Pet"]},
		{name: "Ref into a converted component", target: schemas["PetName"].Ref.Target, want: schemas["Pet"].Properties[1].Schema},
		{name: "Ref into a component converted later", target: schemas["Early"].Ref.Target, want: schemas["Late"].Properties[0].Schema},
		{name: "Ref into an operation", target: schemas["FromPath"].Ref.Target, want: pathSchema},
		{name: "Discriminator mapping", target: schemas["Shape"].Discriminator.Mapping[1].Ref.Target, want: schemas["Square"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, tt.target)
		})
	}
}

func TestComponentUsagesShareSchemas(t *testing.T) {
	t.Parallel()

	params, _ := parseFixture(t, "parameters.yaml")
	responses, _ := parseFixture(t, "responses.yaml")
	bodies, _ := parseFixture(t, "bodies.yaml")

	tests := []struct {
		name  string
		usage *spec.Schema
		want  *spec.Schema
	}{
		{name: "Parameter", usage: params.Operations[0].Params[7].Schema, want: params.Components.Parameters[0].Value.Schema},
		{name: "Parameter through an alias component", usage: params.Operations[0].Params[8].Schema, want: params.Components.Parameters[0].Value.Schema},
		{name: "Response", usage: responses.Operations[0].Responses[2].Contents[0].Schema, want: responses.Components.Responses[0].Value.Contents[0].Schema},
		{name: "Header", usage: responses.Operations[0].Responses[0].Headers[1].Schema, want: responses.Components.Headers[0].Value.Schema},
		{name: "Request body", usage: bodies.Operations[1].Body.Contents[0].Schema, want: bodies.Components.RequestBodies[0].Value.Contents[0].Schema},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, tt.usage)
		})
	}
}

func TestResponsesMissing(t *testing.T) {
	t.Parallel()

	c := newConverter(spec.V31, provider.ParseOptions{})
	assert.Nil(t, c.responses(nil, "/paths/~1a/get/responses"))
}

func parseFixture(t *testing.T, name string) (*spec.Document, []diag.Diagnostic) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	doc, diags, err := New().Parse(context.Background(), data, provider.ParseOptions{File: name})
	require.NoError(t, err)
	return doc, diags
}

// golden compares got with testdata/<name>; UPDATE=1 rewrites the file instead.
func golden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

func dump(v any, withOrigins bool) string {
	var b strings.Builder
	dumper{w: &b, withOrigins: withOrigins}.fields(reflect.ValueOf(v), 0)
	return b.String()
}

func dumpDiagnostics(diags []diag.Diagnostic) string {
	var b strings.Builder
	b.WriteString("Diagnostics:\n")
	for _, d := range diags {
		fmt.Fprintf(&b, "  - %s %s %s:%d:%d %s: %s\n", d.Severity, d.Code, d.Origin.File, d.Origin.Line, d.Origin.Col, d.Pointer, d.Message)
	}
	return b.String()
}

func inline(v reflect.Value) (string, bool) {
	switch x := v.Interface().(type) {
	case spec.Value:
		return jsonText(x), true
	case *spec.Value:
		return jsonText(*x), true
	case spec.TypeSet:
		return typeNames(x), true
	case spec.Origin:
		return fmt.Sprintf("%s:%d:%d %s", x.File, x.Line, x.Col, x.Pointer), true
	case []string:
		return "[" + strings.Join(x, ", ") + "]", true
	}

	switch v = reflect.Indirect(v); v.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int64:
		return fmt.Sprint(v.Interface()), true
	default:
		return "", false
	}
}

func jsonText(v spec.Value) string {
	switch v.Kind {
	case spec.KindNull:
		return "null"
	case spec.KindString:
		return strconv.Quote(v.Str)
	case spec.KindNumber:
		return string(v.Num)
	case spec.KindBool:
		return strconv.FormatBool(v.Bool)
	case spec.KindArray:
		parts := make([]string, len(v.Items))
		for i, item := range v.Items {
			parts[i] = jsonText(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		parts := make([]string, len(v.Fields))
		for i, f := range v.Fields {
			parts[i] = strconv.Quote(f.Name) + ": " + jsonText(f.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
}

func typeNames(t spec.TypeSet) string {
	var names []string
	for i, name := range []string{"string", "number", "integer", "boolean", "object", "array", "null"} {
		if t.Has(spec.TypeSet(1) << i) {
			names = append(names, name)
		}
	}
	return strings.Join(names, "|")
}
