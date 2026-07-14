// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

// boundSource is one side of a numeric range, with the nodes holding the numbers as written.
type boundSource struct {
	value         *float64
	valueNode     *yaml.Node
	exclusive     *base.DynamicValue[bool, float64]
	exclusiveNode *yaml.Node
}

func (b boundSource) resolve(isLower bool) *spec.Bound {
	var out *spec.Bound
	if b.value != nil {
		out = &spec.Bound{Value: number(b.valueNode, *b.value)}
	}

	switch ex := b.exclusive; {
	case ex == nil:
	case ex.IsA():
		if out != nil {
			out.Exclusive = ex.A
		}
	case out == nil || (isLower && ex.B >= *b.value) || (!isLower && ex.B <= *b.value):
		out = &spec.Bound{Value: number(b.exclusiveNode, ex.B), Exclusive: true}
	}
	return out
}

// schema converts each pointer once, which turns recursive specs into pointer cycles.
func (c *converter) schema(p *base.SchemaProxy, ptr string) *spec.Schema {
	if p == nil {
		return nil
	}
	if s, ok := c.schemaMemo[ptr]; ok {
		return s
	}

	s := &spec.Schema{}
	c.schemaMemo[ptr] = s
	low := p.GoLow()
	switch {
	case low != nil && low.IsTransformedRefWithSiblings():
		c.siblingRef(s, p, ptr)
	case p.IsReference():
		s.Ref = c.ref(p)
		s.Origin = c.origin(ptr, p.GetReferenceNode())
	default:
		c.fill(s, p, ptr, p.GetValueNode())
	}
	return s
}

// siblingRef reads libopenapi's allOf [siblings, $ref] rewrite, which it does in every version.
func (c *converter) siblingRef(s *spec.Schema, p *base.SchemaProxy, ptr string) {
	// Building the wrapper cannot fail: its parts are built lazily.
	view := base.NewSchema(p.GoLow().Schema())
	c.fill(s, view.AllOf[0], ptr, p.GetReferenceNode())
	s.Ref = c.ref(view.AllOf[1])
}

// ref converts a component target from its own entry, anything else from the resolved content.
func (c *converter) ref(p *base.SchemaProxy) *spec.Ref {
	target := refPointer(p.GetReference())
	ref := &spec.Ref{Pointer: target, Name: componentName(target)}
	if ref.Target = c.known(target); ref.Target != nil {
		return ref
	}

	s := &spec.Schema{}
	c.schemaMemo[target] = s
	c.fill(s, p, target, p.GetReferenceNode())
	ref.Target = s
	return ref
}

// known returns the schema at ptr when it is already converted or is a component.
func (c *converter) known(ptr string) *spec.Schema {
	if s, ok := c.schemaMemo[ptr]; ok {
		return s
	}
	if p, ok := c.componentSchemas[ptr]; ok {
		return c.schema(p, ptr)
	}
	return nil
}

// fill leaves s empty, so it reads as any, when p cannot be built, and reports that at n.
func (c *converter) fill(s *spec.Schema, p *base.SchemaProxy, ptr string, n *yaml.Node) {
	h, err := p.BuildSchema()
	if h == nil {
		s.Origin = c.origin(ptr, n)
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Error,
			Code:     diag.CodeSchemaBuild,
			Pointer:  ptr,
			Origin:   c.position(ptr, n),
			Message:  fmt.Sprintf("schema cannot be built: %v", err),
		})
		return
	}

	s.Types, s.Nullable = c.types(h, ptr)
	s.Format = h.Format
	s.Title = h.Title
	s.Description = h.Description
	s.Pattern = h.Pattern
	s.ContentEncoding = h.ContentEncoding
	s.ContentMediaType = h.ContentMediaType
	s.Required = slices.Clone(h.Required)
	s.Properties = c.properties(h, ptr)
	s.AdditionalProperties = c.additional(h.AdditionalProperties, ptr+"/additionalProperties")
	if h.Items != nil && h.Items.IsA() {
		s.Items = c.schema(h.Items.A, ptr+"/items")
	}
	s.PrefixItems = c.schemas(h.PrefixItems, ptr+"/prefixItems")
	s.AllOf = c.schemas(h.AllOf, ptr+"/allOf")
	s.OneOf = c.schemas(h.OneOf, ptr+"/oneOf")
	s.AnyOf = c.schemas(h.AnyOf, ptr+"/anyOf")
	s.Not = c.schema(h.Not, ptr+"/not")
	s.If = c.schema(h.If, ptr+"/if")
	s.Then = c.schema(h.Then, ptr+"/then")
	s.Else = c.schema(h.Else, ptr+"/else")
	s.Discriminator = c.discriminator(h.Discriminator, ptr+"/discriminator")
	s.Enum = values(h.Enum)
	s.Const = optionalValue(h.Const)
	s.Default = optionalValue(h.Default)
	s.Examples = examples(h)
	s.Limits = limits(h)
	s.ReadOnly = h.ReadOnly != nil && *h.ReadOnly
	s.WriteOnly = h.WriteOnly != nil && *h.WriteOnly
	s.Deprecated = h.Deprecated != nil && *h.Deprecated
	s.Extensions = extensions(h.Extensions)
	s.Origin = c.origin(ptr, h.GoLow().RootNode)
}

