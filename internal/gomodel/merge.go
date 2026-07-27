// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

var typeWords = []struct {
	set  spec.TypeSet
	word string
}{
	{spec.TypeString, "string"},
	{spec.TypeNumber, "number"},
	{spec.TypeInteger, "integer"},
	{spec.TypeBoolean, "boolean"},
	{spec.TypeObject, "object"},
	{spec.TypeArray, "array"},
	{spec.TypeNull, "null"},
}

// mergePart is one schema whose own keywords go into a merge.
type mergePart struct {
	schema    *spec.Schema
	isForeign bool
}

// partWalk collects the parts of one merge; onStack finds allOf cycles.
type partWalk struct {
	root    *spec.Schema
	parts   []mergePart
	seen    map[*spec.Schema]bool
	onStack map[*spec.Schema]bool
}

// merged is a flattened schema. Foreign children came from a type reached through a $ref: that
// type names them. Refs are the $refs of the schema itself and its inline members.
type merged struct {
	schema  *spec.Schema
	foreign map[*spec.Schema]bool
	refs    []*spec.Ref
}

// flattener merges allOf members into one schema. The spec IR is never changed: merged schemas
// are new, made once per source schema.
type flattener struct {
	memo       map[*spec.Schema]*merged
	inProgress map[*spec.Schema]bool
	// isWrapper marks the $ref made around a foreign schema when it is merged with another.
	isWrapper map[*spec.Schema]bool
	diags     *diag.Collector
}

func newFlattener(diags *diag.Collector) *flattener {
	return &flattener{
		memo:       map[*spec.Schema]*merged{},
		inProgress: map[*spec.Schema]bool{},
		isWrapper:  map[*spec.Schema]bool{},
		diags:      diags,
	}
}

// flatten returns s itself unless it has allOf, or a $ref with shape keywords next to it. Then it
// returns the merge of s, its $ref target and every allOf member, deep: properties, required,
// items, additionalProperties, limits and flags.
func (f *flattener) flatten(s *spec.Schema) *spec.Schema {
	if m := f.merged(s); m != nil {
		return m.schema
	}
	return s
}

// merged returns the merge of s, or nil when s needs none.
func (f *flattener) merged(s *spec.Schema) *merged {
	if refOf(s) != nil || len(s.AllOf) == 0 && s.Ref == nil {
		return nil
	}
	if m, ok := f.memo[s]; ok {
		return m
	}

	f.inProgress[s] = true
	defer delete(f.inProgress, s)
	w := &partWalk{root: s, seen: map[*spec.Schema]bool{}, onStack: map[*spec.Schema]bool{}}
	f.collect(s, false, w)

	m := &merged{schema: &spec.Schema{Origin: s.Origin, Extensions: s.Extensions}, foreign: map[*spec.Schema]bool{}}
	f.memo[s] = m
	for _, p := range w.parts {
		f.merge(m, p)
	}
	for _, p := range m.schema.Properties {
		p.Required = slices.Contains(m.schema.Required, p.Name)
	}
	return m
}

// collect puts a $ref target before the schema and allOf members after it.
func (f *flattener) collect(s *spec.Schema, isForeign bool, w *partWalk) {
	switch {
	case w.onStack[s]:
		f.diags.Append(allOfCycle(w.root, s))
		return
	case w.seen[s]:
		return
	}
	w.seen[s], w.onStack[s] = true, true
	defer delete(w.onStack, s)

	if s.Ref != nil && s.Ref.Target != nil {
		f.collectRef(s.Ref.Target, w)
	}
	w.parts = append(w.parts, mergePart{schema: s, isForeign: isForeign})
	for _, m := range s.AllOf {
		f.collect(m, isForeign, w)
	}
}

// collectRef takes a target that is merged itself as one part, so its merged children keep
// the names they have there.
func (f *flattener) collectRef(t *spec.Schema, w *partWalk) {
	if f.inProgress[t] {
		f.diags.Append(allOfCycle(w.root, t))
		return
	}

	m := f.merged(t)
	switch {
	case m == nil:
		f.collect(t, true, w)
	case !w.seen[m.schema]:
		w.seen[m.schema] = true
		w.parts = append(w.parts, mergePart{schema: m.schema, isForeign: true})
	}
}

