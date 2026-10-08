// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Unions from oneOf, anyOf, if and 3.1 type lists: their members, discriminator values and
// variants.

package gomodel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// unionMember is a member of a union schema that is not null. Index is its place in the spec
// lists; suffix names its declaration when it is inline: its title, else its first discriminator
// value, else Option and its place. Group is the union that lists it.
type unionMember struct {
	schema    *spec.Schema
	group     int
	index     int
	suffix    string
	values    []string
	isDefault bool
	isAbsent  bool
}

// unionSchema is how a union schema reads. The members of a type list are made from its types
// and hold its properties; other unions share their properties across members. It has a group per
// oneOf, anyOf or if.
type unionSchema struct {
	isTypeList bool
	groups     []unionGroup
	members    []unionMember
}

// unionGroup is one union of a union schema; ignored is the property of a discriminator left out.
type unionGroup struct {
	isAnyOf       bool
	isNullable    bool
	isLiteral     bool
	discriminator string
	ignored       string
	origin        spec.Origin
}

// unionReader reads each union schema once, so the collector and the builder see the same
// members, the ones made from a type list included.
type unionReader struct {
	namer *naming.Namer
	flat  *flattener
	memo  map[*spec.Schema]*unionSchema
}

// JSONKinds is what JSON a value of type t is written as. A union takes what its variants take.
func JSONKinds(t Type) JSONKind {
	return jsonKinds(t, map[*Decl]bool{})
}

func newUnionReader(n *naming.Namer, flat *flattener) *unionReader {
	return &unionReader{namer: n, flat: flat, memo: map[*spec.Schema]*unionSchema{}}
}

// read returns the union f stands for; f is flattened and classified as a union.
func (r *unionReader) read(f *spec.Schema) *unionSchema {
	if u, ok := r.memo[f]; ok {
		return u
	}
	u := &unionSchema{}
	r.memo[f] = u

	list := r.flat.unions(f)
	if len(list) == 0 {
		u.isTypeList = true
		u.groups = []unionGroup{{isNullable: f.Nullable}}
		r.typeList(u, f)
	}

	at := 0
	for _, s := range list {
		g := len(u.groups)
		u.groups = append(u.groups, unionGroup{isNullable: f.Nullable, origin: s.Origin})
		switch {
		case s.Then != nil:
			r.add(u, g, at, []*spec.Schema{s.Then, s.Else})
			r.predicate(u, g, at, s.If)
			at += 2
		case s.OneOf != nil:
			r.add(u, g, at, s.OneOf)
			r.discriminate(u, g, f, s.Discriminator)
			at += len(s.OneOf)
		default:
			u.groups[g].isAnyOf = true
			r.add(u, g, at, s.AnyOf)
			r.discriminate(u, g, f, s.Discriminator)
			at += len(s.AnyOf)
		}
	}

	for i := range u.members {
		m := &u.members[i]
		if m.suffix != "" {
			continue
		}
		var value string
		if len(m.values) > 0 {
			value = m.values[0]
		}
		m.suffix = r.namer.UnionVariant("", r.flat.flatten(m.schema).Title, value, m.index)
	}
	return u
}

// add lists the members of group g, at its place in the spec lists of all groups.
func (r *unionReader) add(u *unionSchema, g, at int, list []*spec.Schema) {
	for i, s := range list {
		if isNull(s) {
			u.groups[g].isNullable = true
			continue
		}
		u.members = append(u.members, unionMember{schema: s, group: g, index: at + i})
	}
}

// discriminate gives each member of group g its discriminator values: those the mapping lists for
// it, else the const or single-value enum of its discriminator property, else its component name.
func (r *unionReader) discriminate(u *unionSchema, g int, f *spec.Schema, d *spec.Discriminator) {
	if d == nil {
		return
	}

	// Mapping keys are strings; OpenAPI leaves how other values compare to the implementation.
	if r.isNoStringProperty(u, g, f, d.Property) {
		u.groups[g].ignored = d.Property
		return
	}

	u.groups[g].discriminator = d.Property
	for i := range u.members {
		m := &u.members[i]
		if m.group != g {
			continue
		}
		ref := refOf(m.schema)
		for _, mp := range d.Mapping {
			if ref != nil && mp.Ref != nil && mp.Ref.Target == ref.Target {
				m.values = append(m.values, mp.Value)
			}
		}
		if len(m.values) == 0 {
			m.values = r.propertyValues(m.schema, d.Property)
		}
		if len(m.values) == 0 && ref != nil && ref.Name != "" {
			m.values = []string{ref.Name}
		}
		m.isDefault = d.Default != nil && ref != nil && d.Default.Target == ref.Target
	}
}

