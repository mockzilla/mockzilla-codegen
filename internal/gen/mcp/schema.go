// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The JSON schema of a tool's input: one property per parameter and one for the body.

package mcp

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// inputLocations are the parameter locations the client sends, so the input holds them.
var inputLocations = []string{spec.InPath, spec.InQuery, spec.InHeader, spec.InCookie}

var fileType = gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}

// inputSchema is the JSON schema of the input of t: an object with one property per parameter,
// named as the spec names it, and one for the body. No other property is allowed, so a misspelled
// parameter is an error and not silently dropped.
func inputSchema(t *tool) string {
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
	return string(b.Document(root))
}

// inputBody is the body the tool sends, with its options field: the JSON one, else the first,
// unless that is a file, which JSON input cannot carry.
func inputBody(op *gomodel.Operation, n *naming.Namer) (gomodel.Content, string, bool) {
	c, ok := operation.FirstBody(op.Bodies)
	if !ok || gomodel.Held(fileType) == operation.BodyType(c) {
		return gomodel.Content{}, "", false
	}
	i := slices.IndexFunc(op.Bodies, func(x gomodel.Content) bool { return x.MediaType == c.MediaType })
	return c, operation.BodyFields(op.Bodies, n)[i], true
}

// bodySchema is the schema of the body's media type. Without one, JSON takes anything, text a
// string, and any other media type a base64 string, which is how bytes come in JSON.
func bodySchema(b *jsonschema.Builder, op *gomodel.Operation, c gomodel.Content) *jsonschema.Object {
	if op.Spec.Body != nil {
		i := slices.IndexFunc(op.Spec.Body.Contents, func(mt *spec.MediaType) bool { return mt.Name == c.MediaType })
		if i >= 0 && op.Spec.Body.Contents[i].Schema != nil {
			return b.Schema(op.Spec.Body.Contents[i].Schema)
		}
	}

	s := &jsonschema.Object{}
	switch {
	case runtime.IsJSON(c.MediaType):
	case strings.HasPrefix(c.MediaType, "text/"):
		s.Set("type", "string")
	default:
		s.Set("type", "string").Set("contentEncoding", "base64")
	}
	return s
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
