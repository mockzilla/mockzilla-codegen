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
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
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
	b.requiredCounts(decls)
	settleUnions(decls)
	ambiguousUnions(decls, b.diags)

	planGetters(decls, b.diags)

	for _, op := range ops {
		for i := range op.Params {
			g := &op.Params[i]
			g.Defaults = map[string]string{}
			for j, p := range g.Params {
				if d := g.Decl.Struct.Fields[j].def; d != nil && b.opts.IsServer {
					g.Defaults[p.Name] = string(jsonschema.Marshal(*d))
				}
			}
		}
		if qs := op.QueryString; qs != nil {
			mt := qs.Param.Contents[0]
			qs.Content = b.contents([]*spec.MediaType{mt})[0]
			if b.opts.IsServer && !qs.Param.Required {
				if d := b.paramDefault(qs.Param, mt.Schema); d != nil {
					qs.Default = string(jsonschema.Marshal(*d))
				}
			}
		}
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

	if t := b.flat.goTypeOf(s); t != nil {
		d.Kind, d.Target = KindAlias, goType(t, b.opts.Imports)
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

	if f.PropertyNames != nil && st.AdditionalProperties == nil {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeKeywordUnsupported,
			Pointer:  f.PropertyNames.Origin.Pointer,
			Origin:   origin(f.PropertyNames.Origin),
			Message:  fmt.Sprintf("propertyNames is not checked: %s keeps no keys besides its properties", d.Name),
		})
	}
}

// fillUnion makes one variant per member, next to the properties every member shares. Members of
// the same Go type share one variant, in any group.
func (b *builder) fillUnion(d *Decl, f *spec.Schema) {
	us := b.unions.read(f)
	u := &Union{isTypeList: us.isTypeList}
	for _, g := range us.groups {
		u.Groups = append(u.Groups, &Group{IsAnyOf: g.isAnyOf, IsNullable: g.isNullable, IsLiteral: g.isLiteral, Discriminator: g.discriminator})
		if g.ignored != "" {
			b.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeKeywordUnsupported,
				Pointer:  g.origin.Pointer + "/discriminator",
				Origin:   origin(g.origin),
				Message:  fmt.Sprintf("the discriminator of %s is ignored: its property %q is not a string; variants are matched by shape and checks", d.Name, g.ignored),
			})
		}
	}
	st := &Struct{}
	if !us.isTypeList {
		st.Fields = b.fields(d, f)
	}

	for _, m := range us.members {
		g := u.Groups[m.group]
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
		if !us.isTypeList && isRequiredOnly(m.schema) {
			b.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeKeywordUnsupported,
				Pointer:  m.schema.Origin.Pointer,
				Origin:   origin(m.schema.Origin),
				Message:  fmt.Sprintf("member %d only lists required properties; next to members with a type or null it is not checked", m.index+1),
			})
		}

		v := b.variant(u, g, m, t)
		at := slices.IndexFunc(v.schemas, func(s *spec.Schema) bool { return isSameMember(s, m.schema) })
		if at < 0 {
			at = len(v.schemas)
			v.schemas = append(v.schemas, m.schema)
		}

		if mb := (Member{Variant: v, Index: at}); !slices.Contains(g.Members, mb) {
			g.Members = append(g.Members, mb)
		}
	}

	if !us.isTypeList {
		st.Fields = append(st.Fields, b.discriminatorFields(d, f, us, u)...)
	}
	d.Struct, d.Union = st, u
	resolveFields(d, b.opts.Namer, b.methods(d), b.diags)
	resolveVariants(d, b.methods(d), b.diags)
}

// discriminatorFields keep the values that pick a variant lacking the discriminator property.
func (b *builder) discriminatorFields(d *Decl, f *spec.Schema, us *unionSchema, u *Union) []*Field {
	var props []*spec.Property
	var names []string
	for i, g := range u.Groups {
		name := g.Discriminator
		isHeld := func(s *spec.Schema) bool { return b.unions.property(s, name) != nil }
		isLost := func(v *Variant) bool { return len(v.Values) > 1 && !slices.ContainsFunc(v.schemas, isHeld) }
		if name == "" || slices.Contains(names, name) || isHeld(f) || !slices.ContainsFunc(g.Variants, isLost) {
			continue
		}
		names = append(names, name)
		at := us.groups[i].origin
		at.Pointer += "/discriminator"
		props = append(props, &spec.Property{Name: name, Schema: &spec.Schema{Types: spec.TypeString, Origin: at}})
	}
	return b.fields(d, &spec.Schema{Properties: props})
}

// variant is the variant of u that member m of group g sets: the one of its Go type t, else a new one.
func (b *builder) variant(u *Union, g *Group, m unionMember, t Type) *Variant {
	if i := slices.IndexFunc(u.Variants, func(v *Variant) bool { return v.Type == t }); i >= 0 {
		v := u.Variants[i]
		v.Values = append(v.Values, m.values...)
		v.IsDefault = v.IsDefault || m.isDefault
		v.IsAbsent = v.IsAbsent || m.isAbsent
		if !slices.Contains(g.Variants, v) {
			g.Variants = append(g.Variants, v)
		}
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Info,
			Code:     diag.CodeUnionDuplicate,
			Pointer:  m.schema.Origin.Pointer,
			Origin:   origin(m.schema.Origin),
			Message:  fmt.Sprintf("member %d has the same Go type (%s) as variant %s; they share its field", m.index+1, typeText(t), v.Name),
		})
		return v
	}

	name := typeName(t, b.opts.Namer)
	if _, isInline := b.decls[m.schema]; isInline {
		name = b.opts.Namer.Exported(m.suffix)
	}
	v := &Variant{Name: name, Type: t, Values: m.values, IsDefault: m.isDefault, IsAbsent: m.isAbsent, Origin: origin(m.schema.Origin)}
	u.Variants = append(u.Variants, v)
	g.Variants = append(g.Variants, v)
	return v
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
		if !fd.Required {
			fd.def = b.propertyDefault(d, fd)
		}
		fd.OmitEmpty = !fd.Required || fd.ReadOnly || fd.WriteOnly
		b.applyExtensions(fd, set)
		fd.wrap = b.wrapping(fd, set.Nullable, p.Schema.Origin, fmt.Sprintf("property %q of %s", p.Name, d.Name))
		b.plan(d, fd, b.typeOf(p.Schema))
		out = append(out, fd)
	}
	return out
}