// predicate reads an if that tests one property against a const or single-value enum: then takes
// that value, and no value unless the if requires one, else any other. It names the branches first.
func (r *unionReader) predicate(u *unionSchema, g, at int, cond *spec.Schema) {
	var then, otherwise *unionMember
	for i := range u.members {
		switch m := &u.members[i]; {
		case m.group != g:
		case m.index == at:
			m.suffix, then = "Then", m
		default:
			m.suffix, otherwise = "Else", m
		}
	}
	if cond == nil || then == nil || otherwise == nil {
		return
	}

	f := r.flat.flatten(target(cond))
	if len(f.Properties) != 1 {
		return
	}
	p := f.Properties[0]
	v, ok := r.constValue(p.Schema)
	if !ok || v.Kind == spec.KindArray || v.Kind == spec.KindObject {
		return
	}

	u.groups[g].discriminator = p.Name
	u.groups[g].isLiteral = v.Kind != spec.KindString
	then.values = []string{valueText(v)}
	then.isAbsent = !slices.Contains(f.Required, p.Name)
	otherwise.isDefault = true
}

// typeList makes one member per type. Each keeps the keywords that apply to its type.
func (r *unionReader) typeList(u *unionSchema, f *spec.Schema) {
	for i, w := range typeWords {
		if w.set == spec.TypeNull || !f.Types.Has(w.set) {
			continue
		}

		m := &spec.Schema{Types: w.set, Limits: f.Limits, Enum: enumOfType(f.Enum, w.set), Origin: f.Origin}
		m.Origin.Pointer += "/type/" + w.word
		switch w.set {
		case spec.TypeObject:
			m.Properties, m.Required, m.AdditionalProperties = f.Properties, f.Required, f.AdditionalProperties
		case spec.TypeArray:
			m.Items, m.PrefixItems = f.Items, f.PrefixItems
		case spec.TypeString:
			m.Format, m.Pattern = f.Format, f.Pattern
		case spec.TypeInteger, spec.TypeNumber:
			m.Format = f.Format
		default:
		}
		u.members = append(u.members, unionMember{schema: m, index: i, suffix: r.namer.Exported(w.word)})
	}
}

// isNoStringProperty reports the property prop typed as no string by f or a member of group g.
func (r *unionReader) isNoStringProperty(u *unionSchema, g int, f *spec.Schema, prop string) bool {
	schemas := []*spec.Schema{f}
	for _, m := range u.members {
		if m.group == g {
			schemas = append(schemas, m.schema)
		}
	}
	return slices.ContainsFunc(schemas, func(s *spec.Schema) bool {
		p := r.property(s, prop)
		return p != nil && isNoString(r.flat.flatten(target(p.Schema)))
	})
}

// property is the property prop of s, nil when s has none.
func (r *unionReader) property(s *spec.Schema, prop string) *spec.Property {
	f := r.flat.flatten(target(s))
	if i := slices.IndexFunc(f.Properties, func(p *spec.Property) bool { return p.Name == prop }); i >= 0 {
		return f.Properties[i]
	}
	return nil
}

// propertyValues are the values the property prop of s allows alone.
func (r *unionReader) propertyValues(s *spec.Schema, prop string) []string {
	if p := r.property(s, prop); p != nil {
		if v, ok := r.constValue(p.Schema); ok {
			return []string{valueText(v)}
		}
	}
	return nil
}

// constValue is the const of s, or the value of an enum with one value; null is none.
func (r *unionReader) constValue(s *spec.Schema) (spec.Value, bool) {
	f := r.flat.flatten(target(s))
	switch {
	case f.Const != nil && f.Const.Kind != spec.KindNull:
		return *f.Const, true
	case len(f.Enum) == 1 && f.Enum[0].Kind != spec.KindNull:
		return f.Enum[0], true
	}
	return spec.Value{}, false
}

// settleUnions gives each variant the type of its field and the JSON it takes, once every
// declaration is complete.
func settleUnions(decls []*Decl) {
	for _, d := range decls {
		if d.Union == nil {
			continue
		}
		d.Union.IsText = len(d.Union.Variants) > 0 && len(d.Struct.Fields) == 0
		for _, v := range d.Union.Variants {
			v.FieldType = elemType(v.Type, true, false)
			v.Kinds = JSONKinds(v.Type)
			if v.Kinds == 0 || v.Kinds&^JSONScalar != 0 {
				d.Union.IsText = false
			}
			if st := structOf(v.Type); st != nil {
				sh := structShape(st)
				v.Required, v.Known, v.IsClosed = sh.Required, sh.Known, sh.IsClosed
			} else if inner := unionOf(v.Type); inner != nil {
				v.Shapes = unionShapes(inner, map[*Decl]bool{d: true})
			}
		}
	}
}

