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
	"fmt"
	"iter"
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

// base64Formats are the formats of bytes, which a JSON document holds as base64.
var base64Formats = []string{"binary", "byte"}

// Builder converts schemas into one document and collects what they refer to in its $defs: a
// component under its name, anything else under its JSON pointer.
type Builder struct {
	defs  map[string]*Object
	diags []diag.Diagnostic
}

func NewBuilder() *Builder {
	return &Builder{defs: map[string]*Object{}}
}

// Schema converts s. A nil schema is the empty one, which takes anything. A nullable schema takes
// null: when its $ref, composition, enum or const could turn null away, it is anyOf of itself and
// null, with its docs and values outside. A readOnly property is left out with its required entry,
// since a request does not carry it; readOnly in an allOf member counts for the whole object.
func (b *Builder) Schema(s *spec.Schema) *Object {
	return b.schema(s, nil)
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

// schema converts s; hidden are the readOnly properties of the object s is an allOf member of.
func (b *Builder) schema(s *spec.Schema, hidden []string) *Object {
	o := &Object{}
	if s == nil {
		return o
	}
	if s.Nullable && rejectsNull(s) {
		return b.nullable(s)
	}
	if s.Ref != nil {
		o.Set("$ref", "#/$defs/"+escapePointer(b.define(s.Ref)))
	}

	hidden = slices.Concat(hidden, readOnlyNames(s))
	core(o, s)
	b.composition(o, s, hidden)
	b.objects(o, s, hidden)
	b.arrays(o, s)
	b.values(o, s)
	limits(o, s)
	b.pattern(o, s)
	return o
}

// nullable writes s as anyOf of s without null and of null. The docs and values stay outside, so a
// reader sees them first and a default is checked against the whole.
func (b *Builder) nullable(s *spec.Schema) *Object {
	inner := *s
	inner.Nullable = false
	inner.Title, inner.Description, inner.Default, inner.Examples = "", "", nil, nil
	inner.Deprecated, inner.ReadOnly, inner.WriteOnly = false, false, false

	o := new(Object).Set("anyOf", []any{b.Schema(&inner), new(Object).Set("type", "null")})
	setString(o, "title", s.Title)
	setString(o, "description", s.Description)
	setBool(o, "deprecated", s.Deprecated)
	setBool(o, "readOnly", s.ReadOnly)
	setBool(o, "writeOnly", s.WriteOnly)
	b.samples(o, s)
	return o
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
	encoding := s.ContentEncoding
	if encoding == "" && slices.Contains(base64Formats, strings.ToLower(s.Format)) {
		encoding = "base64"
	}
	setString(o, "contentEncoding", encoding)
	setString(o, "contentMediaType", s.ContentMediaType)
}

// composition sets the subschemas; the allOf members of s hide its readOnly properties too.
func (b *Builder) composition(o *Object, s *spec.Schema, hidden []string) {
	if len(s.AllOf) > 0 {
		members := make([]any, len(s.AllOf))
		for i, m := range s.AllOf {
			members[i] = b.schema(m, hidden)
		}
		o.Set("allOf", members)
	}
	b.setList(o, "oneOf", s.OneOf)
	b.setList(o, "anyOf", s.AnyOf)
	b.setSchema(o, "not", s.Not)
	b.setSchema(o, "if", s.If)
	b.setSchema(o, "then", s.Then)
	b.setSchema(o, "else", s.Else)
}

func (b *Builder) objects(o *Object, s *spec.Schema, hidden []string) {
	props := &Object{}
	for _, p := range s.Properties {
		if !slices.Contains(hidden, p.Name) {
			props.Set(p.Name, b.Schema(p.Schema))
		}
	}
	if props.Len() > 0 {
		o.Set("properties", props)
	}
	required := slices.DeleteFunc(slices.Clone(s.Required), func(name string) bool { return slices.Contains(hidden, name) })
	if len(required) > 0 {
		o.Set("required", required)
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

// values sets enum and const, then the samples.
func (b *Builder) values(o *Object, s *spec.Schema) {
	if len(s.Enum) > 0 {
		o.Set("enum", valueList(s.Enum))
	}
	if s.Const != nil {
		o.Set("const", Value(*s.Const))
	}
	b.samples(o, s)
}

// samples sets the default and the examples. A default that does not fit its schema is left out
// with a warning, since the MCP SDK panics on it when the tool is added.
func (b *Builder) samples(o *Object, s *spec.Schema) {
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

// pattern sets the pattern of s in the form compilePattern gives. One Go's regexp cannot compile
// is left out with a warning, since the MCP SDK panics on it when the tool is added.
func (b *Builder) pattern(o *Object, s *spec.Schema) {
	if s.Pattern == "" {
		return
	}
	re, err := compilePattern(s.Pattern)
	if err != nil {
		b.diags = append(b.diags, unsupported(s, err))
		return
	}

	o.Set("pattern", re.String())
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

// typeList is the JSON types of s, with null added for a nullable schema. A schema without types
// takes null already, so it gets none.
func typeList(s *spec.Schema) []string {
	var out []string
	for _, t := range typeNames {
		if s.Types.Has(t.set) {
			out = append(out, t.name)
		}
	}
	if s.Nullable && len(out) > 0 && !slices.Contains(out, "null") {
		out = append(out, "null")
	}
	return out
}

// rejectsNull reports a keyword of s other than type that null can fail: a $ref, a composition, an
// enum without null or a const that is not null.
func rejectsNull(s *spec.Schema) bool {
	isNull := func(v spec.Value) bool { return v.Kind == spec.KindNull }
	return s.Ref != nil || len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0 || s.Not != nil || s.If != nil ||
		len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, isNull) || s.Const != nil && !isNull(*s.Const)
}

// readOnlyNames are the readOnly properties of s and of what composes it.
func readOnlyNames(s *spec.Schema) []string {
	var names []string
	for x := range composed(s) {
		for _, p := range x.Properties {
			if isReadOnly(p.Schema) {
				names = append(names, p.Name)
			}
		}
	}
	return names
}

// isReadOnly reports s marked readOnly by itself or by what composes it.
func isReadOnly(s *spec.Schema) bool {
	for x := range composed(s) {
		if x.ReadOnly {
			return true
		}
	}
	return false
}

// composed yields s, its allOf members and the targets of the $refs among them, at any depth, once
// each.
func composed(s *spec.Schema) iter.Seq[*spec.Schema] {
	return func(yield func(*spec.Schema) bool) {
		seen := map[*spec.Schema]bool{}
		var walk func(x *spec.Schema) bool
		walk = func(x *spec.Schema) bool {
			if x == nil || seen[x] {
				return true
			}
			seen[x] = true
			if !yield(x) {
				return false
			}
			for _, m := range x.AllOf {
				if !walk(m) {
					return false
				}
			}
			return x.Ref == nil || walk(x.Ref.Target)
		}
		walk(s)
	}
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

// unsupported is the warning for the pattern of s, which Go's regexp cannot compile for err.
func unsupported(s *spec.Schema, err error) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodePatternUnsupported,
		Pointer:  s.Origin.Pointer,
		Origin:   diag.Origin{File: s.Origin.File, Line: s.Origin.Line, Col: s.Origin.Col},
		Message:  fmt.Sprintf("pattern %q is not RE2 (%v), so the tool input leaves it out", s.Pattern, err),
	}
}

// escapePointer writes a definition name as a JSON pointer token.
func escapePointer(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}