// merge adds the keywords of one part. Docs come from parts not reached through a $ref, so a
// schema does not take the description of the type it extends.
func (f *flattener) merge(m *merged, p mergePart) {
	dst, s := m.schema, p.schema
	f.mergeTypes(dst, s)
	dst.Nullable = dst.Nullable || s.Nullable || s.Types == spec.TypeNull
	dst.ReadOnly = dst.ReadOnly || s.ReadOnly
	dst.WriteOnly = dst.WriteOnly || s.WriteOnly
	dst.Deprecated = dst.Deprecated || s.Deprecated
	if !p.isForeign {
		dst.Title = cmp.Or(dst.Title, s.Title)
		dst.Description = cmp.Or(dst.Description, s.Description)
	}
	if !p.isForeign && s.Ref != nil && !f.isWrapper[s] {
		m.refs = append(m.refs, s.Ref)
	}

	dst.Format = cmp.Or(dst.Format, s.Format)
	dst.Pattern = cmp.Or(dst.Pattern, s.Pattern)
	dst.ContentEncoding = cmp.Or(dst.ContentEncoding, s.ContentEncoding)
	dst.ContentMediaType = cmp.Or(dst.ContentMediaType, s.ContentMediaType)
	for _, r := range s.Required {
		if !slices.Contains(dst.Required, r) {
			dst.Required = append(dst.Required, r)
		}
	}

	for _, prop := range s.Properties {
		f.mergeProperty(m, prop, p.isForeign)
	}
	f.mergeAdditional(m, p)
	dst.Items = f.mergeChild(m, dst.Items, s.Items, p.isForeign, "/allOf/items")
	if len(dst.PrefixItems) == 0 {
		dst.PrefixItems = s.PrefixItems
	}
	dst.OneOf = append(dst.OneOf, s.OneOf...)
	dst.AnyOf = append(dst.AnyOf, s.AnyOf...)

	dst.Not = cmp.Or(dst.Not, s.Not)
	dst.If = cmp.Or(dst.If, s.If)
	dst.Then = cmp.Or(dst.Then, s.Then)
	dst.Else = cmp.Or(dst.Else, s.Else)
	dst.Discriminator = cmp.Or(dst.Discriminator, s.Discriminator)
	if len(dst.Enum) == 0 {
		dst.Enum = s.Enum
	}
	dst.Const = cmp.Or(dst.Const, s.Const)
	dst.Default = cmp.Or(dst.Default, s.Default)
	if len(dst.Examples) == 0 {
		dst.Examples = s.Examples
	}
	mergeLimits(&dst.Limits, s.Limits)
}

// mergeTypes keeps the types both allow; a number and an integer give an integer.
func (f *flattener) mergeTypes(m, s *spec.Schema) {
	switch {
	case s.Types == 0, s.Types == spec.TypeNull:
		return
	case m.Types == 0:
		m.Types = s.Types
		return
	}

	t := m.Types & s.Types
	if m.Types.Has(spec.TypeNumber) && s.Types.Has(spec.TypeInteger) || m.Types.Has(spec.TypeInteger) && s.Types.Has(spec.TypeNumber) {
		t |= spec.TypeInteger
	}
	if t != 0 {
		m.Types = t
		return
	}

	f.diags.Append(diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeAllOfConflict,
		Pointer:  m.Origin.Pointer,
		Origin:   origin(m.Origin),
		Message:  fmt.Sprintf("allOf members disagree on the type (%s, %s); keeping %s", typeSetText(m.Types), typeSetText(s.Types), typeSetText(m.Types)),
	})
}