// ambiguousUnions warns about oneOf variants that can be objects requiring the same properties.
func ambiguousUnions(decls []*Decl, diags *diag.Collector) {
	for _, d := range decls {
		if d.Union == nil {
			continue
		}
		for _, g := range d.Union.Groups {
			if !g.IsAnyOf && g.Discriminator == "" {
				ambiguousGroup(d, g, diags)
			}
		}
	}
}

func ambiguousGroup(d *Decl, g *Group, diags *diag.Collector) {
	var keys []string
	byRequired := map[string][]string{}
	for _, v := range g.Variants {
		shapes := v.Shapes
		if len(shapes) == 0 && v.Kinds == JSONObject {
			shapes = []Shape{{Required: v.Required}}
		}
		var own []string
		for _, sh := range shapes {
			key := strings.Join(slices.Sorted(slices.Values(sh.Required)), ", ")
			if slices.Contains(own, key) {
				continue
			}
			own = append(own, key)
			if _, ok := byRequired[key]; !ok {
				keys = append(keys, key)
			}
			byRequired[key] = append(byRequired[key], v.Name)
		}
	}

	for _, key := range keys {
		names := byRequired[key]
		if len(names) < 2 {
			continue
		}
		msg := fmt.Sprintf("variants %s of %s require the same properties (%s), so an object with only those matches each of them and fails to decode", strings.Join(names, ", "), d.Name, key)
		if key == "" {
			msg = fmt.Sprintf("variants %s of %s require no property, so {} matches each of them and fails to decode", strings.Join(names, ", "), d.Name)
		}
		diags.Append(diag.Diagnostic{Severity: diag.Warning, Code: diag.CodeUnionAmbiguous, Pointer: d.ID, Origin: d.Origin, Message: msg})
	}
}

func jsonKinds(t Type, seen map[*Decl]bool) JSONKind {
	switch t := t.(type) {
	case Builtin:
		return builtinKinds(t.Name)
	case Qualified:
		if slices.Contains(stringTypes, Type(t)) {
			return JSONString
		}
		return JSONAny
	case Pointer:
		return jsonKinds(t.Elem, seen)
	case Nullable:
		return jsonKinds(t.Elem, seen)
	case Slice:
		if t.Elem == byteType {
			return JSONString
		}
		return JSONArray
	case Map:
		return JSONObject
	case DeclRef:
		return declKinds(t.Decl, seen)
	}
	return JSONAny
}

func builtinKinds(name string) JSONKind {
	switch {
	case name == "string":
		return JSONString
	case name == "bool":
		return JSONBool
	case strings.HasPrefix(name, "float"):
		return JSONInteger | JSONNumber
	case strings.HasPrefix(name, "int"), strings.HasPrefix(name, "uint"):
		return JSONInteger
	}
	return JSONAny
}

func declKinds(d *Decl, seen map[*Decl]bool) JSONKind {
	if seen[d] {
		return 0
	}
	seen[d] = true

	switch {
	case d.Union != nil:
		k := JSONAny
		for _, g := range d.Union.Groups {
			var own JSONKind
			for _, v := range g.Variants {
				own |= jsonKinds(v.Type, seen)
			}
			k &= own
		}
		return k
	case d.Enum != nil:
		return jsonKinds(d.Enum.Base, seen)
	case d.Struct != nil:
		return JSONObject
	}
	return jsonKinds(d.Target, seen)
}

// structOf returns the struct a type is, through aliases, or nil.
func structOf(t Type) *Struct {
	for {
		r, ok := t.(DeclRef)
		switch {
		case !ok:
			return nil
		case r.Decl.Kind == KindStruct:
			return r.Decl.Struct
		case r.Decl.Kind != KindAlias:
			return nil
		}
		t = r.Decl.Target
	}
}

// unionOf returns the union declaration a type is, through aliases, or nil.
func unionOf(t Type) *Decl {
	if r, ok := unalias(t).(DeclRef); ok && r.Decl.Kind == KindUnion {
		return r.Decl
	}
	return nil
}

// structShape knows every field of st unless st takes additional properties.
func structShape(st *Struct) Shape {
	sh := Shape{IsClosed: st.IsClosed}
	for _, f := range st.Fields {
		if f.Required {
			sh.Required = append(sh.Required, f.JSONName)
		}
	}
	if st.AdditionalProperties == nil {
		sh.Known = make([]string, 0, len(st.Fields))
		for _, f := range st.Fields {
			sh.Known = append(sh.Known, f.JSONName)
		}
	}
	return sh
}

