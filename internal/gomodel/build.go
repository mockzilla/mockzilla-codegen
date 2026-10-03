// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Every declaration the collector listed gets its fields, variants and types.

package gomodel

import (
	"fmt"
	"maps"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// fieldPlan keeps what a field's final type needs until recursion is known.
type fieldPlan struct {
	owner    *Decl
	field    *Field
	base     Type
	presence presence
}

// builder fills named declarations with their types.
type builder struct {
	opts   Options
	flat   *flattener
	unions *unionReader
	ext    *extReader
	diags  *diag.Collector
	decls  map[*spec.Schema]*Decl
	plans  []*fieldPlan
	tags   []string
}

func newBuilder(opts Options, r readers, diags *diag.Collector) *builder {
	var tags []string
	for _, t := range opts.ExtraTags {
		if t != "json" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	slices.Sort(tags)
	return &builder{opts: opts, flat: r.flat, unions: r.unions, ext: r.ext, diags: diags, decls: map[*spec.Schema]*Decl{}, tags: tags}
}

// build fills every declaration, then settles pointers, which need every type known first.
// headers are the typed header structs of responses.
func (b *builder) build(list []*pending, ops []*Operation, headers map[*spec.Response]*Decl) []*Decl {
	decls := make([]*Decl, len(list))
	for i, p := range list {
		decls[i] = p.decl
		p.decl.schema = p.schema
		if p.schema != nil {
			b.decls[p.schema] = p.decl
		}
	}

	for _, p := range list {
		if p.params != nil {
			b.fillParams(p.decl, p.params)
			continue
		}
		b.declare(p.decl, p.schema, p.shape)
	}
	breakAliasCycles(decls, b.diags)
	b.settleFields(decls)
	settleUnions(decls)

	for _, op := range ops {
		if op.Spec.Body != nil {
			op.Bodies = b.contents(op.Spec.Body.Contents)
		}
		for _, r := range op.Spec.Responses {
			op.Responses = append(op.Responses, Response{Status: r.Status, Contents: b.contents(r.Contents), Headers: headers[r]})
		}
	}
	return decls
}

func (b *builder) declare(d *Decl, s *spec.Schema, sh shape) {
	f := b.flat.flatten(s)
	if b.opts.Descriptions {
		d.Doc = description(f)
	}
	d.Deprecated = slices.ContainsFunc(siblings(f), func(x *spec.Schema) bool { return x.Deprecated })
	set := b.ext.of(s.Extensions, s.Origin)
	d.DeprecatedReason, d.enumNames = set.DeprecatedReason, set.EnumNames

	if set.GoType != nil {
		d.Kind, d.Target = KindAlias, goType(set.GoType, b.opts.Imports)
		return
	}
	if r := refOf(s); r != nil {
		d.Kind, d.Target = KindAlias, b.typeOf(r.Target)
		return
	}
	switch sh {
	case shapeStruct:
		d.Kind = KindStruct
		b.fillStruct(d, f)
	case shapeEnum:
		d.Kind, d.Enum = KindEnum, b.enumOf(d, f)
	case shapeUnion:
		d.Kind = KindUnion
		b.fillUnion(d, f)
	case shapeMap, shapeArray:
		d.Kind, d.Target = KindDefined, b.inline(f, sh)
	default:
		d.Kind, d.Target = KindAlias, b.inline(f, sh)
	}
}

func (b *builder) fillStruct(d *Decl, f *spec.Schema) {
	st := &Struct{Fields: b.fields(d, f), IsClosed: f.AdditionalProperties.Mode == spec.AdditionalDenied}
	if m := f.AdditionalProperties.Mode; m == spec.AdditionalSchema || m == spec.AdditionalAllowed {
		st.AdditionalProperties = &Field{
			Name:     "AdditionalProperties",
			JSONName: "-",
			Type:     Map{Key: stringType, Elem: b.valueType(f)},
			Tags:     b.fieldTags("-", false, nil),
		}
	}
	d.Struct = st
	resolveFields(d, b.opts.Namer, b.methods(d), b.diags)
}

// fillUnion makes one variant per member, next to the properties every member shares. Members of
// the same Go type share one variant.
func (b *builder) fillUnion(d *Decl, f *spec.Schema) {
	us := b.unions.read(f)
	u := &Union{IsAnyOf: us.isAnyOf, IsNullable: us.isNullable, Discriminator: us.discriminator}
	st := &Struct{}
	if !us.isTypeList {
		st.Fields = b.fields(d, f)
	}

	for _, m := range us.members {
		t := b.typeOf(m.schema)
		if t == (DeclRef{Decl: d}) {
			b.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeUnionSelf,
				Pointer:  m.schema.Origin.Pointer,
				Origin:   origin(m.schema.Origin),
				Message:  fmt.Sprintf("member %d is the union itself; it is left out", m.index+1),
			})
			continue
		}
		if i := slices.IndexFunc(u.Variants, func(v *Variant) bool { return v.Type == t }); i >= 0 {
			v := u.Variants[i]
			v.Values = append(v.Values, m.values...)
			v.IsDefault = v.IsDefault || m.isDefault
			b.diags.Append(diag.Diagnostic{
				Severity: diag.Info,
				Code:     diag.CodeUnionDuplicate,
				Pointer:  m.schema.Origin.Pointer,
				Origin:   origin(m.schema.Origin),
				Message:  fmt.Sprintf("member %d has the same Go type (%s) as variant %s; they share its field", m.index+1, typeText(t), v.Name),
			})
			continue
		}

		name := typeName(t, b.opts.Namer)
		if _, isInline := b.decls[m.schema]; isInline {
			name = b.opts.Namer.Exported(m.suffix)
		}
		u.Variants = append(u.Variants, &Variant{Name: name, Type: t, Values: m.values, IsDefault: m.isDefault, Origin: origin(m.schema.Origin), schema: m.schema})
	}
	d.Struct, d.Union = st, u
	resolveFields(d, b.opts.Namer, b.methods(d), b.diags)
	resolveVariants(d, b.methods(d), b.diags)
}

