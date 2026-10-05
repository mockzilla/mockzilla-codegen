// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// allOf members, and a $ref with keywords next to it, merged into one schema.

package gomodel

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
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

// mergePart is one schema whose own keywords go into a merge; from is set for a merged $ref target.
type mergePart struct {
	schema    *spec.Schema
	isForeign bool
	from      *merged
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
	joint   jointChecks
	goType  *extension.Type
}

// flattener merges allOf members into one schema. The spec IR is never changed: merged schemas
// are new, made once per source schema.
type flattener struct {
	memo       map[*spec.Schema]*merged
	inProgress map[*spec.Schema]bool
	// isWrapper marks the $ref made around a foreign schema when it is merged with another.
	isWrapper map[*spec.Schema]bool
	ext       *extReader
	diags     *diag.Collector
}

// jointChecks are the patterns and factors that all have to hold, where a schema keeps one of each.
type jointChecks struct {
	patterns  []*spec.Schema
	multiples []json.Number
}

func newFlattener(ext *extReader, diags *diag.Collector) *flattener {
	return &flattener{
		memo:       map[*spec.Schema]*merged{},
		inProgress: map[*spec.Schema]bool{},
		isWrapper:  map[*spec.Schema]bool{},
		ext:        ext,
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
	if refOf(s) != nil || len(members(s)) == 0 && s.Ref == nil && branch(s) == nil {
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
	if f.ownType(s) == nil {
		f.typeFrom(m, w.parts)
	}
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

	// The x-go-type of a member stands for all it is made of.
	isTyped := s != w.root && f.ownType(s) != nil
	if s.Ref != nil && s.Ref.Target != nil && !isTyped {
		f.collectRef(s.Ref.Target, w)
	}
	w.parts = append(w.parts, mergePart{schema: s, isForeign: isForeign})
	if isTyped {
		return
	}
	for _, m := range members(s) {
		f.collect(m, isForeign, w)
	}
	if b := branch(s); b != nil {
		f.collect(f.optional(b), isForeign, w)
	}
}

// optional returns a schema with the properties of s, merged, none of them required: an if with a
// single branch adds that branch's properties, which apply only when the if holds.
func (f *flattener) optional(s *spec.Schema) *spec.Schema {
	return &spec.Schema{Properties: f.flatten(s).Properties, Origin: s.Origin}
}

// collectRef takes a target that is merged itself as one part, so its merged children keep
// the names they have there. A target whose oneOf or anyOf lists the root is a parent the root
// extends: the root is one of its variants, so the parent comes in without that union.
func (f *flattener) collectRef(t *spec.Schema, w *partWalk) {
	if listsSchema(t, w.root) {
		c := *t
		c.OneOf, c.AnyOf = nil, nil
		f.collect(&c, true, w)
		return
	}
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
		w.parts = append(w.parts, mergePart{schema: m.schema, isForeign: true, from: m})
	}
}

// typeFrom gives the merge the x-go-type of the first part that sets one and warns about the rest.
func (f *flattener) typeFrom(m *merged, parts []mergePart) {
	i := slices.IndexFunc(parts, func(p mergePart) bool { return f.partType(p) != nil })
	if i < 0 {
		return
	}
	typed := parts[i]
	m.goType = f.partType(typed)
	for _, p := range parts[i+1:] {
		other := f.partType(p)
		var msg string
		switch {
		case other != nil && *other != *m.goType:
			msg = fmt.Sprintf("allOf members set x-go-type %s and %s; keeping %s", m.goType.Name, other.Name, m.goType.Name)
		case other == nil && addsType(p.schema, typed.schema):
			msg = fmt.Sprintf("allOf member %s sets x-go-type %s; keeping it, what %s adds is not generated", typed.schema.Origin.Pointer, m.goType.Name, p.schema.Origin.Pointer)
		default:
			continue
		}
		f.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeAllOfConflict,
			Pointer:  m.schema.Origin.Pointer,
			Origin:   origin(m.schema.Origin),
			Message:  msg,
		})
		return
	}
}

// goTypeOf is the x-go-type of s: its own, else that of the first member of its merge.
func (f *flattener) goTypeOf(s *spec.Schema) *extension.Type {
	if t := f.ownType(s); t != nil {
		return t
	}
	if m := f.merged(s); m != nil {
		return m.goType
	}
	return nil
}

func (f *flattener) ownType(s *spec.Schema) *extension.Type {
	return f.ext.of(s.Extensions, s.Origin).GoType
}

func (f *flattener) partType(p mergePart) *extension.Type {
	if t := f.ownType(p.schema); t != nil || p.from == nil {
		return t
	}
	return p.from.goType
}

// merge adds the keywords of one part. Docs come from parts not reached through a $ref, so a
// schema does not take the description of the type it extends.
func (f *flattener) merge(m *merged, p mergePart) {
	dst, s := m.schema, p.schema
	if !mergeTypes(dst, s) && m.goType == nil {
		f.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeAllOfConflict,
			Pointer:  dst.Origin.Pointer,
			Origin:   origin(dst.Origin),
			Message:  fmt.Sprintf("allOf members disagree on the type (%s, %s); keeping %s", typeSetText(dst.Types), typeSetText(s.Types), typeSetText(dst.Types)),
		})
	}
	isSole := soleMember(s) != nil
	dst.Nullable = dst.Nullable || s.Nullable || s.Types == spec.TypeNull || isSole && hasNullMember(s)
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
	if p.from != nil {
		m.joint.join(p.from.joint)
	}
	m.joint.add(s)
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
	dst.PropertyNames = bothHold(dst.PropertyNames, s.PropertyNames)
	dst.Items = f.mergeChild(m, dst.Items, s.Items, p.isForeign, "/allOf/items")
	if len(dst.PrefixItems) == 0 {
		dst.PrefixItems = s.PrefixItems
	}
	if !isSole {
		dst.OneOf = append(dst.OneOf, s.OneOf...)
		dst.AnyOf = append(dst.AnyOf, s.AnyOf...)
	}
	if s.Then != nil && s.Else != nil && dst.Then == nil {
		dst.If, dst.Then, dst.Else = s.If, s.Then, s.Else
	}

	dst.Not = cmp.Or(dst.Not, s.Not)
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

