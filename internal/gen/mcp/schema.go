// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The JSON schema of a tool's input: one property per parameter and one for the body.

package mcp

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

var (
	fileType  = gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
	bytesType = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
)

// inputSchema is the JSON schema of the input of t: an object with one property per parameter,
// named as the spec names it, and one for the body. No other property is allowed, so a misspelled
// parameter is an error and not silently dropped. The warnings are those of the builder.
func inputSchema(t *tool) (string, []diag.Diagnostic) {
	b := jsonschema.NewBuilder()
	props := &jsonschema.Object{}
	var required []string
	for _, p := range t.params {
		s := b.Schema(paramSchema(p.spec))
		if p.spec.Description != "" {
			s.Set("description", p.spec.Description)
		}
		if p.spec.Deprecated {
			s.Set("deprecated", true)
		}
		props.Set(p.name, s)
		if p.spec.Required || p.spec.In == spec.InPath {
			required = append(required, p.name)
		}
	}
	if q := t.queryString; q != nil {
		p := t.op.QueryString.Param
		s := &jsonschema.Object{}
		if schema := p.Contents[0].Schema; schema != nil {
			s = b.Schema(schema)
		}
		if p.Description != "" {
			s.Set("description", p.Description)
		}
		if p.Deprecated {
			s.Set("deprecated", true)
		}
		props.Set(q.name, s)
		if q.isRequired {
			required = append(required, q.name)
		}
	}
	if t.body != nil {
		s := bodySchema(b, t.op, t.body.content)
		if desc := bodyDescription(t.op); desc != "" {
			s.Set("description", desc)
		}
		props.Set(t.body.name, s)
		if t.body.isRequired {
			required = append(required, t.body.name)
		}
	}

	root := new(jsonschema.Object).Set("type", "object")
	if props.Len() > 0 {
		root.Set("properties", props)
	}
	if len(required) > 0 {
		root.Set("required", required)
	}
	root.Set("additionalProperties", false)
	return string(b.Document(root)), b.Diagnostics()
}

// inputBody is the body the tool sends, with its options field: the JSON one, else the first.
func inputBody(op *gomodel.Operation, n *naming.Namer) (gomodel.Content, string, bool) {
	c, ok := operation.FirstBody(op.Bodies)
	if !ok {
		return gomodel.Content{}, "", false
	}
	i := slices.IndexFunc(op.Bodies, func(x gomodel.Content) bool { return x.MediaType == c.MediaType })
	return c, operation.BodyFields(op.Bodies, n)[i], true
}

// bodySchema is the schema of the body's media type. Without one, JSON takes anything, text a
// string, and any other media type a base64 string, which is how bytes come in JSON. Bytes the
// client sends as they are name their media type, unless the schema names one.
func bodySchema(b *jsonschema.Builder, op *gomodel.Operation, c gomodel.Content) *jsonschema.Object {
	schema := contentSchema(op, c)
	s := &jsonschema.Object{}
	switch {
	case schema != nil:
		s = b.Schema(schema)
	case runtime.IsJSON(c.MediaType):
	case strings.HasPrefix(operation.BaseMediaType(c.MediaType), "text/"):
		s.Set("type", "string")
	default:
		s.Set("type", "string").Set("contentEncoding", "base64")
	}
	if isRaw(c) && (schema == nil || schema.ContentMediaType == "") {
		s.Set("contentMediaType", c.MediaType)
	}
	return s
}

func contentSchema(op *gomodel.Operation, c gomodel.Content) *spec.Schema {
	if op.Spec.Body == nil {
		return nil
	}
	i := slices.IndexFunc(op.Spec.Body.Contents, func(mt *spec.MediaType) bool { return mt.Name == c.MediaType })
	if i < 0 {
		return nil
	}
	return op.Spec.Body.Contents[i].Schema
}

// isRaw reports a body of bytes the client sends as they are, under its media type: not as JSON,
// and not under a wildcard, which goes out as application/octet-stream.
func isRaw(c gomodel.Content) bool {
	t := operation.BodyType(c)
	if p, ok := t.(gomodel.Pointer); ok {
		t = p.Elem
	}
	under := gomodel.Underlying(t)
	return (under == fileType || under == bytesType) && !runtime.IsJSON(c.MediaType) && !strings.Contains(c.MediaType, "*")
}

func bodyDescription(op *gomodel.Operation) string {
	if op.Spec.Body == nil {
		return ""
	}
	return op.Spec.Body.Description
}

// paramSchema is a parameter's schema, or the schema of its one media type.
func paramSchema(p *spec.Parameter) *spec.Schema {
	if p.Schema == nil && len(p.Contents) > 0 {
		return p.Contents[0].Schema
	}
	return p.Schema
}