// methods are the methods generated on d, which its fields cannot be named after.
func (b *builder) methods(d *Decl) []string {
	out := slices.Clone(structMethods)
	if b.opts.ValidateResponse {
		out = append(out, "ValidateResponse")
	}
	if _, isError := b.opts.ErrorMapping[d.Name]; isError {
		out = append(out, "Error")
	}
	return out
}

// fields makes one field per property of f.
func (b *builder) fields(d *Decl, f *spec.Schema) []*Field {
	out := make([]*Field, 0, len(f.Properties))
	for _, p := range f.Properties {
		set := b.ext.of(p.Schema.Extensions, p.Schema.Origin)
		fd := &Field{
			JSONName:   p.Name,
			Required:   p.Required,
			Nullable:   b.nullable(p.Schema),
			ReadOnly:   b.inChain(p.Schema, func(x *spec.Schema) bool { return x.ReadOnly }),
			WriteOnly:  b.inChain(p.Schema, func(x *spec.Schema) bool { return x.WriteOnly }),
			Deprecated: slices.ContainsFunc(siblings(p.Schema), func(x *spec.Schema) bool { return x.Deprecated }),
			Origin:     origin(p.Schema.Origin),
			schema:     p.Schema,
		}
		if b.opts.Descriptions {
			fd.Doc = description(p.Schema)
		}
		fd.OmitEmpty = !fd.Required || fd.ReadOnly || fd.WriteOnly
		b.applyExtensions(fd, set)
		b.plan(d, fd, b.typeOf(p.Schema))
		out = append(out, fd)
	}
	return out
}

// fillParams makes one field per parameter of a location. A parameter without a schema is a string.
func (b *builder) fillParams(d *Decl, params []*spec.Parameter) {
	d.Struct = &Struct{}
	for _, p := range params {
		s := paramSchema(p)
		fd := &Field{
			JSONName:   p.Name,
			Required:   p.Required,
			Nullable:   b.nullable(s),
			OmitEmpty:  !p.Required,
			Deprecated: p.Deprecated,
			Origin:     origin(p.Origin),
			schema:     s,
		}
		if b.opts.Descriptions {
			fd.Doc = p.Description
		}
		b.applyExtensions(fd, b.ext.of(p.Extensions, p.Origin))

		t := Type(stringType)
		if s != nil {
			t = b.typeOf(s)
		}
		b.plan(d, fd, t)
		d.Struct.Fields = append(d.Struct.Fields, fd)
	}
	resolveFields(d, b.opts.Namer, b.methods(d), b.diags)
}

// applyExtensions sets what the extensions of a field ask for, then its tags.
func (b *builder) applyExtensions(fd *Field, set extension.Set) {
	fd.DeprecatedReason = set.DeprecatedReason
	fd.IsJSONIgnored = set.IsJSONIgnored
	fd.Sensitive = set.Sensitive
	fd.goName = b.ext.goName(set, set.Name)
	fd.isPointerSkipped = set.IsPointerSkipped
	if set.OmitEmpty != nil {
		fd.OmitEmpty = *set.OmitEmpty
	}
	fd.Tags = b.fieldTags(fd.JSONName, fd.OmitEmpty, set.Tags)
}

func (b *builder) enumOf(d *Decl, f *spec.Schema) *Enum {
	kind := enumKindOf(f)
	for _, v := range misfits(kind, f.Enum) {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeEnumValue,
			Pointer:  d.ID,
			Origin:   d.Origin,
			Message:  fmt.Sprintf("enum value %s does not fit type %s; it is left out", valueText(v), typeSetText(f.Types&^spec.TypeNull)),
		})
	}

	e := &Enum{Base: enumBase(kind, f.Format, b.opts.IntType)}
	for _, v := range enumValues(kind, f.Enum) {
		e.Values = append(e.Values, EnumValue{Value: v})
	}
	return e
}