func (f *flattener) mergeProperty(m *merged, prop *spec.Property, isForeign bool) {
	props := m.schema.Properties
	i := slices.IndexFunc(props, func(p *spec.Property) bool { return p.Name == prop.Name })
	if i < 0 {
		m.schema.Properties = append(m.schema.Properties, &spec.Property{Name: prop.Name, Schema: f.mergeChild(m, nil, prop.Schema, isForeign, "")})
		return
	}
	props[i].Schema = f.mergeChild(m, props[i].Schema, prop.Schema, isForeign, "/allOf/properties/"+oasdoc.Escape(prop.Name))
}

// mergeAdditional prefers a schema, then the first mode set.
func (f *flattener) mergeAdditional(m *merged, p mergePart) {
	dst, src := &m.schema.AdditionalProperties, p.schema.AdditionalProperties
	switch {
	case src.Mode == spec.AdditionalSchema:
		var cur *spec.Schema
		if dst.Mode == spec.AdditionalSchema {
			cur = dst.Schema
		}
		*dst = spec.Additional{Mode: spec.AdditionalSchema, Schema: f.mergeChild(m, cur, src.Schema, p.isForeign, "/allOf/additionalProperties")}
	case dst.Mode == spec.AdditionalUnset:
		*dst = src
	}
}

// mergeChild combines two schemas for one place into a new allOf, unless one adds nothing. A
// foreign side goes in as a $ref, so the new type leaves that side's children to their owner.
func (f *flattener) mergeChild(m *merged, a, b *spec.Schema, isForeign bool, suffix string) *spec.Schema {
	switch {
	case b == nil, a == b, a != nil && (isDocOnly(b) || isSameRef(a, b)):
		return a
	case a == nil, isDocOnly(a):
		if isForeign {
			m.foreign[b] = true
		}
		return b
	}

	return &spec.Schema{
		AllOf:  []*spec.Schema{f.side(a, m.foreign[a]), f.side(b, isForeign)},
		Origin: spec.Origin{Pointer: m.schema.Origin.Pointer + suffix, File: a.Origin.File, Line: a.Origin.Line, Col: a.Origin.Col},
	}
}

func (f *flattener) side(s *spec.Schema, isForeign bool) *spec.Schema {
	if !isForeign {
		return s
	}
	w := &spec.Schema{Ref: &spec.Ref{Pointer: s.Origin.Pointer, Target: s}, Origin: s.Origin}
	f.isWrapper[w] = true
	return w
}

func allOfCycle(root, through *spec.Schema) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Error,
		Code:     diag.CodeAllOfCycle,
		Pointer:  root.Origin.Pointer,
		Origin:   origin(root.Origin),
		Message:  "allOf includes itself through " + through.Origin.Pointer + "; the loop is left out",
	}
}

func isSameRef(a, b *spec.Schema) bool {
	ra, rb := refOf(a), refOf(b)
	return ra != nil && rb != nil && ra.Target == rb.Target
}

// mergeLimits keeps the first value set for each limit.
func mergeLimits(dst *spec.Limits, src spec.Limits) {
	dst.Minimum = cmp.Or(dst.Minimum, src.Minimum)
	dst.Maximum = cmp.Or(dst.Maximum, src.Maximum)
	dst.MultipleOf = cmp.Or(dst.MultipleOf, src.MultipleOf)
	dst.MinLength = cmp.Or(dst.MinLength, src.MinLength)
	dst.MaxLength = cmp.Or(dst.MaxLength, src.MaxLength)
	dst.MinItems = cmp.Or(dst.MinItems, src.MinItems)
	dst.MaxItems = cmp.Or(dst.MaxItems, src.MaxItems)
	dst.MinProperties = cmp.Or(dst.MinProperties, src.MinProperties)
	dst.MaxProperties = cmp.Or(dst.MaxProperties, src.MaxProperties)
	dst.UniqueItems = dst.UniqueItems || src.UniqueItems
}

func typeSetText(t spec.TypeSet) string {
	var words []string
	for _, w := range typeWords {
		if t.Has(w.set) {
			words = append(words, w.word)
		}
	}
	return strings.Join(words, " or ")
}