// unionShapes are the objects d can be: one variant of each group, with d's shared properties.
func unionShapes(d *Decl, seen map[*Decl]bool) []Shape {
	if seen[d] {
		return nil
	}
	seen[d] = true
	defer delete(seen, d)

	shared := structShape(d.Struct)
	var combined []Shape
	for i, g := range d.Union.Groups {
		if p := g.Discriminator; p != "" && !slices.Contains(shared.Known, p) {
			shared.Known = append(shared.Known, p)
		}
		var shapes []Shape
		for _, v := range g.Variants {
			shapes = append(shapes, objectShapes(v.Type, seen)...)
		}
		if i == 0 {
			combined = shapes
			continue
		}
		combined = joinShapes(combined, shapes)
	}

	out := make([]Shape, len(combined))
	for i, sh := range combined {
		sh.Required = appendNew(sh.Required, shared.Required)
		if sh.Known != nil {
			sh.Known = appendNew(sh.Known, shared.Known)
		}
		out[i] = sh
	}
	return out
}

// objectShapes are the objects a value of type t can be; a map or any is one that takes any key.
func objectShapes(t Type, seen map[*Decl]bool) []Shape {
	if st := structOf(t); st != nil {
		return []Shape{structShape(st)}
	}
	if u := unionOf(t); u != nil {
		return unionShapes(u, seen)
	}
	if JSONKinds(t)&JSONObject != 0 {
		return []Shape{{}}
	}
	return nil
}

// joinShapes are the objects that are one of a and one of b; a key either knows is known.
func joinShapes(a, b []Shape) []Shape {
	out := make([]Shape, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			sh := Shape{Required: appendNew(x.Required, y.Required), IsClosed: x.IsClosed || y.IsClosed}
			if x.Known != nil && y.Known != nil {
				sh.Known = appendNew(x.Known, y.Known)
			}
			out = append(out, sh)
		}
	}
	return out
}

func appendNew(list, more []string) []string {
	out := slices.Clone(list)
	for _, name := range more {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// typeName names a variant field after its type: String, Int64, Time, Pet, Pets, StringMap.
func typeName(t Type, n *naming.Namer) string {
	switch t := t.(type) {
	case Builtin:
		return n.Exported(t.Name)
	case Qualified:
		return t.Name
	case DeclRef:
		return t.Decl.Name
	case Pointer:
		return typeName(t.Elem, n)
	case Nullable:
		return typeName(t.Elem, n)
	case Slice:
		if t.Elem == byteType {
			return "Bytes"
		}
		name := typeName(t.Elem, n)
		if strings.HasSuffix(name, "s") {
			return name + "List"
		}
		return name + "s"
	case Map:
		return typeName(t.Elem, n) + "Map"
	}
	return "Value"
}

// target follows plain $refs to the schema they stand for.
func target(s *spec.Schema) *spec.Schema {
	seen := map[*spec.Schema]bool{}
	for !seen[s] {
		seen[s] = true
		r := refOf(s)
		if r == nil || r.Target == nil {
			return s
		}
		s = r.Target
	}
	return s
}

// isSameMember reports union members that check the same, or that a variant cannot check at all.
func isSameMember(a, b *spec.Schema) bool {
	return a == b || isBareRef(a) && isBareRef(b) && a.Ref.Target == b.Ref.Target || isRequiredOnly(a) && isRequiredOnly(b)
}

func isBareRef(s *spec.Schema) bool {
	rest := *s
	rest.Ref = nil
	return s.Ref != nil && isBare(&rest)
}

// isNoString reports a schema whose type, const or enum takes no string; null counts as neither.
func isNoString(f *spec.Schema) bool {
	isString := func(v spec.Value) bool { return v.Kind == spec.KindString }
	isOther := func(v spec.Value) bool { return v.Kind != spec.KindString && v.Kind != spec.KindNull }
	types := f.Types &^ spec.TypeNull
	return types != 0 && !types.Has(spec.TypeString) || f.Const != nil && isOther(*f.Const) ||
		slices.ContainsFunc(f.Enum, isOther) && !slices.ContainsFunc(f.Enum, isString)
}

// enumOfType keeps the enum values of one JSON type.
func enumOfType(values []spec.Value, set spec.TypeSet) []spec.Value {
	var out []spec.Value
	for _, v := range values {
		k := valueKind(v)
		if k == enumString && set == spec.TypeString || k == enumBool && set == spec.TypeBoolean ||
			k == enumInteger && (set == spec.TypeInteger || set == spec.TypeNumber) || k == enumNumber && set == spec.TypeNumber {
			out = append(out, v)
		}
	}
	return out
}
