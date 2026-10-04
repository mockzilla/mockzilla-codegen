// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// A oneOf with a discriminator: each variant requires the property and takes the values that pick
// it, so a body matches the one variant the Go union picks for it.

package jsonschema

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// pin returns m requiring the discriminator property. The property takes the values the mapping
// lists for m; without them, a variant that holds it to one value keeps that, and any other takes
// its component name. It reports false, with m as it is, for the default variant and one no value
// picks.
func pin(m *spec.Schema, d *spec.Discriminator) (*spec.Schema, bool) {
	switch {
	case m == nil || refersTo(m, d.Default):
		return m, false
	case takesOnlyNull(m):
		return m, true
	}

	values := mapped(m, d)
	switch {
	case len(values) > 0, pinsValue(m, d.Property):
	case m.Ref != nil && m.Ref.Name != "":
		values = []string{m.Ref.Name}
	default:
		return m, false
	}

	v := *m
	if !slices.Contains(v.Required, d.Property) {
		v.Required = append(slices.Clone(v.Required), d.Property)
	}
	if len(values) > 0 {
		v.Properties = append(slices.Clone(v.Properties), &spec.Property{Name: d.Property, Schema: valueSchema(values), Required: true})
	}
	return &v, true
}

// mapped are the mapping values of the schema m refers to.
func mapped(m *spec.Schema, d *spec.Discriminator) []string {
	var out []string
	for _, mp := range d.Mapping {
		if refersTo(m, mp.Ref) {
			out = append(out, mp.Value)
		}
	}
	return out
}

func refersTo(m *spec.Schema, r *spec.Ref) bool {
	return m.Ref != nil && r != nil && r.Target != nil && r.Target == m.Ref.Target
}

// pinsValue reports the property prop of s, or of what composes it, held to one value.
func pinsValue(s *spec.Schema, prop string) bool {
	for x := range composed(s) {
		for _, p := range x.Properties {
			if p.Name == prop && isPinned(p.Schema) {
				return true
			}
		}
	}
	return false
}

// isPinned reports s, or what composes it, held to one value other than null by a const or an enum.
func isPinned(s *spec.Schema) bool {
	for x := range composed(s) {
		if x.Const != nil && x.Const.Kind != spec.KindNull || len(x.Enum) == 1 && x.Enum[0].Kind != spec.KindNull {
			return true
		}
	}
	return false
}

// takesOnlyNull reports a member of a union that stands for null, which is no variant.
func takesOnlyNull(s *spec.Schema) bool {
	isNull := func(v spec.Value) bool { return v.Kind == spec.KindNull }
	return s.Ref == nil && (s.Types == spec.TypeNull || s.Const != nil && isNull(*s.Const) ||
		len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(v spec.Value) bool { return !isNull(v) }))
}

// valueSchema takes the one value, or any of the values.
func valueSchema(values []string) *spec.Schema {
	list := make([]spec.Value, len(values))
	for i, v := range values {
		list[i] = spec.Value{Kind: spec.KindString, Str: v}
	}
	if len(list) == 1 {
		return &spec.Schema{Const: &list[0]}
	}
	return &spec.Schema{Enum: list}
}