// add takes the pattern and the multipleOf of s, each once.
func (c *jointChecks) add(s *spec.Schema) {
	c.addPattern(s)
	if n := s.Limits.MultipleOf; n != nil {
		c.addMultiple(*n)
	}
}

func (c *jointChecks) join(other jointChecks) {
	for _, p := range other.patterns {
		c.addPattern(p)
	}
	for _, n := range other.multiples {
		c.addMultiple(n)
	}
}

func (c *jointChecks) addPattern(s *spec.Schema) {
	if s.Pattern != "" && !slices.ContainsFunc(c.patterns, func(p *spec.Schema) bool { return p.Pattern == s.Pattern }) {
		c.patterns = append(c.patterns, s)
	}
}

func (c *jointChecks) addMultiple(n json.Number) {
	if !slices.Contains(c.multiples, n) {
		c.multiples = append(c.multiples, n)
	}
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

// bothHold checks what a and b both check; unlike mergeChild it keeps a side with only checks.
func bothHold(a, b *spec.Schema) *spec.Schema {
	switch {
	case b == nil, a == b:
		return a
	case a == nil:
		return b
	}
	return &spec.Schema{AllOf: []*spec.Schema{a, b}, Origin: a.Origin}
}

func isSameRef(a, b *spec.Schema) bool {
	ra, rb := refOf(a), refOf(b)
	return ra != nil && rb != nil && ra.Target == rb.Target
}

// mergeLimits keeps the strictest of each limit; MultipleOf keeps the first, jointChecks keep all.
func mergeLimits(dst *spec.Limits, src spec.Limits) {
	dst.Minimum = tighter(dst.Minimum, src.Minimum, 1)
	dst.Maximum = tighter(dst.Maximum, src.Maximum, -1)
	dst.MultipleOf = cmp.Or(dst.MultipleOf, src.MultipleOf)
	dst.MinLength = larger(dst.MinLength, src.MinLength)
	dst.MaxLength = smaller(dst.MaxLength, src.MaxLength)
	dst.MinItems = larger(dst.MinItems, src.MinItems)
	dst.MaxItems = smaller(dst.MaxItems, src.MaxItems)
	dst.MinProperties = larger(dst.MinProperties, src.MinProperties)
	dst.MaxProperties = smaller(dst.MaxProperties, src.MaxProperties)
	dst.UniqueItems = dst.UniqueItems || src.UniqueItems
}

// tighter is the larger minimum (sign 1) or the smaller maximum (-1), the exclusive one on a tie.
func tighter(a, b *spec.Bound, sign int) *spec.Bound {
	if a == nil || b == nil {
		return cmp.Or(a, b)
	}
	x, isA := new(big.Rat).SetString(a.Value.String())
	y, isB := new(big.Rat).SetString(b.Value.String())
	if !isA || !isB {
		return a
	}
	switch c := x.Cmp(y) * sign; {
	case c < 0, c == 0 && b.Exclusive:
		return b
	}
	return a
}

func larger(a, b *int64) *int64 {
	if a == nil || b != nil && *b > *a {
		return b
	}
	return a
}

func smaller(a, b *int64) *int64 {
	if a == nil || b != nil && *b < *a {
		return b
	}
	return a
}

// mergeTypes keeps the types both allow, and reports false when they allow none in common.
func mergeTypes(m, s *spec.Schema) bool {
	switch {
	case s.Types == 0, s.Types == spec.TypeNull:
		return true
	case m.Types == 0:
		m.Types = s.Types
		return true
	}

	if t := commonTypes(m.Types, s.Types); t != 0 {
		m.Types = t
		return true
	}
	return false
}

// addsType reports a shape or a type in s that the x-go-type of typed leaves out.
func addsType(s, typed *spec.Schema) bool {
	return len(s.Properties) > 0 || s.AdditionalProperties.Mode != spec.AdditionalUnset || s.Items != nil ||
		len(s.PrefixItems) > 0 || isUnion(s) || s.Then != nil || s.Else != nil ||
		s.Types&^spec.TypeNull != 0 && typed.Types&^spec.TypeNull != 0 && commonTypes(s.Types, typed.Types)&^spec.TypeNull == 0
}

// commonTypes are the types a and b both allow; a number and an integer give an integer.
func commonTypes(a, b spec.TypeSet) spec.TypeSet {
	t := a & b
	if a.Has(spec.TypeNumber) && b.Has(spec.TypeInteger) || a.Has(spec.TypeInteger) && b.Has(spec.TypeNumber) {
		t |= spec.TypeInteger
	}
	return t
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

// listsSchema reports a oneOf or anyOf member of s that is a $ref to root.
func listsSchema(s, root *spec.Schema) bool {
	isRoot := func(m *spec.Schema) bool {
		r := refOf(m)
		return r != nil && r.Target == root
	}
	return slices.ContainsFunc(s.OneOf, isRoot) || slices.ContainsFunc(s.AnyOf, isRoot)
}

// branch returns the then or else of an if that has only one of them.
func branch(s *spec.Schema) *spec.Schema {
	if s.Then != nil && s.Else != nil {
		return nil
	}
	return cmp.Or(s.Then, s.Else)
}