func (c *converter) schemas(list []*base.SchemaProxy, ptr string) []*spec.Schema {
	var out []*spec.Schema
	for i, p := range list {
		out = append(out, c.schema(p, ptr+"/"+strconv.Itoa(i)))
	}
	return out
}

func (c *converter) properties(h *base.Schema, ptr string) []*spec.Property {
	var out []*spec.Property
	for name, p := range h.Properties.FromOldest() {
		out = append(out, &spec.Property{
			Name:     name,
			Schema:   c.schema(p, ptr+"/properties/"+oasdoc.Escape(name)),
			Required: slices.Contains(h.Required, name),
		})
	}
	return out
}

func (c *converter) additional(d *base.DynamicValue[*base.SchemaProxy, bool], ptr string) spec.Additional {
	switch {
	case d == nil:
		return spec.Additional{}
	case d.IsA():
		return spec.Additional{Mode: spec.AdditionalSchema, Schema: c.schema(d.A, ptr)}
	case d.B:
		return spec.Additional{Mode: spec.AdditionalAllowed}
	default:
		return spec.Additional{Mode: spec.AdditionalDenied}
	}
}

// types folds "null" in a type list into Nullable, keeping it only when it is the sole type.
func (c *converter) types(h *base.Schema, ptr string) (spec.TypeSet, bool) {
	var set spec.TypeSet
	for _, name := range h.Type {
		t := typeOf(name)
		if t == 0 {
			c.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeUnknownType,
				Pointer:  ptr,
				Origin:   c.position(ptr, h.GoLow().RootNode),
				Message:  "unknown type " + strconv.Quote(name) + " is ignored",
			})
		}
		set |= t
	}

	isNullable := h.Nullable != nil && *h.Nullable
	if set.Has(spec.TypeNull) {
		isNullable = true
		if set != spec.TypeNull {
			set &^= spec.TypeNull
		}
	}
	return set, isNullable
}

// discriminator resolves every mapping value; a bare name means a component schema.
func (c *converter) discriminator(d *base.Discriminator, ptr string) *spec.Discriminator {
	if d == nil {
		return nil
	}

	low := d.GoLow()
	out := &spec.Discriminator{Property: d.PropertyName}
	for k, v := range low.Mapping.Value.FromOldest() {
		at := ptr + "/mapping/" + oasdoc.Escape(k.Value)
		out.Mapping = append(out.Mapping, spec.Mapping{Value: k.Value, Ref: c.mappingRef(v.Value, at, v.ValueNode)})
	}
	if d.DefaultMapping != "" {
		out.Default = c.mappingRef(d.DefaultMapping, ptr+"/defaultMapping", low.DefaultMapping.ValueNode)
	}
	return out
}

func (c *converter) mappingRef(target, ptr string, n *yaml.Node) *spec.Ref {
	to := refPointer(target)
	if !strings.ContainsAny(target, "#/") && !isFileName(target) {
		to = componentPointer("schemas", target)
	}

	ref := &spec.Ref{Pointer: to, Name: componentName(to), Target: c.known(to)}
	if ref.Target == nil {
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeUnresolvedMapping,
			Pointer:  ptr,
			Origin:   c.position(ptr, n),
			Message:  "discriminator mapping " + strconv.Quote(target) + " names no schema",
		})
	}
	return ref
}

func typeOf(name string) spec.TypeSet {
	switch name {
	case "string":
		return spec.TypeString
	case "number":
		return spec.TypeNumber
	case "integer":
		return spec.TypeInteger
	case "boolean":
		return spec.TypeBoolean
	case "object":
		return spec.TypeObject
	case "array":
		return spec.TypeArray
	case "null":
		return spec.TypeNull
	default:
		return 0
	}
}

// limits reads numbers from their nodes so they keep the precision and form they were written in.
func limits(h *base.Schema) spec.Limits {
	low := h.GoLow()
	out := spec.Limits{
		Minimum: boundSource{
			value:         h.Minimum,
			valueNode:     low.Minimum.ValueNode,
			exclusive:     h.ExclusiveMinimum,
			exclusiveNode: low.ExclusiveMinimum.ValueNode,
		}.resolve(true),
		Maximum: boundSource{
			value:         h.Maximum,
			valueNode:     low.Maximum.ValueNode,
			exclusive:     h.ExclusiveMaximum,
			exclusiveNode: low.ExclusiveMaximum.ValueNode,
		}.resolve(false),
		MinLength:     clonePtr(h.MinLength),
		MaxLength:     clonePtr(h.MaxLength),
		MinItems:      clonePtr(h.MinItems),
		MaxItems:      clonePtr(h.MaxItems),
		MinProperties: clonePtr(h.MinProperties),
		MaxProperties: clonePtr(h.MaxProperties),
		UniqueItems:   h.UniqueItems != nil && *h.UniqueItems,
	}
	if h.MultipleOf != nil {
		out.MultipleOf = new(number(low.MultipleOf.ValueNode, *h.MultipleOf))
	}
	return out
}

// examples prefers the 3.1 examples list and falls back to the single example.
func examples(h *base.Schema) []spec.Value {
	if len(h.Examples) > 0 {
		return values(h.Examples)
	}
	if h.Example != nil {
		return []spec.Value{value(h.Example)}
	}
	return nil
}

func number(n *yaml.Node, f float64) json.Number {
	if n != nil && isJSONNumber(n.Value) {
		return json.Number(n.Value)
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
}

func isFileName(s string) bool {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	return new(*p)
}
