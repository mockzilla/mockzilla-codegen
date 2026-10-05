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
// list; suffix names its declaration when it is inline: its title, else its first discriminator
// value, else Option and its place.
type unionMember struct {
	schema    *spec.Schema
	index     int
	suffix    string
	values    []string
	isDefault bool
}

// unionSchema is how a union schema reads. The members of a type list are made from its types
// and hold its properties; other unions share their properties across members.
type unionSchema struct {
	isAnyOf       bool
	isNullable    bool
	isTypeList    bool
	discriminator string
	members       []unionMember
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
	u := &unionSchema{isNullable: f.Nullable}
	r.memo[f] = u

	switch {
	case len(nonNull(f.OneOf))+len(nonNull(f.AnyOf)) > 1:
		list := f.OneOf
		if len(list) == 0 {
			list, u.isAnyOf = f.AnyOf, true
		}
		r.add(u, list)
		r.discriminate(u, f.Discriminator)
	case f.Then != nil && f.Else != nil:
		r.add(u, []*spec.Schema{f.Then, f.Else})
		u.members[0].suffix, u.members[1].suffix = "Then", "Else"
		r.predicate(u, f.If)
	default:
		u.isTypeList = true
		r.typeList(u, f)
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

func (r *unionReader) add(u *unionSchema, list []*spec.Schema) {
	for i, s := range list {
		if isNull(s) {
			u.isNullable = true
			continue
		}
		u.members = append(u.members, unionMember{schema: s, index: i})
	}
}

// discriminate gives each member its discriminator values: those the mapping lists for it, else
// the const or single-value enum of its discriminator property, else its component name.
func (r *unionReader) discriminate(u *unionSchema, d *spec.Discriminator) {
	if d == nil {
		return
	}

	u.discriminator = d.Property
	for i := range u.members {
		m := &u.members[i]
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
// that value, else any other.
func (r *unionReader) predicate(u *unionSchema, cond *spec.Schema) {
	if cond == nil {
		return
	}
	f := r.flat.flatten(target(cond))
	if len(f.Properties) != 1 {
		return
	}
	p := f.Properties[0]
	if values := r.constValues(p.Schema); len(values) > 0 {
		u.discriminator = p.Name
		u.members[0].values = values
		u.members[1].isDefault = true
	}
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

// propertyValues are the values the property prop of s allows alone.
func (r *unionReader) propertyValues(s *spec.Schema, prop string) []string {
	f := r.flat.flatten(target(s))
	if i := slices.IndexFunc(f.Properties, func(p *spec.Property) bool { return p.Name == prop }); i >= 0 {
		return r.constValues(f.Properties[i].Schema)
	}
	return nil
}

// constValues is the const of s, or the value of an enum with one value.
func (r *unionReader) constValues(s *spec.Schema) []string {
	f := r.flat.flatten(target(s))
	switch {
	case f.Const != nil && f.Const.Kind != spec.KindNull:
		return []string{valueText(*f.Const)}
	case len(f.Enum) == 1 && f.Enum[0].Kind != spec.KindNull:
		return []string{valueText(f.Enum[0])}
	}
	return nil
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
			v.FieldType = elemType(v.Type, true)
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
		u := d.Union
		if u == nil || u.IsAnyOf || u.Discriminator != "" {
			continue
		}

		var keys []string
		byRequired := map[string][]string{}
		for _, v := range u.Variants {
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
		var k JSONKind
		for _, v := range d.Union.Variants {
			k |= jsonKinds(v.Type, seen)
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

// unionShapes are the objects d can be, each with d's shared properties.
func unionShapes(d *Decl, seen map[*Decl]bool) []Shape {
	if seen[d] {
		return nil
	}
	seen[d] = true
	defer delete(seen, d)

	shared := structShape(d.Struct)
	if p := d.Union.Discriminator; p != "" && !slices.Contains(shared.Known, p) {
		shared.Known = append(shared.Known, p)
	}
	var out []Shape
	for _, v := range d.Union.Variants {
		for _, sh := range objectShapes(v.Type, seen) {
			sh.Required = appendNew(sh.Required, shared.Required)
			if sh.Known != nil {
				sh.Known = appendNew(sh.Known, shared.Known)
			}
			out = append(out, sh)
		}
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