// fillParams makes one field per parameter of a location. A parameter without a schema is a string.
func (b *builder) fillParams(d *Decl, params []*spec.Parameter) {
	d.Struct, d.isParams = &Struct{}, true
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
		if p.In != spec.InPath && !p.Required {
			fd.def = b.paramDefault(p, s)
		}
		set := b.ext.ofParam(p, s)
		if d.Part == PartResponses {
			set.Tags = append(slices.Clip(set.Tags), headerTags(p)...)
		}
		b.applyExtensions(fd, set)
		fd.wrap = b.wrapping(fd, set.Nullable, p.Origin, fmt.Sprintf("%s parameter %q", p.In, p.Name))

		t := Type(stringType)
		if s != nil {
			t = b.typeOf(s)
		}
		b.plan(d, fd, t)
		d.Struct.Fields = append(d.Struct.Fields, fd)
	}
	resolveFields(d, b.opts.Namer, b.methods(d), b.diags)
}

// paramDefault is the default of p when it is not there, nil when it has none that fits.
func (b *builder) paramDefault(p *spec.Parameter, s *spec.Schema) *spec.Value {
	if s == nil {
		return nil
	}
	d := b.defaultOf(s)
	if d == nil {
		return nil
	}
	if why := jsonschema.Misfit(*d, s); why != "" {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeDefaultIgnored,
			Pointer:  p.Origin.Pointer,
			Origin:   origin(p.Origin),
			Message:  fmt.Sprintf("the default of %s parameter %q does not fit its schema, so it is left out: %s", p.In, p.Name, why),
		})
		return nil
	}
	return d
}

// propertyDefault is the default of f when it is not there, nil when it has none that fits.
func (b *builder) propertyDefault(d *Decl, f *Field) *spec.Value {
	def := b.defaultOf(f.schema)
	if def == nil {
		return nil
	}
	if why := jsonschema.Misfit(*def, f.schema); why != "" {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeDefaultIgnored,
			Pointer:  f.schema.Origin.Pointer,
			Origin:   f.Origin,
			Message:  fmt.Sprintf("the default of property %q of %s does not fit its schema, so it is left out: %s", f.JSONName, d.Name, why),
		})
		return nil
	}
	return def
}

// defaultOf is the first default on s, its doc-only allOf members or the schemas its refs lead to.
func (b *builder) defaultOf(s *spec.Schema) *spec.Value {
	var def *spec.Value
	b.inChain(s, func(x *spec.Schema) bool {
		def = x.Default
		return def != nil
	})
	return def
}

// wrapping is how models.nullable and x-go-nullable, isAsked, make f a Nullable.
func (b *builder) wrapping(f *Field, isAsked *bool, at spec.Origin, what string) wrapping {
	isOn := b.opts.Nullable
	if isAsked != nil {
		isOn = *isAsked
	}
	var why string
	switch {
	case !isOn:
		return wrapNone
	case f.Required && !f.Nullable:
		why = "it is required and not nullable, so it always holds a value"
	case f.isPointerSkipped && !f.Required:
		why = extension.SkipPointer + " makes it a plain value"
	case isAsked != nil:
		return wrapAny
	default:
		return wrapNonNil
	}

	if isAsked != nil {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeExtensionValue,
			Pointer:  at.Pointer,
			Origin:   origin(at),
			Message:  fmt.Sprintf("%s on %s is left out: %s", extension.Nullable, what, why),
		})
	}
	return wrapNone
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
	if t := b.flat.goTypeOf(s); t != nil {
		return goType(t, b.opts.Imports)
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
		return primitive(f, b.opts.IntType, b.opts.FormatTypes)
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
	return elemType(b.typeOf(s), b.nullable(s), b.opts.Nullable)
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
		presence: presence{isRequired: f.Required, isNullable: f.Nullable, isPointerSkipped: f.isPointerSkipped, wrap: f.wrap},
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
		f := p.field
		f.Type = fieldType(p.base, pr)
		f.OmitZero = f.OmitEmpty && (isWrapped(f.Type) || isCollection(f.Type))
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

// fieldTags are the tags next to json: one per config extra tag, then those of x-go-extra-tags,
// which win on the same key. A json key there is left out.
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

// headerTags mark a typed response header that is JSON or exploded, for the runtime.
func headerTags(p *spec.Parameter) []extension.Tag {
	switch {
	case p.Schema == nil && len(p.Contents) > 0 && runtime.IsJSON(p.Contents[0].Name):
		return []extension.Tag{{Key: "header", Value: "json"}}
	case p.Explode:
		return []extension.Tag{{Key: "header", Value: "explode"}}
	}
	return nil
}

// byValue returns the declaration a field holds by value, or nil.
func byValue(p *fieldPlan) *Decl {
	r, ok := p.base.(DeclRef)
	if !ok {
		return nil
	}
	switch fieldType(p.base, p.presence).(type) {
	case Pointer, Nullable:
		return nil
	}
	return r.Decl
}