// typeOf is the type a schema has where it is used: its declaration, or an inline type.
func (b *builder) typeOf(s *spec.Schema) Type {
	if s == nil {
		return anyType
	}
	if d, ok := b.decls[s]; ok {
		return DeclRef{Decl: d}
	}
	if set := b.ext.of(s.Extensions, s.Origin); set.GoType != nil {
		return goType(set.GoType, b.opts.Imports)
	}
	if r := refOf(s); r != nil {
		return b.typeOf(r.Target)
	}
	f := b.flat.flatten(s)
	return b.inline(f, classify(f))
}

// inline is the type of a shape that needs no name. Named shapes never get here.
func (b *builder) inline(f *spec.Schema, sh shape) Type {
	switch sh {
	case shapeMap:
		return Map{Key: stringType, Elem: b.valueType(f)}
	case shapeArray:
		return Slice{Elem: b.elem(f.Items)}
	case shapePrimitive:
		return primitive(f, b.opts.IntType)
	default:
		return anyType
	}
}

func (b *builder) valueType(f *spec.Schema) Type {
	if f.AdditionalProperties.Mode == spec.AdditionalSchema {
		return b.elem(f.AdditionalProperties.Schema)
	}
	return anyType
}

func (b *builder) elem(s *spec.Schema) Type {
	return elemType(b.typeOf(s), b.nullable(s))
}

func (b *builder) nullable(s *spec.Schema) bool {
	return b.inChain(s, func(f *spec.Schema) bool {
		return f.Nullable || hasNullMember(f) || slices.ContainsFunc(f.Enum, func(v spec.Value) bool { return v.Kind == spec.KindNull })
	})
}

// inChain checks s, its siblings and every schema its plain $refs lead to, flattened.
func (b *builder) inChain(s *spec.Schema, fn func(*spec.Schema) bool) bool {
	var seen []*spec.Schema
	for s != nil && !slices.Contains(seen, s) {
		if fn(b.flat.flatten(s)) || slices.ContainsFunc(siblings(s), fn) {
			return true
		}
		seen = append(seen, s)

		r := refOf(s)
		if r == nil {
			return false
		}
		s = r.Target
	}
	return false
}

func (b *builder) plan(d *Decl, f *Field, base Type) {
	b.plans = append(b.plans, &fieldPlan{
		owner:    d,
		field:    f,
		base:     base,
		presence: presence{isRequired: f.Required, isNullable: f.Nullable, isPointerSkipped: f.isPointerSkipped},
	})
}

// settleFields gives every field its final type. A field that holds a type from its own strongly
// connected component by value becomes a pointer, or the struct would contain itself.
func (b *builder) settleFields(decls []*Decl) {
	index := make(map[*Decl]int, len(decls))
	for i, d := range decls {
		index[d] = i
	}

	adj := make([][]int, len(decls))
	for _, p := range b.plans {
		if e := byValue(p); e != nil {
			adj[index[p.owner]] = append(adj[index[p.owner]], index[e])
		}
	}
	for i, d := range decls {
		if r, ok := d.Target.(DeclRef); ok {
			adj[i] = append(adj[i], index[r.Decl])
		}
	}

	comp := components(adj)
	for _, p := range b.plans {
		pr := p.presence
		if e := byValue(p); e != nil && comp[index[p.owner]] == comp[index[e]] {
			pr.isInCycle = true
		}
		p.field.Type = fieldType(p.base, pr)
	}
}

func (b *builder) contents(list []*spec.MediaType) []Content {
	var out []Content
	for _, mt := range list {
		c := Content{MediaType: mt.Name}
		if mt.Schema != nil {
			c.Type = b.typeOf(mt.Schema)
		}
		switch {
		case mt.ItemSchema != nil:
			c.Item = b.typeOf(mt.ItemSchema)
		case runtime.IsSequential(mt.Name):
			c.Item = c.Type
		}
		out = append(out, c)
	}
	return out
}

// fieldTags are the tags next to json: one per config extra tag, then those of
// x-oapi-codegen-extra-tags, which win on the same key. A json key there is left out.
func (b *builder) fieldTags(jsonName string, isOmitEmpty bool, extra []extension.Tag) []Tag {
	value := jsonName
	if isOmitEmpty {
		value += ",omitempty"
	}

	byKey := make(map[string]string, len(b.tags)+len(extra))
	for _, key := range b.tags {
		byKey[key] = value
	}
	for _, t := range extra {
		byKey[t.Key] = t.Value
	}
	delete(byKey, "json")

	var out []Tag
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, Tag{Key: key, Value: byKey[key]})
	}
	return out
}

// byValue returns the declaration a field holds by value, or nil.
func byValue(p *fieldPlan) *Decl {
	r, ok := p.base.(DeclRef)
	if !ok {
		return nil
	}
	if isPointer(fieldType(p.base, p.presence)) {
		return nil
	}
	return r.Decl
}
