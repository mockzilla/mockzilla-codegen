// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package jsonschema writes schemas of the spec IR as JSON Schema 2020-12 documents, the way MCP
// tools describe their input. Every $ref goes through $defs, so a schema that refers to itself
// ends.
package jsonschema

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// typeNames are the JSON types of the type set, in the order the schema lists them.
var typeNames = []struct {
	set  spec.TypeSet
	name string
}{
	{spec.TypeString, "string"},
	{spec.TypeNumber, "number"},
	{spec.TypeInteger, "integer"},
	{spec.TypeBoolean, "boolean"},
	{spec.TypeObject, "object"},
	{spec.TypeArray, "array"},
	{spec.TypeNull, "null"},
}

// Builder converts schemas into one document and collects what they refer to in its $defs: a
// component under its name, anything else under its JSON pointer.
type Builder struct {
	defs  map[string]*Object
	diags []diag.Diagnostic
}

func NewBuilder() *Builder {
	return &Builder{defs: map[string]*Object{}}
}

// Schema converts s. A nil schema is the empty one, which takes anything.
func (b *Builder) Schema(s *spec.Schema) *Object {
	o := &Object{}
	if s == nil {
		return o
	}
	if s.Ref != nil {
		o.Set("$ref", "#/$defs/"+escapePointer(b.define(s.Ref)))
	}

	core(o, s)
	b.composition(o, s)
	b.objects(o, s)
	b.arrays(o, s)
	b.values(o, s)
	limits(o, s)
	return o
}

// Document returns root with the $defs collected so far, compact. The definitions come sorted by
// name.
func (b *Builder) Document(root *Object) []byte {
	if len(b.defs) > 0 {
		defs := &Object{}
		for _, name := range slices.Sorted(maps.Keys(b.defs)) {
			defs.Set(name, b.defs[name])
		}
		root.Set("$defs", defs)
	}
	return root.append(nil)
}

// Diagnostics are the warnings of the schemas converted so far.
func (b *Builder) Diagnostics() []diag.Diagnostic {
	return b.diags
}

// define converts the target of r once and returns the name it is defined under.
func (b *Builder) define(r *spec.Ref) string {
	name := r.Name
	if name == "" {
		name = r.Pointer
	}
	if _, ok := b.defs[name]; ok {
		return name
	}

	// The entry exists before the target is converted, so a cycle stops here.
	b.defs[name] = nil
	b.defs[name] = b.Schema(r.Target)
	return name
}

func core(o *Object, s *spec.Schema) {
	if names := typeList(s); len(names) == 1 {
		o.Set("type", names[0])
	} else if len(names) > 1 {
		o.Set("type", names)
	}
	setString(o, "format", s.Format)
	setString(o, "title", s.Title)
	setString(o, "description", s.Description)
	setBool(o, "deprecated", s.Deprecated)
	setBool(o, "readOnly", s.ReadOnly)
	setBool(o, "writeOnly", s.WriteOnly)
	setString(o, "contentEncoding", s.ContentEncoding)
	setString(o, "contentMediaType", s.ContentMediaType)
}

func (b *Builder) composition(o *Object, s *spec.Schema) {
	b.setList(o, "allOf", s.AllOf)
	b.setList(o, "oneOf", s.OneOf)
	b.setList(o, "anyOf", s.AnyOf)
	b.setSchema(o, "not", s.Not)
	b.setSchema(o, "if", s.If)
	b.setSchema(o, "then", s.Then)
	b.setSchema(o, "else", s.Else)
}

func (b *Builder) objects(o *Object, s *spec.Schema) {
	if len(s.Properties) > 0 {
		props := &Object{}
		for _, p := range s.Properties {
			props.Set(p.Name, b.Schema(p.Schema))
		}
		o.Set("properties", props)
	}
	if len(s.Required) > 0 {
		o.Set("required", s.Required)
	}

	switch s.AdditionalProperties.Mode {
	case spec.AdditionalAllowed:
		o.Set("additionalProperties", true)
	case spec.AdditionalDenied:
		o.Set("additionalProperties", false)
	case spec.AdditionalSchema:
		o.Set("additionalProperties", b.Schema(s.AdditionalProperties.Schema))
	case spec.AdditionalUnset:
	}
}

func (b *Builder) arrays(o *Object, s *spec.Schema) {
	b.setList(o, "prefixItems", s.PrefixItems)
	b.setSchema(o, "items", s.Items)
}

func (b *Builder) setSchema(o *Object, key string, s *spec.Schema) {
	if s != nil {
		o.Set(key, b.Schema(s))
	}
}

func (b *Builder) setList(o *Object, key string, list []*spec.Schema) {
	if len(list) == 0 {
		return
	}
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = b.Schema(s)
	}
	o.Set(key, out)
}

// values sets enum, const, default and examples. A default that does not fit its schema is left
// out with a warning, since the MCP SDK panics on it when the tool is added.
func (b *Builder) values(o *Object, s *spec.Schema) {
	if len(s.Enum) > 0 {
		o.Set("enum", valueList(s.Enum))
	}
	if s.Const != nil {
		o.Set("const", Value(*s.Const))
	}
	if s.Default != nil {
		if why := misfit(*s.Default, s); why == "" {
			o.Set("default", Value(*s.Default))
		} else {
			b.diags = append(b.diags, ignored(s, why))
		}
	}
	if len(s.Examples) > 0 {
		o.Set("examples", valueList(s.Examples))
	}
}

// limits sets the bounds; an exclusive one is written as 2020-12 does, with the bound as the value.
func limits(o *Object, s *spec.Schema) {
	l := s.Limits
	setBound(o, "minimum", "exclusiveMinimum", l.Minimum)
	setBound(o, "maximum", "exclusiveMaximum", l.Maximum)
	if l.MultipleOf != nil {
		o.Set("multipleOf", *l.MultipleOf)
	}
	setCount(o, "minLength", l.MinLength)
	setCount(o, "maxLength", l.MaxLength)
	setCount(o, "minItems", l.MinItems)
	setCount(o, "maxItems", l.MaxItems)
	setBool(o, "uniqueItems", l.UniqueItems)
	setCount(o, "minProperties", l.MinProperties)
	setCount(o, "maxProperties", l.MaxProperties)
}

// Value converts a spec value into what Object holds.
func Value(v spec.Value) any {
	switch v.Kind {
	case spec.KindString:
		return v.Str
	case spec.KindNumber:
		return v.Num
	case spec.KindBool:
		return v.Bool
	case spec.KindArray:
		return valueList(v.Items)
	case spec.KindObject:
		o := &Object{}
		for _, f := range v.Fields {
			o.Set(f.Name, Value(f.Value))
		}
		return o
	case spec.KindNull:
	}
	return nil
}

func valueList(vs []spec.Value) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = Value(v)
	}
	return out
}

// typeList is the JSON types of s, with null added for a nullable schema.
func typeList(s *spec.Schema) []string {
	var out []string
	for _, t := range typeNames {
		if s.Types.Has(t.set) {
			out = append(out, t.name)
		}
	}
	if s.Nullable && !slices.Contains(out, "null") {
		out = append(out, "null")
	}
	return out
}

func setBound(o *Object, key, exclusiveKey string, b *spec.Bound) {
	switch {
	case b == nil:
	case b.Exclusive:
		o.Set(exclusiveKey, b.Value)
	default:
		o.Set(key, b.Value)
	}
}

func setCount(o *Object, key string, n *int64) {
	if n != nil {
		o.Set(key, json.Number(strconv.FormatInt(*n, 10)))
	}
}

func setString(o *Object, key, s string) {
	if s != "" {
		o.Set(key, s)
	}
}

func setBool(o *Object, key string, b bool) {
	if b {
		o.Set(key, true)
	}
}

// ignored is the warning for the default of s, left out for why.
func ignored(s *spec.Schema, why string) diag.Diagnostic {
	subject := "the default"
	if text := shown(*s.Default); text != "" {
		subject += " " + text
	}
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeDefaultIgnored,
		Pointer:  s.Origin.Pointer,
		Origin:   diag.Origin{File: s.Origin.File, Line: s.Origin.Line, Col: s.Origin.Col},
		Message:  subject + " does not fit its schema, so the tool input leaves it out: " + why,
	}
}

// escapePointer writes a definition name as a JSON pointer token.
func escapePointer(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}
